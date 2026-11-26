package catalog_test

import (
	"encoding/json"
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

const roleJSON = `{
  "name": "developer",
  "description": "Builds tasks",
  "actor": "developer",
  "minRig": "0.5.0",
  "rules": "RULES.md",
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
		"roles/README.md":                 {Data: []byte("readme")},
		"roles/developer/role.json":       {Data: []byte(roleJSON)},
		"roles/developer/RULES.md":        {Data: []byte("Rules text\n")},
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
	want := model.Role{
		Name: "developer", Description: "Builds tasks", Actor: "developer", MinRig: "0.5.0",
		Rules: "Rules text\n", Agents: []string{"implementer"}, Skills: []string{"tdd"}, Workflows: []string{"team"},
		Rig:   model.RigPolicy{Allow: []string{"task start", "context"}, Deny: []string{"task new"}},
		Shell: model.ShellPolicy{Allow: []string{"gh search"}},
	}
	if got := c.Roles(); !reflect.DeepEqual(got, []model.Role{want}) {
		t.Fatalf("roles: %+v", got)
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

func TestLoad_UnknownRoleFieldFails(t *testing.T) {
	f := base()
	f["roles/developer/role.json"] = &fstest.MapFile{Data: []byte(strings.Replace(roleJSON, `"actor"`, `"extra": 1, "actor"`, 1))}
	mustFail(t, f, "roles/developer/role.json")
}

func TestLoad_RoleMissingAgentFails(t *testing.T) {
	f := base()
	delete(f, "pool/prompts/implementer.md")
	mustFail(t, f, "roles/developer/role.json")
}

func TestLoad_RoleMissingSkillFails(t *testing.T) {
	f := base()
	delete(f, "pool/skills/tdd/SKILL.md")
	delete(f, "pool/skills/tdd/extra/notes.md")
	mustFail(t, f, "roles/developer/role.json")
}

func TestLoad_RoleMissingWorkflowFails(t *testing.T) {
	f := base()
	delete(f, "workflows/team.json")
	mustFail(t, f, "roles/developer/role.json")
}

func TestLoad_NameMismatchFails(t *testing.T) {
	cases := map[string]struct{ path, data string }{
		"roles/developer/role.json":   {"roles/developer/role.json", strings.Replace(roleJSON, `"name": "developer"`, `"name": "dev"`, 1)},
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
	f["roles/developer/role.json"] = &fstest.MapFile{Data: []byte(strings.Replace(roleJSON, "0.5.0", "0.5", 1))}
	mustFail(t, f, "roles/developer/role.json")
}

func TestLoad_MissingRulesFileFails(t *testing.T) {
	f := base()
	delete(f, "roles/developer/RULES.md")
	mustFail(t, f, "roles/developer/RULES.md")
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
	ov := fstest.MapFS{"roles/developer/role.json": {Data: []byte(strings.Replace(roleJSON, `["implementer"]`, `["ghost"]`, 1))}, "roles/developer/RULES.md": {Data: []byte("r")}}
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
	_, e := c.Role("x")
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

func TestSchema_MatchesRoleStruct(t *testing.T) {
	data, err := fs.ReadFile(crew.Content(), "schema/role.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	checkObject(t, "role", schema, reflect.TypeOf(catalog.RoleFileForTest{}))
}

func checkObject(t *testing.T, where string, schema map[string]any, typ reflect.Type) {
	t.Helper()
	if schema["additionalProperties"] != false {
		t.Errorf("%s: additionalProperties must be false", where)
	}
	props, _ := schema["properties"].(map[string]any)
	var want []string
	for i := 0; i < typ.NumField(); i++ {
		key := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		want = append(want, key)
		sub, ok := props[key].(map[string]any)
		if !ok {
			t.Errorf("%s: schema lacks property %q", where, key)
			continue
		}
		ft := typ.Field(i).Type
		if ft.Kind() == reflect.Pointer && ft.Elem().Kind() == reflect.Struct {
			checkObject(t, where+"."+key, sub, ft.Elem())
		}
	}
	var gotProps []string
	for k := range props {
		gotProps = append(gotProps, k)
	}
	var required []string
	for _, r := range schema["required"].([]any) {
		required = append(required, r.(string))
	}
	sort.Strings(want)
	sort.Strings(gotProps)
	sort.Strings(required)
	if !reflect.DeepEqual(want, gotProps) || !reflect.DeepEqual(want, required) {
		t.Errorf("%s: struct %v, properties %v, required %v", where, want, gotProps, required)
	}
}
