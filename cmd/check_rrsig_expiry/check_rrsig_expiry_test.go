package main

// check_rrsig_expiry does most of its real work over AXFR, which isn't
// practical to exercise from a unit test. What is testable without a real
// DNS server is the zone lookup in main(): an unknown --zone must be
// rejected before any network I/O happens.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "check_rrsig_expiry_bin")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "check_rrsig_expiry")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func run(t *testing.T, args ...string) (stdout string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	switch e := err.(type) {
	case nil:
		exitCode = 0
	case *exec.ExitError:
		exitCode = e.ExitCode()
	default:
		t.Fatalf("failed to run binary: %v", err)
	}
	return out.String(), exitCode
}

func writeConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "abmon.yaml")
	if err := os.WriteFile(path, []byte("zones:\n  example.com:\n    primary: ns1.example.com\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckRRSIGExpiryUnknownZone(t *testing.T) {
	config := writeConfig(t)
	stdout, code := run(t, "--config", config, "--zone", "no-such-zone.example")
	if code != 3 { // UNKNOWN
		t.Fatalf("exit code = %d, want 3 (UNKNOWN), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "No or unknown zone") {
		t.Fatalf("stdout = %q, want it to mention the unknown zone", stdout)
	}
}
