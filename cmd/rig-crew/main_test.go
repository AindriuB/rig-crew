package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func runArgs(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(context.Background(), args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRun_Version(t *testing.T) {
	code, out, _ := runArgs("version")
	if code != 0 || strings.Count(out, "\n") != 1 || strings.TrimSpace(out) == "" {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestRun_VersionJSON(t *testing.T) {
	code, out, _ := runArgs("version", "--json")
	if code != 0 || !strings.HasPrefix(out, "{\n  \"version\": \"") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestRun_ListJSONEmpty(t *testing.T) {
	code, out, _ := runArgs("list", "--json")
	want := "{\n  \"roles\": [],\n  \"harnesses\": [],\n  \"workflows\": []\n}\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if code, _, _ := runArgs("list"); code != 0 {
		t.Fatalf("list code=%d", code)
	}
}

func checkUsage(t *testing.T, args ...string) {
	t.Helper()
	code, out, errs := runArgs(args...)
	if code != 2 || out != "" || errs == "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
}

func TestRun_NoArgsIsUsage(t *testing.T) { checkUsage(t) }

func TestRun_UnknownCommandIsUsage(t *testing.T) { checkUsage(t, "bogus") }

func TestRun_UnknownFlagIsUsage(t *testing.T) {
	checkUsage(t, "--bogus")
	checkUsage(t, "version", "--bogus")
	checkUsage(t, "list", "--bogus")
}
