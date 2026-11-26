package install

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const manifestVersion = 1

const stateDir = ".rig-crew"

type mentry struct {
	Kind  string   `json:"kind"` // "append" or "set"
	Key   []string `json:"key"`
	Value string   `json:"value"`
}

func (e mentry) id() string {
	return e.Kind + "\x00" + strings.Join(e.Key, "\x00") + "\x00" + e.Value
}

type mergeRec struct {
	Path    string   `json:"path"`
	Created bool     `json:"created,omitempty"`
	Entries []mentry `json:"entries"`
}

type roleRec struct {
	Files    map[string]string `json:"files,omitempty"`
	Merges   []mergeRec        `json:"merges,omitempty"`
	Workflow string            `json:"workflow,omitempty"`
	Home     string            `json:"home,omitempty"` // Scope.Home the workflow reference was made under
}

type manifest struct {
	Version int                 `json:"version"`
	Roles   map[string]*roleRec `json:"roles"`
}

type wfRef struct {
	Name string `json:"name"`
	Root string `json:"root"`
	Role string `json:"role"`
}

type wfManifest struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
	Refs    []wfRef           `json:"refs"`
}

func manifestPath(dir string) string { return filepath.Join(dir, stateDir, "manifest.json") }
func lockPath(dir string) string     { return filepath.Join(dir, stateDir, "lock") }

func hashHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func encodeJSON(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	// Encoding maps, slices and strings cannot fail.
	_ = enc.Encode(v)
	return buf.Bytes()
}

func decodeManifest(s fstate) (*manifest, error) {
	m := &manifest{Version: manifestVersion, Roles: map[string]*roleRec{}}
	if !s.exists {
		return m, nil
	}
	if err := json.Unmarshal(s.data, m); err != nil {
		return nil, fmt.Errorf("manifest is not valid: %w", err)
	}
	if m.Roles == nil {
		m.Roles = map[string]*roleRec{}
	}
	return m, nil
}

func decodeWf(s fstate) (*wfManifest, error) {
	m := &wfManifest{Version: manifestVersion, Files: map[string]string{}}
	if !s.exists {
		return m, nil
	}
	if err := json.Unmarshal(s.data, m); err != nil {
		return nil, fmt.Errorf("workflow manifest is not valid: %w", err)
	}
	if m.Files == nil {
		m.Files = map[string]string{}
	}
	return m, nil
}

// owners lists the other roles that record rel as an owned file.
func owners(m *manifest, role, rel string) []string {
	var out []string
	for name, r := range m.Roles {
		if name == role {
			continue
		}
		if _, ok := r.Files[rel]; ok {
			out = append(out, name)
		}
	}
	sortStrings(out)
	return out
}

func findMerge(r *roleRec, rel string) *mergeRec {
	if r == nil {
		return nil
	}
	for i := range r.Merges {
		if r.Merges[i].Path == rel {
			return &r.Merges[i]
		}
	}
	return nil
}
