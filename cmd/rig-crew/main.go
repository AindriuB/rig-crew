// Command rig-crew installs and renders agent roles for rig.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

const usage = "usage: rig-crew <command> [--json]\n\ncommands:\n  version   print the version\n  list      list roles, harnesses and workflows\n"

func run(_ context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		return runCmd("version", args[1:], stdout, stderr, func(asJSON bool) any {
			v := version()
			if asJSON {
				return map[string]string{"version": v}
			}
			return v
		})
	case "list":
		return runCmd("list", args[1:], stdout, stderr, func(asJSON bool) any {
			if asJSON {
				return listOutput{Roles: []string{}, Harnesses: []string{}, Workflows: []string{}}
			}
			return ""
		})
	}
	fmt.Fprintf(stderr, "rig-crew: unknown command %q\n%s", args[0], usage)
	return 2
}

type listOutput struct {
	Roles     []string `json:"roles"`
	Harnesses []string `json:"harnesses"`
	Workflows []string `json:"workflows"`
}

// runCmd parses the shared --json flag and prints build's result: a string
// verbatim (one line, skipped when empty) or any other value as indented JSON.
func runCmd(name string, args []string, stdout, stderr io.Writer, build func(asJSON bool) any) int {
	fs := flag.NewFlagSet("rig-crew "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "rig-crew %s: unexpected argument %q\n", name, fs.Arg(0))
		return 2
	}
	switch v := build(*asJSON).(type) {
	case string:
		if v != "" {
			fmt.Fprintln(stdout, v)
		}
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "rig-crew %s: %v\n", name, err)
			return 1
		}
		fmt.Fprintf(stdout, "%s\n", b)
	}
	return 0
}

func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
