package claude_test

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/AindriuB/rig-crew/internal/catalog"
	"github.com/AindriuB/rig-crew/internal/harness"
	"github.com/AindriuB/rig-crew/internal/harness/claude"
	"github.com/AindriuB/rig-crew/internal/model"
	"github.com/AindriuB/rig-crew/internal/plan"
)

var update = flag.Bool("update", false, "rewrite golden files")

const (
	fakeBase = "/work/proj"
	fakeHome = "/home/u"
)

func fixture(t *testing.T, override map[string]string) *catalog.Catalog {
	t.Helper()
	m := fstest.MapFS{}
	err := fs.WalkDir(os.DirFS("testdata/catalog"), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(filepath.Join("testdata/catalog", p))
		m[p] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range override {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	c, err := catalog.Load(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func request(t *testing.T, c *catalog.Catalog, global bool, pins string) harness.Request {
	t.Helper()
	role, err := c.RolePack("dev")
	if err != nil {
		t.Fatal(err)
	}
	wf, err := c.Workflow("team")
	if err != nil {
		t.Fatal(err)
	}
	s := plan.Scope{Global: global, Base: fakeBase, Home: fakeHome}
	if global {
		s.Base = fakeHome
	}
	req := harness.Request{Pack: role, Workflow: wf, Scope: s}
	if pins != "" {
		req.ScopeFS = fstest.MapFS{".claude/rig-crew-models.json": {Data: []byte(pins)}}
	}
	return req
}

func render(t *testing.T, c *catalog.Catalog, req harness.Request) plan.Plan {
	t.Helper()
	p, err := claude.New().Render(c, req)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func file(t *testing.T, p plan.Plan, suffix string) string {
	t.Helper()
	for _, f := range p.Files {
		if strings.HasSuffix(filepath.ToSlash(f.Path), suffix) {
			return string(f.Data)
		}
	}
	t.Fatalf("no file ending %q", suffix)
	return ""
}

func TestRender_AgentFile(t *testing.T) {
	c := fixture(t, nil)
	p := render(t, c, request(t, c, false, ""))
	want := "---\nname: planner\ndescription: Plans work\nmodel: claude-strong-1\ntools: Read, Grep\n---\nPlan the work.\n"
	if got := file(t, p, "/.claude/agents/planner.md"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := file(t, p, "/.claude/agents/builder.md"); !strings.Contains(got, "model: claude-fast-1\n") {
		t.Fatalf("builder: %q", got)
	}
}

func TestRender_SkillsAndRules(t *testing.T) {
	c := fixture(t, nil)
	p := render(t, c, request(t, c, false, ""))
	if got := file(t, p, "/.claude/skills/tdd/SKILL.md"); !strings.Contains(got, "Red, green.") {
		t.Fatal(got)
	}
	if got := file(t, p, "/.claude/skills/tdd/extra/notes.md"); got != "Notes.\n" {
		t.Fatal(got)
	}
	if got := file(t, p, "/.claude/rules/rig-crew-dev.md"); got != "Dev rules.\n" {
		t.Fatal(got)
	}
}

func checkSettings(t *testing.T, global bool, wantPath string) {
	t.Helper()
	c := fixture(t, nil)
	req := request(t, c, global, "")
	p := render(t, c, req)
	if len(p.Merges) != 1 {
		t.Fatalf("merges = %d", len(p.Merges))
	}
	m := p.Merges[0]
	if got := filepath.ToSlash(m.Path); got != wantPath {
		t.Fatalf("path %s want %s", got, wantPath)
	}
	var got []string
	for _, e := range m.Append {
		got = append(got, strings.Join(e.Key, ".")+"="+e.Value)
	}
	want := []string{
		"permissions.allow=Bash(rig task start *)",
		"permissions.allow=Bash(rig context *)",
		"permissions.allow=Bash(gh search *)",
		"permissions.deny=Bash(rig task new *)",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("append %v", got)
	}
	wf := "/home/u/.rig/workflows/team.json"
	if p.Workflow == nil || filepath.ToSlash(p.Workflow.Path) != wf {
		t.Fatalf("workflow %+v", p.Workflow)
	}
	var set []string
	for _, e := range m.Set {
		set = append(set, strings.Join(e.Key, ".")+"="+filepath.ToSlash(e.Value))
	}
	if strings.Join(set, "|") != "env.RIG_WORKFLOW="+wf+"|env.RIG_ACTOR=developer" {
		t.Fatalf("set %v", set)
	}
}

func TestRender_SettingsGlobal(t *testing.T) {
	checkSettings(t, true, "/home/u/.claude/settings.json")
}

func TestRender_SettingsProject(t *testing.T) {
	checkSettings(t, false, "/work/proj/.claude/settings.local.json")
}

func TestRender_ModelPins(t *testing.T) {
	c := fixture(t, nil)
	p := render(t, c, request(t, c, false, `{"planner":"my-model","builder":null,"ghost":"x"}`))
	if got := file(t, p, "/agents/planner.md"); !strings.Contains(got, "model: my-model\n") {
		t.Fatalf("pin did not win: %q", got)
	}
	if got := file(t, p, "/agents/builder.md"); strings.Contains(got, "model") {
		t.Fatalf("null did not omit: %q", got)
	}
	if len(p.Reports) != 1 || !strings.Contains(p.Reports[0].Message, `"ghost"`) {
		t.Fatalf("reports %+v", p.Reports)
	}
}

func TestRender_PinnedNonStringModelQuoted(t *testing.T) {
	c := fixture(t, nil)
	p := render(t, c, request(t, c, false, `{"planner":"1.5"}`))
	if got := file(t, p, "/agents/planner.md"); !strings.Contains(got, "model: \"1.5\"\n") {
		t.Fatalf("pin not quoted: %q", got)
	}
}

func renderErr(t *testing.T, override map[string]string, pins string) error {
	t.Helper()
	c := fixture(t, override)
	_, err := claude.New().Render(c, request(t, c, false, pins))
	if err == nil {
		t.Fatal("expected error")
	}
	return err
}

func TestRender_MissingAgentFileFails(t *testing.T) {
	// an agent file that is absent: point the role at an agent with a prompt but no harness file
	role := `{"name":"dev","description":"d","actor":"developer","minRig":"0.5.0","steering":[{"file":"RULES.md","load":"always"}],"agents":["planner","extra"],"skills":[],"workflows":["team"],"rig":{"allow":[],"deny":[]},"shell":{"allow":[]}}`
	err := renderErr(t, map[string]string{
		"packs/dev/pack.json":   role,
		"pool/prompts/extra.md": "---\nname: extra\ndescription: x\n---\nB\n",
	}, "")
	if !strings.Contains(err.Error(), "agents/extra.md") {
		t.Fatal(err)
	}
}

func TestRender_AgentBodyFails(t *testing.T) {
	err := renderErr(t, map[string]string{
		"harnesses/claude/agents/builder.md": "---\nname: builder\ndescription: b\nmodel: fast\n---\nstray body\n",
	}, "")
	if !strings.Contains(err.Error(), "agents/builder.md") {
		t.Fatal(err)
	}
}

func TestRender_BadModelHintFails(t *testing.T) {
	err := renderErr(t, map[string]string{
		"harnesses/claude/agents/builder.md": "---\nname: builder\ndescription: b\nmodel: medium\n---\n",
	}, "")
	if !strings.Contains(err.Error(), "agents/builder.md") {
		t.Fatal(err)
	}
}

func TestRender_DuplicateModelKeyFails(t *testing.T) {
	err := renderErr(t, map[string]string{
		"harnesses/claude/agents/builder.md": "---\nname: builder\ndescription: b\nmodel: fast\nmodel: strong\n---\n",
	}, "")
	if !strings.Contains(err.Error(), "agents/builder.md") || !strings.Contains(err.Error(), "model") {
		t.Fatal(err)
	}
}

// This can only fail on Windows, where filepath uses \ and sorts it after digits and capitals.
func TestRender_FilesSortedBySlashPath(t *testing.T) {
	role := `{"name":"dev","description":"d","actor":"developer","minRig":"0.5.0","steering":[{"file":"RULES.md","load":"always"}],"agents":["planner","builder"],"skills":["tdd","tdd2"],"workflows":["team"],"rig":{"allow":[],"deny":[]},"shell":{"allow":[]}}`
	c := fixture(t, map[string]string{
		"packs/dev/pack.json":       role,
		"pool/skills/tdd2/SKILL.md": "---\nname: tdd2\ndescription: Second\n---\nMore.\n",
	})
	p := render(t, c, request(t, c, false, ""))
	var got []string
	for _, f := range p.Files {
		got = append(got, filepath.ToSlash(f.Path))
	}
	if !sort.StringsAreSorted(got) {
		t.Fatalf("files not in slash order: %v", got)
	}
}

func TestRender_MalformedConfigFails(t *testing.T) {
	err := renderErr(t, map[string]string{"harnesses/claude/models.json": `{"fast":"a"}`}, "")
	if !strings.Contains(err.Error(), "models.json") {
		t.Fatal(err)
	}
	err = renderErr(t, nil, `{bad`)
	if !strings.Contains(err.Error(), "rig-crew-models.json") {
		t.Fatal(err)
	}
}

func dump(p plan.Plan) string {
	var b strings.Builder
	n := filepath.ToSlash
	fmt.Fprintf(&b, "harness: %s\nrole: %s\nscope: %s\nroot: %s\n", p.Harness, p.Role, p.Scope.Name(), n(p.Root))
	for _, f := range p.Files {
		fmt.Fprintf(&b, "\n== file %s\n%s", n(f.Path), f.Data)
	}
	for _, m := range p.Merges {
		fmt.Fprintf(&b, "\n== merge %s\n", n(m.Path))
		for _, e := range m.Append {
			fmt.Fprintf(&b, "append %s %s\n", strings.Join(e.Key, "."), e.Value)
		}
		for _, e := range m.Set {
			fmt.Fprintf(&b, "set %s %s\n", strings.Join(e.Key, "."), n(e.Value))
		}
	}
	if w := p.Workflow; w != nil {
		fmt.Fprintf(&b, "\n== workflow %s %s\n%s", w.Name, n(w.Path), w.Data)
	}
	for _, r := range p.Reports {
		fmt.Fprintf(&b, "\n== report %s: %s\n", r.Code, r.Message)
	}
	return b.String()
}

func TestRender_Golden(t *testing.T) {
	for _, global := range []bool{true, false} {
		c := fixture(t, nil)
		p := render(t, c, request(t, c, global, ""))
		got := dump(p)
		path := filepath.Join("testdata", "golden", p.Scope.Name()+".txt")
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Errorf("%s differs:\n%s", path, got)
		}
	}
}

func TestRender_NoPinsFileUsesHintModels(t *testing.T) {
	c := fixture(t, nil)
	req := request(t, c, false, "")
	req.ScopeFS = fstest.MapFS{}
	p := render(t, c, req)
	if len(p.Reports) != 0 {
		t.Fatalf("reports = %v", p.Reports)
	}
	if got := file(t, p, "/.claude/agents/planner.md"); !strings.Contains(got, "model: claude-strong-1\n") {
		t.Fatalf("planner: %q", got)
	}
	if got := file(t, p, "/.claude/agents/builder.md"); !strings.Contains(got, "model: claude-fast-1\n") {
		t.Fatalf("builder: %q", got)
	}
}

func TestRender_EmptyPinFails(t *testing.T) {
	err := renderErr(t, nil, `{"planner":""}`)
	if !strings.Contains(err.Error(), ".claude/rig-crew-models.json") || !strings.Contains(err.Error(), "planner") {
		t.Fatal(err)
	}
}

func TestRender_PinForOtherRoleAgentNotReported(t *testing.T) {
	role2 := `{"name":"other","description":"d","actor":"developer","minRig":"0.5.0","steering":[{"file":"RULES.md","load":"always"}],"agents":["extra"],"skills":[],"workflows":["team"],"rig":{"allow":[],"deny":[]},"shell":{"allow":[]}}`
	c := fixture(t, map[string]string{
		"packs/other/pack.json":            role2,
		"packs/other/RULES.md":             "Other.\n",
		"pool/prompts/extra.md":            "---\nname: extra\ndescription: x\n---\nB\n",
		"harnesses/claude/agents/extra.md": "---\nname: extra\ndescription: x\nmodel: fast\n---\n",
	})
	p := render(t, c, request(t, c, false, `{"extra":"m","ghost":"x"}`))
	if len(p.Reports) != 1 || !strings.Contains(p.Reports[0].Message, `"ghost"`) || !strings.Contains(p.Reports[0].Message, "no role") {
		t.Fatalf("reports %+v", p.Reports)
	}
}

func TestRender_TrailingBracketFails(t *testing.T) {
	for _, tail := range []string{"}", "]"} {
		err := renderErr(t, nil, `{"planner":"m"}`+tail)
		if !strings.Contains(err.Error(), "rig-crew-models.json") {
			t.Fatal(err)
		}
	}
	err := renderErr(t, map[string]string{"harnesses/claude/models.json": `{"fast":"a","strong":"b"}]`}, "")
	if !strings.Contains(err.Error(), "models.json") {
		t.Fatal(err)
	}
}

func TestRender_ReportsSteeringItDoesNotRender(t *testing.T) {
	c := fixture(t, nil)
	req := request(t, c, false, "")
	req.Pack.Steering = append(append([]model.Steering{}, req.Pack.Steering...),
		model.Steering{File: "go.md", Load: "paths", Paths: []string{"**/*.go"}, Description: "Go", Body: "go"},
		model.Steering{File: "deep.md", Load: "on-demand", Description: "Deep", Body: "deep"})
	p := render(t, c, req)
	if len(p.Reports) != 2 || !strings.Contains(p.Reports[0].Message, `"go.md"`) || !strings.Contains(p.Reports[0].Message, `"dev"`) || !strings.Contains(p.Reports[1].Message, `"deep.md"`) {
		t.Fatalf("reports %+v", p.Reports)
	}
	if strings.Contains(file(t, p, "/rules/rig-crew-dev.md"), "go") && strings.Contains(file(t, p, "/rules/rig-crew-dev.md"), "deep") {
		t.Fatal("non-always steering must not be in the rules file")
	}
}
