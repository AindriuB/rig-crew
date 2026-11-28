package catalog_test

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	crew "github.com/AindriuB/rig-crew"
	"github.com/AindriuB/rig-crew/internal/catalog"
)

func withFiles(extra map[string]string) fstest.MapFS {
	f := base()
	for k, v := range extra {
		f[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return f
}

func loadErr(t *testing.T, f fs.FS, want ...string) {
	t.Helper()
	_, err := catalog.Load(f, nil)
	if err == nil {
		t.Fatal("expected a load error")
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Fatalf("error %q does not contain %q", err, w)
		}
	}
}

const capJSON = `{"name": "ui", "description": "UI steering", "steering": [{"file": "UI.md", "load": "always"}]}`

func withCap(extra map[string]string) fstest.MapFS {
	f := withFiles(map[string]string{"packs/ui/pack.json": capJSON, "packs/ui/UI.md": "ui\n"})
	for k, v := range extra {
		f[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return f
}

func replaceDev(old, new string) map[string]string {
	return map[string]string{"packs/developer/pack.json": strings.Replace(packJSON, old, new, 1)}
}

func TestPack_UnknownKeyNamesFile(t *testing.T) {
	loadErr(t, withFiles(replaceDev(`"actor"`, `"bogus": 1, "actor"`)), "packs/developer/pack.json", "bogus")
}

func TestPack_RolePackNeedsMinRigAndWorkflows(t *testing.T) {
	loadErr(t, withFiles(replaceDev(`"minRig": "0.5.0",`, ``)), "packs/developer/pack.json", "minRig")
	loadErr(t, withFiles(replaceDev(`"workflows": ["team"],`, ``)), "packs/developer/pack.json", "workflows")
	loadErr(t, withFiles(replaceDev(`"workflows": ["team"]`, `"workflows": []`)), "packs/developer/pack.json", "workflows must not be empty")
}

func TestPack_CapabilityPackWithWorkflowsFails(t *testing.T) {
	cap := `{"name": "ui", "description": "d", "workflows": ["team"]}`
	loadErr(t, withFiles(map[string]string{"packs/ui/pack.json": cap}), "packs/ui/pack.json", "must not have workflows")
}

func TestPack_CapabilityPackLoads(t *testing.T) {
	c, err := catalog.Load(withCap(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.Pack("ui")
	if err != nil || p.IsRole() {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := c.RolePack("ui"); err == nil {
		t.Fatal("a capability pack is not a role")
	}
	if got := c.RolePacks(); len(got) != 1 || got[0].Name != "developer" || !got[0].IsRole() {
		t.Fatalf("role packs %+v", got)
	}
}

func TestPack_MissingSteeringFileNamesFile(t *testing.T) {
	f := base()
	delete(f, "packs/developer/RULES.md")
	loadErr(t, f, "packs/developer/pack.json", "RULES.md")
}

func steeringPack(entry string) map[string]string {
	return replaceDev(`[{"file": "RULES.md", "load": "always"}]`, `[`+entry+`]`)
}

func TestPack_SteeringRules(t *testing.T) {
	cases := map[string]struct{ entry, want string }{
		"unknown load":          {`{"file": "RULES.md", "load": "sometimes"}`, `unknown load "sometimes"`},
		"paths without paths":   {`{"file": "RULES.md", "load": "paths"}`, "needs a non-empty paths list"},
		"paths empty":           {`{"file": "RULES.md", "load": "paths", "paths": []}`, "needs a non-empty paths list"},
		"paths with always":     {`{"file": "RULES.md", "load": "always", "paths": ["*.go"]}`, "paths is only allowed"},
		"paths with on-demand":  {`{"file": "RULES.md", "load": "on-demand", "description": "d", "paths": ["*.go"]}`, "paths is only allowed"},
		"on-demand no desc":     {`{"file": "RULES.md", "load": "on-demand"}`, `needs a description`},
		"paths no desc":         {`{"file": "go.md", "load": "paths", "paths": ["*.go"]}`, `load "paths" needs a description`},
		"paths skill clash":     {`{"file": "tdd.md", "load": "paths", "paths": ["*.go"], "description": "d"}`, `renders as a skill named "tdd"`},
		"on-demand skill clash": {`{"file": "tdd.md", "load": "on-demand", "description": "d"}`, `renders as a skill named "tdd"`},
		"steering outside":      {`{"file": "../x.md", "load": "always"}`, "must be a file name"},
		"steering listed twice": {`{"file": "RULES.md", "load": "always"}, {"file": "RULES.md", "load": "always"}`, "listed twice"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m := steeringPack(c.entry)
			m["packs/developer/tdd.md"] = "x"
			loadErr(t, withFiles(m), "packs/developer/pack.json", c.want)
		})
	}
}

func TestPack_SteeringModesLoad(t *testing.T) {
	m := steeringPack(`{"file": "RULES.md", "load": "always"}, {"file": "go.md", "load": "paths", "paths": ["**/*.go"], "description": "Go"}, {"file": "deep.md", "load": "on-demand", "description": "Deep dive"}`)
	m["packs/developer/go.md"], m["packs/developer/deep.md"] = "go", "deep"
	c, err := catalog.Load(withFiles(m), nil)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Pack("developer")
	if len(p.Steering) != 3 || p.Steering[1].Paths[0] != "**/*.go" || p.Steering[2].Description != "Deep dive" || p.Steering[2].Body != "deep" {
		t.Fatalf("%+v", p.Steering)
	}
}

func TestPack_RequiresRules(t *testing.T) {
	cap2 := func(name, req string) map[string]string {
		return map[string]string{"packs/" + name + "/pack.json": `{"name": "` + name + `", "description": "d", "requires": ` + req + `}`}
	}
	loadErr(t, withFiles(cap2("ui", `["ghost"]`)), "packs/ui/pack.json", `requires "ghost"`)
	loadErr(t, withFiles(cap2("ui", `["developer"]`)), "packs/ui/pack.json", "role pack")
	cyc := cap2("a", `["b"]`)
	cyc["packs/b/pack.json"] = `{"name": "b", "description": "d", "requires": ["a"]}`
	loadErr(t, withFiles(cyc), "requires cycle", "a -> b -> a")
	loadErr(t, withFiles(cap2("a", `["a"]`)), "requires cycle")
	ok := cap2("a", `["b"]`)
	ok["packs/b/pack.json"] = `{"name": "b", "description": "d"}`
	if _, err := catalog.Load(withFiles(ok), nil); err != nil {
		t.Fatal(err)
	}
}

const localSkill = "---\nname: lint\ndescription: Lints\n---\nbody\n"

func TestPack_LocalSkillsLoad(t *testing.T) {
	m := replaceDev(`"skills": ["tdd"]`, `"skills": ["tdd", "lint"]`)
	m["packs/developer/skills/lint/SKILL.md"] = localSkill
	c, err := catalog.Load(withFiles(m), nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.Skill("lint")
	if err != nil || s.Pack != "developer" {
		t.Fatalf("%+v %v", s, err)
	}
	if !reflect.DeepEqual(c.LocalSkills("developer"), []string{"lint"}) {
		t.Fatal(c.LocalSkills("developer"))
	}
}

func TestPack_LocalSkillClashes(t *testing.T) {
	pool := withFiles(map[string]string{"packs/developer/skills/tdd/SKILL.md": "---\nname: tdd\ndescription: d\n---\n"})
	loadErr(t, pool, "packs/developer/skills/tdd", "pool skill")
	other := withCap(map[string]string{
		"packs/developer/skills/lint/SKILL.md": localSkill,
		"packs/ui/skills/lint/SKILL.md":        localSkill,
	})
	loadErr(t, other, "local skill", `"lint"`, "local skill of pack")
}

func TestPack_LocalSkillListedByOtherPackFails(t *testing.T) {
	m := map[string]string{"packs/ui/skills/lint/SKILL.md": localSkill}
	for k, v := range replaceDev(`"skills": ["tdd"]`, `"skills": ["tdd", "lint"]`) {
		m[k] = v
	}
	loadErr(t, withCap(m), "packs/developer/pack.json", "local to pack \"ui\"")
}

func TestPack_OnDemandNameClashesWithLocalSkill(t *testing.T) {
	m := map[string]string{
		"packs/ui/pack.json":            `{"name": "ui", "description": "d", "steering": [{"file": "lint.md", "load": "on-demand", "description": "d"}]}`,
		"packs/ui/lint.md":              "x",
		"packs/ui/skills/lint/SKILL.md": localSkill,
	}
	loadErr(t, withFiles(m), "packs/ui/pack.json", `"lint"`)
}

func TestProfile_Rules(t *testing.T) {
	good := `{"name": "dev", "packs": ["developer"], "workflow": "team"}`
	c, err := catalog.Load(withFiles(map[string]string{"profiles/dev.json": good}), nil)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := c.Profile("dev")
	if err != nil || pr.Source != "catalog" || pr.Workflow != "team" || !reflect.DeepEqual(pr.Packs, []string{"developer"}) {
		t.Fatalf("%+v %v", pr, err)
	}
	cases := map[string]struct{ body, want string }{
		"unknown pack":     {`{"name": "dev", "packs": ["ghost"]}`, `pack "ghost"`},
		"unknown workflow": {`{"name": "dev", "packs": ["developer"], "workflow": "ghost"}`, `workflow "ghost"`},
		"empty workflow":   {`{"name": "dev", "packs": ["developer"], "workflow": ""}`, "not kebab-case"},
		"empty packs":      {`{"name": "dev", "packs": []}`, "packs must not be empty"},
		"name mismatch":    {`{"name": "other", "packs": ["developer"]}`, "does not match file name"},
		"unknown key":      {`{"name": "dev", "packs": ["developer"], "x": 1}`, "unknown field"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			loadErr(t, withFiles(map[string]string{"profiles/dev.json": c.body}), "profiles/dev.json", c.want)
		})
	}
}

func TestUser_PackAndProfileRoots(t *testing.T) {
	userPacks := fstest.MapFS{
		"mine/pack.json": {Data: []byte(`{"name": "mine", "description": "d", "requires": ["ui"]}`)},
	}
	userProfiles := fstest.MapFS{
		"dev.json":  {Data: []byte(`{"name": "dev", "packs": ["mine", "developer"]}`)},
		"mine.json": {Data: []byte(`{"name": "mine", "packs": ["mine"]}`)},
	}
	shipped := withCap(map[string]string{"profiles/dev.json": `{"name": "dev", "packs": ["developer"]}`})
	c, err := catalog.Load(shipped, nil, catalog.WithUser(catalog.UserRoots{Packs: userPacks, PacksDir: "up", Profiles: userProfiles, ProfilesDir: "upr"}))
	if err != nil {
		t.Fatal(err)
	}
	dev, _ := c.Profile("dev")
	mine, _ := c.Profile("mine")
	if dev.Source != "user" || len(dev.Packs) != 2 || mine.Source != "user" {
		t.Fatalf("%+v %+v", dev, mine)
	}
	if _, err := c.Pack("mine"); err != nil {
		t.Fatal(err)
	}
	// Without the user profile the shipped one shows, with its source.
	c, err = catalog.Load(shipped, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dev, _ := c.Profile("dev"); dev.Source != "catalog" || len(dev.Packs) != 1 {
		t.Fatalf("%+v", dev)
	}
}

func TestUser_PackWithCatalogNameFails(t *testing.T) {
	userPacks := fstest.MapFS{"developer/pack.json": {Data: []byte(`{"name": "developer", "description": "d"}`)}}
	_, err := catalog.Load(base(), nil, catalog.WithUser(catalog.UserRoots{Packs: userPacks, PacksDir: "up"}))
	if err == nil || !strings.Contains(err.Error(), filepath.Join("up", "developer", "pack.json")) || !strings.Contains(err.Error(), "catalog pack") {
		t.Fatalf("got %v", err)
	}
}

func TestUser_ProfileNamingUnknownPackFails(t *testing.T) {
	up := fstest.MapFS{"x.json": {Data: []byte(`{"name": "x", "packs": ["ghost"]}`)}}
	_, err := catalog.Load(base(), nil, catalog.WithUser(catalog.UserRoots{Profiles: up, ProfilesDir: "upr"}))
	if err == nil || !strings.Contains(err.Error(), filepath.Join("upr", "x.json")) {
		t.Fatalf("got %v", err)
	}
}

func TestValidatePackDir(t *testing.T) {
	c, err := catalog.Load(withCap(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := fstest.MapFS{
		"pack.json":            {Data: []byte(`{"name": "mine", "description": "d", "requires": ["ui"], "steering": [{"file": "S.md", "load": "always"}]}`)},
		"S.md":                 {Data: []byte("s")},
		"README.md":            {Data: []byte("r")},
		"skills/lint/SKILL.md": {Data: []byte(localSkill)},
		".DS_Store":            {Data: []byte("x")},
	}
	p, files, err := c.ValidatePackDir(dir, "mine")
	if err != nil || p.Name != "mine" {
		t.Fatalf("%+v %v", p, err)
	}
	var got []string
	for _, f := range files {
		got = append(got, f.Path)
	}
	if want := []string{"README.md", "S.md", "pack.json", "skills/lint/SKILL.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files %v", got)
	}
	if _, err := c.Pack("mine"); err == nil {
		t.Fatal("validation must not add the pack to the catalog")
	}
	// A catalog name, a bad reference and a clashing local skill all fail.
	if _, _, err := c.ValidatePackDir(fstest.MapFS{"pack.json": {Data: []byte(`{"name": "ui", "description": "d"}`)}}, "ui"); err == nil || !strings.Contains(err.Error(), "catalog pack") {
		t.Fatalf("got %v", err)
	}
	bad := fstest.MapFS{"pack.json": {Data: []byte(`{"name": "mine", "description": "d", "requires": ["ghost"]}`)}}
	if _, _, err := c.ValidatePackDir(bad, "mine"); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("got %v", err)
	}
	clash := fstest.MapFS{
		"pack.json":           {Data: []byte(`{"name": "mine", "description": "d"}`)},
		"skills/tdd/SKILL.md": {Data: []byte("---\nname: tdd\ndescription: d\n---\n")},
	}
	if _, _, err := c.ValidatePackDir(clash, "mine"); err == nil || !strings.Contains(err.Error(), "pool skill") {
		t.Fatalf("got %v", err)
	}
}

func TestOverlay_RolesFolderIsAnError(t *testing.T) {
	ov := fstest.MapFS{"roles/x/role.json": {Data: []byte("{}")}}
	_, err := catalog.Load(base(), &catalog.Overlay{FS: ov, Dir: "ov"})
	if err == nil || !strings.Contains(err.Error(), "packs/") || !strings.Contains(err.Error(), filepath.Join("ov", "roles")) {
		t.Fatalf("got %v", err)
	}
}

func TestShipped_ProfilesValidate(t *testing.T) {
	c, err := catalog.Load(crew.Content(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"developer", "tester"} {
		pr, err := c.Profile(n)
		if err != nil || pr.Workflow != "solo" || !reflect.DeepEqual(pr.Packs, []string{n}) || pr.Source != "catalog" {
			t.Fatalf("%s: %+v %v", n, pr, err)
		}
		p, _ := c.Pack(n)
		if len(p.Steering) != 1 || p.Steering[0].File != "RULES.md" || p.Steering[0].Load != "always" || p.Steering[0].Body == "" {
			t.Fatalf("%s steering: %+v", n, p.Steering)
		}
	}
}

func schemaOf(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := fs.ReadFile(crew.Content(), "schema/"+name)
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSchema_MatchesPackAndProfileStructs(t *testing.T) {
	pack := schemaOf(t, "pack.schema.json")
	checkObject(t, "pack", pack, reflect.TypeOf(catalog.PackFileForTest{}))
	item := pack["properties"].(map[string]any)["steering"].(map[string]any)["items"].(map[string]any)
	checkObject(t, "steering", item, reflect.TypeOf(catalog.SteeringFileForTest{}))
	checkObject(t, "profile", schemaOf(t, "profile.schema.json"), reflect.TypeOf(catalog.ProfileFileForTest{}))
	if _, err := fs.Stat(crew.Content(), "schema/role.schema.json"); err == nil {
		t.Fatal("schema/role.schema.json must be removed")
	}
}

func checkObject(t *testing.T, where string, schema map[string]any, typ reflect.Type) {
	t.Helper()
	if schema["additionalProperties"] != false {
		t.Errorf("%s: additionalProperties must be false", where)
	}
	props, _ := schema["properties"].(map[string]any)
	var want, got []string
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
	for k := range props {
		got = append(got, k)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !reflect.DeepEqual(want, got) {
		t.Errorf("%s: struct %v, properties %v", where, want, got)
	}
	for _, r := range schema["required"].([]any) {
		if _, ok := props[r.(string)]; !ok {
			t.Errorf("%s: required %v is not a property", where, r)
		}
	}
}

func TestPack_SteeringSkillNamesUniqueAcrossPacks(t *testing.T) {
	two := func(name string) map[string]string {
		return map[string]string{
			"packs/" + name + "/pack.json": `{"name": "` + name + `", "description": "d", "steering": [{"file": "style.md", "load": "on-demand", "description": "d"}]}`,
			"packs/" + name + "/style.md":  "x",
		}
	}
	m := two("ui")
	for k, v := range two("api") {
		m[k] = v
	}
	loadErr(t, withFiles(m), `renders as a skill named "style"`, "also renders")
}

func steeringOnlyPack(name, file, load string) map[string]string {
	extra := ""
	if load == "paths" {
		extra = `, "paths": ["*.go"]`
	}
	return map[string]string{
		"packs/" + name + "/pack.json": `{"name": "` + name + `", "description": "d", "steering": [{"file": "` + file + `", "load": "` + load + `"` + extra + `, "description": "d"}]}`,
		"packs/" + name + "/" + file:   "x",
	}
}

func TestPack_SteeringNameClashesWithOtherPacksSkills(t *testing.T) {
	// developer lists pool skill tdd.
	loadErr(t, withFiles(steeringOnlyPack("ui", "tdd.md", "on-demand")), "packs/ui/pack.json", `skill named "tdd"`, `"developer"`)
	// developer holds local skill lint.
	m := replaceDev(`"skills": ["tdd"]`, `"skills": ["tdd", "lint"]`)
	m["packs/developer/skills/lint/SKILL.md"] = localSkill
	for k, v := range steeringOnlyPack("ui", "lint.md", "paths") {
		m[k] = v
	}
	loadErr(t, withFiles(m), "packs/ui/pack.json", `skill named "lint"`, `"developer"`)
}

func TestValidatePackDir_SteeringNameClashesWithSkills(t *testing.T) {
	c, err := catalog.Load(base(), nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := fstest.MapFS{
		"pack.json": {Data: []byte(`{"name": "mine", "description": "d", "steering": [{"file": "tdd.md", "load": "on-demand", "description": "d"}]}`)},
		"tdd.md":    {Data: []byte("x")},
	}
	if _, _, err := c.ValidatePackDir(dir, "mine"); err == nil || !strings.Contains(err.Error(), `skill named "tdd"`) {
		t.Fatalf("got %v", err)
	}
}

func TestValidatePackDir_ChecksDependentsAndSteeringNames(t *testing.T) {
	c, err := catalog.Load(withFiles(map[string]string{
		"packs/ui/pack.json": `{"name": "ui", "description": "d", "steering": [{"file": "style.md", "load": "on-demand", "description": "d"}]}`,
		"packs/ui/style.md":  "x",
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	dup := fstest.MapFS{
		"pack.json": {Data: []byte(`{"name": "mine", "description": "d", "steering": [{"file": "style.md", "load": "paths", "paths": ["*.go"], "description": "d"}]}`)},
		"style.md":  {Data: []byte("x")},
	}
	if _, _, err := c.ValidatePackDir(dup, "mine"); err == nil || !strings.Contains(err.Error(), `skill named "style"`) {
		t.Fatalf("cross-pack steering clash: %v", err)
	}
}

func TestValidatePackDir_ReplacementBreakingDependentFails(t *testing.T) {
	user := fstest.MapFS{
		"a/pack.json": {Data: []byte(`{"name": "a", "description": "d"}`)},
		"b/pack.json": {Data: []byte(`{"name": "b", "description": "d", "requires": ["a"]}`)},
	}
	c, err := catalog.Load(base(), nil, catalog.WithUser(catalog.UserRoots{Packs: user, PacksDir: "up"}))
	if err != nil {
		t.Fatal(err)
	}
	role := fstest.MapFS{"pack.json": {Data: []byte(strings.Replace(packJSON, `"name": "developer"`, `"name": "a"`, 1))}, "RULES.md": {Data: []byte("x")}}
	_, _, err = c.ValidatePackDir(role, "a")
	if err == nil || !strings.Contains(err.Error(), filepath.Join("up", "b", "pack.json")) || !strings.Contains(err.Error(), "role pack") {
		t.Fatalf("got %v", err)
	}
}

func TestPack_SteeringSkillNamesUniqueInOnePack(t *testing.T) {
	m := steeringPack(`{"file": "guide.md", "load": "on-demand", "description": "d"}, {"file": "guide.txt", "load": "paths", "paths": ["*.go"], "description": "d"}`)
	m["packs/developer/guide.md"], m["packs/developer/guide.txt"] = "x", "y"
	loadErr(t, withFiles(m), "packs/developer/pack.json", `renders as a skill named "guide"`, "another steering file")
}
