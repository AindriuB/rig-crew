package catalog_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	crew "github.com/AindriuB/rig-crew"
	"github.com/AindriuB/rig-crew/internal/catalog"
	"github.com/AindriuB/rig-crew/internal/model"
)

const packJSON = `{
  "name": "developer",
  "description": "Builds tasks",
  "actor": "developer",
  "minRig": "0.5.0",
  "steering": [{"file": "RULES.md", "load": "always"}],
  "agents": ["implementer"],
  "skills": ["tdd"],
  "workflows": ["team"],
  "rig": {"allow": ["task start", "context"], "deny": ["task new"]},
  "shell": {"allow": ["gh search"]}
}`

func base() fstest.MapFS {
	return fstest.MapFS{
		"pool/README.md":                  {Data: []byte("readme")},
		"pool/prompts/implementer.md":     {Data: []byte("---\nname: implementer\ndescription: \"Does: work\"\n---\nBody here.\n")},
		"pool/skills/tdd/SKILL.md":        {Data: []byte("---\nname: tdd\ndescription: Test first\n---\nSteps.\n")},
		"pool/skills/tdd/extra/notes.md":  {Data: []byte("notes")},
		"packs/README.md":                 {Data: []byte("readme")},
		"packs/developer/pack.json":       {Data: []byte(packJSON)},
		"packs/developer/RULES.md":        {Data: []byte("Rules text\n")},
		"workflows/README.md":             {Data: []byte("readme")},
		"workflows/team.json":             {Data: []byte(`{"name":"team"}`)},
		"harnesses/README.md":             {Data: []byte("readme")},
		"harnesses/claude/agents.json":    {Data: []byte("base-agents")},
		"harnesses/claude/sub/other.json": {Data: []byte("base-other")},
	}
}

func mustFail(t *testing.T, fsys fs.FS, wantPath string) {
	t.Helper()
	_, err := catalog.Load(fsys, nil)
	if err == nil {
		t.Fatal("expected a load error")
	}
	if !strings.Contains(err.Error(), wantPath) {
		t.Fatalf("error %q does not name %q", err, wantPath)
	}
}

func TestLoad_ValidCatalog(t *testing.T) {
	c, err := catalog.Load(base(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := model.Pack{
		Name: "developer", Description: "Builds tasks", Actor: "developer", MinRig: "0.5.0",
		Steering: []model.Steering{{File: "RULES.md", Load: "always", Body: "Rules text\n"}}, Agents: []string{"implementer"}, Skills: []string{"tdd"}, Workflows: []string{"team"},
		Rig:   model.RigPolicy{Allow: []string{"task start", "context"}, Deny: []string{"task new"}},
		Shell: model.ShellPolicy{Allow: []string{"gh search"}},
	}
	if got := c.Packs(); !reflect.DeepEqual(got, []model.Pack{want}) {
		t.Fatalf("packs: %+v", got)
	}
	p, err := c.Prompt("implementer")
	if err != nil || p != (model.Prompt{Name: "implementer", Description: "Does: work", Body: "Body here.\n"}) {
		t.Fatalf("prompt: %+v %v", p, err)
	}
	s, err := c.Skill("tdd")
	if err != nil || s.Name != "tdd" || s.Description != "Test first" {
		t.Fatalf("skill: %+v %v", s, err)
	}
	var paths []string
	for _, f := range s.Files {
		paths = append(paths, f.Path)
	}
	if !reflect.DeepEqual(paths, []string{"SKILL.md", "extra/notes.md"}) || string(s.Files[1].Data) != "notes" {
		t.Fatalf("skill files: %v", paths)
	}
	w, err := c.Workflow("team")
	if err != nil || w.Name != "team" || string(w.Data) != `{"name":"team"}` || w.SourcePath != "" {
		t.Fatalf("workflow: %+v %v", w, err)
	}
	if len(c.Workflows()) != 1 {
		t.Fatal("workflows")
	}
	if !reflect.DeepEqual(c.Harnesses(), []string{"claude"}) {
		t.Fatalf("harnesses: %v", c.Harnesses())
	}
	h, err := c.HarnessFS("claude")
	if err != nil {
		t.Fatal(err)
	}
	var walked []string
	_ = fs.WalkDir(h, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			walked = append(walked, p)
		}
		return err
	})
	sort.Strings(walked)
	if !reflect.DeepEqual(walked, []string{"agents.json", "sub/other.json"}) {
		t.Fatalf("harness files: %v", walked)
	}
}

func TestLoad_UnknownPackFieldFails(t *testing.T) {
	f := base()
	f["packs/developer/pack.json"] = &fstest.MapFile{Data: []byte(strings.Replace(packJSON, `"actor"`, `"extra": 1, "actor"`, 1))}
	mustFail(t, f, "packs/developer/pack.json")
}

func TestLoad_PackMissingAgentFails(t *testing.T) {
	f := base()
	delete(f, "pool/prompts/implementer.md")
	mustFail(t, f, "packs/developer/pack.json")
}

func TestLoad_PackMissingSkillFails(t *testing.T) {
	f := base()
	delete(f, "pool/skills/tdd/SKILL.md")
	delete(f, "pool/skills/tdd/extra/notes.md")
	mustFail(t, f, "packs/developer/pack.json")
}

func TestLoad_PackMissingWorkflowFails(t *testing.T) {
	f := base()
	delete(f, "workflows/team.json")
	mustFail(t, f, "packs/developer/pack.json")
}

func TestLoad_NameMismatchFails(t *testing.T) {
	cases := map[string]struct{ path, data string }{
		"packs/developer/pack.json":   {"packs/developer/pack.json", strings.Replace(packJSON, `"name": "developer"`, `"name": "dev"`, 1)},
		"pool/prompts/implementer.md": {"pool/prompts/implementer.md", "---\nname: other\ndescription: d\n---\n"},
		"pool/skills/tdd/SKILL.md":    {"pool/skills/tdd/SKILL.md", "---\nname: other\ndescription: d\n---\n"},
		"workflows/team.json":         {"workflows/team.json", `{"name":"solo"}`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := base()
			f[c.path] = &fstest.MapFile{Data: []byte(c.data)}
			mustFail(t, f, c.path)
		})
	}
}

func TestLoad_BadMinRigFails(t *testing.T) {
	f := base()
	f["packs/developer/pack.json"] = &fstest.MapFile{Data: []byte(strings.Replace(packJSON, "0.5.0", "0.5", 1))}
	mustFail(t, f, "packs/developer/pack.json")
}

func TestLoad_MissingSteeringFileFails(t *testing.T) {
	f := base()
	delete(f, "packs/developer/RULES.md")
	mustFail(t, f, "packs/developer/RULES.md")
}

func TestLoad_OverlayReplacesAndAdds(t *testing.T) {
	ov := fstest.MapFS{
		"pool/prompts/implementer.md": {Data: []byte("---\nname: implementer\ndescription: replaced\n---\nNew body\n")},
		"pool/prompts/extra.md":       {Data: []byte("---\nname: extra\ndescription: added\n---\nx\n")},
		"workflows/team.json":         {Data: []byte(`{"name":"team","x":1}`)},
		"workflows/mine.json":         {Data: []byte(`{"name":"mine"}`)},
	}
	dir := filepath.Join("some", "dir")
	c, err := catalog.Load(base(), &catalog.Overlay{FS: ov, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Prompt("implementer")
	if p.Description != "replaced" || p.Body != "New body\n" {
		t.Fatalf("prompt not replaced: %+v", p)
	}
	if _, err := c.Prompt("extra"); err != nil {
		t.Fatal(err)
	}
	w, _ := c.Workflow("team")
	if w.SourcePath != filepath.Join(dir, "workflows", "team.json") || !strings.Contains(string(w.Data), `"x"`) {
		t.Fatalf("workflow: %+v", w)
	}
	m, _ := c.Workflow("mine")
	if !strings.HasPrefix(m.SourcePath, dir) {
		t.Fatalf("added workflow source: %q", m.SourcePath)
	}
	// Whole-item replace: overlay skill folder drops the base's extra file.
	ov["pool/skills/tdd/SKILL.md"] = &fstest.MapFile{Data: []byte("---\nname: tdd\ndescription: new\n---\n")}
	c, err = catalog.Load(base(), &catalog.Overlay{FS: ov, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := c.Skill("tdd")
	if s.Description != "new" || len(s.Files) != 1 {
		t.Fatalf("skill not replaced whole: %+v", s)
	}
}

func TestLoad_OverlayBreakingReferenceFails(t *testing.T) {
	ov := fstest.MapFS{"packs/developer/pack.json": {Data: []byte(strings.Replace(packJSON, `["implementer"]`, `["ghost"]`, 1))}, "packs/developer/RULES.md": {Data: []byte("r")}}
	if _, err := catalog.Load(base(), &catalog.Overlay{FS: ov, Dir: "ov"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestHarnessFS_OverlayFileWins(t *testing.T) {
	ov := fstest.MapFS{
		"harnesses/claude/agents.json": {Data: []byte("overlay-agents")},
		"harnesses/claude/new.json":    {Data: []byte("new")},
	}
	c, err := catalog.Load(base(), &catalog.Overlay{FS: ov, Dir: "ov"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.HarnessFS("claude")
	if err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{"agents.json": "overlay-agents", "sub/other.json": "base-other", "new.json": "new"} {
		got, err := fs.ReadFile(h, file)
		if err != nil || string(got) != want {
			t.Errorf("%s: %q %v", file, got, err)
		}
	}
}

func TestCatalog_MissingNameIsErrNotFound(t *testing.T) {
	c, err := catalog.Load(base(), nil)
	if err != nil {
		t.Fatal(err)
	}
	errs := []error{}
	_, e := c.RolePack("x")
	errs = append(errs, e)
	_, e = c.Pack("x")
	errs = append(errs, e)
	_, e = c.Profile("x")
	errs = append(errs, e)
	_, e = c.Prompt("x")
	errs = append(errs, e)
	_, e = c.Skill("x")
	errs = append(errs, e)
	_, e = c.Workflow("x")
	errs = append(errs, e)
	_, e = c.HarnessFS("x")
	errs = append(errs, e)
	for i, e := range errs {
		if !errors.Is(e, catalog.ErrNotFound) {
			t.Errorf("lookup %d: %v", i, e)
		}
	}
}

func TestEmbedded_Loads(t *testing.T) {
	if _, err := catalog.Load(crew.Content(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_OverlaySkipsDotAndUnderscoreEntries(t *testing.T) {
	ov := fstest.MapFS{
		"packs/.DS_Store":                    {Data: []byte("x")},
		"packs/_draft/pack.json":             {Data: []byte("not json")},
		"pool/skills/.DS_Store":              {Data: []byte("x")},
		"pool/skills/_draft/notes.txt":       {Data: []byte("x")},
		"pool/prompts/.DS_Store":             {Data: []byte("x")},
		"workflows/.DS_Store":                {Data: []byte("x")},
		"pool/skills/extra/SKILL.md":         {Data: []byte("---\nname: extra\ndescription: d\n---\n")},
		"pool/skills/extra/.DS_Store":        {Data: []byte("x")},
		"pool/skills/extra/sub/.DS_Store":    {Data: []byte("x")},
		"pool/skills/extra/_hidden/file.txt": {Data: []byte("x")},
		"pool/skills/extra/sub/real.txt":     {Data: []byte("y")},
	}
	c, err := catalog.Load(base(), &catalog.Overlay{FS: ov, Dir: "ov"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.Skill("extra")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range s.Files {
		got = append(got, f.Path)
	}
	if strings.Join(got, ",") != "SKILL.md,sub/real.txt" {
		t.Fatalf("files: %v", got)
	}
}

func TestLoad_TrailingBracketFails(t *testing.T) {
	for _, tail := range []string{"}", "]", " {}"} {
		f := base()
		f["packs/developer/pack.json"] = &fstest.MapFile{Data: []byte(packJSON + tail)}
		mustFail(t, f, "packs/developer/pack.json")
	}
}

func TestLoad_WorkflowNameNotKebabFails(t *testing.T) {
	f := base()
	f["workflows/My_Flow.json"] = &fstest.MapFile{Data: []byte(`{"name":"My_Flow"}`)}
	_, err := catalog.Load(f, nil)
	if err == nil || !strings.Contains(err.Error(), "not kebab-case") {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_EmptyDescriptionOrActorFails(t *testing.T) {
	for _, k := range []string{"description", "actor"} {
		t.Run(k, func(t *testing.T) {
			f := base()
			old := map[string]string{"description": `"Builds tasks",`, "actor": `"developer",`}[k]
			f["packs/developer/pack.json"] = &fstest.MapFile{Data: []byte(strings.Replace(packJSON, `"`+k+`": `+old, `"`+k+`": "",`, 1))}
			_, err := catalog.Load(f, nil)
			if err == nil || !strings.Contains(err.Error(), k+" must not be empty") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestLoad_SkippedSteeringNameFails(t *testing.T) {
	for _, n := range []string{"_rules.md", ".rules.md"} {
		role := strings.Replace(packJSON, "RULES.md", n, 1)
		mem := base()
		mem["packs/developer/pack.json"] = &fstest.MapFile{Data: []byte(role)}
		mem["packs/developer/"+n] = &fstest.MapFile{Data: []byte("r")}
		mustFail(t, mem, "packs/developer/pack.json")

		ov := fstest.MapFS{
			"packs/developer/pack.json": {Data: []byte(role)},
			"packs/developer/" + n:      {Data: []byte("r")},
		}
		_, err := catalog.Load(base(), &catalog.Overlay{FS: ov, Dir: "ov"})
		if err == nil || !strings.Contains(err.Error(), "pack.json") {
			t.Fatalf("overlay %s: got %v", n, err)
		}
	}
}
