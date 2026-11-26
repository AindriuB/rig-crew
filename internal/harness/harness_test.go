package harness_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/AindriuB/rig-crew/internal/harness"
	"github.com/AindriuB/rig-crew/internal/harness/fake"
)

func TestRegistry_UnknownHarness(t *testing.T) {
	r := harness.NewRegistry(&fake.Harness{HarnessName: "b"}, &fake.Harness{HarnessName: "a"})
	if _, err := r.Lookup("zzz"); !errors.Is(err, harness.ErrUnknownHarness) {
		t.Fatalf("err = %v", err)
	}
	if h, err := r.Lookup("a"); err != nil || h.Name() != "a" {
		t.Fatalf("lookup a: %v %v", h, err)
	}
	if got := r.Names(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("names = %v", got)
	}
}
