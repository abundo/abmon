package cmdbase

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestRunHelperProcess is not a real test. runHelper re-executes the test
// binary with this test selected, so Run's os.Exit calls can be observed
// from the outside without killing the real test process.
func TestRunHelperProcess(t *testing.T) {
	if os.Getenv("CMDBASE_RUN_HELPER") != "1" {
		return
	}
	switch os.Getenv("CMDBASE_RUN_MODE") {
	case "error":
		Run(func() error { return fmt.Errorf("kaboom") })
	case "ok":
		Run(func() error { return nil })
	}
}

func runHelper(t *testing.T, mode string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunHelperProcess$")
	cmd.Env = append(os.Environ(), "CMDBASE_RUN_HELPER=1", "CMDBASE_RUN_MODE="+mode)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	switch e := err.(type) {
	case nil:
		exitCode = 0
	case *exec.ExitError:
		exitCode = e.ExitCode()
	default:
		t.Fatalf("failed to run helper process: %v", err)
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// Runtime error: Run must print exactly one "Error: ..." line to stderr and
// exit 1, without any usage/help dump.
func TestRun_Error(t *testing.T) {
	stdout, stderr, code := runHelper(t, "error")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if strings.TrimSpace(stderr) != "Error: kaboom" {
		t.Fatalf("stderr = %q, want %q", stderr, "Error: kaboom")
	}
}

// Success: Run must exit 0 and print nothing of its own. (stdout still
// carries the test binary's own "PASS" line - that's the `go test` runtime,
// not Run.)
func TestRun_OK(t *testing.T) {
	stdout, stderr, code := runHelper(t, "ok")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout) != "PASS" {
		t.Fatalf("stdout = %q, want just the test binary's PASS line", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}
