package install

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AindriuB/rig-crew/internal/plan"
)

// A project manifest can arrive in a cloned repo, so every field of its
// records is attacker-controlled. These tests plant such records.

func (e env) plant(sd string, owner string, rec *roleRec, projects ...string) {
	e.t.Helper()
	m := manifestV2{Version: manifestVersion2, Owners: map[string]*roleRec{owner: rec}, Projects: projects}
	e.write(filepath.Join(sd, "manifest.json"), string(encodeJSON(m)))
}

// refused calls f twice, wants want both times, and wants the tree under the
// home unchanged by both.
func (e env) refused(want error, f func() error) {
	e.t.Helper()
	before := tree(e.t, e.home)
	for i := 0; i < 2; i++ {
		if err := f(); !errors.Is(err, want) {
			e.t.Fatalf("call %d: want %v, got %v", i, want, err)
		}
		if !reflect.DeepEqual(before, tree(e.t, e.home)) {
			e.t.Fatalf("call %d changed the tree", i)
		}
	}
}

func (e env) remove(sd, owner string) func() error {
	return func() error {
		_, err := Remove(context.Background(), e.home, sd, owner, Options{})
		return err
	}
}

func (e env) applyErr(p plan.Plan) func() error {
	return func() error {
		_, err := Apply(context.Background(), p, Options{})
		return err
	}
}

func (e env) projSD() string { return filepath.Join(e.proj(), ".rig-crew") }

func TestUntrusted_ApplyRootOutsideStateDirParent(t *testing.T) {
	e := newEnv(t)
	h := filepath.Join(e.home, "H")
	p := plan.Plan{Harness: "claude", Owner: "fresh", StateDir: filepath.Join(h, ".rig-crew"),
		Root: filepath.Join(e.home, "elsewhere", "claude"), Scope: plan.Scope{Global: true, Home: h}}
	e.refused(ErrOutsideRoot, e.applyErr(p))
}

func TestUntrusted_ApplyRootEqualsStateDir(t *testing.T) {
	e := newEnv(t)
	sd := filepath.Join(e.home, ".rig-crew")
	p := plan.Plan{Harness: "claude", Owner: "fresh", StateDir: sd, Root: sd, Scope: plan.Scope{Global: true, Home: e.home}}
	e.refused(ErrOutsideRoot, e.applyErr(p))
}

func TestUntrusted_RemoveEmptyRoot(t *testing.T) {
	e := newEnv(t)
	e.plant(e.projSD(), "x", &roleRec{Files: map[string]string{"a.md": hashHex([]byte("A"))}, Harness: "claude"})
	e.write(filepath.Join(e.proj(), "a.md"), "A")
	e.refused(ErrOutsideRoot, e.remove(e.projSD(), "x"))
}

func TestUntrusted_RemoveRootEqualsStateDir(t *testing.T) {
	e := newEnv(t)
	sd := e.projSD()
	e.plant(sd, "x", &roleRec{Root: sd, Files: map[string]string{"manifest.json": hashHex([]byte("A"))}})
	e.refused(ErrOutsideRoot, e.remove(sd, "x"))
}

func TestUntrusted_RemoveRootAboveStateDirParent(t *testing.T) {
	e := newEnv(t)
	e.plant(e.projSD(), "x", &roleRec{Root: e.home, Files: map[string]string{"a.md": hashHex([]byte("A"))}})
	e.refused(ErrOutsideRoot, e.remove(e.projSD(), "x"))
}

func TestUntrusted_RemoveRecordHomeDiffers(t *testing.T) {
	e := newEnv(t)
	other := filepath.Join(e.home, "other")
	e.plant(e.projSD(), "x", &roleRec{Root: filepath.Join(e.proj(), ".claude"), Home: other})
	e.refused(ErrOutsideRoot, e.remove(e.projSD(), "x"))
	if err := e.remove(e.projSD(), "x")(); err == nil || !strings.Contains(err.Error(), other) {
		t.Fatalf("error does not name the home: %v", err)
	}
}

func TestUntrusted_ApplyRecordHomeDiffers(t *testing.T) {
	e := newEnv(t)
	other := filepath.Join(e.home, "other")
	e.plant(e.projSD(), "x", &roleRec{Root: filepath.Join(e.proj(), ".claude"), Home: other})
	p := e.v2Project("x", map[string]string{"a.md": "A"}, "")
	e.refused(ErrOutsideRoot, e.applyErr(p))
	if err := e.applyErr(p)(); err == nil || !strings.Contains(err.Error(), other) {
		t.Fatalf("error does not name the home: %v", err)
	}
}

// planted workflow reference aimed at the global manifest
func (e env) plantAimedAtGlobal() (gm string) {
	e.t.Helper()
	g := e.v2("pack-g", map[string]string{"g.md": "G"})
	e.apply(g)
	return e.read(e.gman())
}

func TestUntrusted_RemoveWorkflowAimedAtManifest(t *testing.T) {
	e := newEnv(t)
	gm := e.plantAimedAtGlobal()
	e.plant(e.projSD(), "x", &roleRec{Root: filepath.Join(e.proj(), ".claude"), Home: e.home, Workflow: filepath.Join(e.home, "no-such-file")})
	if _, err := Remove(context.Background(), e.home, e.projSD(), "x", Options{}); err != nil {
		t.Fatal(err)
	}
	if e.read(e.gman()) != gm {
		t.Fatal("global manifest rewritten")
	}
}

func TestUntrusted_ApplyWorkflowAimedAtManifest(t *testing.T) {
	e := newEnv(t)
	gm := e.plantAimedAtGlobal()
	e.plant(e.projSD(), "x", &roleRec{Root: filepath.Join(e.proj(), ".claude"), Home: e.home, Workflow: filepath.Join(e.home, "no-such-file")})
	e.applyTwice(e.v2Project("x", map[string]string{"a.md": "A"}, ""))
	if e.read(e.gman()) != gm {
		t.Fatal("global manifest rewritten")
	}
}

func TestUntrusted_UninstallWorkflowAimedAtManifest(t *testing.T) {
	e := newEnv(t)
	gm := e.plantAimedAtGlobal()
	root := filepath.Join(e.proj(), ".claude")
	e.write(manifestPath(root), `{"version":1,"roles":{"r":{"workflow":"`+filepath.Join(e.home, "no-such-file")+`","home":"`+e.home+`"}}}`)
	if _, err := Uninstall(context.Background(), root, "r", Options{}); err != nil {
		t.Fatal(err)
	}
	if e.read(e.gman()) != gm {
		t.Fatal("global manifest rewritten")
	}
}

func TestUntrusted_ApplyProjectActivationStateDirMismatch(t *testing.T) {
	e := newEnv(t)
	p := e.v2Project("pack-a", nil, LevelProject)
	p.StateDir = filepath.Join(e.home, "x", "state")
	e.refused(ErrBadActivation, e.applyErr(p))
}

func TestUntrusted_RemoveRecordProjectDiffers(t *testing.T) {
	e := newEnv(t)
	e.applyTwice(e.v2Project("pack-a", nil, LevelProject))
	gm := e.read(e.gman())
	if !strings.Contains(gm, e.proj()) {
		t.Fatal("project not listed")
	}
	evil := filepath.Join(e.home, "evil", ".rig-crew")
	e.plant(evil, "x", &roleRec{Root: filepath.Join(e.home, "evil", ".claude"), Home: e.home, Project: e.proj(),
		Activation: &actRec{Level: LevelProject}})
	e.refused(ErrBadActivation, e.remove(evil, "x"))
	if e.read(e.gman()) != gm {
		t.Fatal("global list changed")
	}
}

func TestUntrusted_RemoveProjectLevelInGlobalDir(t *testing.T) {
	e := newEnv(t)
	e.plant(filepath.Join(e.home, ".rig-crew"), "x", &roleRec{Root: e.root, Home: e.home, Activation: &actRec{Level: LevelProject}})
	e.refused(ErrBadActivation, e.remove(filepath.Join(e.home, ".rig-crew"), "x"))
}

func TestUntrusted_RemoveEmptyOwner(t *testing.T) {
	e := newEnv(t)
	sd := filepath.Join(e.home, ".rig-crew")
	if _, err := CreateFile(context.Background(), sd, filepath.Join(e.home, "profiles", "a.yaml"), []byte("a"), Options{}); err != nil {
		t.Fatal(err)
	}
	e.refused(ErrBadOwner, e.remove(sd, ""))
}

func TestUntrusted_RemoveUserOwner(t *testing.T) {
	e := newEnv(t)
	sd := filepath.Join(e.home, ".rig-crew")
	if _, err := CreateFile(context.Background(), sd, filepath.Join(e.home, "profiles", "a.yaml"), []byte("a"), Options{}); err != nil {
		t.Fatal(err)
	}
	e.refused(ErrBadOwner, e.remove(sd, userOwner))
}

func TestRawEntry_EqualNumberIDs(t *testing.T) {
	e := newEnv(t)
	a := e.v2("pack-a", nil)
	a.Merges = []plan.JSONMerge{e.raw("m", `{"v":1}`)}
	b := e.v2("pack-b", nil)
	b.Merges = []plan.JSONMerge{e.raw("m", `{"v":1.0}`)}
	e.applyTwice(a)
	e.applyTwice(b)
	if _, err := Remove(context.Background(), e.home, a.StateDir, "pack-a", Options{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.settings()["m"]; !ok {
		t.Fatal("m deleted while pack-b still holds it")
	}
	if _, err := Remove(context.Background(), e.home, b.StateDir, "pack-b", Options{}); err != nil {
		t.Fatal(err)
	}
	if exists(e.p("settings.json")) {
		if _, ok := e.settings()["m"]; ok {
			t.Fatal("m left behind")
		}
	}
}
