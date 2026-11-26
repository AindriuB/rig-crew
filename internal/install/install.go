// Package install is the only code that writes to disk: it applies a
// plan.Plan into one scope under a manifest and removes exactly what it
// recorded.
package install

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AindriuB/rig-crew/internal/plan"
)

// Action kinds.
const (
	KindCreate    = "create"
	KindUpdate    = "update"
	KindAdopt     = "adopt"
	KindUnchanged = "unchanged"
	KindRemove    = "remove"
	KindMerge     = "merge"
	KindUnmerge   = "unmerge"
)

type Options struct {
	DryRun bool

	beforeRecheck func()                  // test seam: runs under the lock, before the re-read
	beforeWrite   func(path string) error // test seam: may fail a write
}

type Action struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type Result struct {
	Actions []Action `json:"actions"`
	Changed bool     `json:"changed"`
}

type Conflict struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type ConflictError struct{ Conflicts []Conflict }

func (e *ConflictError) Error() string {
	var b strings.Builder
	b.WriteString("refusing to continue, nothing written:")
	for _, c := range e.Conflicts {
		fmt.Fprintf(&b, "\n  %s: %s", c.Path, c.Reason)
	}
	return b.String()
}

type LockedError struct{ Path string }

func (e *LockedError) Error() string {
	return fmt.Sprintf("another install holds the lock %s; remove it if no install is running", e.Path)
}

type SymlinkError struct{ Path string }

func (e *SymlinkError) Error() string {
	return fmt.Sprintf("%s is a symlink or junction; refusing to write through it", e.Path)
}

var (
	ErrNotInstalled  = errors.New("role is not installed in this scope")
	ErrOutsideRoot   = errors.New("path is not an absolute path under the root")
	errDuplicatePath = errors.New("path appears twice in the plan")
)

func relSlash(root, p string) string {
	rel, _ := filepath.Rel(root, p) // validated beforehand
	return filepath.ToSlash(rel)
}

func underRoot(root, p string) error {
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%s: %w", p, ErrOutsideRoot)
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		rel == stateDir || strings.HasPrefix(rel, stateDir+string(filepath.Separator)) {
		return fmt.Errorf("%s: %w", p, ErrOutsideRoot)
	}
	return nil
}

func conflictsErr(c []Conflict) error {
	sort.Slice(c, func(i, j int) bool { return c[i].Path < c[j].Path })
	return &ConflictError{Conflicts: c}
}

func readManifestFile(root string) (*manifest, error) {
	s, err := readSnap([]target{{manifestPath(root), root}})
	if err != nil {
		return nil, err
	}
	m, err := decodeManifest(s[manifestPath(root)])
	if err != nil {
		return nil, conflictsErr([]Conflict{{manifestPath(root), err.Error()}})
	}
	return m, nil
}

// Apply writes the plan into its scope.
func Apply(ctx context.Context, p plan.Plan, opts Options) (Result, error) {
	root := filepath.Clean(p.Root)
	if !filepath.IsAbs(root) {
		return Result{}, fmt.Errorf("root %s: %w", p.Root, ErrOutsideRoot)
	}
	seen := map[string]bool{}
	check := func(path string) error {
		if err := underRoot(root, path); err != nil {
			return err
		}
		if seen[path] {
			return fmt.Errorf("%s: %w", path, errDuplicatePath)
		}
		seen[path] = true
		return nil
	}
	for _, f := range p.Files {
		if err := check(f.Path); err != nil {
			return Result{}, err
		}
	}
	for _, m := range p.Merges {
		if err := check(m.Path); err != nil {
			return Result{}, err
		}
	}
	if p.Workflow != nil && !filepath.IsAbs(p.Workflow.Path) {
		return Result{}, fmt.Errorf("%s: %w", p.Workflow.Path, ErrOutsideRoot)
	}
	old, err := readManifestFile(root)
	if err != nil {
		return Result{}, err
	}
	ts := []target{{manifestPath(root), root}}
	for _, f := range p.Files {
		ts = append(ts, target{f.Path, root})
	}
	for _, m := range p.Merges {
		ts = append(ts, target{m.Path, root})
	}
	if r := old.Roles[p.Role]; r != nil {
		for rel := range r.Files {
			ts = append(ts, target{filepath.Join(root, filepath.FromSlash(rel)), root})
		}
	}
	j := job{root: root, targets: ts}
	if w := p.Workflow; w != nil {
		j.wfDir = filepath.Dir(w.Path)
		ts = append(ts, target{w.Path, j.wfDir}, target{manifestPath(j.wfDir), j.wfDir})
		j.targets = ts
	}
	j.compute = func(s snapshot) (*outcome, error) { return computeApply(p, root, s) }
	return run(ctx, j, opts)
}

func computeApply(p plan.Plan, root string, snap snapshot) (*outcome, error) {
	out := &outcome{root: root}
	mpath := manifestPath(root)
	man, err := decodeManifest(snap[mpath])
	if err != nil {
		return nil, conflictsErr([]Conflict{{mpath, err.Error()}})
	}
	old := man.Roles[p.Role]
	if old == nil {
		old = &roleRec{}
	}
	nr := &roleRec{Files: map[string]string{}, Workflow: old.Workflow}
	var conflicts []Conflict
	conflict := func(path, reason string) { conflicts = append(conflicts, Conflict{path, reason}) }
	act := func(path, kind string) { out.actions = append(out.actions, Action{path, kind}) }

	inPlan := map[string]bool{}
	for _, f := range p.Files {
		rel := relSlash(root, f.Path)
		inPlan[rel] = true
		cur := snap[f.Path]
		ownHash, own := old.Files[rel]
		others := owners(man, p.Role, rel)
		var kind string
		switch {
		case !cur.exists:
			kind = KindCreate
		case own && hashHex(cur.data) != ownHash:
			conflict(f.Path, "modified since rig-crew wrote it")
		case bytesEq(cur.data, f.Data) && own:
			kind = KindUnchanged
		case bytesEq(cur.data, f.Data):
			kind = KindAdopt
		case own && len(others) == 0:
			kind = KindUpdate
		case len(others) > 0:
			conflict(f.Path, fmt.Sprintf("also owned by role %s and differs", others[0]))
		default:
			conflict(f.Path, "exists and differs from what rig-crew would write")
		}
		if kind == "" {
			continue
		}
		nr.Files[rel] = hashHex(f.Data)
		act(f.Path, kind)
		if kind == KindCreate || kind == KindUpdate {
			out.ops = append(out.ops, op{path: f.Path, data: f.Data})
		}
	}
	var rels []string
	for rel := range old.Files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		if inPlan[rel] || len(owners(man, p.Role, rel)) > 0 {
			continue // still in the plan, or shared: only our record goes
		}
		abs := filepath.Join(root, filepath.FromSlash(rel))
		cur := snap[abs]
		switch {
		case !cur.exists:
		case hashHex(cur.data) != old.Files[rel]:
			conflict(abs, "modified since rig-crew wrote it")
		default:
			act(abs, KindRemove)
			out.ops = append(out.ops, op{path: abs, remove: true})
			out.prune = append(out.prune, abs)
		}
	}

	planned := map[string]bool{}
	for _, m := range p.Merges {
		rel := relSlash(root, m.Path)
		planned[rel] = true
		var others []*mergeRec
		for name, r := range man.Roles {
			if name != p.Role {
				if mr := findMerge(r, rel); mr != nil {
					others = append(others, mr)
				}
			}
		}
		res := applyMerge(snap[m.Path], m, rel, findMerge(old, rel), others)
		if res.conflict != "" {
			conflict(m.Path, res.conflict)
			continue
		}
		nr.Merges = append(nr.Merges, res.rec)
		switch {
		case res.changed:
			act(m.Path, KindMerge)
			out.ops = append(out.ops, op{path: m.Path, data: res.data})
		case !sameRec(findMerge(old, rel), res.rec):
			act(m.Path, KindAdopt)
		default:
			act(m.Path, KindUnchanged)
		}
	}
	for _, mr := range old.Merges {
		if !planned[mr.Path] {
			nr.Merges = append(nr.Merges, mr)
		}
	}
	sort.Slice(nr.Merges, func(i, j int) bool { return nr.Merges[i].Path < nr.Merges[j].Path })

	if w := p.Workflow; w != nil {
		wdir := filepath.Dir(w.Path)
		wmp := manifestPath(wdir)
		wm, err := decodeWf(snap[wmp])
		if err != nil {
			conflict(wmp, err.Error())
		} else {
			name := filepath.Base(w.Path)
			cur := snap[w.Path]
			ownHash, own := wm.Files[name]
			var kind string
			switch {
			case !cur.exists:
				kind = KindCreate
			case own && hashHex(cur.data) != ownHash:
				conflict(w.Path, "workflow file modified since rig-crew wrote it")
			case bytesEq(cur.data, w.Data) && own:
				kind = KindUnchanged
			case bytesEq(cur.data, w.Data):
				kind = KindAdopt
			case own:
				kind = KindUpdate
			default:
				conflict(w.Path, "workflow file exists and differs")
			}
			if kind != "" {
				act(w.Path, kind)
				if kind == KindCreate || kind == KindUpdate {
					out.ops = append(out.ops, op{path: w.Path, data: w.Data})
				}
				wm.Files[name] = hashHex(w.Data)
				ref := wfRef{Name: name, Root: root, Role: p.Role}
				if !containsRef(wm.Refs, ref) {
					wm.Refs = append(wm.Refs, ref)
				}
				sortRefs(wm.Refs)
				nr.Workflow = w.Path
				if nb := encodeJSON(wm); !snap[wmp].exists || !bytesEq(nb, snap[wmp].data) {
					out.ops = append(out.ops, op{path: wmp, data: nb})
				}
			}
		}
	}
	if len(conflicts) > 0 {
		return nil, conflictsErr(conflicts)
	}
	man.Roles[p.Role] = nr
	if nb := encodeJSON(man); !snap[mpath].exists || !bytesEq(nb, snap[mpath].data) {
		out.ops = append(out.ops, op{path: mpath, data: nb})
	}
	return out, nil
}

// Uninstall removes exactly what the role's manifest entry records.
func Uninstall(ctx context.Context, root, role string, opts Options) (Result, error) {
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return Result{}, fmt.Errorf("root %s: %w", root, ErrOutsideRoot)
	}
	man, err := readManifestFile(root)
	if err != nil {
		return Result{}, err
	}
	rec := man.Roles[role]
	if rec == nil {
		return Result{}, fmt.Errorf("%s in %s: %w", role, root, ErrNotInstalled)
	}
	ts := []target{{manifestPath(root), root}}
	for rel := range rec.Files {
		ts = append(ts, target{filepath.Join(root, filepath.FromSlash(rel)), root})
	}
	for _, m := range rec.Merges {
		ts = append(ts, target{filepath.Join(root, filepath.FromSlash(m.Path)), root})
	}
	j := job{root: root}
	if rec.Workflow != "" {
		j.wfDir = filepath.Dir(rec.Workflow)
		ts = append(ts, target{rec.Workflow, j.wfDir}, target{manifestPath(j.wfDir), j.wfDir})
	}
	j.targets = ts
	j.compute = func(s snapshot) (*outcome, error) { return computeUninstall(root, role, s) }
	return run(ctx, j, opts)
}

func computeUninstall(root, role string, snap snapshot) (*outcome, error) {
	out := &outcome{root: root}
	mpath := manifestPath(root)
	man, err := decodeManifest(snap[mpath])
	if err != nil {
		return nil, conflictsErr([]Conflict{{mpath, err.Error()}})
	}
	rec := man.Roles[role]
	if rec == nil {
		return nil, fmt.Errorf("%s in %s: %w", role, root, ErrNotInstalled)
	}
	var conflicts []Conflict
	conflict := func(path, reason string) { conflicts = append(conflicts, Conflict{path, reason}) }
	act := func(path, kind string) { out.actions = append(out.actions, Action{path, kind}) }

	var rels []string
	for rel := range rec.Files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		cur := snap[abs]
		switch {
		case len(owners(man, role, rel)) > 0:
			act(abs, KindUnchanged) // another role still owns it
		case !cur.exists:
		case hashHex(cur.data) != rec.Files[rel]:
			conflict(abs, "modified since rig-crew wrote it")
		default:
			act(abs, KindRemove)
			out.ops = append(out.ops, op{path: abs, remove: true})
			out.prune = append(out.prune, abs)
		}
	}
	delete(man.Roles, role)
	for _, mr := range rec.Merges {
		abs := filepath.Join(root, filepath.FromSlash(mr.Path))
		keep := map[string]bool{}
		createdAny := mr.Created
		var rest []string
		for name, r := range man.Roles {
			if o := findMerge(r, mr.Path); o != nil {
				rest = append(rest, name)
				for _, e := range o.Entries {
					keep[e.id()] = true
				}
				createdAny = createdAny || o.Created
			}
		}
		if mr.Created && len(rest) > 0 {
			sort.Strings(rest)
			findMerge(man.Roles[rest[0]], mr.Path).Created = true
		}
		cur := snap[abs]
		if !cur.exists {
			continue
		}
		res := removeMerge(cur, &mr, keep, createdAny && len(rest) == 0)
		switch {
		case res.conflict != "":
			conflict(abs, res.conflict)
		case res.del:
			act(abs, KindRemove)
			out.ops = append(out.ops, op{path: abs, remove: true})
			out.prune = append(out.prune, abs)
		case res.changed:
			act(abs, KindUnmerge)
			out.ops = append(out.ops, op{path: abs, data: res.data})
		default:
			act(abs, KindUnchanged)
		}
	}
	if rec.Workflow != "" {
		wdir := filepath.Dir(rec.Workflow)
		wmp := manifestPath(wdir)
		if snap[wmp].exists {
			wm, err := decodeWf(snap[wmp])
			if err != nil {
				conflict(wmp, err.Error())
			} else {
				name := filepath.Base(rec.Workflow)
				var refs []wfRef
				left := false
				for _, r := range wm.Refs {
					if r.Name == name && r.Root == root && r.Role == role {
						continue
					}
					if r.Name == name {
						left = true
					}
					refs = append(refs, r)
				}
				wm.Refs = refs
				cur := snap[rec.Workflow]
				switch {
				case left:
					act(rec.Workflow, KindUnchanged)
				case !cur.exists:
					delete(wm.Files, name)
				case hashHex(cur.data) != wm.Files[name]:
					conflict(rec.Workflow, "workflow file modified since rig-crew wrote it")
				default:
					delete(wm.Files, name)
					act(rec.Workflow, KindRemove)
					out.ops = append(out.ops, op{path: rec.Workflow, remove: true})
				}
				if len(wm.Files) == 0 && len(wm.Refs) == 0 {
					out.ops = append(out.ops, op{path: wmp, remove: true})
					out.cleanup = append(out.cleanup, filepath.Dir(wmp))
				} else {
					out.ops = append(out.ops, op{path: wmp, data: encodeJSON(wm)})
				}
			}
		}
	}
	if len(conflicts) > 0 {
		return nil, conflictsErr(conflicts)
	}
	if len(man.Roles) == 0 {
		out.ops = append(out.ops, op{path: mpath, remove: true})
		out.cleanup = append(out.cleanup, filepath.Dir(mpath))
	} else {
		out.ops = append(out.ops, op{path: mpath, data: encodeJSON(man)})
	}
	return out, nil
}

func bytesEq(a, b []byte) bool { return string(a) == string(b) }

func sameRec(a *mergeRec, b mergeRec) bool {
	if a == nil {
		return len(b.Entries) == 0
	}
	return string(encodeJSON(*a)) == string(encodeJSON(b))
}

func containsRef(rs []wfRef, r wfRef) bool {
	for _, x := range rs {
		if x == r {
			return true
		}
	}
	return false
}

func sortRefs(rs []wfRef) {
	sort.Slice(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Root != b.Root {
			return a.Root < b.Root
		}
		return a.Role < b.Role
	})
}
