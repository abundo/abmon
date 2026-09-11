package main

// Black-box tests: the check's logic terminates the process via
// abmon.MonitoringCheck.Exit (os.Exit), so it is exercised the same way
// Icinga/Nagios would invoke it - by building the binary once and running it
// as a subprocess with different arguments, then asserting on stdout and the
// exit code.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "check_file_status_bin")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "check_file_status")
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
	if err := os.WriteFile(path, []byte("zones: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckFileStatusMissingFile(t *testing.T) {
	config := writeConfig(t)
	dir := t.TempDir()
	stdout, code := run(t, "--config", config, "--file", filepath.Join(dir, "missing.txt"))
	if code != 3 { // UNKNOWN
		t.Fatalf("exit code = %d, want 3 (UNKNOWN), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "does not exist") {
		t.Fatalf("stdout = %q, want it to mention the file does not exist", stdout)
	}
}

func TestCheckFileStatusTooOldCritical(t *testing.T) {
	config := writeConfig(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "status.txt")
	if err := os.WriteFile(f, []byte("OK all good\n"), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-40 * time.Hour)
	if err := os.Chtimes(f, old, old); err != nil {
		t.Fatal(err)
	}
	stdout, code := run(t, "--config", config, "--file", f, "--age_warning", "48", "--age_critical", "36")
	if code != 2 { // CRITICAL
		t.Fatalf("exit code = %d, want 2 (CRITICAL), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "older than limit") {
		t.Fatalf("stdout = %q, want it to mention the age limit", stdout)
	}
}

func TestCheckFileStatusTooOldWarning(t *testing.T) {
	config := writeConfig(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "status.txt")
	if err := os.WriteFile(f, []byte("OK all good\n"), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(f, old, old); err != nil {
		t.Fatal(err)
	}
	stdout, code := run(t, "--config", config, "--file", f, "--age_warning", "1", "--age_critical", "36")
	if code != 1 { // WARNING
		t.Fatalf("exit code = %d, want 1 (WARNING), stdout: %s", code, stdout)
	}
}

func TestCheckFileStatusFirstLineStatus(t *testing.T) {
	config := writeConfig(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "status.txt")
	if err := os.WriteFile(f, []byte("CRITICAL disk full\nmore details\n"), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, code := run(t, "--config", config, "--file", f, "--age_warning", "48", "--age_critical", "36")
	if code != 2 { // CRITICAL, from the file's first line
		t.Fatalf("exit code = %d, want 2 (CRITICAL), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "disk full") {
		t.Fatalf("stdout = %q, want it to contain the file's first line", stdout)
	}
}

func TestCheckFileStatusOK(t *testing.T) {
	config := writeConfig(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "status.txt")
	if err := os.WriteFile(f, []byte("OK everything fine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, code := run(t, "--config", config, "--file", f, "--age_warning", "48", "--age_critical", "36")
	if code != 0 { // OK
		t.Fatalf("exit code = %d, want 0 (OK), stdout: %s", code, stdout)
	}
}

func TestCheckFileStatusMissingRequiredFlag(t *testing.T) {
	config := writeConfig(t)
	_, code := run(t, "--config", config)
	if code == 0 {
		t.Fatal("expected non-zero exit code when --file is missing")
	}
}
