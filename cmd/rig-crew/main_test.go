package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	crew "github.com/AindriuB/rig-crew"
	"github.com/AindriuB/rig-crew/internal/harness"
	"github.com/AindriuB/rig-crew/internal/harness/claude"
	"github.com/AindriuB/rig-crew/internal/rigcli"
)

type env struct {
	home, proj string
	fake       *rigcli.Fake
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{home: t.TempDir(), proj: t.TempDir(), fake: rigcli.NewFake()}
	e.fake.On([]string{"rig", "version"}, rigcli.Output{Stdout: []byte("0.5.0\n")}, nil)
	return e
}

func (e *env) run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	d := deps{Home: e.home, Content: crew.Content(), Registry: harness.NewRegistry(claude.New()), Rig: e.fake}
	code := run(context.Background(), args, &out, &errb, d)
	return code, out.String(), errb.String()
}

func (e *env) installArgs(extra ...string) []string {
	return append([]string{"install", "--role", "developer", "--harness", "claude", "--workflow", "solo", "--project", e.proj}, extra...)
}

func snapshot(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				m[p] = "<dir>"
				return nil
			}
			b, err := os.ReadFile(p)
			m[p] = string(b)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func TestRun_Version(t *testing.T) {
	code, out, _ := newEnv(t).run("version")
	if code != 0 || strings.Count(out, "\n") != 1 || strings.TrimSpace(out) == "" {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, out, _ = newEnv(t).run("version", "--json")
	if code != 0 || !strings.HasPrefix(out, "{\n  \"version\": \"") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func checkUsage(t *testing.T, args ...string) {
	t.Helper()
	code, out, errs := newEnv(t).run(args...)
	if code != 2 || out != "" || errs == "" {
		t.Fatalf("%v: code=%d out=%q err=%q", args, code, out, errs)
	}
}

func TestRun_UsageErrors(t *testing.T) {
	checkUsage(t)
	checkUsage(t, "bogus")
	checkUsage(t, "--bogus")
	checkUsage(t, "version", "--bogus")
	checkUsage(t, "list", "--bogus")
}

func TestList_JSON(t *testing.T) {
	code, out, errs := newEnv(t).run("list", "--json")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errs)
	}
	var got listOut
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Roles) != 2 || got.Roles[0].Name != "developer" || got.Roles[1].Name != "tester" {
		t.Fatalf("roles %+v", got.Roles)
	}
	for _, r := range got.Roles {
		if r.Description == "" || r.MinRig == "" || !reflect.DeepEqual(r.Workflows, []string{"solo"}) {
			t.Fatalf("role %+v", r)
		}
	}
	if !reflect.DeepEqual(got.Harnesses, []string{"claude"}) || !reflect.DeepEqual(got.Workflows, []string{"solo"}) {
		t.Fatalf("got %+v", got)
	}
	if code, _, _ := newEnv(t).run("list"); code != 0 {
		t.Fatalf("list code=%d", code)
	}
}

func checkIdempotent(t *testing.T, e *env, args []string) {
	t.Helper()
	code, out, errs := e.run(args...)
	if code != 0 || strings.Contains(out, "nothing to change") {
		t.Fatalf("first: code=%d out=%q err=%q", code, out, errs)
	}
	before := snapshot(t, e.home, e.proj)
	code, out, errs = e.run(args...)
	if code != 0 || !strings.Contains(out, "nothing to change") {
		t.Fatalf("second: code=%d out=%q err=%q", code, out, errs)
	}
	if after := snapshot(t, e.home, e.proj); !reflect.DeepEqual(before, after) {
		t.Fatal("trees differ between runs")
	}
}

func TestInstall_ProjectTwiceIsIdempotent(t *testing.T) {
	e := newEnv(t)
	checkIdempotent(t, e, e.installArgs())
}

func TestInstall_GlobalTwiceIsIdempotent(t *testing.T) {
	e := newEnv(t)
	checkIdempotent(t, e, []string{"install", "--role", "tester", "--harness", "claude", "--workflow", "solo", "--global"})
}

func TestUninstall_RestoresProjectTree(t *testing.T) {
	e := newEnv(t)
	settings := filepath.Join(e.proj, ".claude", "settings.local.json")
	mine := filepath.Join(e.proj, ".claude", "agents", "mine.md")
	for p, data := range map[string]string{settings: "{\"model\": \"x\", \"permissions\": {\"allow\": [\"Bash(ls)\"]}}\n", mine: "mine\n"} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	projBefore, homeBefore := snapshot(t, e.proj), snapshot(t, e.home)
	if code, _, errs := e.run(e.installArgs()...); code != 0 {
		t.Fatalf("install: %d %s", code, errs)
	}
	code, _, errs := e.run("uninstall", "--role", "developer", "--harness", "claude", "--project", e.proj)
	if code != 0 {
		t.Fatalf("uninstall: %d %s", code, errs)
	}
	projAfter := snapshot(t, e.proj)
	for p, want := range projBefore {
		got, ok := projAfter[p]
		if !ok {
			t.Fatalf("lost %s", p)
		}
		if p == settings {
			var a, b any
			if json.Unmarshal([]byte(want), &a) != nil || json.Unmarshal([]byte(got), &b) != nil || !reflect.DeepEqual(a, b) {
				t.Fatalf("settings: %q want %q", got, want)
			}
		} else if got != want {
			t.Fatalf("%s changed", p)
		}
	}
	for p := range projAfter {
		if _, ok := projBefore[p]; !ok {
			t.Fatalf("new entry %s", p)
		}
	}
	for p, v := range snapshot(t, e.home) {
		if _, ok := homeBefore[p]; !ok && v != "<dir>" {
			t.Fatalf("new file in home %s", p)
		}
	}
}

func TestInstall_DryRunTouchesNothing(t *testing.T) {
	e := newEnv(t)
	before := snapshot(t, e.home, e.proj)
	code, out, errs := e.run(e.installArgs("--dry-run")...)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errs)
	}
	if !reflect.DeepEqual(before, snapshot(t, e.home, e.proj)) {
		t.Fatal("dry run wrote")
	}
	code, rout, _ := e.run("render", "--role", "developer", "--harness", "claude", "--workflow", "solo", "--project", e.proj, "--json")
	if code != 0 {
		t.Fatal(rout)
	}
	var r renderOut
	if err := json.Unmarshal([]byte(rout), &r); err != nil {
		t.Fatal(err)
	}
	paths := append(append([]string{}, r.Files...), r.Workflow)
	for _, m := range r.Merges {
		paths = append(paths, m.Path)
	}
	for _, p := range paths {
		if !strings.Contains(out, p) {
			t.Errorf("dry run does not print %s", p)
		}
	}
}

func TestUninstall_DryRunTouchesNothing(t *testing.T) {
	e := newEnv(t)
	if code, _, errs := e.run(e.installArgs()...); code != 0 {
		t.Fatalf("install: %d %s", code, errs)
	}
	before := snapshot(t, e.home, e.proj)
	uninstall := []string{"uninstall", "--role", "developer", "--harness", "claude", "--project", e.proj}
	code, dry, errs := e.run(append(append([]string{}, uninstall...), "--dry-run")...)
	if code != 0 {
		t.Fatalf("dry run: code=%d err=%s", code, errs)
	}
	if !reflect.DeepEqual(before, snapshot(t, e.home, e.proj)) {
		t.Fatal("dry run wrote")
	}
	code, rout, _ := e.run("render", "--role", "developer", "--harness", "claude", "--workflow", "solo", "--project", e.proj, "--json")
	if code != 0 {
		t.Fatal(rout)
	}
	var r renderOut
	if err := json.Unmarshal([]byte(rout), &r); err != nil {
		t.Fatal(err)
	}
	paths := append(append([]string{}, r.Files...), r.Workflow)
	for _, m := range r.Merges {
		paths = append(paths, m.Path)
	}
	for _, p := range paths {
		if !strings.Contains(dry, p) {
			t.Errorf("dry run does not print %s", p)
		}
	}
	code, real, errs := e.run(uninstall...)
	if code != 0 {
		t.Fatalf("uninstall: code=%d err=%s", code, errs)
	}
	printed := 0
	for _, line := range strings.Split(real, "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		_, p, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("unexpected uninstall line %q", line)
		}
		printed++
		if !strings.Contains(dry, p) {
			t.Errorf("real uninstall prints %s, dry run does not", p)
		}
	}
	if printed == 0 {
		t.Fatalf("real uninstall printed nothing: %q", real)
	}
}

func TestInstall_ConflictExitsOne(t *testing.T) {
	e := newEnv(t)
	p := filepath.Join(e.proj, ".claude", "agents", "implementer.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("not ours\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errs := e.run(e.installArgs()...)
	if code != 1 || !strings.Contains(errs, p) {
		t.Fatalf("code=%d err=%q", code, errs)
	}
	if b, _ := os.ReadFile(p); string(b) != "not ours\n" {
		t.Fatal("conflicting file changed")
	}
}

func TestInstall_RigTooOldExitsOne(t *testing.T) {
	e := newEnv(t)
	e.fake.On([]string{"rig", "version"}, rigcli.Output{Stdout: []byte("0.4.0\n")}, nil)
	code, _, errs := e.run(e.installArgs()...)
	if code != 1 || !strings.Contains(errs, "0.4.0") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

func TestInstall_RigMissingExitsOne(t *testing.T) {
	e := newEnv(t)
	e.fake.On([]string{"rig", "version"}, rigcli.Output{}, rigcli.ErrRigNotFound)
	code, _, errs := e.run(e.installArgs()...)
	if code != 1 || !strings.Contains(errs, "rig") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

func TestInstall_UnknownRoleExitsOne(t *testing.T) {
	e := newEnv(t)
	code, _, errs := e.run("install", "--role", "nobody", "--harness", "claude", "--workflow", "solo", "--project", e.proj)
	if code != 1 || !strings.Contains(errs, "nobody") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
	code, _, errs = e.run("install", "--role", "developer", "--harness", "nope", "--workflow", "solo", "--project", e.proj)
	if code != 1 || !strings.Contains(errs, "nope") {
		t.Fatalf("harness: code=%d err=%q", code, errs)
	}
	code, _, errs = e.run("install", "--role", "developer", "--harness", "claude", "--workflow", "nope", "--project", e.proj)
	if code != 1 || !strings.Contains(errs, "nope") {
		t.Fatalf("workflow: code=%d err=%q", code, errs)
	}
}

func TestInstall_WorkflowNotInRoleExitsOne(t *testing.T) {
	e := newEnv(t)
	from := t.TempDir()
	if err := os.MkdirAll(filepath.Join(from, "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "workflows", "other.json"), []byte(`{"name":"other"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"install", "--role", "developer", "--harness", "claude", "--workflow", "other", "--project", e.proj, "--from", from}
	code, _, errs := e.run(args...)
	if code != 1 || !strings.Contains(errs, "not one of role") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

func TestUninstall_NotInstalledExitsOne(t *testing.T) {
	e := newEnv(t)
	code, _, errs := e.run("uninstall", "--role", "developer", "--harness", "claude", "--project", e.proj)
	if code != 1 || errs == "" {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

func TestInstall_ScopeFlagsAreExclusive(t *testing.T) {
	e := newEnv(t)
	base := []string{"install", "--role", "developer", "--harness", "claude", "--workflow", "solo"}
	checkUsage(t, base...)
	checkUsage(t, append(base, "--global", "--project", e.proj)...)
}

func TestInstall_MissingRoleFlagIsUsage(t *testing.T) {
	checkUsage(t, "install", "--harness", "claude", "--workflow", "solo", "--global")
	checkUsage(t, "install", "--role", "developer", "--harness", "claude", "--global")
}

func TestInstall_OverlayWorkflowValidated(t *testing.T) {
	e := newEnv(t)
	from := t.TempDir()
	if err := os.MkdirAll(filepath.Join(from, "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	wf := filepath.Join(from, "workflows", "solo.json")
	if err := os.WriteFile(wf, []byte(`{"name":"solo","broken":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(wf)
	if err != nil {
		t.Fatal(err)
	}
	e.fake.On([]string{"rig", "workflow", "validate", abs, "--json"},
		rigcli.Output{Stdout: []byte(`{"valid":false,"problems":[{"path":"$.broken","message":"unknown field"}]}`), ExitCode: 1}, nil)
	code, _, errs := e.run(e.installArgs("--from", from)...)
	if code != 1 || !strings.Contains(errs, "unknown field") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
	called := false
	for _, c := range e.fake.Calls {
		if strings.Join(c, " ") == "rig workflow validate "+abs+" --json" {
			called = true
		}
	}
	if !called {
		t.Fatalf("calls %v", e.fake.Calls)
	}
}

func TestRender_JSONShape(t *testing.T) {
	e := newEnv(t)
	before := snapshot(t, e.home, e.proj)
	code, out, errs := e.run("render", "--role", "tester", "--harness", "claude", "--workflow", "solo", "--project", e.proj, "--json")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errs)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"files", "merges", "workflow", "reports"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %s", k)
		}
	}
	if len(m) != 4 {
		t.Errorf("keys %v", m)
	}
	if !reflect.DeepEqual(before, snapshot(t, e.home, e.proj)) {
		t.Fatal("render wrote")
	}
	if len(e.fake.Calls) != 0 {
		t.Fatalf("render called rig: %v", e.fake.Calls)
	}
	code, out, _ = e.run(e.installArgs("--json")...)
	var io map[string]json.RawMessage
	if code != 0 || json.Unmarshal([]byte(out), &io) != nil || len(io) != 4 {
		t.Fatalf("install json: %d %q", code, out)
	}
}

func TestHomeDir_IsCleaned(t *testing.T) {
	base := t.TempDir()
	t.Setenv("HOME", base+string(filepath.Separator))
	t.Setenv("USERPROFILE", base+string(filepath.Separator))
	got, err := homeDir()
	want := filepath.Clean(base)
	if err != nil || got != want {
		t.Fatalf("homeDir() = %q, %v; want %s", got, err, want)
	}
}
