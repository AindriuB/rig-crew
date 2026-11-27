// Command rig-crew installs and renders agent roles for rig.
package main

import (
	"context"
	"os"
	"runtime/debug"
	"time"

	crew "github.com/AindriuB/rig-crew"
	"github.com/AindriuB/rig-crew/internal/harness"
	"github.com/AindriuB/rig-crew/internal/harness/claude"
	"github.com/AindriuB/rig-crew/internal/rigcli"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		os.Stderr.WriteString("rig-crew: cannot find the home directory: " + err.Error() + "\n")
		os.Exit(2)
	}
	d := deps{
		Home:     home,
		Content:  crew.Content(),
		Registry: harness.NewRegistry(claude.New()),
		Rig:      rigcli.ExecRunner{Timeout: 30 * time.Second},
	}
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, d))
}

func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
