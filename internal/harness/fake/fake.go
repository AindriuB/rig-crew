// Package fake is an adapter that returns a configured plan or error, for tests.
package fake

import (
	"path/filepath"

	"github.com/AindriuB/rig-crew/internal/catalog"
	"github.com/AindriuB/rig-crew/internal/harness"
	"github.com/AindriuB/rig-crew/internal/plan"
)

// Harness returns Plan and Err from Render, and records the last request.
type Harness struct {
	HarnessName string
	Plan        plan.Plan
	Err         error
	Last        harness.Request
}

// Name returns HarnessName, or "fake" when it is empty.
func (h *Harness) Name() string {
	if h.HarnessName == "" {
		return "fake"
	}
	return h.HarnessName
}

// Root is <Base>/.fake.
func (h *Harness) Root(s plan.Scope) string { return filepath.Join(s.Base, ".fake") }

// Render returns the configured plan and error.
func (h *Harness) Render(_ *catalog.Catalog, req harness.Request) (plan.Plan, error) {
	h.Last = req
	return h.Plan, h.Err
}
