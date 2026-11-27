package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/AindriuB/rig-crew/internal/catalog"
	"github.com/AindriuB/rig-crew/internal/harness"
	"github.com/AindriuB/rig-crew/internal/install"
	"github.com/AindriuB/rig-crew/internal/model"
	"github.com/AindriuB/rig-crew/internal/plan"
	"github.com/AindriuB/rig-crew/internal/rigcli"
)

// deps is everything run needs from the outside, so tests can replace it.
type deps struct {
	HomeDir  func() (string, error)
	Content  fs.FS
	Registry *harness.Registry
	Rig      rigcli.Runner
}

const usage = `usage: rig-crew <command> [flags]

commands:
  version    print the version
  list       list roles, harnesses and workflows
  install    install a role for a harness
  render     show what an install would write, touching nothing
  uninstall  remove an installed role
`

func run(ctx context.Context, args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "list":
		return runList(args[1:], stdout, stderr, d)
	case "install", "render", "uninstall":
		return runRole(ctx, args[0], args[1:], stdout, stderr, d)
	}
	fmt.Fprintf(stderr, "rig-crew: unknown command %q\n%s", args[0], usage)
	return 2
}

// cmdFlags is the flag set of one command.
type cmdFlags struct {
	fs      *flag.FlagSet
	json    *bool
	from    *string
	role    *string
	harness *string
	wf      *string
	global  *bool
	project *string
	dryRun  *bool
}

func newFlags(name string, stderr io.Writer, withFrom, withRole, withWorkflow, withDry bool) *cmdFlags {
	f := &cmdFlags{fs: flag.NewFlagSet("rig-crew "+name, flag.ContinueOnError)}
	f.fs.SetOutput(stderr)
	f.json = f.fs.Bool("json", false, "print JSON")
	if withFrom {
		f.from = f.fs.String("from", "", "overlay catalog directory")
	}
	if withRole {
		f.role = f.fs.String("role", "", "role name")
		f.harness = f.fs.String("harness", "", "harness name")
		f.global = f.fs.Bool("global", false, "use the home directory")
		f.project = f.fs.String("project", "", "use this project directory")
	}
	if withWorkflow {
		f.wf = f.fs.String("workflow", "", "workflow name")
	}
	if withDry {
		f.dryRun = f.fs.Bool("dry-run", false, "list changes without writing")
	}
	return f
}

func (f *cmdFlags) parse(name string, args []string, stderr io.Writer) bool {
	if err := f.fs.Parse(args); err != nil {
		return false
	}
	if f.fs.NArg() > 0 {
		fmt.Fprintf(stderr, "rig-crew %s: unexpected argument %q\n", name, f.fs.Arg(0))
		return false
	}
	return true
}

func printJSON(stdout io.Writer, v any) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return 1
	}
	fmt.Fprintf(stdout, "%s\n", b)
	return 0
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	f := newFlags("version", stderr, false, false, false, false)
	if !f.parse("version", args, stderr) {
		return 2
	}
	if *f.json {
		return printJSON(stdout, map[string]string{"version": version()})
	}
	fmt.Fprintln(stdout, version())
	return 0
}

type roleOut struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	MinRig      string   `json:"minRig"`
	Workflows   []string `json:"workflows"`
}

type listOut struct {
	Roles     []roleOut `json:"roles"`
	Harnesses []string  `json:"harnesses"`
	Workflows []string  `json:"workflows"`
}

// loadCatalog loads the embedded content and the optional overlay directory.
func loadCatalog(d deps, from string) (*catalog.Catalog, error) {
	var ov *catalog.Overlay
	if from != "" {
		abs, err := filepath.Abs(from)
		if err != nil {
			return nil, err
		}
		ov = &catalog.Overlay{FS: os.DirFS(abs), Dir: abs}
	}
	return catalog.Load(d.Content, ov)
}

func runList(args []string, stdout, stderr io.Writer, d deps) int {
	f := newFlags("list", stderr, true, false, false, false)
	if !f.parse("list", args, stderr) {
		return 2
	}
	c, err := loadCatalog(d, *f.from)
	if err != nil {
		fmt.Fprintf(stderr, "rig-crew list: %v\n", err)
		return 1
	}
	out := listOut{Roles: []roleOut{}, Harnesses: d.Registry.Names(), Workflows: []string{}}
	for _, r := range c.Roles() {
		wfs := r.Workflows
		if wfs == nil {
			wfs = []string{}
		}
		out.Roles = append(out.Roles, roleOut{Name: r.Name, Description: r.Description, MinRig: r.MinRig, Workflows: wfs})
	}
	for _, w := range c.Workflows() {
		out.Workflows = append(out.Workflows, w.Name)
	}
	if *f.json {
		return printJSON(stdout, out)
	}
	fmt.Fprintln(stdout, "roles:")
	for _, r := range out.Roles {
		fmt.Fprintf(stdout, "  %s  %s\n", r.Name, r.Description)
	}
	fmt.Fprintf(stdout, "harnesses: %s\n", strings.Join(out.Harnesses, ", "))
	fmt.Fprintf(stdout, "workflows: %s\n", strings.Join(out.Workflows, ", "))
	return 0
}

type installOut struct {
	Actions []install.Action `json:"actions"`
	Changed bool             `json:"changed"`
	Reports []reportOut      `json:"reports"`
	Exports []exportOut      `json:"exports"`
}

type reportOut struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type exportOut struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type entryOut struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type mergeOut struct {
	Path   string     `json:"path"`
	Append []entryOut `json:"append"`
	Set    []entryOut `json:"set"`
}

type renderOut struct {
	Files    []string    `json:"files"`
	Merges   []mergeOut  `json:"merges"`
	Workflow string      `json:"workflow"`
	Reports  []reportOut `json:"reports"`
}

func reportsOf(p plan.Plan) []reportOut {
	out := []reportOut{}
	for _, r := range p.Reports {
		out = append(out, reportOut{Code: r.Code, Message: r.Message})
	}
	return out
}

func entriesOf(es []plan.Entry) []entryOut {
	out := []entryOut{}
	for _, e := range es {
		out = append(out, entryOut{Key: strings.Join(e.Key, "."), Value: e.Value})
	}
	return out
}

// scopeOf resolves --global / --project into a scope; ok is false on a usage error.
func scopeOf(name string, f *cmdFlags, d deps, stderr io.Writer) (plan.Scope, bool) {
	if *f.global == (*f.project != "") {
		fmt.Fprintf(stderr, "rig-crew %s: give exactly one of --global and --project DIR\n", name)
		return plan.Scope{}, false
	}
	home, err := d.HomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "rig-crew %s: cannot find the home directory: %v\n", name, err)
		return plan.Scope{}, false
	}
	if *f.global {
		return plan.Scope{Global: true, Base: home, Home: home}, true
	}
	abs, err := filepath.Abs(*f.project)
	if err != nil {
		fmt.Fprintf(stderr, "rig-crew %s: --project: %v\n", name, err)
		return plan.Scope{}, false
	}
	fi, err := os.Stat(abs)
	if err != nil {
		fmt.Fprintf(stderr, "rig-crew %s: --project %s is not an existing directory: %v\n", name, *f.project, err)
		return plan.Scope{}, false
	}
	if !fi.IsDir() {
		fmt.Fprintf(stderr, "rig-crew %s: --project %s is not an existing directory\n", name, *f.project)
		return plan.Scope{}, false
	}
	return plan.Scope{Base: abs, Home: home}, true
}

func runRole(ctx context.Context, name string, args []string, stdout, stderr io.Writer, d deps) int {
	f := newFlags(name, stderr, name != "uninstall", true, name != "uninstall", name != "render")
	if !f.parse(name, args, stderr) {
		return 2
	}
	for _, req := range []struct{ flag, val string }{{"role", *f.role}, {"harness", *f.harness}} {
		if req.val == "" {
			fmt.Fprintf(stderr, "rig-crew %s: --%s is required\n", name, req.flag)
			return 2
		}
	}
	if f.wf != nil && *f.wf == "" {
		fmt.Fprintf(stderr, "rig-crew %s: --workflow is required\n", name)
		return 2
	}
	scope, ok := scopeOf(name, f, d, stderr)
	if !ok {
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "rig-crew %s: %v\n", name, err)
		var se *install.SymlinkError
		if errors.As(err, &se) && scope.Global {
			fmt.Fprintln(stderr, "rig-crew: a harness directory in your home is a symlink; try --project <dir> instead")
		}
		return 1
	}
	h, err := d.Registry.Lookup(*f.harness)
	if err != nil {
		return fail(err)
	}
	if name == "uninstall" {
		res, err := install.Uninstall(ctx, h.Root(scope), *f.role, install.Options{DryRun: *f.dryRun})
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *f.json, res, plan.Plan{})
	}

	c, err := loadCatalog(d, *f.from)
	if err != nil {
		return fail(err)
	}
	role, err := c.Role(*f.role)
	if err != nil {
		return fail(err)
	}
	wf, err := c.Workflow(*f.wf)
	if err != nil {
		return fail(err)
	}
	if !contains(role.Workflows, wf.Name) {
		return fail(fmt.Errorf("workflow %q is not one of role %q's workflows (%s)", wf.Name, role.Name, strings.Join(role.Workflows, ", ")))
	}
	if name == "install" {
		if code := preflight(ctx, d, role, wf, stderr, fail); code != 0 {
			return code
		}
	}
	p, err := h.Render(c, harness.Request{Role: role, Workflow: wf, Scope: scope, ScopeFS: os.DirFS(scope.Base)})
	if err != nil {
		return fail(err)
	}
	if name == "render" {
		return printRender(stdout, stderr, *f.json, p)
	}
	res, err := install.Apply(ctx, p, install.Options{DryRun: *f.dryRun})
	if err != nil {
		return fail(err)
	}
	return printResult(stdout, stderr, *f.json, res, p)
}

// preflight checks the installed rig and, for an overlay workflow, validates it.
func preflight(ctx context.Context, d deps, role model.Role, wf model.Workflow, stderr io.Writer, fail func(error) int) int {
	client := rigcli.Client{Runner: d.Rig}
	if err := client.RequireMin(ctx, role.MinRig); err != nil {
		return fail(err)
	}
	if wf.SourcePath == "" {
		return 0
	}
	v, err := client.ValidateWorkflow(ctx, wf.SourcePath)
	if err != nil {
		return fail(err)
	}
	if !v.Valid {
		fmt.Fprintf(stderr, "rig-crew: workflow %s is not valid:\n", wf.SourcePath)
		for _, p := range v.Problems {
			fmt.Fprintf(stderr, "  %s: %s\n", p.Path, p.Message)
		}
		return 1
	}
	return 0
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func printResult(stdout, stderr io.Writer, asJSON bool, res install.Result, p plan.Plan) int {
	out := installOut{Actions: res.Actions, Changed: res.Changed, Reports: reportsOf(p), Exports: []exportOut{}}
	if out.Actions == nil {
		out.Actions = []install.Action{}
	}
	for _, e := range p.Exports {
		out.Exports = append(out.Exports, exportOut{Name: e.Name, Value: e.Value})
	}
	if asJSON {
		return printJSON(stdout, out)
	}
	for _, a := range res.Actions {
		if a.Kind != install.KindUnchanged {
			fmt.Fprintf(stdout, "%s %s\n", a.Kind, a.Path)
		}
	}
	if !res.Changed {
		fmt.Fprintln(stdout, "nothing to change")
	}
	for _, e := range out.Exports {
		fmt.Fprintf(stdout, "export %s=%s\n", e.Name, e.Value)
	}
	for _, r := range out.Reports {
		fmt.Fprintf(stderr, "report %s: %s\n", r.Code, r.Message)
	}
	return 0
}

func printRender(stdout, stderr io.Writer, asJSON bool, p plan.Plan) int {
	out := renderOut{Files: []string{}, Merges: []mergeOut{}, Reports: reportsOf(p)}
	for _, f := range p.Files {
		out.Files = append(out.Files, f.Path)
	}
	for _, m := range p.Merges {
		out.Merges = append(out.Merges, mergeOut{Path: m.Path, Append: entriesOf(m.Append), Set: entriesOf(m.Set)})
	}
	if p.Workflow != nil {
		out.Workflow = p.Workflow.Path
	}
	if asJSON {
		return printJSON(stdout, out)
	}
	for _, f := range out.Files {
		fmt.Fprintf(stdout, "file %s\n", f)
	}
	for _, m := range out.Merges {
		fmt.Fprintf(stdout, "merge %s\n", m.Path)
		for _, e := range m.Append {
			fmt.Fprintf(stdout, "  append %s %s\n", e.Key, e.Value)
		}
		for _, e := range m.Set {
			fmt.Fprintf(stdout, "  set %s %s\n", e.Key, e.Value)
		}
	}
	if out.Workflow != "" {
		fmt.Fprintf(stdout, "workflow %s\n", out.Workflow)
	}
	for _, e := range p.Exports {
		fmt.Fprintf(stdout, "export %s=%s\n", e.Name, e.Value)
	}
	for _, r := range out.Reports {
		fmt.Fprintf(stderr, "report %s: %s\n", r.Code, r.Message)
	}
	return 0
}
