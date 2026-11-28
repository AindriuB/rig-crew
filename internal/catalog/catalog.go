// Package catalog loads rig-crew content from an fs.FS into neutral types.
package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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
	packs     map[string]packEntry
	profiles  map[string]profileEntry
	shipped   map[string]bool // pack names from the base tree or overlay
	prompts   map[string]model.Prompt
	skills    map[string]model.Skill // pool skills and pack-local skills
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

// source holds the items read from one tree. Paths are for error messages.
type source struct {
	packs     map[string]packEntry
	profiles  map[string]profileEntry
	prompts   map[string]model.Prompt
	skills    map[string]model.Skill
	workflows map[string]model.Workflow
	harness   map[string]map[string][]byte
}

// Option adjusts Load.
type Option func(*loadOpts)

type loadOpts struct{ user UserRoots }

// UserRoots are the user's own pack and profile folders. Packs holds
// <name>/pack.json folders, as `pack add` copies them; Profiles holds
// <name>.json files. A nil FS is an empty root. The Dir fields are OS paths
// used in error messages.
type UserRoots struct {
	Packs       fs.FS
	PacksDir    string
	Profiles    fs.FS
	ProfilesDir string
}

// WithUser makes Load read the user's pack and profile roots.
func WithUser(u UserRoots) Option { return func(o *loadOpts) { o.user = u } }

// Load reads base, then applies overlay (which may be nil), then the user
// roots, then checks that every reference a pack or profile makes resolves.
func Load(base fs.FS, overlay *Overlay, opts ...Option) (*Catalog, error) {
	var o loadOpts
	for _, f := range opts {
		f(&o)
	}
	srcs := []*source{}
	b, err := readSource(base, "", "")
	if err != nil {
		return nil, err
	}
	srcs = append(srcs, b)
	if overlay != nil {
		if _, err := fs.Stat(overlay.FS, "roles"); err == nil {
			return nil, fmt.Errorf("%s: roles/ was renamed to packs/; move each folder to packs/ and rename role.json to pack.json", filepath.Join(overlay.Dir, "roles"))
		}
		ov, err := readSource(overlay.FS, overlay.Dir, overlay.Dir)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, ov)
	}
	c := &Catalog{
		packs:     map[string]packEntry{},
		profiles:  map[string]profileEntry{},
		shipped:   map[string]bool{},
		prompts:   map[string]model.Prompt{},
		skills:    map[string]model.Skill{},
		workflows: map[string]model.Workflow{},
		harnesses: map[string]memFS{},
	}
	for _, s := range srcs {
		for k, v := range s.packs {
			c.packs[k] = v
			c.shipped[k] = true
		}
		for k, v := range s.profiles {
			c.profiles[k] = v
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
	if err := readUserPacks(o.user, c); err != nil {
		return nil, err
	}
	for _, name := range sortedKeys(c.packs) {
		if err := c.addLocalSkills(c.packs[name]); err != nil {
			return nil, err
		}
	}
	for _, name := range sortedKeys(c.packs) {
		if err := c.checkPack(c.packs[name]); err != nil {
			return nil, err
		}
	}
	if err := c.checkSteeringNames(); err != nil {
		return nil, err
	}
	if err := readUserProfiles(o.user, c); err != nil {
		return nil, err
	}
	for _, name := range sortedKeys(c.profiles) {
		if err := c.checkProfile(c.profiles[name]); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func readSource(fsys fs.FS, label, osDir string) (*source, error) {
	s := &source{
		packs:     map[string]packEntry{},
		profiles:  map[string]profileEntry{},
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
	if err := readPacks(fsys, lp, s); err != nil {
		return nil, err
	}
	if err := readProfiles(fsys, lp, s); err != nil {
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
		skill, err := readSkillDir(fsys, dir, e.Name(), lp)
		if err != nil {
			return err
		}
		s.skills[e.Name()] = skill
	}
	return nil
}

// readSkillDir reads the skill folder dir, whose name must be name.
func readSkillDir(fsys fs.FS, dir, name string, lp func(string) string) (model.Skill, error) {
	files, err := readTree(fsys, dir)
	if err != nil {
		return model.Skill{}, fmt.Errorf("%s: %w", lp(dir), err)
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
		return model.Skill{}, fmt.Errorf("%s: missing SKILL.md", lp(dir))
	}
	desc, _, err := readDoc(main, name)
	if err != nil {
		return model.Skill{}, fmt.Errorf("%s: %w", lp(dir+"/SKILL.md"), err)
	}
	return model.Skill{Name: name, Description: desc, Files: skill}, nil
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
