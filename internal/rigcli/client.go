package rigcli

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a parsed semantic version with an optional pre-release.
type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

var versionRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?$`)

// ParseVersion parses 0.5.3, v0.5.3 and 0.6.0-rc.1.
func ParseVersion(s string) (Version, error) {
	m := versionRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return Version{}, fmt.Errorf("rigcli: unparseable version %q", s)
	}
	var v Version
	for i, p := range []*int{&v.Major, &v.Minor, &v.Patch} {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return Version{}, fmt.Errorf("rigcli: unparseable version %q", s)
		}
		*p = n
	}
	v.Pre = m[4]
	return v, nil
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Compare returns -1, 0 or 1. A pre-release is below its release.
func (v Version) Compare(o Version) int {
	for _, p := range [][2]int{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if p[0] != p[1] {
			if p[0] < p[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case v.Pre == o.Pre:
		return 0
	case v.Pre == "":
		return 1
	case o.Pre == "":
		return -1
	}
	return comparePre(v.Pre, o.Pre)
}

func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		var c int
		switch {
		case aerr == nil && berr == nil:
			c = cmpInt(an, bn)
		case aerr == nil:
			c = -1
		case berr == nil:
			c = 1
		default:
			c = strings.Compare(as[i], bs[i])
		}
		if c != 0 {
			return c
		}
	}
	return cmpInt(len(as), len(bs))
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// TooOldError reports an installed rig older than required.
type TooOldError struct{ Have, Need string }

func (e *TooOldError) Error() string {
	return fmt.Sprintf("rig %s is older than the required %s", e.Have, e.Need)
}

// Problem is one validation problem.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Validation is the result of rig workflow validate.
type Validation struct {
	Valid    bool      `json:"valid"`
	Problems []Problem `json:"problems"`
}

// Client runs rig commands through a Runner.
type Client struct{ Runner Runner }

// Version runs `rig version`.
func (c Client) Version(ctx context.Context) (Version, error) {
	out, err := c.Runner.Run(ctx, []string{"rig", "version"})
	if err != nil {
		return Version{}, err
	}
	if out.ExitCode != 0 {
		return Version{}, fmt.Errorf("rigcli: rig version exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	v, err := ParseVersion(string(out.Stdout))
	if err != nil {
		return Version{}, fmt.Errorf("rigcli: unparseable rig version output %q", string(out.Stdout))
	}
	return v, nil
}

// RequireMin returns *TooOldError when the installed rig is older than min.
func (c Client) RequireMin(ctx context.Context, min string) error {
	need, err := ParseVersion(min)
	if err != nil {
		return err
	}
	have, err := c.Version(ctx)
	if err != nil {
		return err
	}
	if have.Compare(need) < 0 {
		return &TooOldError{Have: have.String(), Need: need.String()}
	}
	return nil
}

// ValidateWorkflow runs `rig workflow validate <path> --json`.
func (c Client) ValidateWorkflow(ctx context.Context, path string) (Validation, error) {
	out, err := c.Runner.Run(ctx, []string{"rig", "workflow", "validate", path, "--json"})
	if err != nil {
		return Validation{}, err
	}
	stderr := strings.TrimSpace(string(out.Stderr))
	if out.ExitCode != 0 && out.ExitCode != 1 {
		return Validation{}, fmt.Errorf("rigcli: rig workflow validate %s exited %d: %s", path, out.ExitCode, stderr)
	}
	var v Validation
	if err := json.Unmarshal(out.Stdout, &v); err != nil {
		return Validation{}, fmt.Errorf("rigcli: unparseable validate output for %s (exit %d): %w: %s", path, out.ExitCode, err, stderr)
	}
	return v, nil
}
