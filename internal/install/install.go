// Package install is the only code that writes to disk: it applies a
// plan.Plan into one scope under a manifest and removes exactly what it
// recorded.
package install

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/AindriuB/rig-crew/internal/plan"
)

// Activation levels.
const (
	LevelGlobal  = "global"
	LevelProject = "project"
	LevelSession = "session"
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

	afterManifestRead func() // test seam: runs after the scope manifest is read, before the snapshot
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
	ErrUncleanPath   = errors.New("path is not in its cleaned form")
	errDuplicatePath = errors.New("path appears twice in the plan")
	ErrBadOwner      = errors.New("owner is empty or reserved")
	ErrBadActivation = errors.New("activation is not valid")
)

func relSlash(root, p string) string {
	rel, _ := filepath.Rel(root, p) // validated beforehand
	return filepath.ToSlash(rel)
}

// checkPath refuses a path that is relative or not already cleaned: the OS
// would resolve ".." through a symlink that the lexical checks never see.
func checkPath(p string) error {
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%s: %w", p, ErrOutsideRoot)
	}
	if filepath.Clean(p) != p {
		return fmt.Errorf("%s: %w", p, ErrUncleanPath)
	}
	return nil
}

// below reports whether p is strictly inside dir, and not in a state directory.
func below(dir, p string) error {
	if err := checkPath(p); err != nil {
		return err
	}
	rel, err := filepath.Rel(dir, p)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s: %w", p, ErrOutsideRoot)
	}
	return nil
}

func underRoot(root, p string) error {
	if err := below(root, p); err != nil {
		return err
	}
	rel, _ := filepath.Rel(root, p) // validated by below
	if rel == stateDir || strings.HasPrefix(rel, stateDir+string(filepath.Separator)) {
		return fmt.Errorf("%s: %w", p, ErrOutsideRoot)
	}
	return nil
}

// underHome checks a workflow path: below home, and not a state file of its directory.
func underHome(home, p string) error {
	if err := checkPath(home); err != nil {
		return fmt.Errorf("home: %w", err)
	}
	if err := below(home, p); err != nil {
		return err
	}
	if filepath.Base(p) == stateDir || filepath.Base(filepath.Dir(p)) == stateDir {
		return fmt.Errorf("%s: %w", p, ErrOutsideRoot)
	}
	return nil
}

// absRel turns a manifest path into an absolute one, refusing anything that
// is not exactly a clean path under root.
func absRel(root, rel string) (string, bool) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if underRoot(root, abs) != nil || relSlash(root, abs) != rel {
		return "", false
	}
	return abs, true
}

func conflictsErr(c []Conflict) error {
	sort.Slice(c, func(i, j int) bool { return c[i].Path < c[j].Path })
	return &ConflictError{Conflicts: c}
}

// readManifestAt reads the manifest at mp; base is where the symlink check starts.
func readManifestAt(mp, base string, v2 bool) (*manifest, error) {
	s, err := readSnap([]target{{path: mp, base: base}})
	if err != nil {
		return nil, err
	}
	m, err := decodeManifest(s[mp], v2)
	if err != nil {
		return nil, conflictsErr([]Conflict{{mp, err.Error()}})
	}
	return m, nil
}

func planLevel(a *plan.Activation) string {
	if a == nil {
		return ""
	}
	return a.Level
}

func recLevel(a *actRec) string {
	if a == nil {
		return ""
	}
	return a.Level
}

func v2ManifestPath(sd string) string { return filepath.Join(sd, "manifest.json") }

// globalDirOf is the global state directory a project activation is listed in.
func globalDirOf(level, home string) string {
	if level != LevelProject || home == "" {
		return ""
	}
	return filepath.Join(home, stateDir)
}

// checkRoot is the one root predicate Apply and Remove share: the root is
// non-empty, strictly below the state dir's parent, not the state dir, and the
// state dir is not inside it.
func checkRoot(root, sd string) error {
	if root == "" || root == sd || below(filepath.Dir(sd), root) != nil || below(root, sd) == nil {
		return fmt.Errorf("root %q is not strictly under %s, or overlaps the state dir %s: %w", root, filepath.Dir(sd), sd, ErrOutsideRoot)
	}
	return nil
}

// checkRec refuses a v2 record whose fields name another home or project than
// the caller's: a project manifest can come from a cloned repo.
func checkRec(rec *roleRec, home, sd string) error {
	if rec.Home != "" && rec.Home != home {
		return fmt.Errorf("record names home %s, not %s: %w", rec.Home, home, ErrOutsideRoot)
	}
	if rec.Project != "" && rec.Project != filepath.Dir(sd) {
		return fmt.Errorf("record names project %s, not %s: %w", rec.Project, filepath.Dir(sd), ErrBadActivation)
	}
	if recLevel(rec.Activation) == LevelProject && sd == filepath.Join(home, stateDir) {
		return fmt.Errorf("a project activation cannot be recorded in the global state dir %s: %w", sd, ErrBadActivation)
	}
	return nil
}

// checkV2 validates the v2 fields of a plan.
func checkV2(p plan.Plan) error {
	if err := checkPath(p.StateDir); err != nil {
		return fmt.Errorf("state dir: %w", err)
	}
	if p.Owner == "" || p.Owner == userOwner {
		return fmt.Errorf("owner %q: %w", p.Owner, ErrBadOwner)
	}
	a := p.Activation
	if a == nil {
		return checkRoot(p.Root, p.StateDir)
	}
	switch a.Level {
	case LevelGlobal, LevelSession:
	case LevelProject:
		if p.Scope.Global {
			return fmt.Errorf("a project activation needs a project scope: %w", ErrBadActivation)
		}
		if err := checkPath(p.Scope.Base); err != nil {
			return fmt.Errorf("project: %w", err)
		}
		if err := checkPath(p.Scope.Home); err != nil {
			return fmt.Errorf("home: %w", err)
		}
		if p.StateDir != filepath.Join(p.Scope.Base, stateDir) {
			return fmt.Errorf("a project activation needs the state dir %s: %w", filepath.Join(p.Scope.Base, stateDir), ErrBadActivation)
		}
		if globalDirOf(a.Level, p.Scope.Home) == p.StateDir {
			return fmt.Errorf("a project activation needs its own state dir: %w", ErrBadActivation)
		}
	default:
		return fmt.Errorf("level %q: %w", a.Level, ErrBadActivation)
	}
	return checkRoot(p.Root, p.StateDir)
}

// wfTargets are the paths to read for one workflow file.
func wfTargets(home, path string) []target {
	return []target{
		{path: path, base: home, skipBase: true},
		{path: manifestPath(filepath.Dir(path)), base: home, skipBase: true},
	}
}

// Apply writes the plan into its scope.
func Apply(ctx context.Context, p plan.Plan, opts Options) (Result, error) {
	root := p.Root
	if err := checkPath(root); err != nil {
		return Result{}, fmt.Errorf("root: %w", err)
	}
	v2 := p.StateDir != ""
	role, mp, base := p.Role, manifestPath(root), root
	if v2 {
		if err := checkV2(p); err != nil {
			return Result{}, err
		}
		role, mp, base = p.Owner, v2ManifestPath(p.StateDir), p.StateDir
	}
	seen := map[string]bool{}
	check := func(path string, err error) error {
		if err != nil {
			return err
		}
		c := filepath.Clean(path)
		if seen[c] {
			return fmt.Errorf("%s: %w", path, errDuplicatePath)
		}
		seen[c] = true
		return nil
	}
	for _, f := range p.Files {
		if err := check(f.Path, underRoot(root, f.Path)); err != nil {
			return Result{}, err
		}
	}
	for _, m := range p.Merges {
		if err := check(m.Path, underRoot(root, m.Path)); err != nil {
			return Result{}, err
		}
	}
	if w := p.Workflow; w != nil {
		if err := check(w.Path, underHome(p.Scope.Home, w.Path)); err != nil {
			return Result{}, err
		}
	}
	old, err := readManifestAt(mp, base, v2)
	if err != nil {
		return Result{}, err
	}
	if v2 && old.Roles[role] != nil {
		if err := checkRec(old.Roles[role], p.Scope.Home, p.StateDir); err != nil {
			return Result{}, err
		}
	}
	if opts.afterManifestRead != nil {
		opts.afterManifestRead()
	}
	ts := []target{{path: mp, base: base}}
	for _, f := range p.Files {
		ts = append(ts, target{path: f.Path, base: root})
	}
	for _, m := range p.Merges {
		ts = append(ts, target{path: m.Path, base: root})
	}
	j := job{root: root, state: p.StateDir}
	if v2 {
		for _, gd := range projectDirs(p, old.Roles[role]) {
			ts = append(ts, target{path: v2ManifestPath(gd), base: gd})
			j.before = append(j.before, gd)
		}
	}
	if r := old.Roles[role]; r != nil {
		wfHome := r.Home
		if v2 {
			wfHome = p.Scope.Home
		}
		for rel := range r.Files {
			if abs, ok := absRel(root, rel); ok {
				ts = append(ts, target{path: abs, base: root})
			}
		}
		for _, m := range r.Merges {
			if abs, ok := absRel(root, m.Path); ok {
				ts = append(ts, target{path: abs, base: root})
			}
		}
		if r.Workflow != "" && (p.Workflow == nil || p.Workflow.Path != r.Workflow) && underHome(wfHome, r.Workflow) == nil {
			ts = append(ts, wfTargets(wfHome, r.Workflow)...)
			j.wfDirs = append(j.wfDirs, filepath.Dir(r.Workflow))
		}
	}
	if w := p.Workflow; w != nil {
		ts = append(ts, wfTargets(p.Scope.Home, w.Path)...)
		j.wfDirs = append(j.wfDirs, filepath.Dir(w.Path))
	}
	j.targets = ts
	j.compute = func(s snapshot) (*outcome, error) { return computeApply(p, root, mp, s) }
	return run(ctx, j, opts)
}

// projectDirs lists the global state directory a plan's project activation, or
// the one its owner had before, is listed in.
func projectDirs(p plan.Plan, old *roleRec) []string {
	var out []string
	if gd := globalDirOf(planLevel(p.Activation), p.Scope.Home); gd != "" {
		out = append(out, gd)
	}
	if old != nil {
		if gd := globalDirOf(recLevel(old.Activation), p.Scope.Home); gd != "" && !slices.Contains(out, gd) {
			out = append(out, gd)
		}
	}
	return out
}

// syncProject makes the global manifest in gd list proj exactly when some
// record of man holds a project activation for it.
func syncProject(c *collector, man *manifest, gd, proj string) {
	want := false
	for _, r := range man.Roles {
		if r.Activation != nil && r.Activation.Level == LevelProject && r.Project == proj {
			want = true
		}
	}
	gmp := v2ManifestPath(gd)
	cur := c.get(gmp)
	gm, err := decodeManifest(cur, true)
	if err != nil {
		c.conflict(gmp, err.Error())
		return
	}
	if slices.Contains(gm.projects, proj) == want {
		return
	}
	if want {
		gm.projects = append(gm.projects, proj)
		sort.Strings(gm.projects)
	} else {
		gm.projects = slices.DeleteFunc(gm.projects, func(x string) bool { return x == proj })
	}
	if len(gm.Roles) == 0 && len(gm.projects) == 0 {
		if cur.exists {
			c.out.ops = append(c.out.ops, op{path: gmp, remove: true})
			c.out.cleanup = append(c.out.cleanup, gd)
		}
		return
	}
	c.out.ops = append(c.out.ops, op{path: gmp, data: encodeManifest(gm)})
}

// collector gathers conflicts and actions while a plan is computed, and
// refuses, as a conflict, any path the snapshot does not hold.
type collector struct {
	snap      snapshot
	conflicts []Conflict
	out       *outcome
}

func (c *collector) conflict(path, reason string) {
	c.conflicts = append(c.conflicts, Conflict{path, reason})
}

func (c *collector) act(path, kind string) { c.out.actions = append(c.out.actions, Action{path, kind}) }

func (c *collector) get(path string) fstate {
	s, ok := c.snap[path]
	if !ok {
		c.conflict(path, "recorded or planned after it was read; run again")
	}
	return s
}

// wfEditor edits the manifests of the workflow directories a run touches.
type wfEditor struct {
	*collector
	root, role string
	mans       map[string]*wfManifest // by directory; nil when unreadable
	dirty      map[string]bool        // directories a drop or add changed
}

func (w *wfEditor) man(dir string) *wfManifest {
	if m, ok := w.mans[dir]; ok {
		return m
	}
	wmp := manifestPath(dir)
	m, err := decodeWf(w.get(wmp))
	if err != nil {
		w.conflict(wmp, err.Error())
		m = nil
	}
	w.mans[dir] = m
	return m
}

// drop removes this role's reference to the workflow at path, and the file
// itself when it was the last reference and is unmodified.
func (w *wfEditor) drop(path string) {
	dir, name := filepath.Dir(path), filepath.Base(path)
	if !w.get(manifestPath(dir)).exists {
		return
	}
	wm := w.man(dir)
	if wm == nil {
		return
	}
	if !containsRef(wm.Refs, wfRef{Name: name, Root: w.root, Role: w.role}) {
		return // not this record's manifest: leave it alone
	}
	w.dirty[dir] = true
	var refs []wfRef
	left := false
	for _, r := range wm.Refs {
		if r.Name == name && r.Root == w.root && r.Role == w.role {
			continue
		}
		if r.Name == name {
			left = true
		}
		refs = append(refs, r)
	}
	wm.Refs = refs
	cur := w.get(path)
	switch {
	case left:
		w.act(path, KindUnchanged)
	case !cur.exists:
		delete(wm.Files, name)
	case hashHex(cur.data) != wm.Files[name]:
		w.conflict(path, "workflow file modified since rig-crew wrote it")
	default:
		delete(wm.Files, name)
		w.act(path, KindRemove)
		w.out.ops = append(w.out.ops, op{path: path, remove: true})
	}
}

// add makes the workflow file exist and records this role's reference to it.
func (w *wfEditor) add(wf *plan.WorkflowFile) {
	dir, name := filepath.Dir(wf.Path), filepath.Base(wf.Path)
	wm := w.man(dir)
	if wm == nil {
		return
	}
	w.dirty[dir] = true
	cur := w.get(wf.Path)
	ownHash, own := wm.Files[name]
	var kind string
	switch {
	case !cur.exists:
		kind = KindCreate
	case own && hashHex(cur.data) != ownHash:
		w.conflict(wf.Path, "workflow file modified since rig-crew wrote it")
	case bytesEq(cur.data, wf.Data) && own:
		kind = KindUnchanged
	case bytesEq(cur.data, wf.Data):
		kind = KindAdopt
	case own:
		kind = KindUpdate
	default:
		w.conflict(wf.Path, "workflow file exists and differs")
	}
	if kind == "" {
		return
	}
	w.act(wf.Path, kind)
	if kind == KindCreate || kind == KindUpdate {
		w.out.ops = append(w.out.ops, op{path: wf.Path, data: wf.Data})
	}
	wm.Files[name] = hashHex(wf.Data)
	ref := wfRef{Name: name, Root: w.root, Role: w.role}
	if !containsRef(wm.Refs, ref) {
		wm.Refs = append(wm.Refs, ref)
	}
	sortRefs(wm.Refs)
}

// flush writes each edited workflow manifest, or removes it with its state
// directory when nothing references it any more.
func (w *wfEditor) flush() {
	var dirs []string
	for d := range w.mans {
		if w.dirty[d] {
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		wm, wmp := w.mans[d], manifestPath(d)
		if wm == nil {
			continue
		}
		cur := w.snap[wmp]
		if len(wm.Files) == 0 && len(wm.Refs) == 0 {
			if cur.exists {
				w.out.ops = append(w.out.ops, op{path: wmp, remove: true})
				w.out.cleanup = append(w.out.cleanup, filepath.Dir(wmp))
			}
			continue
		}
		if nb := encodeJSON(wm); !cur.exists || !bytesEq(nb, cur.data) {
			w.out.ops = append(w.out.ops, op{path: wmp, data: nb})
		}
	}
}

// dropMerge removes the entries role recorded in one settings file, as
// Uninstall does: entries another role also recorded stay, and a file the
// install created is deleted once only empty objects and arrays remain.
func dropMerge(c *collector, man *manifest, role string, mr mergeRec, abs string) {
	keep := map[string]bool{}
	createdAny := mr.Created
	var rest []string
	for name, r := range man.Roles {
		if name == role || !man.peer(r) {
			continue
		}
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
	cur := c.get(abs)
	if !cur.exists {
		return
	}
	res := removeMerge(cur, &mr, keep, createdAny && len(rest) == 0)
	switch {
	case res.conflict != "":
		c.conflict(abs, res.conflict)
	case res.del:
		c.act(abs, KindRemove)
		c.out.ops = append(c.out.ops, op{path: abs, remove: true})
		c.out.prune = append(c.out.prune, abs)
	case res.changed:
		c.act(abs, KindUnmerge)
		c.out.ops = append(c.out.ops, op{path: abs, data: res.data})
	default:
		c.act(abs, KindUnchanged)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func computeApply(p plan.Plan, root, mpath string, snap snapshot) (*outcome, error) {
	v2 := p.StateDir != ""
	role := p.Role
	if v2 {
		role = p.Owner
	}
	out := &outcome{root: root}
	c := &collector{snap: snap, out: out}
	man, err := decodeManifest(c.get(mpath), v2)
	if err != nil {
		return nil, conflictsErr([]Conflict{{mpath, err.Error()}})
	}
	man.root = root
	old := man.Roles[role]
	if old == nil {
		old = &roleRec{}
	}
	if v2 {
		if err := checkRec(old, p.Scope.Home, p.StateDir); err != nil {
			return nil, err
		}
	}
	if v2 && old.Root != "" && old.Root != root {
		return nil, conflictsErr([]Conflict{{mpath, fmt.Sprintf("owner %s is recorded under root %s", role, old.Root)}})
	}
	nr := &roleRec{Files: map[string]string{}}

	inPlan := map[string]bool{}
	for _, f := range p.Files {
		rel := relSlash(root, f.Path)
		inPlan[rel] = true
		cur := c.get(f.Path)
		ownHash, own := old.Files[rel]
		others := owners(man, role, rel)
		var kind string
		switch {
		case !cur.exists:
			kind = KindCreate
		case own && hashHex(cur.data) != ownHash:
			c.conflict(f.Path, "modified since rig-crew wrote it")
		case bytesEq(cur.data, f.Data) && own:
			kind = KindUnchanged
		case bytesEq(cur.data, f.Data):
			kind = KindAdopt
		case own && len(others) == 0:
			kind = KindUpdate
		case len(others) > 0:
			c.conflict(f.Path, fmt.Sprintf("also owned by role %s and differs", others[0]))
		default:
			c.conflict(f.Path, "exists and differs from what rig-crew would write")
		}
		if kind == "" {
			continue
		}
		nr.Files[rel] = hashHex(f.Data)
		c.act(f.Path, kind)
		if kind == KindCreate || kind == KindUpdate {
			out.ops = append(out.ops, op{path: f.Path, data: f.Data})
		}
	}
	for _, rel := range sortedKeys(old.Files) {
		abs, ok := absRel(root, rel)
		if !ok {
			c.conflict(mpath, fmt.Sprintf("manifest lists %q, which is not a path under the root", rel))
			continue
		}
		if inPlan[rel] || len(owners(man, role, rel)) > 0 {
			continue // still in the plan, or shared: only our record goes
		}
		cur := c.get(abs)
		switch {
		case !cur.exists:
		case hashHex(cur.data) != old.Files[rel]:
			c.conflict(abs, "modified since rig-crew wrote it")
		default:
			c.act(abs, KindRemove)
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
			if name != role && man.peer(r) {
				if mr := findMerge(r, rel); mr != nil {
					others = append(others, mr)
				}
			}
		}
		res := applyMerge(c.get(m.Path), m, rel, findMerge(old, rel), others)
		if res.conflict != "" {
			c.conflict(m.Path, res.conflict)
			continue
		}
		nr.Merges = append(nr.Merges, res.rec)
		switch {
		case res.changed && res.added:
			c.act(m.Path, KindMerge)
			out.ops = append(out.ops, op{path: m.Path, data: res.data})
		case res.changed:
			c.act(m.Path, KindUnmerge)
			out.ops = append(out.ops, op{path: m.Path, data: res.data})
		case res.dropped:
			c.act(m.Path, KindUnmerge)
		case !sameRec(findMerge(old, rel), res.rec):
			c.act(m.Path, KindAdopt)
		default:
			c.act(m.Path, KindUnchanged)
		}
	}
	for _, mr := range old.Merges {
		if planned[mr.Path] {
			continue
		}
		abs, ok := absRel(root, mr.Path)
		if !ok {
			c.conflict(mpath, fmt.Sprintf("manifest lists %q, which is not a path under the root", mr.Path))
			continue
		}
		dropMerge(c, man, role, mr, abs)
	}
	sort.Slice(nr.Merges, func(i, j int) bool { return nr.Merges[i].Path < nr.Merges[j].Path })

	w := &wfEditor{collector: c, root: root, role: role, mans: map[string]*wfManifest{}, dirty: map[string]bool{}}
	if old.Workflow != "" && (p.Workflow == nil || p.Workflow.Path != old.Workflow) {
		wfHome := old.Home
		if v2 {
			wfHome = p.Scope.Home
		}
		if err := underHome(wfHome, old.Workflow); err != nil {
			c.conflict(old.Workflow, "recorded workflow reference is not valid: "+err.Error())
		} else {
			w.drop(old.Workflow)
		}
	}
	if p.Workflow != nil {
		w.add(p.Workflow)
		nr.Workflow, nr.Home = p.Workflow.Path, p.Scope.Home
	}
	w.flush()
	man.Roles[role] = nr
	if v2 {
		nr.Root, nr.Harness, nr.Home = root, p.Harness, p.Scope.Home
		nr.UserFiles = old.UserFiles
		if a := p.Activation; a != nil {
			nr.Activation = &actRec{Harness: a.Harness, Level: a.Level, Profiles: a.Profiles, Packs: a.Packs}
			if a.Level == LevelProject {
				nr.Project = filepath.Dir(p.StateDir)
			}
		}
		proj := filepath.Dir(p.StateDir)
		for _, gd := range projectDirs(p, old) {
			syncProject(c, man, gd, proj)
		}
	}

	if len(c.conflicts) > 0 {
		return nil, conflictsErr(c.conflicts)
	}
	if nb := encodeManifest(man); !snap[mpath].exists || !bytesEq(nb, snap[mpath].data) {
		out.ops = append(out.ops, op{path: mpath, data: nb})
	}
	return out, nil
}

// Uninstall removes exactly what the role's manifest entry records.
func Uninstall(ctx context.Context, root, role string, opts Options) (Result, error) {
	if err := checkPath(root); err != nil {
		return Result{}, fmt.Errorf("root: %w", err)
	}
	return removeRec(ctx, root, "", "", role, opts)
}

// Remove deletes exactly one owner's record from the v2 manifest in stateDir:
// its files, merged entries and workflow reference, as Uninstall does for a role.
// home is the caller's home: no global path comes from the record.
func Remove(ctx context.Context, home, stateDir, owner string, opts Options) (Result, error) {
	if err := checkPath(home); err != nil {
		return Result{}, fmt.Errorf("home: %w", err)
	}
	if err := checkPath(stateDir); err != nil {
		return Result{}, fmt.Errorf("state dir: %w", err)
	}
	if owner == "" || owner == userOwner {
		return Result{}, fmt.Errorf("owner %q: %w", owner, ErrBadOwner)
	}
	return removeRec(ctx, "", home, stateDir, owner, opts)
}

// removeRec serves Uninstall (sd empty, root given) and Remove (root read from the record).
func removeRec(ctx context.Context, root, home, sd, role string, opts Options) (Result, error) {
	v2 := sd != ""
	mp, base, where := manifestPath(root), root, root
	if v2 {
		mp, base, where = v2ManifestPath(sd), sd, sd
	}
	man, err := readManifestAt(mp, base, v2)
	if err != nil {
		return Result{}, err
	}
	rec := man.Roles[role]
	if rec == nil {
		return Result{}, fmt.Errorf("%s in %s: %w", role, where, ErrNotInstalled)
	}
	wfHome := rec.Home
	if v2 {
		if err := checkRec(rec, home, sd); err != nil {
			return Result{}, err
		}
		if err := checkRoot(rec.Root, sd); err != nil {
			return Result{}, err
		}
		if err := checkPath(rec.Root); err != nil {
			return Result{}, fmt.Errorf("root: %w", err)
		}
		root, wfHome = rec.Root, home
	}
	if opts.afterManifestRead != nil {
		opts.afterManifestRead()
	}
	ts := []target{{path: mp, base: base}}
	for rel := range rec.Files {
		if abs, ok := absRel(root, rel); ok {
			ts = append(ts, target{path: abs, base: root})
		}
	}
	for _, m := range rec.Merges {
		if abs, ok := absRel(root, m.Path); ok {
			ts = append(ts, target{path: abs, base: root})
		}
	}
	j := job{root: root, state: sd}
	if gd := globalDirOf(recLevel(rec.Activation), home); v2 && gd != "" {
		ts = append(ts, target{path: v2ManifestPath(gd), base: gd})
		j.before = []string{gd}
	}
	if rec.Workflow != "" && underHome(wfHome, rec.Workflow) == nil {
		ts = append(ts, wfTargets(wfHome, rec.Workflow)...)
		j.wfDirs = []string{filepath.Dir(rec.Workflow)}
	}
	j.targets = ts
	j.compute = func(s snapshot) (*outcome, error) { return computeUninstall(root, role, mp, v2, s, where, home, sd) }
	return run(ctx, j, opts)
}

func computeUninstall(root, role, mpath string, v2 bool, snap snapshot, where, home, sd string) (*outcome, error) {
	out := &outcome{root: root}
	c := &collector{snap: snap, out: out}
	man, err := decodeManifest(c.get(mpath), v2)
	if err != nil {
		return nil, conflictsErr([]Conflict{{mpath, err.Error()}})
	}
	man.root = root
	rec := man.Roles[role]
	if rec == nil {
		return nil, fmt.Errorf("%s in %s: %w", role, where, ErrNotInstalled)
	}
	wfHome := rec.Home
	if v2 {
		if err := checkRec(rec, home, sd); err != nil {
			return nil, err
		}
		if checkRoot(rec.Root, sd) != nil || rec.Root != root {
			return nil, conflictsErr([]Conflict{{mpath, "record changed since it was read; run again"}})
		}
		wfHome = home
	}

	for _, rel := range sortedKeys(rec.Files) {
		abs, ok := absRel(root, rel)
		if !ok {
			c.conflict(mpath, fmt.Sprintf("manifest lists %q, which is not a path under the root", rel))
			continue
		}
		cur := c.get(abs)
		switch {
		case len(owners(man, role, rel)) > 0:
			c.act(abs, KindUnchanged) // another role still owns it
		case !cur.exists:
		case hashHex(cur.data) != rec.Files[rel]:
			c.conflict(abs, "modified since rig-crew wrote it")
		default:
			c.act(abs, KindRemove)
			out.ops = append(out.ops, op{path: abs, remove: true})
			out.prune = append(out.prune, abs)
		}
	}
	for _, mr := range rec.Merges {
		abs, ok := absRel(root, mr.Path)
		if !ok {
			c.conflict(mpath, fmt.Sprintf("manifest lists %q, which is not a path under the root", mr.Path))
			continue
		}
		dropMerge(c, man, role, mr, abs)
	}
	delete(man.Roles, role)
	if gd := globalDirOf(recLevel(rec.Activation), home); v2 && gd != "" {
		syncProject(c, man, gd, filepath.Dir(sd))
	}
	if rec.Workflow != "" {
		if err := underHome(wfHome, rec.Workflow); err != nil {
			c.conflict(rec.Workflow, "recorded workflow reference is not valid: "+err.Error())
		} else {
			w := &wfEditor{collector: c, root: root, role: role, mans: map[string]*wfManifest{}, dirty: map[string]bool{}}
			w.drop(rec.Workflow)
			w.flush()
		}
	}
	if len(c.conflicts) > 0 {
		return nil, conflictsErr(c.conflicts)
	}
	if len(man.Roles) == 0 && len(man.projects) == 0 {
		out.ops = append(out.ops, op{path: mpath, remove: true})
		out.cleanup = append(out.cleanup, filepath.Dir(mpath))
	} else {
		out.ops = append(out.ops, op{path: mpath, data: encodeManifest(man)})
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
