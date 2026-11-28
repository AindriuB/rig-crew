package install

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/AindriuB/rig-crew/internal/plan"
)

// CreateFile writes a user-owned file only when path does not exist, and
// records its path, without a hash, under the reserved "user" owner of the v2
// manifest in stateDir. A path that exists and is not recorded is a conflict;
// one already recorded is left as it is.
func CreateFile(ctx context.Context, stateDir, path string, data []byte, opts Options) (Result, error) {
	if err := checkPath(stateDir); err != nil {
		return Result{}, fmt.Errorf("state dir: %w", err)
	}
	if err := checkPath(path); err != nil {
		return Result{}, err
	}
	if rel, err := filepath.Rel(stateDir, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Result{}, fmt.Errorf("%s: %w", path, ErrOutsideRoot)
	}
	mp := v2ManifestPath(stateDir)
	j := job{state: stateDir, targets: []target{
		{path: mp, base: stateDir},
		{path: path, base: filepath.Dir(path), skipBase: true},
	}}
	j.compute = func(snap snapshot) (*outcome, error) {
		out := &outcome{}
		c := &collector{snap: snap, out: out}
		man, err := decodeManifest(c.get(mp), true)
		if err != nil {
			return nil, conflictsErr([]Conflict{{mp, err.Error()}})
		}
		rec := man.Roles[userOwner]
		if rec == nil {
			rec = &roleRec{}
			man.Roles[userOwner] = rec
		}
		cur := c.get(path)
		recorded := slices.Contains(rec.UserFiles, path)
		switch {
		case !cur.exists:
			c.act(path, KindCreate)
			out.ops = append(out.ops, op{path: path, data: data})
		case recorded:
			c.act(path, KindUnchanged)
		default:
			c.conflict(path, "exists and is not a file rig-crew created")
		}
		if len(c.conflicts) > 0 {
			return nil, conflictsErr(c.conflicts)
		}
		if !recorded {
			rec.UserFiles = append(rec.UserFiles, path)
			sort.Strings(rec.UserFiles)
		}
		if nb := encodeManifest(man); !snap[mp].exists || !bytesEq(nb, snap[mp].data) {
			out.ops = append(out.ops, op{path: mp, data: nb})
		}
		return out, nil
	}
	return run(ctx, j, opts)
}

// V1Role is one role in a v1 manifest.
type V1Role struct {
	Role     string
	Workflow string // the workflow's name, without its .json suffix; "" when none
}

// ReadV1 lists the roles in the v1 manifest at <root>/.rig-crew/manifest.json,
// sorted by name. No manifest gives an empty list.
func ReadV1(root string) ([]V1Role, error) {
	if err := checkPath(root); err != nil {
		return nil, fmt.Errorf("root: %w", err)
	}
	man, err := readManifestAt(manifestPath(root), root, false)
	if err != nil {
		return nil, err
	}
	out := []V1Role{}
	for _, name := range sortedKeys(man.Roles) {
		r := V1Role{Role: name}
		if wf := man.Roles[name].Workflow; wf != "" {
			r.Workflow = strings.TrimSuffix(filepath.Base(wf), ".json")
		}
		out = append(out, r)
	}
	return out, nil
}

// PackState is one installed record in a v2 manifest.
type PackState struct {
	Owner   string
	Harness string
	Scope   string // "global" or "project"
	Project string // the project directory, for Scope "project"
}

// ActivationState is one recorded activation.
type ActivationState struct {
	Owner   string
	Scope   string
	Project string
	plan.Activation
}

// State is what the v2 manifests record.
type State struct {
	Packs       []PackState
	Activations []ActivationState
	Stale       []string // listed projects whose manifest is gone
}

// ReadState reads the global manifest under home, the manifest of project when
// given, and those of every project the global one lists. A listed project
// with no manifest is returned in Stale, not as an error.
func ReadState(home, project string) (State, error) {
	st := State{Packs: []PackState{}, Activations: []ActivationState{}, Stale: []string{}}
	if err := checkPath(home); err != nil {
		return st, fmt.Errorf("home: %w", err)
	}
	if project != "" {
		if err := checkPath(project); err != nil {
			return st, fmt.Errorf("project: %w", err)
		}
	}
	gdir := filepath.Join(home, stateDir)
	g, err := readManifestAt(v2ManifestPath(gdir), gdir, true)
	if err != nil {
		return st, err
	}
	collect := func(m *manifest, scope, proj string) {
		for _, name := range sortedKeys(m.Roles) {
			r := m.Roles[name]
			if name == userOwner {
				continue
			}
			if len(r.Files) > 0 || len(r.Merges) > 0 || r.Workflow != "" {
				st.Packs = append(st.Packs, PackState{Owner: name, Harness: r.Harness, Scope: scope, Project: proj})
			}
			if r.Activation != nil {
				st.Activations = append(st.Activations, ActivationState{Owner: name, Scope: scope, Project: proj, Activation: *r.Activation})
			}
		}
	}
	collect(g, "global", "")
	projects := slices.Clone(g.projects)
	listed := slices.Contains(projects, project)
	if project != "" && !listed {
		projects = append(projects, project)
	}
	sort.Strings(projects)
	for _, proj := range projects {
		pdir := filepath.Join(proj, stateDir)
		s, err := readSnap([]target{{path: v2ManifestPath(pdir), base: pdir}})
		if err != nil {
			return st, err
		}
		if !s[v2ManifestPath(pdir)].exists {
			if proj != project || listed {
				st.Stale = append(st.Stale, proj)
			}
			continue
		}
		m, err := readManifestAt(v2ManifestPath(pdir), pdir, true)
		if err != nil {
			return st, err
		}
		collect(m, "project", proj)
	}
	return st, nil
}
