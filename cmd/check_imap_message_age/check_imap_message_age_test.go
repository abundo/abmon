package main

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ===========================================================================
//   loadCredentials - pure logic, tested in-process
// ===========================================================================

func TestLoadCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.ini")
	content := "[imap]\n# a comment\n; another comment\nusername = alice\npassword = s3cret\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	username, password, err := loadCredentials(path)
	if err != nil {
		t.Fatalf("loadCredentials error: %v", err)
	}
	if username != "alice" || password != "s3cret" {
		t.Errorf("got username=%q password=%q, want alice/s3cret", username, password)
	}
}

func TestLoadCredentialsCaseInsensitiveKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.ini")
	content := "USERNAME=bob\nPassword=hunter2\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	username, password, err := loadCredentials(path)
	if err != nil {
		t.Fatalf("loadCredentials error: %v", err)
	}
	if username != "bob" || password != "hunter2" {
		t.Errorf("got username=%q password=%q, want bob/hunter2", username, password)
	}
}

func TestLoadCredentialsMissingFile(t *testing.T) {
	if _, _, err := loadCredentials("/no/such/file"); err == nil {
		t.Fatal("loadCredentials with missing file: want error, got nil")
	}
}

func TestLoadCredentialsIgnoresMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.ini")
	content := "not a key value line\nusername = carol\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	username, password, err := loadCredentials(path)
	if err != nil {
		t.Fatalf("loadCredentials error: %v", err)
	}
	if username != "carol" || password != "" {
		t.Errorf("got username=%q password=%q, want carol/empty", username, password)
	}
}

// ===========================================================================
//   Black-box tests of the built binary
// ===========================================================================

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "check_imap_message_age_bin")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "check_imap_message_age")
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

func TestCheckImapMessageAgeConnectionRefused(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().(*net.TCPAddr)
	port := addr.Port
	l.Close()

	config := writeConfig(t)
	stdout, code := run(t, "--config", config,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--username", "user",
		"--password", "pass",
		"--warning", "10",
		"--critical", "20",
		"--timeout", "2",
	)
	if code != 3 { // UNKNOWN
		t.Fatalf("exit code = %d, want 3 (UNKNOWN), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "Could not connect") {
		t.Fatalf("stdout = %q, want it to mention the connection failure", stdout)
	}
}

func TestCheckImapMessageAgeMissingRequiredFlag(t *testing.T) {
	config := writeConfig(t)
	_, code := run(t, "--config", config)
	if code == 0 {
		t.Fatal("expected non-zero exit code when required flags are missing")
	}
}
