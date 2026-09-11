package main

// Black-box tests: the check's logic terminates the process via
// abmon.MonitoringCheck.Exit (os.Exit), so it is exercised the same way
// Icinga/Nagios would invoke it - by building the binary once and running it
// as a subprocess against a local httptest server.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "check_http_redirect_bin")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "check_http_redirect")
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

func hostPort(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func TestCheckHTTPRedirectOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://example.com/new")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer srv.Close()

	config := writeConfig(t)
	stdout, code := run(t, "--config", config,
		"--host", hostPort(t, srv),
		"--url", srv.URL+"/old",
		"--redir", "https://example.com/new",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (OK), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "Moved permanently") {
		t.Fatalf("stdout = %q, want it to mention Moved permanently", stdout)
	}
}

func TestCheckHTTPRedirectFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://example.com/new")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	config := writeConfig(t)
	stdout, code := run(t, "--config", config,
		"--host", hostPort(t, srv),
		"--url", srv.URL+"/old",
		"--redir", "https://example.com/new",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (OK), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "Found/Moved temporarily") {
		t.Fatalf("stdout = %q, want it to mention Found/Moved temporarily", stdout)
	}
}

func TestCheckHTTPRedirectWrongTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://example.com/wrong")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	config := writeConfig(t)
	stdout, code := run(t, "--config", config,
		"--host", hostPort(t, srv),
		"--url", srv.URL+"/old",
		"--redir", "https://example.com/new",
	)
	if code != 2 { // CRITICAL
		t.Fatalf("exit code = %d, want 2 (CRITICAL), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "Redirect to wrong URL") {
		t.Fatalf("stdout = %q, want it to mention the wrong redirect", stdout)
	}
}

func TestCheckHTTPRedirectNoRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	config := writeConfig(t)
	stdout, code := run(t, "--config", config,
		"--host", hostPort(t, srv),
		"--url", srv.URL+"/old",
		"--redir", "https://example.com/new",
	)
	if code != 2 { // CRITICAL
		t.Fatalf("exit code = %d, want 2 (CRITICAL), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "No redirect returned") {
		t.Fatalf("stdout = %q, want it to mention no redirect", stdout)
	}
}

func TestCheckHTTPRedirectBadURL(t *testing.T) {
	config := writeConfig(t)
	stdout, code := run(t, "--config", config,
		"--host", "example.com",
		"--url", "://not-a-url",
		"--redir", "https://example.com/new",
	)
	if code != 3 { // UNKNOWN
		t.Fatalf("exit code = %d, want 3 (UNKNOWN), stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "Invalid URL") {
		t.Fatalf("stdout = %q, want it to mention invalid URL", stdout)
	}
}

func TestCheckHTTPRedirectUnreachableHost(t *testing.T) {
	config := writeConfig(t)
	stdout, code := run(t, "--config", config,
		"--host", "127.0.0.1:1", // nothing listens on port 1
		"--url", "http://example.com/old",
		"--redir", "https://example.com/new",
		"--timeout", "2",
	)
	if code != 3 { // UNKNOWN
		t.Fatalf("exit code = %d, want 3 (UNKNOWN), stdout: %s", code, stdout)
	}
}
