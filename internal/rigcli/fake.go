package rigcli

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// FakeResult is a canned reply for one argv.
type FakeResult struct {
	Output Output
	Err    error
}

// Fake is a Runner for tests. Replies are keyed by argv joined with a space.
type Fake struct {
	mu      sync.Mutex
	Results map[string]FakeResult
	Calls   [][]string
}

// NewFake returns an empty Fake.
func NewFake() *Fake { return &Fake{Results: map[string]FakeResult{}} }

// On registers the reply for an argv.
func (f *Fake) On(argv []string, out Output, err error) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Results == nil {
		f.Results = map[string]FakeResult{}
	}
	f.Results[strings.Join(argv, " ")] = FakeResult{Output: out, Err: err}
	return f
}

// Run records the call and returns the canned reply, or an error if argv is unexpected.
func (f *Fake) Run(_ context.Context, argv []string) (Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, append([]string(nil), argv...))
	res, ok := f.Results[strings.Join(argv, " ")]
	if !ok {
		return Output{}, fmt.Errorf("rigcli.Fake: unexpected argv %q", argv)
	}
	return res.Output, res.Err
}
