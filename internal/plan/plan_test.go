package plan

import "testing"

func TestScope_Name(t *testing.T) {
	if (Scope{Global: true}).Name() != "global" || (Scope{}).Name() != "project" {
		t.Fatal("scope names")
	}
}
