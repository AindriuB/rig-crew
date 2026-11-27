// Package catalog loads rig-crew content from an fs.FS into neutral types.
package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/AindriuB/rig-crew/internal/frontmatter"
	"github.com/AindriuB/rig-crew/internal/model"
)

// ErrNotFound matches lookups of a name the catalog does not hold.
var ErrNotFound = errors.New("not found")

// Overlay is a second content tree whose items replace or add to the base.
// Dir is its OS path, used for error messages and Workflow.SourcePath.
type Overlay struct {
	FS  fs.FS
	Dir string
}

// Catalog is a loaded, validated content set.
type Catalog struct {
	roles     map[string]model.Role
	prompts   map[string]model.Prompt
	skills    map[string]model.Skill
	workflows map[string]model.Workflow
	harnesses map[string]memFS
}

var (
	namePat   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	minRigPat = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	cmdPat    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*( [a-z0-9][a-z0-9-]*)*$`)
)

type rigFile struct {
	Allow *[]string `json:"allow"`
	Deny  *[]string `json:"deny"`
}

type shellFile struct {
	Allow *[]string `json:"allow"`
}

type roleFile struct {
	Name        *string    `json:"name"`
	Description *string    `json:"description"`
	Actor       *string    `json:"actor"`
	MinRig      *string    `json:"minRig"`
	Rules       *string    `json:"rules"`
	Agents      *[]string  `json:"agents"`
	Skills      *[]string  `json:"skills"`
	Workflows   *[]string  `json:"workflows"`
	Rig         *rigFile   `json:"rig"`
	Shell       *shellFile `json:"shell"`
}

// source holds the items read from one tree. Paths are for error messages.
type source struct {
	roles     map[string]model.Role
	rolePaths map[string]string
	prompts   map[string]model.Prompt
	skills    map[string]model.Skill
	workflows map[string]model.Workflow
	harness   map[string]map[string][]byte
}

// Load reads base, then applies overlay (which may be nil), then checks that
// every reference a role makes resolves.
func Load(base fs.FS, overlay *Overlay) (*Catalog, error) {
	srcs := []*source{}
	b, err := readSource(base, "", "")
	if err != nil {
		return nil, err
	}
	srcs = append(srcs, b)
	if overlay != nil {
		o, err := readSource(overlay.FS, overlay.Dir, overlay.Dir)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, o)
	}
	c := &Catalog{
		roles:     map[string]model.Role{},
		prompts:   map[string]model.Prompt{},
		skills:    map[string]model.Skill{},
		workflows: map[string]model.Workflow{},
		harnesses: map[string]memFS{},
	}
	rolePaths := map[string]string{}
	for _, s := range srcs {
		for k, v := range s.roles {
			c.roles[k] = v
			rolePaths[k] = s.rolePaths[k]
		}
		for k, v := range s.prompts {
			c.prompts[k] = v
		}
		for k, v := range s.skills {
			c.skills[k] = v
		}
		for k, v := range s.workflows {
			c.workflows[k] = v
		}
		for h, files := range s.harness {
			m, ok := c.harnesses[h]
			if !ok {
				m = memFS{files: map[string][]byte{}}
				c.harnesses[h] = m
			}
			for p, d := range files {
				m.files[p] = d
			}
		}
	}
	for _, name := range sortedKeys(c.roles) {
		r := c.roles[name]
		p := rolePaths[name]
		for _, a := range r.Agents {
			if _, ok := c.prompts[a]; !ok {
				return nil, fmt.Errorf("%s: role %q uses agent %q with no prompt in pool/prompts", p, name, a)
			}
		}
		for _, s := range r.Skills {
			if _, ok := c.skills[s]; !ok {
				return nil, fmt.Errorf("%s: role %q uses skill %q with no folder in pool/skills", p, name, s)
			}
		}
		for _, w := range r.Workflows {
			if _, ok := c.workflows[w]; !ok {
				return nil, fmt.Errorf("%s: role %q uses workflow %q with no file in workflows", p, name, w)
			}
		}
	}
	return c, nil
}

func readSource(fsys fs.FS, label, osDir string) (*source, error) {
	s := &source{
		roles:     map[string]model.Role{},
		rolePaths: map[string]string{},
		prompts:   map[string]model.Prompt{},
		skills:    map[string]model.Skill{},
		workflows: map[string]model.Workflow{},
		harness:   map[string]map[string][]byte{},
	}
	lp := func(p string) string {
		if label == "" {
			return p
		}
		return filepath.Join(label, filepath.FromSlash(p))
	}
	if err := readPrompts(fsys, lp, s); err != nil {
		return nil, err
	}
	if err := readSkills(fsys, lp, s); err != nil {
		return nil, err
	}
	if err := readWorkflows(fsys, lp, osDir, s); err != nil {
		return nil, err
	}
	if err := readRoles(fsys, lp, s); err != nil {
		return nil, err
	}
	if err := readHarnesses(fsys, lp, s); err != nil {
		return nil, err
	}
	return s, nil
}

// listDir returns the entries of dir, or nil when it does not exist.
func listDir(fsys fs.FS, dir string) ([]fs.DirEntry, error) {
	ents, err := fs.ReadDir(fsys, dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	kept := ents[:0]
	for _, e := range ents {
		if !skipName(e.Name()) {
			kept = append(kept, e)
		}
	}
	return kept, err
}

// skipName reports whether an entry is ignored, matching go:embed's rule so an
// embedded tree and an overlay load the same way.
func skipName(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

func isReadme(e fs.DirEntry) bool { return !e.IsDir() && e.Name() == "README.md" }

func readPrompts(fsys fs.FS, lp func(string) string, s *source) error {
	ents, err := listDir(fsys, "pool/prompts")
	if err != nil {
		return fmt.Errorf("%s: %w", lp("pool/prompts"), err)
	}
	for _, e := range ents {
		p := "pool/prompts/" + e.Name()
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if e.IsDir() || !ok {
			return fmt.Errorf("%s: a prompt must be a <agent>.md file", lp(p))
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return fmt.Errorf("%s: %w", lp(p), err)
		}
		d, body, err := readDoc(data, name)
		if err != nil {
			return fmt.Errorf("%s: %w", lp(p), err)
		}
		s.prompts[name] = model.Prompt{Name: name, Description: d, Body: body}
	}
	return nil
}

// readDoc parses front matter with exactly name and description, and checks the name.
func readDoc(data []byte, want string) (desc, body string, err error) {
	fields, body, err := frontmatter.Parse(data)
	if err != nil {
		return "", "", err
	}
	var name string
	seen := map[string]bool{}
	for _, f := range fields {
		if seen[f.Key] {
			return "", "", fmt.Errorf("duplicate front matter key %q", f.Key)
		}
		seen[f.Key] = true
		switch f.Key {
		case "name":
			name = f.Value
		case "description":
			desc = f.Value
		default:
			return "", "", fmt.Errorf("unknown front matter key %q", f.Key)
		}
	}
	if !seen["name"] || !seen["description"] {
		return "", "", errors.New("front matter needs name and description")
	}
	if name != want {
		return "", "", fmt.Errorf("name %q does not match %q", name, want)
	}
	if !namePat.MatchString(name) {
		return "", "", fmt.Errorf("name %q is not kebab-case", name)
	}
	return desc, body, nil
}

func readSkills(fsys fs.FS, lp func(string) string, s *source) error {
	ents, err := listDir(fsys, "pool/skills")
	if err != nil {
		return fmt.Errorf("%s: %w", lp("pool/skills"), err)
	}
	for _, e := range ents {
		if isReadme(e) {
			continue
		}
		dir := "pool/skills/" + e.Name()
		if !e.IsDir() {
			return fmt.Errorf("%s: a skill must be a folder", lp(dir))
		}
		files, err := readTree(fsys, dir)
		if err != nil {
			return fmt.Errorf("%s: %w", lp(dir), err)
		}
		var skill []model.File
		var main []byte
		for _, p := range sortedKeys(files) {
			skill = append(skill, model.File{Path: p, Data: files[p]})
			if p == "SKILL.md" {
				main = files[p]
			}
		}
		if main == nil {
			return fmt.Errorf("%s: missing SKILL.md", lp(dir))
		}
		desc, _, err := readDoc(main, e.Name())
		if err != nil {
			return fmt.Errorf("%s: %w", lp(dir+"/SKILL.md"), err)
		}
		s.skills[e.Name()] = model.Skill{Name: e.Name(), Description: desc, Files: skill}
	}
	return nil
}

// readTree returns every file under dir keyed by slash path relative to dir.
func readTree(fsys fs.FS, dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != dir && skipName(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		out[strings.TrimPrefix(p, dir+"/")] = data
		return nil
	})
	return out, err
}

func readWorkflows(fsys fs.FS, lp func(string) string, osDir string, s *source) error {
	ents, err := listDir(fsys, "workflows")
	if err != nil {
		return fmt.Errorf("%s: %w", lp("workflows"), err)
	}
	for _, e := range ents {
		if isReadme(e) {
			continue
		}
		p := "workflows/" + e.Name()
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if e.IsDir() || !ok {
			return fmt.Errorf("%s: a workflow must be a <name>.json file", lp(p))
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return fmt.Errorf("%s: %w", lp(p), err)
		}
		var head struct {
			Name *string `json:"name"`
		}
		if err := json.Unmarshal(data, &head); err != nil {
			return fmt.Errorf("%s: %w", lp(p), err)
		}
		if head.Name == nil || *head.Name != name {
			return fmt.Errorf("%s: top-level name does not match file name %q", lp(p), name)
		}
		if !namePat.MatchString(name) {
			return fmt.Errorf("%s: name %q is not kebab-case", lp(p), name)
		}
		w := model.Workflow{Name: name, Data: data}
		if osDir != "" {
			w.SourcePath = filepath.Join(osDir, "workflows", e.Name())
		}
		s.workflows[name] = w
	}
	return nil
}

func readRoles(fsys fs.FS, lp func(string) string, s *source) error {
	ents, err := listDir(fsys, "roles")
	if err != nil {
		return fmt.Errorf("%s: %w", lp("roles"), err)
	}
	for _, e := range ents {
		if isReadme(e) {
			continue
		}
		dir := "roles/" + e.Name()
		if !e.IsDir() {
			return fmt.Errorf("%s: a role must be a folder", lp(dir))
		}
		p := dir + "/role.json"
		role, err := readRole(fsys, dir, e.Name())
		if err != nil {
			return fmt.Errorf("%s: %w", lp(p), err)
		}
		s.roles[e.Name()] = role
		s.rolePaths[e.Name()] = lp(p)
	}
	return nil
}

func readRole(fsys fs.FS, dir, want string) (model.Role, error) {
	data, err := fs.ReadFile(fsys, dir+"/role.json")
	if err != nil {
		return model.Role{}, err
	}
	var f roleFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return model.Role{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return model.Role{}, errors.New("trailing data after JSON object")
	}
	missing := func(k string) error { return fmt.Errorf("missing required key %q", k) }
	switch {
	case f.Name == nil:
		return model.Role{}, missing("name")
	case f.Description == nil:
		return model.Role{}, missing("description")
	case f.Actor == nil:
		return model.Role{}, missing("actor")
	case f.MinRig == nil:
		return model.Role{}, missing("minRig")
	case f.Rules == nil:
		return model.Role{}, missing("rules")
	case f.Agents == nil:
		return model.Role{}, missing("agents")
	case f.Skills == nil:
		return model.Role{}, missing("skills")
	case f.Workflows == nil:
		return model.Role{}, missing("workflows")
	case f.Rig == nil || f.Rig.Allow == nil || f.Rig.Deny == nil:
		return model.Role{}, missing("rig (allow and deny)")
	case f.Shell == nil || f.Shell.Allow == nil:
		return model.Role{}, missing("shell (allow)")
	}
	if *f.Name != want {
		return model.Role{}, fmt.Errorf("name %q does not match folder %q", *f.Name, want)
	}
	if !namePat.MatchString(*f.Name) {
		return model.Role{}, fmt.Errorf("name %q is not kebab-case", *f.Name)
	}
	if *f.Description == "" {
		return model.Role{}, errors.New("description must not be empty")
	}
	if *f.Actor == "" {
		return model.Role{}, errors.New("actor must not be empty")
	}
	if !minRigPat.MatchString(*f.MinRig) {
		return model.Role{}, fmt.Errorf("minRig %q is not x.y.z", *f.MinRig)
	}
	if *f.Rules != path.Base(*f.Rules) || *f.Rules == "." || *f.Rules == ".." || *f.Rules == "" || skipName(*f.Rules) {
		return model.Role{}, fmt.Errorf("rules %q must be a file name in the role folder, not starting with . or _", *f.Rules)
	}
	for _, list := range [][]string{*f.Agents, *f.Skills, *f.Workflows} {
		for _, n := range list {
			if !namePat.MatchString(n) {
				return model.Role{}, fmt.Errorf("reference %q is not kebab-case", n)
			}
		}
	}
	for _, list := range [][]string{*f.Rig.Allow, *f.Rig.Deny, *f.Shell.Allow} {
		for _, c := range list {
			if !cmdPat.MatchString(c) {
				return model.Role{}, fmt.Errorf("command entry %q has characters outside a-z, 0-9, - and single spaces", c)
			}
		}
	}
	rules, err := fs.ReadFile(fsys, dir+"/"+*f.Rules)
	if err != nil {
		return model.Role{}, fmt.Errorf("rules file: %w", err)
	}
	return model.Role{
		Name: *f.Name, Description: *f.Description, Actor: *f.Actor, MinRig: *f.MinRig,
		Rules:  string(rules),
		Agents: *f.Agents, Skills: *f.Skills, Workflows: *f.Workflows,
		Rig:   model.RigPolicy{Allow: *f.Rig.Allow, Deny: *f.Rig.Deny},
		Shell: model.ShellPolicy{Allow: *f.Shell.Allow},
	}, nil
}

func readHarnesses(fsys fs.FS, lp func(string) string, s *source) error {
	ents, err := listDir(fsys, "harnesses")
	if err != nil {
		return fmt.Errorf("%s: %w", lp("harnesses"), err)
	}
	for _, e := range ents {
		if isReadme(e) {
			continue
		}
		dir := "harnesses/" + e.Name()
		if !e.IsDir() {
			return fmt.Errorf("%s: a harness must be a folder", lp(dir))
		}
		files, err := readTree(fsys, dir)
		if err != nil {
			return fmt.Errorf("%s: %w", lp(dir), err)
		}
		s.harness[e.Name()] = files
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func notFound(kind, name string) error {
	return fmt.Errorf("%s %q: %w", kind, name, ErrNotFound)
}

// Roles returns every role sorted by name.
func (c *Catalog) Roles() []model.Role {
	out := []model.Role{}
	for _, k := range sortedKeys(c.roles) {
		out = append(out, c.roles[k])
	}
	return out
}

// Role returns the named role.
func (c *Catalog) Role(name string) (model.Role, error) {
	r, ok := c.roles[name]
	if !ok {
		return model.Role{}, notFound("role", name)
	}
	return r, nil
}

// Prompt returns the named agent prompt.
func (c *Catalog) Prompt(name string) (model.Prompt, error) {
	p, ok := c.prompts[name]
	if !ok {
		return model.Prompt{}, notFound("prompt", name)
	}
	return p, nil
}

// Skill returns the named skill.
func (c *Catalog) Skill(name string) (model.Skill, error) {
	s, ok := c.skills[name]
	if !ok {
		return model.Skill{}, notFound("skill", name)
	}
	return s, nil
}

// Workflow returns the named workflow.
func (c *Catalog) Workflow(name string) (model.Workflow, error) {
	w, ok := c.workflows[name]
	if !ok {
		return model.Workflow{}, notFound("workflow", name)
	}
	return w, nil
}

// Workflows returns every workflow sorted by name.
func (c *Catalog) Workflows() []model.Workflow {
	out := []model.Workflow{}
	for _, k := range sortedKeys(c.workflows) {
		out = append(out, c.workflows[k])
	}
	return out
}

// Harnesses returns the harness folder names, sorted.
func (c *Catalog) Harnesses() []string {
	return sortedKeys(c.harnesses)
}

// HarnessFS returns the files of one harness, rooted at its folder, with
// overlay files winning over embedded ones.
func (c *Catalog) HarnessFS(name string) (fs.FS, error) {
	m, ok := c.harnesses[name]
	if !ok {
		return nil, notFound("harness", name)
	}
	return m, nil
}
