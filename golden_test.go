package crew

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AindriuB/rig-crew/internal/catalog"
	"github.com/AindriuB/rig-crew/internal/harness"
	"github.com/AindriuB/rig-crew/internal/harness/claude"
	"github.com/AindriuB/rig-crew/internal/plan"
)

var update = flag.Bool("update", false, "rewrite golden files")

const (
	goldenHome = "/home/u"
	goldenBase = "/work/proj"
)

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
	for _, e := range p.Exports {
		fmt.Fprintf(&b, "\n== export %s=%s\n", e.Name, e.Value)
	}
	for _, r := range p.Reports {
		fmt.Fprintf(&b, "\n== report %s: %s\n", r.Code, r.Message)
	}
	return b.String()
}

func TestGolden_EveryRoleEveryHarness(t *testing.T) {
	c, err := catalog.Load(Content(), nil)
	if err != nil {
		t.Fatal(err)
	}
	reg := harness.NewRegistry(claude.New())
	for _, hn := range reg.Names() {
		h, err := reg.Lookup(hn)
		if err != nil {
			t.Fatal(err)
		}
		for _, role := range c.RolePacks() {
			for _, wn := range role.Workflows {
				wf, err := c.Workflow(wn)
				if err != nil {
					t.Fatal(err)
				}
				for _, global := range []bool{false, true} {
					s := plan.Scope{Global: global, Base: goldenBase, Home: goldenHome}
					if global {
						s.Base = goldenHome
					}
					p, err := h.Render(c, harness.Request{Pack: role, Workflow: wf, Scope: s})
					if err != nil {
						t.Fatalf("%s %s %s %s: %v", hn, role.Name, wn, s.Name(), err)
					}
					got := dump(p)
					path := filepath.Join("testdata", "golden", hn, role.Name, wn+"-"+s.Name()+".txt")
					if *update {
						if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
							t.Fatal(err)
						}
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
						t.Errorf("%s differs; run go test . -update and review the diff", path)
					}
				}
			}
		}
	}
}
