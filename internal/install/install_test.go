package install

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AindriuB/rig-crew/internal/plan"
)

type env struct {
	t          *testing.T
	home, root string
}

func newEnv(t *testing.T) env {
	home := t.TempDir()
	return env{t, home, filepath.Join(home, ".claude")}
}

func (e env) p(rel string) string { return filepath.Join(e.root, filepath.FromSlash(rel)) }

func (e env) plan(role string, files map[string]string) plan.Plan {
	p := plan.Plan{Harness: "claude", Role: role, Root: e.root,
		Scope: plan.Scope{Global: true, Base: e.home, Home: e.home}}
	for rel, d := range files {
		p.Files = append(p.Files, plan.File{Path: e.p(rel), Data: []byte(d)})
	}
	return p
}

func (e env) apply(p plan.Plan) Result {
	e.t.Helper()
	r, err := Apply(context.Background(), p, Options{})
	if err != nil {
		e.t.Fatalf("Apply: %v", err)
	}
	return r
}

func (e env) write(path, data string) {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e env) read(path string) string {
	e.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			out[p] = "<dir>"
			return nil
		}
		b, _ := os.ReadFile(p)
		out[p] = string(b)
		return nil
	})
	return out
}

func kinds(r Result) map[string]string {
	m := map[string]string{}
	for _, a := range r.Actions {
		m[a.Path] = a.Kind
	}
	return m
}

func conflictPaths(t *testing.T, err error) []string {
	t.Helper()
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("want ConflictError, got %v", err)
	}
	var out []string
	for _, c := range ce.Conflicts {
		out = append(out, c.Path)
	}
	return out
}

func settings(e env, appendVals []string, set map[string]string) plan.JSONMerge {
	m := plan.JSONMerge{Path: e.p("settings.json")}
	for _, v := range appendVals {
		m.Append = append(m.Append, plan.Entry{Key: []string{"permissions", "allow"}, Value: v})
	}
	for k, v := range set {
		m.Set = append(m.Set, plan.Entry{Key: []string{"env", k}, Value: v})
	}
	return m
}

func TestApply_CreatesFilesAndManifest(t *testing.T) {
	e := newEnv(t)
	r := e.apply(e.plan("dev", map[string]string{"agents/a.md": "A", "rules/r.md": "R"}))
	if !r.Changed || kinds(r)[e.p("agents/a.md")] != KindCreate {
		t.Fatalf("result %+v", r)
	}
	if e.read(e.p("agents/a.md")) != "A" {
		t.Fatal("content")
	}
	var m manifest
	if err := json.Unmarshal([]byte(e.read(manifestPath(e.root))), &m); err != nil {
		t.Fatal(err)
	}
	if m.Roles["dev"].Files["agents/a.md"] != hashHex([]byte("A")) {
		t.Fatalf("manifest %+v", m)
	}
	if exists(lockPath(e.root)) {
		t.Fatal("lock left behind")
	}
}

func TestApply_SecondRunChangesNothing(t *testing.T) {
	e := newEnv(t)
	p := e.plan("dev", map[string]string{"agents/a.md": "A"})
	p.Merges = []plan.JSONMerge{settings(e, []string{"Bash(rig *)"}, map[string]string{"RIG_ACTOR": "dev"})}
	e.apply(p)
	before := tree(t, e.home)
	r := e.apply(p)
	if r.Changed {
		t.Fatalf("changed: %+v", r)
	}
	for _, a := range r.Actions {
		if a.Kind != KindUnchanged {
			t.Fatalf("action %+v", a)
		}
	}
	if !reflect.DeepEqual(before, tree(t, e.home)) {
		t.Fatal("tree changed")
	}
}

func TestApply_AdoptsIdenticalUnownedFile(t *testing.T) {
	e := newEnv(t)
	e.write(e.p("agents/a.md"), "A")
	r := e.apply(e.plan("dev", map[string]string{"agents/a.md": "A"}))
	if kinds(r)[e.p("agents/a.md")] != KindAdopt {
		t.Fatalf("%+v", r)
	}
	if _, err := Uninstall(context.Background(), e.root, "dev", Options{}); err != nil {
		t.Fatal(err)
	}
	if exists(e.p("agents/a.md")) {
		t.Fatal("adopted file not removed on uninstall")
	}
}

func TestApply_DifferentUnownedFileStops(t *testing.T) {
	e := newEnv(t)
	e.write(e.p("agents/a.md"), "mine")
	before := tree(t, e.home)
	_, err := Apply(context.Background(), e.plan("dev", map[string]string{"agents/a.md": "A", "agents/b.md": "B"}), Options{})
	if got := conflictPaths(t, err); !reflect.DeepEqual(got, []string{e.p("agents/a.md")}) {
		t.Fatalf("%v", got)
	}
	if !reflect.DeepEqual(before, tree(t, e.home)) {
		t.Fatal("wrote despite conflict")
	}
}

func TestApply_ModifiedOwnedFileStops(t *testing.T) {
	e := newEnv(t)
	e.apply(e.plan("dev", map[string]string{"a.md": "A", "b.md": "B", "c.md": "C"}))
	e.write(e.p("a.md"), "edited")
	e.write(e.p("b.md"), "edited")
	before := tree(t, e.home)
	_, err := Apply(context.Background(), e.plan("dev", map[string]string{"a.md": "A2", "b.md": "B2", "c.md": "C2"}), Options{})
	if got := conflictPaths(t, err); !reflect.DeepEqual(got, []string{e.p("a.md"), e.p("b.md")}) {
		t.Fatalf("%v", got)
	}
	if !reflect.DeepEqual(before, tree(t, e.home)) {
		t.Fatal("wrote despite conflict")
	}
}

func TestApply_UpdatesUnmodifiedOwnedFile(t *testing.T) {
	e := newEnv(t)
	e.apply(e.plan("dev", map[string]string{"a.md": "A"}))
	r := e.apply(e.plan("dev", map[string]string{"a.md": "A2"}))
	if kinds(r)[e.p("a.md")] != KindUpdate || e.read(e.p("a.md")) != "A2" {
		t.Fatalf("%+v", r)
	}
}

func TestApply_RemovesOwnedFileDroppedFromPlan(t *testing.T) {
	e := newEnv(t)
	e.apply(e.plan("dev", map[string]string{"x/a.md": "A", "b.md": "B"}))
	r := e.apply(e.plan("dev", map[string]string{"b.md": "B"}))
	if kinds(r)[e.p("x/a.md")] != KindRemove || exists(e.p("x/a.md")) {
		t.Fatalf("%+v", r)
	}
}

func TestApply_MergesSettingsKeepingUserEntries(t *testing.T) {
	e := newEnv(t)
	e.write(e.p("settings.json"), `{"model":"x","n":1.50,"permissions":{"allow":["Bash(ls)","Bash(rig *)"]}}`)
	p := e.plan("dev", nil)
	p.Merges = []plan.JSONMerge{settings(e, []string{"Bash(rig *)", "Bash(rig task *)"}, map[string]string{"RIG_ACTOR": "dev"})}
	r := e.apply(p)
	if kinds(r)[e.p("settings.json")] != KindMerge {
		t.Fatalf("%+v", r)
	}
	got := e.read(e.p("settings.json"))
	for _, want := range []string{`"Bash(ls)"`, `"Bash(rig task *)"`, `"RIG_ACTOR": "dev"`, `"model": "x"`, "1.50"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	var m manifest
	_ = json.Unmarshal([]byte(e.read(manifestPath(e.root))), &m)
	for _, en := range m.Roles["dev"].Merges[0].Entries {
		if en.Value == "Bash(rig *)" {
			t.Fatal("user's pre-existing entry was recorded")
		}
	}
	if _, err := Uninstall(context.Background(), e.root, "dev", Options{}); err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal([]byte(e.read(e.p("settings.json"))), &s); err != nil {
		t.Fatal(err)
	}
	allow := s["permissions"].(map[string]any)["allow"].([]any)
	if len(allow) != 2 || s["env"] != nil {
		t.Fatalf("after uninstall: %v", s)
	}
}

func TestApply_EnvConflictStops(t *testing.T) {
	e := newEnv(t)
	e.write(e.p("settings.json"), `{"env":{"RIG_ACTOR":"someone"}}`)
	p := e.plan("dev", nil)
	p.Merges = []plan.JSONMerge{settings(e, nil, map[string]string{"RIG_ACTOR": "dev"})}
	_, err := Apply(context.Background(), p, Options{})
	conflictPaths(t, err)
	if strings.Contains(e.read(e.p("settings.json")), `"dev"`) {
		t.Fatal("overwrote")
	}
}

func TestApply_NonObjectSettingsStops(t *testing.T) {
	e := newEnv(t)
	e.write(e.p("settings.json"), `["x"]`)
	p := e.plan("dev", nil)
	p.Merges = []plan.JSONMerge{settings(e, []string{"a"}, nil)}
	_, err := Apply(context.Background(), p, Options{})
	conflictPaths(t, err)
}

func TestApply_HeldLockStops(t *testing.T) {
	e := newEnv(t)
	e.write(lockPath(e.root), "")
	_, err := Apply(context.Background(), e.plan("dev", map[string]string{"a.md": "A"}), Options{})
	var le *LockedError
	if !errors.As(err, &le) {
		t.Fatalf("got %v", err)
	}
	if !exists(lockPath(e.root)) || exists(e.p("a.md")) {
		t.Fatal("lock broken or file written")
	}
}

func TestApply_SymlinkedPathStops(t *testing.T) {
	e := newEnv(t)
	other := t.TempDir()
	if err := os.MkdirAll(e.root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, e.p("agents")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	_, err := Apply(context.Background(), e.plan("dev", map[string]string{"agents/a.md": "A"}), Options{})
	var se *SymlinkError
	if !errors.As(err, &se) {
		t.Fatalf("got %v", err)
	}
	if exists(filepath.Join(other, "a.md")) {
		t.Fatal("wrote through symlink")
	}
}

func TestApply_TargetChangedUnderLockStops(t *testing.T) {
	e := newEnv(t)
	opts := Options{beforeRecheck: func() { e.write(e.p("a.md"), "sneaky") }}
	_, err := Apply(context.Background(), e.plan("dev", map[string]string{"a.md": "A", "b.md": "B"}), opts)
	if got := conflictPaths(t, err); !reflect.DeepEqual(got, []string{e.p("a.md")}) {
		t.Fatalf("%v", got)
	}
	if exists(e.p("b.md")) || exists(manifestPath(e.root)) || exists(lockPath(e.root)) {
		t.Fatal("left files behind")
	}
	if e.read(e.p("a.md")) != "sneaky" {
		t.Fatal("clobbered")
	}
}

func TestApply_FailedWriteRollsBack(t *testing.T) {
	e := newEnv(t)
	e.apply(e.plan("dev", map[string]string{"a.md": "A"}))
	before := tree(t, e.home)
	boom := errors.New("disk full")
	opts := Options{beforeWrite: func(p string) error {
		if p == manifestPath(e.root) {
			return boom
		}
		return nil
	}}
	_, err := Apply(context.Background(), e.plan("dev", map[string]string{"a.md": "A2", "new/b.md": "B"}), opts)
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if !reflect.DeepEqual(before, tree(t, e.home)) {
		t.Fatalf("not restored:\n%v\n%v", before, tree(t, e.home))
	}
}

func TestApply_DryRunWritesNothing(t *testing.T) {
	e := newEnv(t)
	p := e.plan("dev", map[string]string{"a.md": "A"})
	p.Merges = []plan.JSONMerge{settings(e, []string{"x"}, nil)}
	p.Workflow = &plan.WorkflowFile{Name: "solo", Path: filepath.Join(e.home, ".rig", "workflows", "solo.json"), Data: []byte("{}")}
	r, err := Apply(context.Background(), p, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	k := kinds(r)
	if len(k) != 3 || !r.Changed {
		t.Fatalf("%+v", r)
	}
	if exists(e.root) || exists(filepath.Join(e.home, ".rig")) {
		t.Fatal("dry run wrote")
	}
}

func TestApply_PathOutsideRootRefused(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{filepath.Join(e.home, "evil.md"), "rel/a.md", filepath.Join(e.root, "..", "x.md")} {
		p := e.plan("dev", nil)
		p.Files = []plan.File{{Path: path, Data: []byte("x")}}
		_, err := Apply(context.Background(), p, Options{})
		if !errors.Is(err, ErrOutsideRoot) {
			t.Fatalf("%s: %v", path, err)
		}
	}
	if exists(e.root) {
		t.Fatal("touched disk")
	}
}

func wfPlan(e env, role, data string) plan.Plan {
	p := e.plan(role, map[string]string{"a-" + role + ".md": "A"})
	p.Workflow = &plan.WorkflowFile{Name: "solo", Path: filepath.Join(e.home, ".rig", "workflows", "solo.json"), Data: []byte(data)}
	return p
}

func TestApply_WorkflowSharedAcrossScopes(t *testing.T) {
	e := newEnv(t)
	proj := env{t, e.home, filepath.Join(t.TempDir(), ".claude")}
	r1 := e.apply(wfPlan(e, "dev", "{}"))
	r2 := proj.apply(wfPlan(proj, "dev", "{}"))
	wf := filepath.Join(e.home, ".rig", "workflows", "solo.json")
	if kinds(r1)[wf] != KindCreate || kinds(r2)[wf] != KindUnchanged {
		t.Fatalf("%+v %+v", r1, r2)
	}
	if _, err := Uninstall(context.Background(), e.root, "dev", Options{}); err != nil {
		t.Fatal(err)
	}
	if !exists(wf) {
		t.Fatal("workflow removed while still referenced")
	}
}

func TestApply_ModifiedWorkflowStops(t *testing.T) {
	e := newEnv(t)
	e.apply(wfPlan(e, "dev", "{}"))
	wf := filepath.Join(e.home, ".rig", "workflows", "solo.json")
	e.write(wf, "edited")
	_, err := Apply(context.Background(), wfPlan(e, "dev", `{"v":2}`), Options{})
	if got := conflictPaths(t, err); !reflect.DeepEqual(got, []string{wf}) {
		t.Fatalf("%v", got)
	}
}

func TestUninstall_LastScopeRemovesWorkflow(t *testing.T) {
	e := newEnv(t)
	proj := env{t, e.home, filepath.Join(t.TempDir(), ".claude")}
	e.apply(wfPlan(e, "dev", "{}"))
	proj.apply(wfPlan(proj, "dev", "{}"))
	for _, x := range []env{e, proj} {
		if _, err := Uninstall(context.Background(), x.root, "dev", Options{}); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(e.home, ".rig", "workflows")
	if exists(filepath.Join(dir, "solo.json")) || exists(filepath.Join(dir, stateDir)) {
		t.Fatalf("left: %v", tree(t, dir))
	}
}

func TestUninstall_RestoresPriorState(t *testing.T) {
	e := newEnv(t)
	e.write(e.p("keep.md"), "mine")
	before := tree(t, e.home)
	p := e.plan("dev", map[string]string{"agents/deep/a.md": "A"})
	p.Merges = []plan.JSONMerge{settings(e, []string{"x"}, map[string]string{"K": "v"})}
	e.apply(p)
	if _, err := Uninstall(context.Background(), e.root, "dev", Options{}); err != nil {
		t.Fatal(err)
	}
	if after := tree(t, e.home); !reflect.DeepEqual(before, after) {
		t.Fatalf("before %v\nafter %v", before, after)
	}
	// A settings file the user created first survives, minus our entries.
	e2 := newEnv(t)
	e2.write(e2.p("settings.json"), `{"permissions":{"allow":["u"]}}`)
	p2 := e2.plan("dev", nil)
	p2.Merges = []plan.JSONMerge{settings(e2, []string{"x"}, map[string]string{"K": "v"})}
	e2.apply(p2)
	if _, err := Uninstall(context.Background(), e2.root, "dev", Options{}); err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	_ = json.Unmarshal([]byte(e2.read(e2.p("settings.json"))), &s)
	if !reflect.DeepEqual(s, map[string]any{"permissions": map[string]any{"allow": []any{"u"}}}) {
		t.Fatalf("%v", s)
	}
}

func TestUninstall_ModifiedOwnedFileStops(t *testing.T) {
	e := newEnv(t)
	e.apply(e.plan("dev", map[string]string{"a.md": "A", "b.md": "B"}))
	e.write(e.p("a.md"), "edited")
	_, err := Uninstall(context.Background(), e.root, "dev", Options{})
	if got := conflictPaths(t, err); !reflect.DeepEqual(got, []string{e.p("a.md")}) {
		t.Fatalf("%v", got)
	}
	if !exists(e.p("b.md")) || !exists(manifestPath(e.root)) {
		t.Fatal("partial uninstall")
	}
}

func TestUninstall_SharedFileKeptForOtherRole(t *testing.T) {
	e := newEnv(t)
	e.apply(e.plan("dev", map[string]string{"shared.md": "S", "d.md": "D"}))
	e.apply(e.plan("tester", map[string]string{"shared.md": "S"}))
	if _, err := Uninstall(context.Background(), e.root, "dev", Options{}); err != nil {
		t.Fatal(err)
	}
	if !exists(e.p("shared.md")) || exists(e.p("d.md")) {
		t.Fatal("shared file handling")
	}
	if _, err := Uninstall(context.Background(), e.root, "tester", Options{}); err != nil {
		t.Fatal(err)
	}
	if exists(e.p("shared.md")) || exists(filepath.Join(e.root, stateDir)) {
		t.Fatalf("left: %v", tree(t, e.root))
	}
}

func TestUninstall_NotInstalledRefuses(t *testing.T) {
	e := newEnv(t)
	if _, err := Uninstall(context.Background(), e.root, "dev", Options{}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("%v", err)
	}
	e.apply(e.plan("tester", map[string]string{"a.md": "A"}))
	if _, err := Uninstall(context.Background(), e.root, "dev", Options{}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("%v", err)
	}
}
