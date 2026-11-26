// Package harness declares the port every harness adapter implements and the
// registry the caller builds from adapters. Rendering is pure: an adapter
// reads only the catalog and the request, never the disk.
package harness

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"

	"github.com/AindriuB/rig-crew/internal/catalog"
	"github.com/AindriuB/rig-crew/internal/model"
	"github.com/AindriuB/rig-crew/internal/plan"
)

// ErrUnknownHarness is matched with errors.Is when Lookup finds no adapter.
var ErrUnknownHarness = errors.New("unknown harness")

// Request is what one render is asked for.
type Request struct {
	Role     model.Role
	Workflow model.Workflow
	Scope    plan.Scope
	ScopeFS  fs.FS // read-only view rooted at Scope.Base, or nil
}

// Harness turns a request into a plan.
type Harness interface {
	Name() string
	Root(s plan.Scope) string
	Render(c *catalog.Catalog, req Request) (plan.Plan, error)
}

// Registry holds the adapters its caller gave it.
type Registry struct {
	byName map[string]Harness
}

// NewRegistry builds a registry; a later adapter with the same name replaces an earlier one.
func NewRegistry(adapters ...Harness) *Registry {
	r := &Registry{byName: map[string]Harness{}}
	for _, a := range adapters {
		r.byName[a.Name()] = a
	}
	return r
}

// Lookup returns the named adapter or an error matching ErrUnknownHarness.
func (r *Registry) Lookup(name string) (Harness, error) {
	h, ok := r.byName[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownHarness, name)
	}
	return h, nil
}

// Names lists the registered adapter names, sorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.byName))
	for n := range r.byName {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
