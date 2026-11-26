// Package plan declares what an adapter renders and the install engine
// applies. It holds types only; nothing in it touches the disk.
package plan

// Scope is where an install lands: the user's home or one project.
type Scope struct {
	Global bool
	Base   string // absolute: home, or the project directory
	Home   string // absolute
}

// Name is "global" or "project".
func (s Scope) Name() string {
	if s.Global {
		return "global"
	}
	return "project"
}

// Plan is everything one role needs written for one harness in one scope.
type Plan struct {
	Harness  string
	Role     string
	Scope    Scope
	Root     string // absolute harness directory in the scope
	Files    []File
	Merges   []JSONMerge
	Workflow *WorkflowFile
	Exports  []EnvVar
	Reports  []Report
}

// File is an owned file, written whole.
type File struct {
	Path string // absolute, under Root
	Data []byte
}

// JSONMerge adds entries to a shared JSON settings file.
type JSONMerge struct {
	Path   string  // absolute, under Root
	Append []Entry // array members
	Set    []Entry // object string values
}

// Entry addresses one value in a settings file.
type Entry struct {
	Key   []string // for example ["permissions","allow"] or ["env","RIG_ACTOR"]
	Value string
}

// WorkflowFile is the workflow copied outside the scope and shared by scopes.
type WorkflowFile struct {
	Name string
	Path string // absolute
	Data []byte
}

// EnvVar is an export line for a harness with no environment setting.
type EnvVar struct {
	Name  string
	Value string
}

// Report is a harness limitation to show the user.
type Report struct {
	Code    string
	Message string
}
