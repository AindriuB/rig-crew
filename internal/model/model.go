// Package model holds the harness-neutral types the catalog loads.
package model

// RigPolicy lists the rig command prefixes a pack may and may not run.
type RigPolicy struct {
	Allow []string
	Deny  []string
}

// ShellPolicy lists other read-only command prefixes a pack may run.
type ShellPolicy struct {
	Allow []string
}

// Steering is one steering file of a pack. Load is "always", "paths" or
// "on-demand"; Paths is set only for "paths" and Description is required for
// "paths" and "on-demand" (it may be set for "always"). Body is the file content.
type Steering struct {
	File        string
	Load        string
	Paths       []string
	Description string
	Body        string
}

// Pack is one loaded pack. A role pack has an Actor; a capability pack has
// none and no Workflows.
type Pack struct {
	Name        string
	Description string
	Actor       string
	MinRig      string
	Requires    []string
	Steering    []Steering
	Agents      []string
	Skills      []string
	Workflows   []string
	Rig         RigPolicy
	Shell       ShellPolicy
}

// IsRole reports whether the pack is a role pack, which is when Actor is set.
func (p Pack) IsRole() bool { return p.Actor != "" }

// Profile is a named list of packs. Source is "catalog" for a shipped or
// overlay profile and "user" for one from the user's profile root.
type Profile struct {
	Name     string
	Packs    []string
	Workflow string
	Source   string
}

// Prompt is an agent prompt.
type Prompt struct {
	Name        string
	Description string
	Body        string
}

// File is one file of a skill; Path is slash-separated, relative to the skill folder.
type File struct {
	Path string
	Data []byte
}

// Skill is a skill folder; Files are sorted by Path and include SKILL.md.
type Skill struct {
	Name        string
	Description string
	Pack        string // owning pack for a pack-local skill, empty for a pool skill
	Files       []File
}

// Workflow is an opaque rig workflow. SourcePath is the OS path when the file
// came from the overlay, empty when embedded.
type Workflow struct {
	Name       string
	Data       []byte
	SourcePath string
}
