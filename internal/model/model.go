// Package model holds the harness-neutral types the catalog loads.
package model

// RigPolicy lists the rig command prefixes a role may and may not run.
type RigPolicy struct {
	Allow []string
	Deny  []string
}

// ShellPolicy lists other read-only command prefixes a role may run.
type ShellPolicy struct {
	Allow []string
}

// Role is one loaded role; Rules holds the content of its rules file.
type Role struct {
	Name        string
	Description string
	Actor       string
	MinRig      string
	Rules       string
	Agents      []string
	Skills      []string
	Workflows   []string
	Rig         RigPolicy
	Shell       ShellPolicy
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
	Files       []File
}

// Workflow is an opaque rig workflow. SourcePath is the OS path when the file
// came from the overlay, empty when embedded.
type Workflow struct {
	Name       string
	Data       []byte
	SourcePath string
}
