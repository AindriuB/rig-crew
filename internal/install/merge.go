package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/AindriuB/rig-crew/internal/plan"
)

func sortStrings(s []string) { sort.Strings(s) }

// decodeObject parses a settings file. It refuses what Go cannot write back
// unchanged: bytes that are not UTF-8, a lone surrogate escape (decoded as
// U+FFFD) and a duplicate key at any level (the later one would win silently).
func decodeObject(b []byte) (map[string]any, error) {
	if !utf8.Valid(b) {
		return nil, fmt.Errorf("not valid UTF-8")
	}
	lossy := !bytes.Contains(bytes.ToLower(b), []byte(`\ufffd`)) && !bytes.Contains(b, []byte("\xef\xbf\xbd"))
	chk := json.NewDecoder(bytes.NewReader(b))
	chk.UseNumber()
	if err := scanDup(chk, lossy); err != nil {
		return nil, err
	}
	if _, err := chk.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber() // keep numbers exactly as written
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("not a JSON object")
	}
	return m, nil
}

// scanDup walks one value and fails on a duplicate object key.
func scanDup(dec *json.Decoder, lossy bool) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if s, ok := t.(string); ok && lossy && strings.ContainsRune(s, utf8.RuneError) {
		return fmt.Errorf("a string holds an invalid surrogate escape")
	}
	d, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	if d == '[' {
		for dec.More() {
			if err := scanDup(dec, lossy); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	}
	seen := map[string]bool{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return err
		}
		k := kt.(string) // a key is always a string
		if seen[k] {
			return fmt.Errorf("duplicate key %q", k)
		}
		if lossy && strings.ContainsRune(k, utf8.RuneError) {
			return fmt.Errorf("a key holds an invalid surrogate escape")
		}
		seen[k] = true
		if err := scanDup(dec, lossy); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

// walk descends through nested objects; create adds missing ones.
func walk(root map[string]any, path []string, create bool) (map[string]any, bool) {
	cur := root
	for _, k := range path {
		v, has := cur[k]
		if !has {
			if !create {
				return nil, false
			}
			n := map[string]any{}
			cur[k] = n
			cur = n
			continue
		}
		n, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		cur = n
	}
	return cur, true
}

type mergeResult struct {
	data     []byte
	rec      mergeRec
	changed  bool // the file bytes change
	added    bool // the plan added an entry to the file
	dropped  bool // an entry the role recorded earlier is no longer planned
	conflict string
}

func keyStr(k []string) string { return strings.Join(k, "\x00") }

// applyMerge makes the settings bytes in cur hold the plan's entries. An entry
// is recorded when this run added it, or when it is already present and some
// role recorded it; a user's own entry is never recorded. An entry this role
// recorded earlier and no longer plans is removed, unless another role also
// recorded it.
func applyMerge(cur fstate, m plan.JSONMerge, rel string, own *mergeRec, others []*mergeRec) mergeResult {
	root := map[string]any{}
	if cur.exists {
		var err error
		if root, err = decodeObject(cur.data); err != nil {
			return mergeResult{conflict: "settings file is not usable: " + err.Error()}
		}
	}
	rec := mergeRec{Path: rel, Created: !cur.exists}
	var kept []mentry
	if own != nil {
		rec.Created = own.Created || !cur.exists
		kept = slices.Clone(own.Entries)
	}
	otherIDs := map[string]bool{}
	for _, o := range others {
		for _, e := range o.Entries {
			otherIDs[e.id()] = true
		}
	}
	planned := map[string]bool{}
	plannedSet := map[string]bool{}
	for _, e := range m.Append {
		planned[mentry{Kind: "append", Key: e.Key, Value: e.Value}.id()] = true
	}
	for _, e := range m.Set {
		plannedSet[keyStr(e.Key)] = true
	}
	var gone []mentry
	kept = slices.DeleteFunc(kept, func(k mentry) bool {
		if planned[k.id()] || (k.Kind == "set" && plannedSet[keyStr(k.Key)]) {
			return false
		}
		if !otherIDs[k.id()] {
			gone = append(gone, k)
		}
		return true
	})
	dropped := len(kept) != len(own.entriesOrNil())
	removed := removeEntries(root, gone, nil)
	ownIDs := map[string]bool{}
	for _, e := range kept {
		ownIDs[e.id()] = true
	}
	added := false
	record := func(e mentry) {
		if !ownIDs[e.id()] {
			ownIDs[e.id()] = true
			kept = append(kept, e)
		}
	}
	for _, e := range m.Append {
		parent, leaf, bad := locate(root, e.Key, true)
		if bad != "" {
			return mergeResult{conflict: bad}
		}
		me := mentry{Kind: "append", Key: e.Key, Value: e.Value}
		v, has := parent[leaf]
		arr, isArr := v.([]any)
		if has && !isArr {
			return mergeResult{conflict: fmt.Sprintf("%s is not an array", strings.Join(e.Key, "."))}
		}
		if slices.Contains(arr, any(e.Value)) {
			if ownIDs[me.id()] || otherIDs[me.id()] {
				record(me)
			}
			continue
		}
		parent[leaf] = append(arr, e.Value)
		if arr == nil {
			parent[leaf] = []any{e.Value}
		}
		added = true
		record(me)
	}
	for _, e := range m.Set {
		parent, leaf, bad := locate(root, e.Key, true)
		if bad != "" {
			return mergeResult{conflict: bad}
		}
		me := mentry{Kind: "set", Key: e.Key, Value: e.Value}
		v, has := parent[leaf]
		if !has {
			parent[leaf] = e.Value
			added = true
			record(me)
			continue
		}
		s, isStr := v.(string)
		switch {
		case isStr && s == e.Value:
			if ownIDs[me.id()] || otherIDs[me.id()] {
				record(me)
			}
		case isStr && replaceable(kept, otherIDs, e.Key, s):
			parent[leaf] = e.Value
			added = true
			kept = slices.DeleteFunc(kept, func(k mentry) bool {
				return k.Kind == "set" && slices.Equal(k.Key, e.Key)
			})
			delete(ownIDs, mentry{Kind: "set", Key: e.Key, Value: s}.id())
			record(me)
		default:
			return mergeResult{conflict: fmt.Sprintf("%s is already set to a different value", strings.Join(e.Key, "."))}
		}
	}
	rec.Entries = kept
	res := mergeResult{rec: rec, changed: added || removed, added: added, dropped: dropped}
	if res.changed {
		res.data = encodeJSON(root)
	}
	return res
}

func (r *mergeRec) entriesOrNil() []mentry {
	if r == nil {
		return nil
	}
	return r.Entries
}

// replaceable reports whether the value at key is one this role set alone.
func replaceable(kept []mentry, otherIDs map[string]bool, key []string, cur string) bool {
	id := mentry{Kind: "set", Key: key, Value: cur}.id()
	return slices.ContainsFunc(kept, func(k mentry) bool { return k.id() == id }) && !otherIDs[id]
}

// locate returns the object holding the key's last element.
func locate(root map[string]any, key []string, create bool) (map[string]any, string, string) {
	if len(key) == 0 {
		return nil, "", "an entry has an empty key"
	}
	parent, ok := walk(root, key[:len(key)-1], create)
	if !ok {
		return nil, "", fmt.Sprintf("%s crosses a value that is not an object", strings.Join(key, "."))
	}
	return parent, key[len(key)-1], ""
}

func isEmptyContainer(v any) bool {
	switch t := v.(type) {
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

func onlyEmpty(v any) bool {
	switch t := v.(type) {
	case []any:
		return len(t) == 0
	case map[string]any:
		for _, c := range t {
			if !onlyEmpty(c) {
				return false
			}
		}
		return true
	}
	return false
}

func prune(root map[string]any, path []string) {
	for i := len(path); i > 0; i-- {
		parent, ok := walk(root, path[:i-1], false)
		if !ok || !isEmptyContainer(parent[path[i-1]]) {
			return
		}
		delete(parent, path[i-1])
	}
}

type unmergeResult struct {
	data     []byte
	changed  bool
	del      bool
	conflict string
}

// removeEntries takes the entries out of root, except those in keep, and
// reports whether root changed. A value the user changed since is left alone.
func removeEntries(root map[string]any, entries []mentry, keep map[string]bool) bool {
	changed := false
	for _, e := range entries {
		if keep[e.id()] {
			continue
		}
		parent, leaf, bad := locate(root, e.Key, false)
		if bad != "" || parent == nil {
			continue // already gone
		}
		v, has := parent[leaf]
		if !has {
			continue
		}
		switch e.Kind {
		case "append":
			arr, ok := v.([]any)
			if !ok {
				continue
			}
			i := slices.Index(arr, any(e.Value))
			if i < 0 {
				continue
			}
			arr = slices.Delete(arr, i, i+1)
			parent[leaf] = arr
			changed = true
			if len(arr) == 0 {
				prune(root, e.Key)
			}
		case "set":
			if s, ok := v.(string); !ok || s != e.Value {
				continue // the user changed it since; leave it
			}
			delete(parent, leaf)
			changed = true
			prune(root, e.Key[:len(e.Key)-1])
		}
	}
	return changed
}

// removeMerge takes out the entries rec recorded, except those another role
// also recorded (keep). createdAny allows deleting a file that ends up empty.
func removeMerge(cur fstate, rec *mergeRec, keep map[string]bool, createdAny bool) unmergeResult {
	root, err := decodeObject(cur.data)
	if err != nil {
		return unmergeResult{conflict: "settings file is not usable: " + err.Error()}
	}
	changed := removeEntries(root, rec.Entries, keep)
	res := unmergeResult{changed: changed}
	if createdAny && onlyEmpty(root) {
		res.del = true
		return res
	}
	if changed {
		res.data = encodeJSON(root)
	}
	return res
}
