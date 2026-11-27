// Package claude renders a role as Claude Code agent files, skills, a rules
// file and merged settings. Rendering is pure.
package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/AindriuB/rig-crew/internal/catalog"
	"github.com/AindriuB/rig-crew/internal/frontmatter"
	"github.com/AindriuB/rig-crew/internal/harness"
	"github.com/AindriuB/rig-crew/internal/plan"
)

const pinsFile = ".claude/rig-crew-models.json"

// Harness is the Claude Code adapter.
type Harness struct{}

// New returns the Claude Code adapter.
func New() *Harness { return &Harness{} }

// Name is "claude".
func (*Harness) Name() string { return "claude" }

// Root is <Base>/.claude.
func (*Harness) Root(s plan.Scope) string { return filepath.Join(s.Base, ".claude") }

// Render builds the plan for one role in one scope.
func (h *Harness) Render(c *catalog.Catalog, req harness.Request) (plan.Plan, error) {
	root := h.Root(req.Scope)
	p := plan.Plan{Harness: h.Name(), Role: req.Role.Name, Scope: req.Scope, Root: root}

	hfs, err := c.HarnessFS("claude")
	if err != nil {
		return p, err
	}
	models, err := readModels(hfs)
	if err != nil {
		return p, err
	}
	pins, err := readPins(req.ScopeFS)
	if err != nil {
		return p, err
	}

	for _, agent := range req.Role.Agents {
		data, err := renderAgent(c, hfs, models, pins, agent)
		if err != nil {
			return p, err
		}
		p.Files = append(p.Files, plan.File{Path: filepath.Join(root, "agents", agent+".md"), Data: data})
	}
	known := map[string]bool{}
	for _, r := range c.Roles() {
		for _, a := range r.Agents {
			known[a] = true
		}
	}
	var pinned []string
	for a := range pins {
		if !known[a] {
			pinned = append(pinned, a)
		}
	}
	sort.Strings(pinned)
	for _, a := range pinned {
		p.Reports = append(p.Reports, plan.Report{
			Code:    "model-pin-unknown-agent",
			Message: fmt.Sprintf("%s pins a model for %q, which no role has as an agent", pinsFile, a),
		})
	}

	for _, name := range req.Role.Skills {
		sk, err := c.Skill(name)
		if err != nil {
			return p, err
		}
		for _, f := range sk.Files {
			p.Files = append(p.Files, plan.File{Path: filepath.Join(root, "skills", name, filepath.FromSlash(f.Path)), Data: f.Data})
		}
	}
	p.Files = append(p.Files, plan.File{
		Path: filepath.Join(root, "rules", "rig-crew-"+req.Role.Name+".md"),
		Data: []byte(req.Role.Rules),
	})
	sort.SliceStable(p.Files, func(i, j int) bool { return filepath.ToSlash(p.Files[i].Path) < filepath.ToSlash(p.Files[j].Path) })

	wfPath := filepath.Join(req.Scope.Home, ".rig", "workflows", req.Workflow.Name+".json")
	p.Workflow = &plan.WorkflowFile{Name: req.Workflow.Name, Path: wfPath, Data: req.Workflow.Data}

	settings := "settings.json"
	if !req.Scope.Global {
		settings = "settings.local.json"
	}
	m := plan.JSONMerge{Path: filepath.Join(root, settings)}
	for _, e := range req.Role.Rig.Allow {
		m.Append = append(m.Append, plan.Entry{Key: []string{"permissions", "allow"}, Value: "Bash(rig " + e + " *)"})
	}
	for _, e := range req.Role.Shell.Allow {
		m.Append = append(m.Append, plan.Entry{Key: []string{"permissions", "allow"}, Value: "Bash(" + e + " *)"})
	}
	for _, e := range req.Role.Rig.Deny {
		m.Append = append(m.Append, plan.Entry{Key: []string{"permissions", "deny"}, Value: "Bash(rig " + e + " *)"})
	}
	m.Set = []plan.Entry{
		{Key: []string{"env", "RIG_WORKFLOW"}, Value: wfPath},
		{Key: []string{"env", "RIG_ACTOR"}, Value: req.Role.Actor},
	}
	p.Merges = []plan.JSONMerge{m}
	return p, nil
}

func renderAgent(c *catalog.Catalog, hfs fs.FS, models map[string]string, pins map[string]*string, agent string) ([]byte, error) {
	rel := "agents/" + agent + ".md"
	raw, err := fs.ReadFile(hfs, rel)
	if err != nil {
		return nil, fmt.Errorf("harnesses/claude/%s: %w", rel, err)
	}
	fields, body, err := frontmatter.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("harnesses/claude/%s: %w", rel, err)
	}
	if body != "" {
		return nil, fmt.Errorf("harnesses/claude/%s: body must be empty; the prompt comes from the pool", rel)
	}
	prompt, err := c.Prompt(agent)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i, f := range fields {
		if f.Key == "model" {
			if idx >= 0 {
				return nil, fmt.Errorf("harnesses/claude/%s: model is set more than once", rel)
			}
			idx = i
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("harnesses/claude/%s: missing model (fast or strong)", rel)
	}
	hint := fields[idx].Value
	id, ok := models[hint]
	if !ok {
		return nil, fmt.Errorf("harnesses/claude/%s: model %q is not fast or strong", rel, hint)
	}
	if pin, pinned := pins[agent]; pinned {
		if pin == nil {
			fields = append(fields[:idx:idx], fields[idx+1:]...)
		} else {
			fields[idx].Value = *pin
		}
	} else {
		fields[idx].Value = id
	}
	return frontmatter.Format(fields, prompt.Body), nil
}

func readModels(hfs fs.FS) (map[string]string, error) {
	const name = "harnesses/claude/models.json"
	data, err := fs.ReadFile(hfs, "models.json")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	m, err := decodeStrict[map[string]string](data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(m) != 2 || m["fast"] == "" || m["strong"] == "" {
		return nil, fmt.Errorf(`%s: must have exactly the keys "fast" and "strong", each a model id`, name)
	}
	return m, nil
}

// readPins reads the user-owned pins; absent file or nil view means no pins.
func readPins(sfs fs.FS) (map[string]*string, error) {
	if sfs == nil {
		return nil, nil
	}
	data, err := fs.ReadFile(sfs, pinsFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pinsFile, err)
	}
	m, err := decodeStrict[map[string]*string](data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pinsFile, err)
	}
	for a, pin := range m {
		if pin != nil && *pin == "" {
			return nil, fmt.Errorf("%s: pin for %q is empty; use null for the harness default", pinsFile, a)
		}
	}
	return m, nil
}

func decodeStrict[T any](data []byte) (T, error) {
	var v T
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&v); err != nil {
		return v, err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return v, errors.New("trailing data after JSON object")
	}
	return v, nil
}
