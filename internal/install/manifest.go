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

const manifestVersion2 = 2

// userOwner is the reserved v2 record holding files CreateFile made.
const userOwner = "user"

const stateDir = ".rig-crew"

type mentry struct {
	Kind  string   `json:"kind"` // "append" or "set"
	Key   []string `json:"key"`
	Value string   `json:"value"`
	// Raw is the JSON value of a "raw" entry, in canonical form.
	Raw json.RawMessage `json:"raw,omitempty"`
}

func (e mentry) id() string {
	id := e.Kind + "\x00" + strings.Join(e.Key, "\x00") + "\x00" + e.Value
	if len(e.Raw) > 0 {
		id += "\x00" + canonRaw(e.Raw)
	}
	return id
}

type mergeRec struct {
	Path    string   `json:"path"`
	Created bool     `json:"created,omitempty"`
	Entries []mentry `json:"entries"`
}

// actRec is the recorded form of a plan.Activation.
type actRec struct {
	Harness  string   `json:"harness,omitempty"`
	Level    string   `json:"level"`
	Profiles []string `json:"profiles,omitempty"`
	Packs    []string `json:"packs,omitempty"`
}

type roleRec struct {
	Files    map[string]string `json:"files,omitempty"`
	Merges   []mergeRec        `json:"merges,omitempty"`
	Workflow string            `json:"workflow,omitempty"`
	Home     string            `json:"home,omitempty"` // Scope.Home the workflow reference was made under

	// v2 fields.
	Harness    string   `json:"harness,omitempty"`
	Root       string   `json:"root,omitempty"`
	Project    string   `json:"project,omitempty"` // project directory of a project activation
	Activation *actRec  `json:"activation,omitempty"`
	UserFiles  []string `json:"userFiles,omitempty"` // absolute, user-owned, no hash
}

type manifest struct {
	Version int                 `json:"version"`
	Roles   map[string]*roleRec `json:"roles"`

	v2       bool     // read from and written as a version 2 manifest
	root     string   // v2: the root whose records are peers of each other
	projects []string // v2 global: project directories with a project activation
}

// manifestV2 is the on-disk form of a version 2 manifest.
type manifestV2 struct {
	Version  int                 `json:"version"`
	Owners   map[string]*roleRec `json:"owners"`
	Projects []string            `json:"projects,omitempty"`
}

// peer reports whether r shares files with the records of m's current root.
func (m *manifest) peer(r *roleRec) bool { return !m.v2 || r.Root == m.root }

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

func encodeManifest(m *manifest) []byte {
	if m.v2 {
		return encodeJSON(manifestV2{Version: manifestVersion2, Owners: m.Roles, Projects: m.projects})
	}
	return encodeJSON(m)
}

func decodeManifest(s fstate, v2 bool) (*manifest, error) {
	if v2 {
		m := &manifest{Version: manifestVersion2, Roles: map[string]*roleRec{}, v2: true}
		if !s.exists {
			return m, nil
		}
		var d manifestV2
		if err := json.Unmarshal(s.data, &d); err != nil {
			return nil, fmt.Errorf("manifest is not valid: %w", err)
		}
		if d.Version != manifestVersion2 {
			return nil, fmt.Errorf("manifest is version %d, not %d", d.Version, manifestVersion2)
		}
		if d.Owners != nil {
			m.Roles = d.Owners
		}
		m.projects = d.Projects
		return m, nil
	}
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
		if name == role || !m.peer(r) {
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
