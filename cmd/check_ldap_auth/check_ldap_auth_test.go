package main

// Black-box tests: the check's logic terminates the process via
// abmon.MonitoringCheck.Exit (os.Exit), so it is exercised the same way
// Icinga/Nagios would invoke it - by building the binary once and running it
// as a subprocess.

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "check_ldap_auth_bin")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "check_ldap_auth")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func run(t *testing.T, env []string, args ...string) (stdout string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(), env...)
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

func TestCheckLdapAuthMissingPasswordEnv(t *testing.T) {
	config := writeConfig(t)
	stdout, code := run(t, []string{"LDAP_PASSWORD="},
		"--config", config,
		"--host", "ldap://127.0.0.1:1",
		"--username", "cn=monitor,dc=example,dc=com",
	)
	if code != 3 { // UNKNOWN
		t.Fatalf("exit code = %d, want 3 (UNKNOWN), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "LDAP_PASSWORD") {
		t.Fatalf("stdout = %q, want it to mention LDAP_PASSWORD", stdout)
	}
}

func TestCheckLdapAuthConnectionRefused(t *testing.T) {
	// bind and immediately close a listener to get a reliably-refused port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	config := writeConfig(t)
	stdout, code := run(t, []string{"LDAP_PASSWORD=secret"},
		"--config", config,
		"--host", "ldap://"+addr,
		"--username", "cn=monitor,dc=example,dc=com",
		"--timeout", "2",
	)
	if code != 2 { // CRITICAL
		t.Fatalf("exit code = %d, want 2 (CRITICAL), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "LDAP check error") {
		t.Fatalf("stdout = %q, want it to mention the connection error", stdout)
	}
}

func TestCheckLdapAuthMissingRequiredFlag(t *testing.T) {
	config := writeConfig(t)
	_, code := run(t, []string{"LDAP_PASSWORD=secret"}, "--config", config)
	if code == 0 {
		t.Fatal("expected non-zero exit code when --host/--username are missing")
	}
}
