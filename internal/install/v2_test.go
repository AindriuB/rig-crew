package install

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AindriuB/rig-crew/internal/plan"
)

// v2 plans: global state in <home>/.rig-crew, project state in <proj>/.rig-crew.

func (e env) v2(owner string, files map[string]string) plan.Plan {
	p := e.plan("", files)
	p.Owner = owner
	p.StateDir = filepath.Join(e.home, ".rig-crew")
	return p
}

func (e env) proj() string { return filepath.Join(e.home, "proj") }

func (e env) v2Project(owner string, files map[string]string, level string) plan.Plan {
	proj := e.proj()
	p := plan.Plan{Harness: "claude", Owner: owner, Root: filepath.Join(proj, ".claude"),
		StateDir: filepath.Join(proj, ".rig-crew"),
		Scope:    plan.Scope{Base: proj, Home: e.home}}
	for rel, d := range files {
		p.Files = append(p.Files, plan.File{Path: filepath.Join(p.Root, rel), Data: []byte(d)})
	}
	if level != "" {
		p.Activation = &plan.Activation{Harness: "claude", Level: level, Packs: []string{owner}}
	}
	return p
}

func (e env) gman() string { return filepath.Join(e.home, ".rig-crew", "manifest.json") }

func readV2(t *testing.T, e env, path string) manifestV2 {
	t.Helper()
	var m manifestV2
	if err := json.Unmarshal([]byte(e.read(path)), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func (e env) raw(key string, raw string) plan.JSONMerge {
	return plan.JSONMerge{Path: e.p("settings.json"), Set: []plan.Entry{{Key: strings.Split(key, "."), Raw: []byte(raw)}}}
}

func (e env) settings() map[string]any {
	var v map[string]any
	if err := json.Unmarshal([]byte(e.read(e.p("settings.json"))), &v); err != nil {
		e.t.Fatal(err)
	}
	return v
}

func TestApplyV2_RecordsOwnerUnderStateDir(t *testing.T) {
	e := newEnv(t)
	p := e.v2("pack-a", map[string]string{"a.md": "A"})
	p.Activation = &plan.Activation{Harness: "claude", Level: "global", Packs: []string{"pack-a"}}
	e.applyTwice(p)
	m := readV2(t, e, e.gman())
	rec := m.Owners["pack-a"]
	if m.Version != 2 || rec == nil || rec.Files["a.md"] != hashHex([]byte("A")) || rec.Root != e.root {
		t.Fatalf("manifest %+v", m)
	}
	if rec.Activation == nil || rec.Activation.Level != "global" {
		t.Fatalf("activation %+v", rec.Activation)
	}
	if exists(filepath.Join(e.home, ".rig-crew", "lock")) || exists(manifestPath(e.root)) {
		t.Fatal("lock left behind, or a v1 manifest written")
	}
}

func TestApplyV2_DryRunWritesNothing(t *testing.T) {
	e := newEnv(t)
	before := e.treeAll()
	p := e.v2Project("pack-a", map[string]string{"a.md": "A"}, "project")
	r, err := Apply(context.Background(), p, Options{DryRun: true})
	if err != nil || !r.Changed {
		t.Fatalf("%v %+v", err, r)
	}
	if !reflect.DeepEqual(before, e.treeAll()) {
		t.Fatal("dry run wrote")
	}
}

func TestApplyV2_BadFieldsRefused(t *testing.T) {
	e := newEnv(t)
	bad := []plan.Plan{e.v2("", nil), e.v2("user", nil), e.v2Project("x", nil, "weekly")}
	rel := e.v2("x", nil)
	rel.StateDir = "rel"
	bad = append(bad, rel)
	for i, p := range bad {
		if _, err := Apply(context.Background(), p, Options{}); err == nil {
			t.Fatalf("plan %d accepted", i)
		}
	}
	if exists(filepath.Join(e.home, ".rig-crew")) {
		t.Fatal("state written")
	}
}

func TestApplyV2_OwnersSharingARootStayApart(t *testing.T) {
	e := newEnv(t)
	e.applyTwice(e.v2("pack-a", map[string]string{"a.md": "A"}))
	e.applyTwice(e.v2("pack-b", map[string]string{"b.md": "B"}))
	m := readV2(t, e, e.gman())
	if len(m.Owners) != 2 || m.Owners["pack-a"].Files["b.md"] != "" {
		t.Fatalf("%+v", m.Owners)
	}
}

func TestRawEntry_WritesObjectBoolAndString(t *testing.T) {
	e := newEnv(t)
	p := e.v2("pack-a", nil)
	p.Merges = []plan.JSONMerge{{Path: e.p("settings.json"), Set: []plan.Entry{
		{Key: []string{"enabledPlugins", "x@y"}, Raw: []byte(`true`)},
		{Key: []string{"extraKnownMarketplaces", "m"}, Raw: []byte(`{"source":{"source":"github","repo":"o/r"}}`)},
		{Key: []string{"model"}, Raw: []byte(`"opus"`)},
	}}}
	e.applyTwice(p)
	s := e.settings()
	if s["enabledPlugins"].(map[string]any)["x@y"] != true || s["model"] != "opus" {
		t.Fatalf("%v", s)
	}
	src := s["extraKnownMarketplaces"].(map[string]any)["m"].(map[string]any)["source"].(map[string]any)
	if src["repo"] != "o/r" {
		t.Fatalf("%v", s)
	}
}

func TestRawEntry_BadJSONRefused(t *testing.T) {
	e := newEnv(t)
	for _, raw := range []string{`{`, `12`, `{"a":1,"a":2}`, `true false`} {
		p := e.v2("pack-a", nil)
		p.Merges = []plan.JSONMerge{e.raw("k", raw)}
		var ce *ConflictError
		if _, err := Apply(context.Background(), p, Options{}); !errors.As(err, &ce) {
			t.Fatalf("%s: %v", raw, err)
		}
	}
}

func TestRawEntry_EqualValueAdoptedByJSONValue(t *testing.T) {
	e := newEnv(t)
	e.write(e.p("settings.json"), "{\n \"m\": {\"b\": 1,   \"a\": [true]}\n}\n")
	p := e.v2("pack-a", nil)
	p.Merges = []plan.JSONMerge{e.raw("m", `{"a":[true],"b":1}`)}
	before := e.read(e.p("settings.json"))
	e.applyTwice(p)
	if e.read(e.p("settings.json")) != before {
		t.Fatal("an equal value was rewritten")
	}
	// Adopted, not written: removal leaves the user's value.
	if _, err := Remove(context.Background(), e.home, p.StateDir, "pack-a", Options{}); err != nil {
		t.Fatal(err)
	}
	if e.read(e.p("settings.json")) != before {
		t.Fatal("removal touched a value rig-crew did not write")
	}
}

func TestRawEntry_DifferentUserValueConflicts(t *testing.T) {
	e := newEnv(t)
	e.write(e.p("settings.json"), `{"enabledPlugins":{"x@y":false}}`)
	p := e.v2("pack-a", nil)
	p.Merges = []plan.JSONMerge{e.raw("enabledPlugins.x@y", `true`)}
	var ce *ConflictError
	if _, err := Apply(context.Background(), p, Options{}); !errors.As(err, &ce) {
		t.Fatalf("%v", err)
	}
	if exists(e.gman()) || e.read(e.p("settings.json")) != `{"enabledPlugins":{"x@y":false}}` {
		t.Fatal("wrote despite conflict")
	}
}

func TestRawEntry_ReapplyUpdatesOwnValue(t *testing.T) {
	e := newEnv(t)
	p := e.v2("pack-a", nil)
	p.Merges = []plan.JSONMerge{e.raw("m", `{"v":1}`)}
	e.applyTwice(p)
	p.Merges = []plan.JSONMerge{e.raw("m", `{"v":2}`)}
	e.applyTwice(p)
	if e.settings()["m"].(map[string]any)["v"].(float64) != 2 {
		t.Fatalf("%v", e.settings())
	}
	rec := readV2(t, e, e.gman()).Owners["pack-a"]
	if len(rec.Merges) != 1 || len(rec.Merges[0].Entries) != 1 {
		t.Fatalf("stale entries: %+v", rec.Merges)
	}
}

func TestRawEntry_RemoveOnlyWhileValueIsWhatWasWritten(t *testing.T) {
	e := newEnv(t)
	p := e.v2("pack-a", nil)
	p.Merges = []plan.JSONMerge{e.raw("a", `"one"`)}
	p.Merges[0].Set = append(p.Merges[0].Set, plan.Entry{Key: []string{"b"}, Raw: []byte(`"two"`)})
	e.applyTwice(p)
	e.write(e.p("settings.json"), `{"a":"one","b":"edited","keep":1}`)
	if _, err := Remove(context.Background(), e.home, p.StateDir, "pack-a", Options{}); err != nil {
		t.Fatal(err)
	}
	s := e.settings()
	if _, ok := s["a"]; ok || s["b"] != "edited" || s["keep"] == nil {
		t.Fatalf("%v", s)
	}
}

func TestRawEntry_NestedKeyTouchesOnlyOwnSubKey(t *testing.T) {
	e := newEnv(t)
	e.write(e.p("settings.json"), `{"extraKnownMarketplaces":{"theirs":{"source":{"repo":"u/r"}}}}`)
	p := e.v2("pack-a", nil)
	p.Merges = []plan.JSONMerge{e.raw("extraKnownMarketplaces.mine", `{"source":{"repo":"o/r"}}`)}
	e.applyTwice(p)
	km := e.settings()["extraKnownMarketplaces"].(map[string]any)
	if len(km) != 2 || km["theirs"] == nil || km["mine"] == nil {
		t.Fatalf("%v", km)
	}
	if _, err := Remove(context.Background(), e.home, p.StateDir, "pack-a", Options{}); err != nil {
		t.Fatal(err)
	}
	km = e.settings()["extraKnownMarketplaces"].(map[string]any)
	if len(km) != 1 || km["theirs"] == nil {
		t.Fatalf("%v", km)
	}
}

func TestProjectActivation_ListedInGlobalManifestAndDropped(t *testing.T) {
	e := newEnv(t)
	p := e.v2Project("pack-a", map[string]string{"a.md": "A"}, "project")
	e.applyTwice(p)
	if g := readV2(t, e, e.gman()); !reflect.DeepEqual(g.Projects, []string{e.proj()}) {
		t.Fatalf("projects %v", g.Projects)
	}
	if exists(filepath.Join(e.home, ".rig-crew", "lock")) || exists(filepath.Join(e.proj(), ".rig-crew", "lock")) {
		t.Fatal("lock left")
	}
	// A second owner in the same project keeps the listing when the first goes.
	e.applyTwice(e.v2Project("pack-b", map[string]string{"b.md": "B"}, "project"))
	if _, err := Remove(context.Background(), e.home, p.StateDir, "pack-a", Options{}); err != nil {
		t.Fatal(err)
	}
	if g := readV2(t, e, e.gman()); len(g.Projects) != 1 {
		t.Fatalf("projects %v", g.Projects)
	}
	if _, err := Remove(context.Background(), e.home, p.StateDir, "pack-b", Options{}); err != nil {
		t.Fatal(err)
	}
	if exists(e.gman()) || exists(filepath.Join(e.home, ".rig-crew")) || exists(filepath.Join(e.proj(), ".rig-crew")) {
		t.Fatal("state left after the last removal")
	}
}

func TestProjectActivation_DroppedWhenReappliedWithoutIt(t *testing.T) {
	e := newEnv(t)
	e.applyTwice(e.v2Project("pack-a", map[string]string{"a.md": "A"}, "project"))
	e.applyTwice(e.v2Project("pack-a", map[string]string{"a.md": "A"}, ""))
	if exists(e.gman()) {
		t.Fatal("project still listed")
	}
}

func TestProjectActivation_SessionIsNotListed(t *testing.T) {
	e := newEnv(t)
	e.applyTwice(e.v2Project("pack-a", map[string]string{"a.md": "A"}, "session"))
	if exists(e.gman()) {
		t.Fatal("session activation listed")
	}
}

func TestProjectActivation_HeldGlobalLockStops(t *testing.T) {
	e := newEnv(t)
	// Both locks are held: the error names the one taken first, which must be the global lock.
	gl := filepath.Join(e.home, ".rig-crew", "lock")
	e.write(gl, "")
	e.write(filepath.Join(e.proj(), ".rig-crew", "lock"), "")
	var le *LockedError
	_, err := Apply(context.Background(), e.v2Project("pack-a", map[string]string{"a.md": "A"}, "project"), Options{})
	if !errors.As(err, &le) || le.Path != gl {
		t.Fatalf("%v", err)
	}
	if exists(filepath.Join(e.proj(), ".claude", "a.md")) {
		t.Fatal("wrote after a refused lock")
	}
}

func TestReadState_ListsPacksActivationsAndStaleProjects(t *testing.T) {
	e := newEnv(t)
	g := e.v2("pack-g", map[string]string{"g.md": "G"})
	g.Activation = &plan.Activation{Harness: "claude", Level: "global", Profiles: []string{"dev"}}
	e.applyTwice(g)
	e.applyTwice(e.v2Project("pack-p", map[string]string{"p.md": "P"}, "project"))
	// A project listed, then deleted.
	gone := filepath.Join(e.home, "gone")
	m := readV2(t, e, e.gman())
	m.Projects = append(m.Projects, gone)
	e.write(e.gman(), string(encodeJSON(m)))

	st, err := ReadState(e.home, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Packs) != 2 || st.Packs[0].Owner != "pack-g" || st.Packs[0].Scope != "global" ||
		st.Packs[1].Owner != "pack-p" || st.Packs[1].Scope != "project" || st.Packs[1].Project != e.proj() {
		t.Fatalf("packs %+v", st.Packs)
	}
	if len(st.Activations) != 2 || st.Activations[0].Level != "global" || st.Activations[0].Profiles[0] != "dev" ||
		st.Activations[1].Level != "project" {
		t.Fatalf("activations %+v", st.Activations)
	}
	if !reflect.DeepEqual(st.Stale, []string{gone}) {
		t.Fatalf("stale %v", st.Stale)
	}
	// An unlisted project named by the caller is read, and is not stale when empty.
	other := filepath.Join(e.home, "other")
	if st, err = ReadState(e.home, other); err != nil || len(st.Stale) != 1 {
		t.Fatalf("%v %+v", err, st)
	}
}

func TestReadState_NothingInstalledIsEmpty(t *testing.T) {
	st, err := ReadState(t.TempDir(), "")
	if err != nil || len(st.Packs) != 0 || len(st.Activations) != 0 || len(st.Stale) != 0 {
		t.Fatalf("%v %+v", err, st)
	}
}

func TestRemove_DeletesExactlyOneOwner(t *testing.T) {
	e := newEnv(t)
	a := e.v2("pack-a", map[string]string{"a.md": "A"})
	e.applyTwice(a)
	e.applyTwice(e.v2("pack-b", map[string]string{"b.md": "B"}))
	r, err := Remove(context.Background(), e.home, a.StateDir, "pack-a", Options{})
	if err != nil || !r.Changed {
		t.Fatalf("%v %+v", err, r)
	}
	if exists(e.p("a.md")) || !exists(e.p("b.md")) {
		t.Fatal("wrong files removed")
	}
	if m := readV2(t, e, e.gman()); len(m.Owners) != 1 || m.Owners["pack-b"] == nil {
		t.Fatalf("%+v", m.Owners)
	}
	if _, err := Remove(context.Background(), e.home, a.StateDir, "pack-a", Options{}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("second run: %v", err)
	}
}

func TestRemove_DryRunWritesNothing(t *testing.T) {
	e := newEnv(t)
	p := e.v2Project("pack-a", map[string]string{"a.md": "A"}, "project")
	e.applyTwice(p)
	before := e.treeAll()
	if _, err := Remove(context.Background(), e.home, p.StateDir, "pack-a", Options{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, e.treeAll()) {
		t.Fatal("dry run wrote")
	}
}

func TestRemove_ModifiedFileStops(t *testing.T) {
	e := newEnv(t)
	p := e.v2("pack-a", map[string]string{"a.md": "A"})
	e.applyTwice(p)
	e.write(e.p("a.md"), "edited")
	var ce *ConflictError
	if _, err := Remove(context.Background(), e.home, p.StateDir, "pack-a", Options{}); !errors.As(err, &ce) {
		t.Fatalf("%v", err)
	}
	if !exists(e.gman()) {
		t.Fatal("manifest dropped on conflict")
	}
}

func TestCreateFile_WritesOnlyWhenAbsent(t *testing.T) {
	e := newEnv(t)
	sd := filepath.Join(e.home, ".rig-crew")
	path := filepath.Join(e.home, "profiles", "dev.yaml")
	do := func(data string, o Options) (Result, error) {
		return CreateFile(context.Background(), sd, path, []byte(data), o)
	}
	if r, err := do("one", Options{}); err != nil || !r.Changed || e.read(path) != "one" {
		t.Fatalf("%v %+v", err, r)
	}
	m := readV2(t, e, e.gman())
	if !reflect.DeepEqual(m.Owners[userOwner].UserFiles, []string{path}) || len(m.Owners[userOwner].Files) != 0 {
		t.Fatalf("%+v", m.Owners[userOwner])
	}
	// Second run: recorded, so no change and never rewritten, even with other data.
	e.write(path, "user edit")
	before := e.treeAll()
	if r, err := do("two", Options{}); err != nil || r.Changed || e.read(path) != "user edit" {
		t.Fatalf("%v %+v", err, r)
	}
	if !reflect.DeepEqual(before, e.treeAll()) {
		t.Fatal("second run changed the tree")
	}
	// The user record is not removable, so the file stays (AC15).
	if _, err := Remove(context.Background(), e.home, sd, userOwner, Options{}); !errors.Is(err, ErrBadOwner) || e.read(path) != "user edit" {
		t.Fatalf("%v", err)
	}
}

func TestCreateFile_ExistingUnrecordedPathConflicts(t *testing.T) {
	e := newEnv(t)
	sd := filepath.Join(e.home, ".rig-crew")
	path := filepath.Join(e.home, "mine.yaml")
	e.write(path, "mine")
	var ce *ConflictError
	if _, err := CreateFile(context.Background(), sd, path, []byte("x"), Options{}); !errors.As(err, &ce) {
		t.Fatalf("%v", err)
	}
	if e.read(path) != "mine" || exists(e.gman()) {
		t.Fatal("wrote despite conflict")
	}
}

func TestCreateFile_DryRunWritesNothing(t *testing.T) {
	e := newEnv(t)
	before := e.treeAll()
	r, err := CreateFile(context.Background(), filepath.Join(e.home, ".rig-crew"), filepath.Join(e.home, "n.yaml"), []byte("x"), Options{DryRun: true})
	if err != nil || !r.Changed || !reflect.DeepEqual(before, e.treeAll()) {
		t.Fatalf("%v %+v", err, r)
	}
}

func TestReadV1_ListsRolesWithWorkflowName(t *testing.T) {
	e := newEnv(t)
	wf := filepath.Join(e.home, ".rig", "workflows", "solo.json")
	p := e.plan("dev", map[string]string{"a.md": "A"})
	p.Workflow = &plan.WorkflowFile{Name: "solo", Path: wf, Data: []byte("{}")}
	e.applyTwice(p)
	e.applyTwice(e.plan("qa", map[string]string{"q.md": "Q"}))
	got, err := ReadV1(e.root)
	want := []V1Role{{Role: "dev", Workflow: "solo"}, {Role: "qa"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestReadV1_NoManifestGivesEmptyList(t *testing.T) {
	got, err := ReadV1(t.TempDir())
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("%v %#v", err, got)
	}
}

func TestCreateFile_ProfilesUnderStateDirAllowed(t *testing.T) {
	e := newEnv(t)
	sd := filepath.Join(e.home, ".rig-crew")
	path := filepath.Join(sd, "profiles", "dev.json")
	for i := 0; i < 2; i++ {
		r, err := CreateFile(context.Background(), sd, path, []byte("{}"), Options{})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if i == 1 && r.Changed {
			t.Fatalf("second run changed: %+v", r)
		}
	}
	for _, bad := range []string{sd, filepath.Join(sd, "manifest.json"), filepath.Join(sd, "lock")} {
		if _, err := CreateFile(context.Background(), sd, bad, []byte("x"), Options{}); !errors.Is(err, ErrOutsideRoot) {
			t.Fatalf("%s: %v", bad, err)
		}
	}
}

func TestApplyV2_RootBelowStateDirAllowed(t *testing.T) {
	e := newEnv(t)
	sd := filepath.Join(e.home, ".rig-crew")
	p := plan.Plan{Harness: "claude", Owner: "pack:dev:claude", StateDir: sd, Root: filepath.Join(sd, "claude"),
		Scope: plan.Scope{Global: true, Home: e.home}}
	p.Files = []plan.File{{Path: filepath.Join(sd, "claude", "dev", "a.md"), Data: []byte("A")}}
	e.applyTwice(p)
	if !exists(p.Files[0].Path) {
		t.Fatal("file not written")
	}
}

func TestApplyV2_ActivationRecordHasLowerCamelKeys(t *testing.T) {
	e := newEnv(t)
	p := e.v2("pack-a", map[string]string{"a.md": "A"})
	p.Activation = &plan.Activation{Harness: "claude", Level: LevelGlobal, Packs: []string{"pack-a"}}
	e.applyTwice(p)
	got := e.read(e.gman())
	if !strings.Contains(got, `"activation": {`) && !strings.Contains(got, `"activation":{`) || !strings.Contains(got, `"level"`) || strings.Contains(got, `"Level"`) {
		t.Fatal(got)
	}
}

func TestRawEntry_NumbersCompareByValue(t *testing.T) {
	e := newEnv(t)
	e.write(e.p("settings.json"), `{"m":{"v":1.0}}`)
	p := e.v2("pack-a", nil)
	p.Merges = []plan.JSONMerge{e.raw("m", `{"v":1}`)}
	e.applyTwice(p)
	// Removal leaves a value the user only reformatted.
	e.write(e.p("settings.json"), `{"m":{"v":1e0}}`)
	e.write(e.p("settings.json"), `{"m":{"v":1e0}}`)
	q := e.v2("pack-b", nil)
	q.Merges = []plan.JSONMerge{e.raw("n", `{"v":1}`)}
	e.applyTwice(q)
	e.write(e.p("settings.json"), `{"n":{"v":1e0}}`)
	if _, err := Remove(context.Background(), e.home, q.StateDir, "pack-b", Options{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.settings()["n"]; ok {
		t.Fatal("a value equal by number was left behind")
	}
}

func TestRemove_RefusesRootOutsideStateDirParent(t *testing.T) {
	e := newEnv(t)
	g := e.v2("pack-a", map[string]string{"agents/developer.md": "D"})
	e.applyTwice(g)
	psd := filepath.Join(e.proj(), ".rig-crew")
	e.write(filepath.Join(psd, "manifest.json"), e.read(e.gman()))
	e.refused(ErrOutsideRoot, e.remove(psd, "pack-a"))
	if !exists(e.p("agents/developer.md")) {
		t.Fatal("file deleted")
	}
}

func TestRawEntry_HugeExponentIsFast(t *testing.T) {
	start := time.Now()
	if !valEq(json.Number("1e99999999"), json.Number("1e99999999")) {
		t.Fatal("equal huge numbers differ")
	}
	if !valEq(json.Number("1e2"), json.Number("100")) {
		t.Fatal("1e2 != 100")
	}
	if valEq(json.Number("1e99999999"), json.Number("1")) {
		t.Fatal("huge equals 1")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestApplyV2_WorkflowReferenceAddedAndDroppedWithOwner(t *testing.T) {
	e := newEnv(t)
	wf := filepath.Join(e.home, ".rig", "workflows", "solo.json")
	p := e.v2("pack-a", map[string]string{"a.md": "A"})
	p.Workflow = &plan.WorkflowFile{Name: "solo", Path: wf, Data: []byte("{}")}
	e.applyTwice(p)
	if e.read(wf) != "{}" {
		t.Fatal("workflow file not written")
	}
	m := readV2(t, e, e.gman())
	if m.Owners["pack-a"].Workflow != wf {
		t.Fatalf("owner record %+v", m.Owners["pack-a"])
	}
	wman := manifestPath(filepath.Dir(wf))
	var wm wfManifest
	if err := json.Unmarshal([]byte(e.read(wman)), &wm); err != nil || len(wm.Refs) != 1 || wm.Refs[0].Role != "pack-a" {
		t.Fatalf("%v %+v", err, wm)
	}
	if _, err := Remove(context.Background(), e.home, p.StateDir, "pack-a", Options{}); err != nil {
		t.Fatal(err)
	}
	if exists(wf) || exists(wman) {
		t.Fatal("workflow file or reference left after Remove")
	}
}

func TestReadState_GivenProjectNotListed(t *testing.T) {
	e := newEnv(t)
	e.applyTwice(e.v2Project("pack-s", map[string]string{"s.md": "S"}, "session"))
	st, err := ReadState(e.home, e.proj())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Packs) != 1 || st.Packs[0].Owner != "pack-s" || st.Packs[0].Scope != "project" || st.Packs[0].Project != e.proj() ||
		len(st.Activations) != 1 || st.Activations[0].Level != "session" || len(st.Stale) != 0 {
		t.Fatalf("%+v", st)
	}
}
