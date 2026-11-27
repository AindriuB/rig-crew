package rigcli

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	switch os.Getenv("RIGCLI_HELPER") {
	case "echo":
		for _, a := range os.Args[1:] {
			if _, err := os.Stdout.WriteString(a + "\n"); err != nil {
				os.Exit(2)
			}
		}
		os.Exit(3)
	case "block":
		// Keep the write end open (KeepAlive) so the Read blocks until killed.
		r, w, err := os.Pipe()
		if err != nil {
			os.Exit(2)
		}
		buf := make([]byte, 1)
		_, _ = r.Read(buf) // result irrelevant: we only block here
		runtime.KeepAlive(w)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestExecRunner_RunsArgvWithoutShell(t *testing.T) {
	t.Setenv("RIGCLI_HELPER", "echo")
	out, err := ExecRunner{Timeout: 10 * time.Second}.Run(context.Background(), []string{os.Args[0], "a; echo pwned", "b c"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out.Stdout), "a; echo pwned\nb c\n"; got != want {
		t.Fatalf("stdout %q want %q", got, want)
	}
	if out.ExitCode != 3 {
		t.Fatalf("exit %d", out.ExitCode)
	}
}

func TestExecRunner_TimeoutKillsChild(t *testing.T) {
	t.Setenv("RIGCLI_HELPER", "block")
	_, err := ExecRunner{Timeout: 200 * time.Millisecond}.Run(context.Background(), []string{os.Args[0]})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
}

func TestExecRunner_MissingBinaryIsErrRigNotFound(t *testing.T) {
	_, err := ExecRunner{}.Run(context.Background(), []string{"rig-does-not-exist-xyz"})
	if !errors.Is(err, ErrRigNotFound) {
		t.Fatalf("err %v", err)
	}
}

func versionFake(out string) Client {
	return Client{Runner: NewFake().On([]string{"rig", "version"}, Output{Stdout: []byte(out)}, nil)}
}

func TestVersion_Parses(t *testing.T) {
	for in, want := range map[string]string{
		"0.5.3": "0.5.3", "v0.5.3": "0.5.3", "0.5.3\n": "0.5.3", "0.6.0-rc.1": "0.6.0-rc.1", "v1.2.3-beta\n": "1.2.3-beta",
	} {
		v, err := versionFake(in).Version(context.Background())
		if err != nil || v.String() != want {
			t.Errorf("%q: %v %v", in, v, err)
		}
	}
}

func TestVersion_UnparseableFails(t *testing.T) {
	for _, in := range []string{"", "rig 0.5.3", "0.5", "x.y.z", "0.5.3 extra", "1.2.3.4"} {
		if _, err := versionFake(in).Version(context.Background()); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
	_, err := versionFake("garbage").Version(context.Background())
	if err == nil || !contains(err.Error(), `"garbage"`) {
		t.Errorf("error does not quote output: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRequireMin_TooOld(t *testing.T) {
	err := versionFake("0.5.3\n").RequireMin(context.Background(), "0.6.0")
	var tooOld *TooOldError
	if !errors.As(err, &tooOld) || tooOld.Have != "0.5.3" || tooOld.Need != "0.6.0" {
		t.Fatalf("err %v", err)
	}
}

func TestRequireMin_EqualPasses(t *testing.T) {
	if err := versionFake("0.5.3").RequireMin(context.Background(), "0.5.3"); err != nil {
		t.Fatal(err)
	}
	if err := versionFake("0.10.0").RequireMin(context.Background(), "0.9.9"); err != nil {
		t.Fatal(err)
	}
}

func TestRequireMin_PrereleaseBelowRelease(t *testing.T) {
	var tooOld *TooOldError
	if err := versionFake("0.6.0-rc.1").RequireMin(context.Background(), "0.6.0"); !errors.As(err, &tooOld) {
		t.Fatalf("err %v", err)
	}
	if err := versionFake("0.6.0").RequireMin(context.Background(), "0.6.0-rc.1"); err != nil {
		t.Fatal(err)
	}
}

func validateClient(out Output) (Client, *Fake) {
	f := NewFake().On([]string{"rig", "workflow", "validate", "wf.json", "--json"}, out, nil)
	return Client{Runner: f}, f
}

func assertArgv(t *testing.T, f *Fake) {
	t.Helper()
	want := []string{"rig", "workflow", "validate", "wf.json", "--json"}
	if len(f.Calls) != 1 || len(f.Calls[0]) != len(want) {
		t.Fatalf("calls %v", f.Calls)
	}
	for i := range want {
		if f.Calls[0][i] != want[i] {
			t.Fatalf("calls %v", f.Calls)
		}
	}
}

func TestValidateWorkflow_Valid(t *testing.T) {
	c, f := validateClient(Output{Stdout: []byte(`{"valid":true,"problems":[],"unknownStates":["x"]}`)})
	v, err := c.ValidateWorkflow(context.Background(), "wf.json")
	if err != nil || !v.Valid || len(v.Problems) != 0 {
		t.Fatalf("%v %v", v, err)
	}
	assertArgv(t, f)
}

func TestValidateWorkflow_InvalidListsProblems(t *testing.T) {
	c, f := validateClient(Output{ExitCode: 1, Stdout: []byte(`{"valid":false,"problems":[{"path":"a.b","message":"bad"}]}`)})
	v, err := c.ValidateWorkflow(context.Background(), "wf.json")
	if err != nil || v.Valid || len(v.Problems) != 1 || v.Problems[0] != (Problem{"a.b", "bad"}) {
		t.Fatalf("%v %v", v, err)
	}
	assertArgv(t, f)
}

func TestValidateWorkflow_BadOutputFails(t *testing.T) {
	for _, out := range []Output{
		{Stdout: []byte("not json"), Stderr: []byte("boom")},
		{ExitCode: 2, Stdout: []byte(`{"valid":false}`), Stderr: []byte("boom")},
	} {
		c, f := validateClient(out)
		_, err := c.ValidateWorkflow(context.Background(), "wf.json")
		if err == nil || !contains(err.Error(), "boom") {
			t.Fatalf("err %v", err)
		}
		assertArgv(t, f)
	}
}

func TestFake_UnexpectedArgvFails(t *testing.T) {
	f := NewFake()
	if _, err := f.Run(context.Background(), []string{"rig", "nope"}); err == nil {
		t.Fatal("expected error")
	}
	if len(f.Calls) != 1 {
		t.Fatal("call not recorded")
	}
}

func TestVersion_UnparseableWrapsCause(t *testing.T) {
	_, err := versionFake("garbage").Version(context.Background())
	_, want := ParseVersion("garbage")
	if err == nil || want == nil || !errors.Is(err, errors.Unwrap(err)) || errors.Unwrap(err).Error() != want.Error() {
		t.Fatalf("err=%v unwrap=%v want=%v", err, errors.Unwrap(err), want)
	}
}
