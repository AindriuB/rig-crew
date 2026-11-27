package crew

import (
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/AindriuB/rig-crew/internal/catalog"
	"github.com/AindriuB/rig-crew/internal/frontmatter"
)

func description(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	fields, _, err := frontmatter.Parse(data)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	for _, f := range fields {
		if f.Key == "description" {
			return f.Value
		}
	}
	t.Fatalf("%s has no description", file)
	return ""
}

func TestDrift_AgentsMatchPool(t *testing.T) {
	c, err := catalog.Load(Content(), nil)
	if err != nil {
		t.Fatal(err)
	}
	harnesses, err := os.ReadDir("harnesses")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range harnesses {
		if !h.IsDir() {
			continue
		}
		dir := path.Join("harnesses", h.Name(), "agents")
		files := map[string]bool{}
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			files[name] = true
			pool := path.Join("pool", "prompts", e.Name())
			if _, err := fs.Stat(os.DirFS("."), pool); err != nil {
				t.Errorf("%s has no pool prompt %s", path.Join(dir, e.Name()), pool)
				continue
			}
			if a, b := description(t, path.Join(dir, e.Name())), description(t, pool); a != b {
				t.Errorf("%s description %q differs from %s %q", path.Join(dir, e.Name()), a, pool, b)
			}
		}
		for _, role := range c.Roles() {
			for _, a := range role.Agents {
				if !files[a] {
					t.Errorf("role %s names agent %s with no file in %s", role.Name, a, dir)
				}
			}
		}
	}
}

// wordPrefix reports whether a equals b or is a whole-word prefix of it.
func wordPrefix(a, b string) bool { return a == b || strings.HasPrefix(b, a+" ") }

func TestDrift_DenyDoesNotOverlapAllow(t *testing.T) {
	c, err := catalog.Load(Content(), nil)
	if err != nil {
		t.Fatal(err)
	}
	roles := c.Roles()
	for _, d := range roles {
		for _, a := range roles {
			if d.Name == a.Name {
				continue
			}
			for _, deny := range d.Rig.Deny {
				for _, allow := range a.Rig.Allow {
					if wordPrefix(deny, allow) || wordPrefix(allow, deny) {
						t.Errorf("rig.deny %q of role %s overlaps rig.allow %q of role %s", deny, d.Name, allow, a.Name)
					}
				}
			}
		}
	}
	if !wordPrefix("pr", "pr open") || wordPrefix("pr", "prx") || wordPrefix("pr open", "pr") {
		t.Fatal("wordPrefix is wrong")
	}
}
