package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/AindriuB/rig-crew/internal/model"
)

const (
	sourceCatalog = "catalog"
	sourceUser    = "user"
)

type steeringFile struct {
	File        *string   `json:"file"`
	Load        *string   `json:"load"`
	Paths       *[]string `json:"paths"`
	Description *string   `json:"description"`
}

type packFile struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Actor       *string         `json:"actor"`
	MinRig      *string         `json:"minRig"`
	Requires    *[]string       `json:"requires"`
	Steering    *[]steeringFile `json:"steering"`
	Agents      *[]string       `json:"agents"`
	Skills      *[]string       `json:"skills"`
	Workflows   *[]string       `json:"workflows"`
	Rig         *rigFile        `json:"rig"`
	Shell       *shellFile      `json:"shell"`
}

type profileFile struct {
	Name     *string   `json:"name"`
	Packs    *[]string `json:"packs"`
	Workflow *string   `json:"workflow"`
}

// packEntry is a loaded pack with the paths and local skills that came with it.
type packEntry struct {
	pack       model.Pack
	path       string // pack.json, for error messages
	local      map[string]model.Skill
	localPaths map[string]string
}

type profileEntry struct {
	profile model.Profile
	path    string
}

func readPacks(fsys fs.FS, lp func(string) string, s *source) error {
	ents, err := listDir(fsys, "packs")
	if err != nil {
		return fmt.Errorf("%s: %w", lp("packs"), err)
	}
	for _, e := range ents {
		if isReadme(e) {
			continue
		}
		dir := "packs/" + e.Name()
		if !e.IsDir() {
			return fmt.Errorf("%s: a pack must be a folder", lp(dir))
		}
		pe, err := readPack(fsys, dir, e.Name(), lp)
		if err != nil {
			return err
		}
		s.packs[e.Name()] = pe
	}
	return nil
}

func readUserPacks(u UserRoots, c *Catalog) error {
	if u.Packs == nil {
		return nil
	}
	lp := func(p string) string { return filepath.Join(u.PacksDir, filepath.FromSlash(p)) }
	ents, err := listDir(u.Packs, ".")
	if err != nil {
		return fmt.Errorf("%s: %w", u.PacksDir, err)
	}
	for _, e := range ents {
		if isReadme(e) {
			continue
		}
		if !e.IsDir() {
			return fmt.Errorf("%s: a pack must be a folder", lp(e.Name()))
		}
		pe, err := readPack(u.Packs, e.Name(), e.Name(), lp)
		if err != nil {
			return err
		}
		if c.shipped[e.Name()] {
			return fmt.Errorf("%s: user pack %q has the name of a catalog pack; rename it", pe.path, e.Name())
		}
		c.packs[e.Name()] = pe
	}
	return nil
}

// readPack reads and checks the pack folder dir, whose name must be want.
// Checks that need other items wait for checkPack.
func readPack(fsys fs.FS, dir, want string, lp func(string) string) (packEntry, error) {
	file := path.Join(dir, "pack.json")
	pe := packEntry{path: lp(file)}
	fail := func(format string, a ...any) (packEntry, error) {
		return packEntry{}, fmt.Errorf("%s: %s", pe.path, fmt.Sprintf(format, a...))
	}
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return fail("%v", err)
	}
	var f packFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return fail("%v", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fail("trailing data after JSON object")
	}
	missing := func(k string) (packEntry, error) { return fail("missing required key %q", k) }
	if f.Name == nil {
		return missing("name")
	}
	if f.Description == nil {
		return missing("description")
	}
	isRole := f.Actor != nil
	if isRole {
		switch {
		case f.MinRig == nil:
			return fail("role pack (it has an actor) is missing required key %q", "minRig")
		case f.Workflows == nil:
			return fail("role pack (it has an actor) is missing required key %q", "workflows")
		case f.Agents == nil:
			return missing("agents")
		case f.Skills == nil:
			return missing("skills")
		case f.Rig == nil || f.Rig.Allow == nil || f.Rig.Deny == nil:
			return missing("rig (allow and deny)")
		case f.Shell == nil || f.Shell.Allow == nil:
			return missing("shell (allow)")
		}
		if len(*f.Workflows) == 0 {
			return fail("workflows must not be empty for a role pack")
		}
	} else {
		if f.Workflows != nil {
			return fail("capability pack (it has no actor) must not have workflows")
		}
		if f.Rig != nil && (f.Rig.Allow == nil || f.Rig.Deny == nil) {
			return missing("rig (allow and deny)")
		}
		if f.Shell != nil && f.Shell.Allow == nil {
			return missing("shell (allow)")
		}
	}
	if *f.Name != want {
		return fail("name %q does not match folder %q", *f.Name, want)
	}
	if !namePat.MatchString(*f.Name) {
		return fail("name %q is not kebab-case", *f.Name)
	}
	if *f.Description == "" {
		return fail("description must not be empty")
	}
	p := model.Pack{Name: *f.Name, Description: *f.Description}
	if isRole {
		if *f.Actor == "" {
			return fail("actor must not be empty")
		}
		p.Actor = *f.Actor
		p.Workflows = *f.Workflows
	}
	if f.MinRig != nil {
		if !minRigPat.MatchString(*f.MinRig) {
			return fail("minRig %q is not x.y.z", *f.MinRig)
		}
		p.MinRig = *f.MinRig
	}
	for _, l := range []*[]string{f.Agents, f.Skills, f.Workflows, f.Requires} {
		if l == nil {
			continue
		}
		for _, n := range *l {
			if !namePat.MatchString(n) {
				return fail("reference %q is not kebab-case", n)
			}
		}
	}
	if f.Agents != nil {
		p.Agents = *f.Agents
	}
	if f.Skills != nil {
		p.Skills = *f.Skills
	}
	if f.Requires != nil {
		p.Requires = *f.Requires
	}
	if f.Rig != nil {
		p.Rig = model.RigPolicy{Allow: *f.Rig.Allow, Deny: *f.Rig.Deny}
	}
	if f.Shell != nil {
		p.Shell = model.ShellPolicy{Allow: *f.Shell.Allow}
	}
	for _, list := range [][]string{p.Rig.Allow, p.Rig.Deny, p.Shell.Allow} {
		for _, c := range list {
			if !cmdPat.MatchString(c) {
				return fail("command entry %q has characters outside a-z, 0-9, - and single spaces", c)
			}
		}
	}
	rendered := map[string]bool{}
	seenFile := map[string]bool{}
	if f.Steering != nil {
		for i, sf := range *f.Steering {
			st, err := readSteering(fsys, dir, i, sf)
			if err != nil {
				return fail("%v", err)
			}
			if seenFile[st.File] {
				return fail("steering file %q is listed twice", st.File)
			}
			seenFile[st.File] = true
			if st.Load != "always" {
				name := steeringSkillName(st.File)
				if !namePat.MatchString(name) {
					return fail("%s steering file %q becomes the skill name %q, which is not kebab-case", st.Load, st.File, name)
				}
				if rendered[name] {
					return fail("%s steering file %q renders as a skill named %q, which another steering file of the pack also renders", st.Load, st.File, name)
				}
				for _, sk := range p.Skills {
					if sk == name {
						return fail("%s steering file %q renders as a skill named %q, which the pack also lists in skills", st.Load, st.File, name)
					}
				}
				rendered[name] = true
			}
			p.Steering = append(p.Steering, st)
		}
	}
	pe.pack = p
	local, paths, err := readLocalSkills(fsys, dir, lp)
	if err != nil {
		return packEntry{}, err
	}
	for name := range local {
		if rendered[name] {
			return fail("steering renders as a skill named %q, which is also a local skill of the pack", name)
		}
	}
	pe.local, pe.localPaths = local, paths
	return pe, nil
}

func readSteering(fsys fs.FS, dir string, i int, sf steeringFile) (model.Steering, error) {
	if sf.File == nil || sf.Load == nil {
		return model.Steering{}, fmt.Errorf("steering entry %d needs file and load", i)
	}
	file := *sf.File
	if file != path.Base(file) || file == "." || file == ".." || file == "" || skipName(file) {
		return model.Steering{}, fmt.Errorf("steering file %q must be a file name in the pack folder, not starting with . or _", file)
	}
	st := model.Steering{File: file, Load: *sf.Load}
	switch *sf.Load {
	case "always":
		if sf.Paths != nil {
			return st, fmt.Errorf("steering file %q: paths is only allowed with load \"paths\"", file)
		}
	case "paths":
		if sf.Paths == nil || len(*sf.Paths) == 0 {
			return st, fmt.Errorf("steering file %q: load \"paths\" needs a non-empty paths list", file)
		}
		for _, g := range *sf.Paths {
			if g == "" {
				return st, fmt.Errorf("steering file %q: a paths entry must not be empty", file)
			}
		}
		if sf.Description == nil || *sf.Description == "" {
			return st, fmt.Errorf("steering file %q: load \"paths\" needs a description", file)
		}
		st.Paths = *sf.Paths
	case "on-demand":
		if sf.Paths != nil {
			return st, fmt.Errorf("steering file %q: paths is only allowed with load \"paths\"", file)
		}
		if sf.Description == nil || *sf.Description == "" {
			return st, fmt.Errorf("steering file %q: load \"on-demand\" needs a description", file)
		}
	default:
		return st, fmt.Errorf("steering file %q: unknown load %q, want always, paths or on-demand", file, *sf.Load)
	}
	if sf.Description != nil {
		st.Description = *sf.Description
	}
	body, err := fs.ReadFile(fsys, path.Join(dir, file))
	if err != nil {
		return st, fmt.Errorf("steering file %q: %w", file, err)
	}
	st.Body = string(body)
	return st, nil
}

// readLocalSkills reads dir/skills/<name>/SKILL.md folders.
func readLocalSkills(fsys fs.FS, dir string, lp func(string) string) (map[string]model.Skill, map[string]string, error) {
	local := map[string]model.Skill{}
	paths := map[string]string{}
	sdir := path.Join(dir, "skills")
	ents, err := listDir(fsys, sdir)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", lp(sdir), err)
	}
	for _, e := range ents {
		if isReadme(e) {
			continue
		}
		d := path.Join(sdir, e.Name())
		if !e.IsDir() {
			return nil, nil, fmt.Errorf("%s: a skill must be a folder", lp(d))
		}
		sk, err := readSkillDir(fsys, d, e.Name(), lp)
		if err != nil {
			return nil, nil, err
		}
		local[e.Name()] = sk
		paths[e.Name()] = lp(d)
	}
	return local, paths, nil
}

// addLocalSkills puts a pack's local skills into the catalog's skill set,
// failing when a name is already taken.
func (c *Catalog) addLocalSkills(pe packEntry) error {
	for _, name := range sortedKeys(pe.local) {
		if prev, ok := c.skills[name]; ok {
			if prev.Pack == "" {
				return fmt.Errorf("%s: local skill %q has the name of a pool skill", pe.localPaths[name], name)
			}
			return fmt.Errorf("%s: local skill %q has the name of a local skill of pack %q", pe.localPaths[name], name, prev.Pack)
		}
		sk := pe.local[name]
		sk.Pack = pe.pack.Name
		c.skills[name] = sk
	}
	return nil
}

// steeringSkillName is the skill name a harness without a mode for the
// steering file would give it: the file's base name.
func steeringSkillName(file string) string {
	return strings.TrimSuffix(file, path.Ext(file))
}

// checkSteeringNames fails when two packs have steering files that would render
// as skills of one name.
func (c *Catalog) checkSteeringNames() error {
	owner := map[string]string{}
	for _, pn := range sortedKeys(c.packs) {
		pe := c.packs[pn]
		for _, st := range pe.pack.Steering {
			if st.Load == "always" {
				continue
			}
			name := steeringSkillName(st.File)
			if prev, ok := owner[name]; ok && prev != pn {
				return fmt.Errorf("%s: %s steering file %q renders as a skill named %q, which pack %q also renders", pe.path, st.Load, st.File, name, prev)
			}
			owner[name] = pn
		}
	}
	return nil
}

// checkPack checks every reference the pack makes against the catalog.
func (c *Catalog) checkPack(pe packEntry) error {
	p, at := pe.pack, pe.path
	for _, a := range p.Agents {
		if _, ok := c.prompts[a]; !ok {
			return fmt.Errorf("%s: pack %q uses agent %q with no prompt in pool/prompts", at, p.Name, a)
		}
	}
	for _, s := range p.Skills {
		sk, ok := c.skills[s]
		if !ok {
			return fmt.Errorf("%s: pack %q uses skill %q with no folder in pool/skills or its own skills", at, p.Name, s)
		}
		if sk.Pack != "" && sk.Pack != p.Name {
			return fmt.Errorf("%s: pack %q uses skill %q, which is local to pack %q", at, p.Name, s, sk.Pack)
		}
	}
	for _, w := range p.Workflows {
		if _, ok := c.workflows[w]; !ok {
			return fmt.Errorf("%s: pack %q uses workflow %q with no file in workflows", at, p.Name, w)
		}
	}
	for _, r := range p.Requires {
		dep, ok := c.packs[r]
		if !ok {
			return fmt.Errorf("%s: pack %q requires %q, which is not a pack in the catalog", at, p.Name, r)
		}
		if dep.pack.IsRole() {
			return fmt.Errorf("%s: pack %q requires %q, which is a role pack; requires may name only capability packs", at, p.Name, r)
		}
	}
	return c.checkCycle(pe, []string{p.Name})
}

func (c *Catalog) checkCycle(pe packEntry, chain []string) error {
	for _, r := range pe.pack.Requires {
		for _, seen := range chain {
			if seen == r {
				return fmt.Errorf("%s: pack %q has a requires cycle: %s", pe.path, chain[0], strings.Join(append(append([]string{}, chain...), r), " -> "))
			}
		}
		dep, ok := c.packs[r]
		if !ok {
			continue
		}
		if err := c.checkCycle(dep, append(chain, r)); err != nil {
			return err
		}
	}
	return nil
}

func readProfiles(fsys fs.FS, lp func(string) string, s *source) error {
	ents, err := listDir(fsys, "profiles")
	if err != nil {
		return fmt.Errorf("%s: %w", lp("profiles"), err)
	}
	for _, e := range ents {
		if isReadme(e) {
			continue
		}
		pe, err := readProfile(fsys, "profiles", e, sourceCatalog, lp)
		if err != nil {
			return err
		}
		s.profiles[pe.profile.Name] = pe
	}
	return nil
}

func readUserProfiles(u UserRoots, c *Catalog) error {
	if u.Profiles == nil {
		return nil
	}
	lp := func(p string) string { return filepath.Join(u.ProfilesDir, filepath.FromSlash(p)) }
	ents, err := listDir(u.Profiles, ".")
	if err != nil {
		return fmt.Errorf("%s: %w", u.ProfilesDir, err)
	}
	for _, e := range ents {
		if isReadme(e) {
			continue
		}
		pe, err := readProfile(u.Profiles, ".", e, sourceUser, lp)
		if err != nil {
			return err
		}
		c.profiles[pe.profile.Name] = pe
	}
	return nil
}

func readProfile(fsys fs.FS, dir string, e fs.DirEntry, src string, lp func(string) string) (profileEntry, error) {
	file := path.Join(dir, e.Name())
	at := lp(file)
	if dir == "." {
		at = lp(e.Name())
	}
	name, ok := strings.CutSuffix(e.Name(), ".json")
	if e.IsDir() || !ok {
		return profileEntry{}, fmt.Errorf("%s: a profile must be a <name>.json file", at)
	}
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return profileEntry{}, fmt.Errorf("%s: %w", at, err)
	}
	var f profileFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return profileEntry{}, fmt.Errorf("%s: %w", at, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return profileEntry{}, fmt.Errorf("%s: trailing data after JSON object", at)
	}
	if f.Name == nil {
		return profileEntry{}, fmt.Errorf("%s: missing required key %q", at, "name")
	}
	if f.Packs == nil {
		return profileEntry{}, fmt.Errorf("%s: missing required key %q", at, "packs")
	}
	if *f.Name != name {
		return profileEntry{}, fmt.Errorf("%s: name %q does not match file name %q", at, *f.Name, name)
	}
	if !namePat.MatchString(name) {
		return profileEntry{}, fmt.Errorf("%s: name %q is not kebab-case", at, name)
	}
	if len(*f.Packs) == 0 {
		return profileEntry{}, fmt.Errorf("%s: packs must not be empty", at)
	}
	pr := model.Profile{Name: name, Packs: *f.Packs, Source: src}
	if f.Workflow != nil {
		pr.Workflow = *f.Workflow
	}
	return profileEntry{profile: pr, path: at}, nil
}

func (c *Catalog) checkProfile(pe profileEntry) error {
	pr := pe.profile
	for _, p := range pr.Packs {
		if _, ok := c.packs[p]; !ok {
			return fmt.Errorf("%s: profile %q names pack %q, which is not in the catalog", pe.path, pr.Name, p)
		}
	}
	if pr.Workflow != "" {
		if _, ok := c.workflows[pr.Workflow]; !ok {
			return fmt.Errorf("%s: profile %q names workflow %q with no file in workflows", pe.path, pr.Name, pr.Workflow)
		}
	}
	return nil
}

// ValidatePackDir validates one pack folder against the catalog, as `pack add`
// does. fsys is rooted at the pack folder and folder is its name. It returns
// the pack and every file of the folder. A pack of the same name already in
// the catalog from the user's root is replaced for the check; one shipped with
// the catalog is an error.
func (c *Catalog) ValidatePackDir(fsys fs.FS, folder string) (model.Pack, []model.File, error) {
	lp := func(p string) string { return filepath.Join(folder, filepath.FromSlash(p)) }
	pe, err := readPack(fsys, ".", folder, lp)
	if err != nil {
		return model.Pack{}, nil, err
	}
	if c.shipped[folder] {
		return model.Pack{}, nil, fmt.Errorf("%s: pack %q has the name of a catalog pack; rename it", pe.path, folder)
	}
	d := &Catalog{
		packs: map[string]packEntry{}, shipped: c.shipped, profiles: c.profiles, prompts: c.prompts,
		skills: map[string]model.Skill{}, workflows: c.workflows, harnesses: c.harnesses,
	}
	for k, v := range c.packs {
		if k != folder {
			d.packs[k] = v
		}
	}
	for k, v := range c.skills {
		if v.Pack != folder {
			d.skills[k] = v
		}
	}
	d.packs[folder] = pe
	if err := d.addLocalSkills(pe); err != nil {
		return model.Pack{}, nil, err
	}
	for _, name := range sortedKeys(d.packs) {
		if err := d.checkPack(d.packs[name]); err != nil {
			return model.Pack{}, nil, err
		}
	}
	if err := d.checkSteeringNames(); err != nil {
		return model.Pack{}, nil, err
	}
	files, err := readTree(fsys, ".")
	if err != nil {
		return model.Pack{}, nil, fmt.Errorf("%s: %w", folder, err)
	}
	var out []model.File
	for _, p := range sortedKeys(files) {
		out = append(out, model.File{Path: p, Data: files[p]})
	}
	return pe.pack, out, nil
}

// Packs returns every pack sorted by name.
func (c *Catalog) Packs() []model.Pack {
	out := []model.Pack{}
	for _, k := range sortedKeys(c.packs) {
		out = append(out, c.packs[k].pack)
	}
	return out
}

// RolePacks returns every role pack sorted by name.
func (c *Catalog) RolePacks() []model.Pack {
	out := []model.Pack{}
	for _, p := range c.Packs() {
		if p.IsRole() {
			out = append(out, p)
		}
	}
	return out
}

// Pack returns the named pack.
func (c *Catalog) Pack(name string) (model.Pack, error) {
	p, ok := c.packs[name]
	if !ok {
		return model.Pack{}, notFound("pack", name)
	}
	return p.pack, nil
}

// RolePack returns the named role pack; a capability pack is not found.
func (c *Catalog) RolePack(name string) (model.Pack, error) {
	p, ok := c.packs[name]
	if !ok || !p.pack.IsRole() {
		return model.Pack{}, notFound("role", name)
	}
	return p.pack, nil
}

// LocalSkills returns the names of the skills local to the named pack, sorted.
func (c *Catalog) LocalSkills(pack string) []string {
	out := []string{}
	for _, k := range sortedKeys(c.skills) {
		if c.skills[k].Pack == pack {
			out = append(out, k)
		}
	}
	return out
}

// Profiles returns every profile sorted by name.
func (c *Catalog) Profiles() []model.Profile {
	out := []model.Profile{}
	for _, k := range sortedKeys(c.profiles) {
		out = append(out, c.profiles[k].profile)
	}
	return out
}

// Profile returns the named profile.
func (c *Catalog) Profile(name string) (model.Profile, error) {
	p, ok := c.profiles[name]
	if !ok {
		return model.Profile{}, notFound("profile", name)
	}
	return p.profile, nil
}
