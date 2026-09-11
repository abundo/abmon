package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	abmon "github.com/abundo/abmon/internal"
)

// ===========================================================================
//   becsClient - unit tests against a fake JSON-RPC server
// ===========================================================================

// fakeBecsServer implements just enough of the BECS EAPI (JSON-RPC 2.0) for
// becsClient: sessionLogin, rdrGet and sessionLogout.
func fakeBecsServer(t *testing.T, wantUsername, wantPassword string, rdrData []rdrData, rdrErrTxt string) *httptest.Server {
	t.Helper()
	var loggedIn bool
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
			ID     int            `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		writeResult := func(result any) {
			resp := struct {
				Result any `json:"result"`
			}{Result: result}
			body, _ := json.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			w.Write(body)
		}
		writeError := func(msg string, code int) {
			resp := struct {
				Error struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}{}
			resp.Error.Code = code
			resp.Error.Message = msg
			body, _ := json.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			w.Write(body)
		}

		switch req.Method {
		case "sessionLogin":
			username, _ := req.Params["username"].(string)
			password, _ := req.Params["password"].(string)
			if username != wantUsername || password != wantPassword {
				writeResult(map[string]string{"errtxt": "invalid credentials"})
				return
			}
			loggedIn = true
			writeResult(map[string]string{"sessionid": "session-123"})
		case "rdrGet":
			if !loggedIn {
				writeError("not logged in", 401)
				return
			}
			if rdrErrTxt != "" {
				writeResult(map[string]any{
					"entries": []map[string]any{{"errtxt": rdrErrTxt}},
				})
				return
			}
			writeResult(map[string]any{
				"entries": []map[string]any{{"data": rdrData}},
			})
		case "sessionLogout":
			loggedIn = false
			writeResult(map[string]string{})
		default:
			writeError(fmt.Sprintf("unknown method %s", req.Method), 404)
		}
	}))
}

func newTestConfig(url string) *abmon.ConfigFile {
	config := new(abmon.ConfigFile)
	config.Becs.URL = url
	config.Becs.Username = "monitor"
	config.Becs.Password = "secret"
	return config
}

func TestBecsClientLoginAndDHCPScopeUtilization(t *testing.T) {
	data := []rdrData{
		{
			VData: []rdrData{
				{Name: "free", VGauge: 10},
				{Name: "assigned", VGauge: 240},
				{Name: "excluded", VGauge: 6},
			},
		},
	}
	srv := fakeBecsServer(t, "monitor", "secret", data, "")
	defer srv.Close()

	client, err := newBecsClient(newTestConfig(srv.URL))
	if err != nil {
		t.Fatalf("newBecsClient error: %v", err)
	}
	defer client.Logout()

	util, err := client.DHCPScopeUtilization("42")
	if err != nil {
		t.Fatalf("DHCPScopeUtilization error: %v", err)
	}
	if util.Free != 10 || util.Assigned != 240 || util.Total != 256 {
		t.Fatalf("util = %+v, want Free=10 Assigned=240 Total=256", util)
	}
}

func TestBecsClientMultiplePrefixesSummed(t *testing.T) {
	data := []rdrData{
		{VData: []rdrData{{Name: "free", VGauge: 5}, {Name: "assigned", VGauge: 10}}},
		{VData: []rdrData{{Name: "free", VGauge: 3}, {Name: "assigned", VGauge: 7}}},
	}
	srv := fakeBecsServer(t, "monitor", "secret", data, "")
	defer srv.Close()

	client, err := newBecsClient(newTestConfig(srv.URL))
	if err != nil {
		t.Fatalf("newBecsClient error: %v", err)
	}
	defer client.Logout()

	util, err := client.DHCPScopeUtilization("42")
	if err != nil {
		t.Fatalf("DHCPScopeUtilization error: %v", err)
	}
	if util.Free != 8 || util.Assigned != 17 {
		t.Fatalf("util = %+v, want Free=8 Assigned=17", util)
	}
}

func TestBecsClientLoginFailure(t *testing.T) {
	srv := fakeBecsServer(t, "monitor", "secret", nil, "")
	defer srv.Close()

	config := newTestConfig(srv.URL)
	config.Becs.Password = "wrong"
	client, err := newBecsClient(config)
	if err != nil {
		t.Fatalf("newBecsClient error: %v", err)
	}
	if _, err := client.DHCPScopeUtilization("42"); err == nil {
		t.Fatal("DHCPScopeUtilization with bad credentials: want error, got nil")
	}
}

func TestBecsClientRdrGetError(t *testing.T) {
	srv := fakeBecsServer(t, "monitor", "secret", nil, "no such object")
	defer srv.Close()

	client, err := newBecsClient(newTestConfig(srv.URL))
	if err != nil {
		t.Fatalf("newBecsClient error: %v", err)
	}
	if _, err := client.DHCPScopeUtilization("42"); err == nil {
		t.Fatal("DHCPScopeUtilization with rdrGet error: want error, got nil")
	}
}

func TestBecsClientInvalidOID(t *testing.T) {
	srv := fakeBecsServer(t, "monitor", "secret", nil, "")
	defer srv.Close()

	client, err := newBecsClient(newTestConfig(srv.URL))
	if err != nil {
		t.Fatalf("newBecsClient error: %v", err)
	}
	if _, err := client.DHCPScopeUtilization("not-a-number"); err == nil {
		t.Fatal("DHCPScopeUtilization with invalid oid: want error, got nil")
	}
}

func TestNewBecsClientMissingConfig(t *testing.T) {
	if _, err := newBecsClient(new(abmon.ConfigFile)); err == nil {
		t.Fatal("newBecsClient with empty config: want error, got nil")
	}
	config := new(abmon.ConfigFile)
	config.Becs.URL = "https://becs.example.com/eapi"
	if _, err := newBecsClient(config); err == nil {
		t.Fatal("newBecsClient with no username: want error, got nil")
	}
}

// ===========================================================================
//   writeIcingaConfig
// ===========================================================================

func TestWriteIcingaConfigCreatesFile(t *testing.T) {
	origReload := abmon.ReloadIcinga
	abmon.ReloadIcinga = func() {}
	defer func() { abmon.ReloadIcinga = origReload }()

	dir := t.TempDir()
	target := filepath.Join(dir, "dhcp-scopes.conf")
	scopes := []DHCPScopeUtilization{{Name: "office-lan", Free: 10, Assigned: 240, Total: 256}}

	if err := writeIcingaConfig(target, "monitor-host", scopes); err != nil {
		t.Fatalf("writeIcingaConfig error: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("target file not written: %v", err)
	}
	if !strings.Contains(string(content), `"DHCP Scope office-lan"`) {
		t.Errorf("content missing scope service block: %s", content)
	}
	if !strings.Contains(string(content), `host.name == "monitor-host"`) {
		t.Errorf("content missing icinga host: %s", content)
	}
}

func TestWriteIcingaConfigNoopWhenUnchanged(t *testing.T) {
	origReload := abmon.ReloadIcinga
	abmon.ReloadIcinga = func() {}
	defer func() { abmon.ReloadIcinga = origReload }()

	dir := t.TempDir()
	target := filepath.Join(dir, "dhcp-scopes.conf")
	scopes := []DHCPScopeUtilization{{Name: "office-lan", Free: 10, Assigned: 240, Total: 256}}

	if err := writeIcingaConfig(target, "monitor-host", scopes); err != nil {
		t.Fatalf("first writeIcingaConfig error: %v", err)
	}
	info1, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}

	if err := writeIcingaConfig(target, "monitor-host", scopes); err != nil {
		t.Fatalf("second writeIcingaConfig error: %v", err)
	}
	info2, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info1.ModTime() != info2.ModTime() {
		t.Error("writeIcingaConfig rewrote an unchanged file, want a no-op")
	}
}

// ===========================================================================
//   sendIcingaResults - just needs to not panic and print the expected line;
//   Notify is a no-op when NotifyIcinga/NotifyNagios are both false.
// ===========================================================================

func TestSendIcingaResultsSkipsEmptyScopes(t *testing.T) {
	origOpts := opts
	defer func() { opts = origOpts }()
	opts.FreeWarning = 20
	opts.FreeCritical = 5
	opts.IcingaHost = "monitor-host"

	config := new(abmon.ConfigFile)
	scopes := []DHCPScopeUtilization{
		{Name: "empty-scope", Total: 0},
		{Name: "office-lan", Total: 256, Free: 2, Assigned: 254},
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	sendIcingaResults(config, scopes)
	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if strings.Contains(out, "empty-scope") {
		t.Errorf("output should skip scopes with Total=0: %s", out)
	}
	if !strings.Contains(out, "office-lan") || !strings.Contains(out, "CRITICAL") {
		t.Errorf("output missing expected office-lan CRITICAL line: %s", out)
	}
}

// ===========================================================================
//   Black-box test of the built binary for a config-error path
// ===========================================================================

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "check_becs_dhcp_scope_bin")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "check_becs_dhcp_scope")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func TestCheckBecsDhcpScopeNoBecsConfig(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "abmon.yaml")
	if err := os.WriteFile(config, []byte("zones: {}\ndhcp_scopes:\n  office-lan:\n    oid: \"1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binPath, "--config", config)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()

	var code int
	switch e := err.(type) {
	case nil:
		code = 0
	case *exec.ExitError:
		code = e.ExitCode()
	default:
		t.Fatalf("failed to run binary: %v", err)
	}
	if code != 3 { // UNKNOWN
		t.Fatalf("exit code = %d, want 3 (UNKNOWN), stdout: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "becs.url is not configured") {
		t.Fatalf("stdout = %q, want it to mention missing becs config", out.String())
	}
}
