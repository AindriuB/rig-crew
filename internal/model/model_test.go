package model

import "testing"

func TestPack_IsRoleExactlyWhenActorSet(t *testing.T) {
	if !(Pack{Actor: "developer"}).IsRole() || (Pack{}).IsRole() {
		t.Fatal("IsRole must follow Actor")
	}
}
