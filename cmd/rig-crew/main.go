// Command rig-crew installs and renders agent roles for rig.
package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	crew "github.com/AindriuB/rig-crew"
	"github.com/AindriuB/rig-crew/internal/harness"
	"github.com/AindriuB/rig-crew/internal/harness/claude"
	"github.com/AindriuB/rig-crew/internal/rigcli"
)

func main() {
	home, err := homeDir()
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

// homeDir returns the user's home as an absolute, cleaned path, which is what
// install.Apply and Uninstall require.
func homeDir() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Abs(h)
}
