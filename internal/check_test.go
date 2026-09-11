package abmon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ===========================================================================
//   Range
// ===========================================================================

func TestNewRangeSingleValue(t *testing.T) {
	r, err := NewRange("500")
	if err != nil {
		t.Fatalf("NewRange error: %v", err)
	}
	if r.Start != 0 {
		t.Errorf("Start = %v, want 0", r.Start)
	}
	if r.End != 500 {
		t.Errorf("End = %v, want 500", r.End)
	}
}

func TestNewRangeStartEnd(t *testing.T) {
	r, err := NewRange("250:500")
	if err != nil {
		t.Fatalf("NewRange error: %v", err)
	}
	if r.Start != 250 || r.End != 500 {
		t.Errorf("Start/End = %v/%v, want 250/500", r.Start, r.End)
	}
}

func TestNewRangeOpenEnd(t *testing.T) {
	r, err := NewRange("10:")
	if err != nil {
		t.Fatalf("NewRange error: %v", err)
	}
	if r.Start != 10 {
		t.Errorf("Start = %v, want 10", r.Start)
	}
	if r.End < 1e300 {
		t.Errorf("End = %v, want effectively infinite", r.End)
	}
}

func TestNewRangeEmptyStart(t *testing.T) {
	r, err := NewRange(":500")
	if err != nil {
		t.Fatalf("NewRange error: %v", err)
	}
	if r.Start != 0 || r.End != 500 {
		t.Errorf("Start/End = %v/%v, want 0/500", r.Start, r.End)
	}
}

func TestNewRangeInsideMarker(t *testing.T) {
	r, err := NewRange("$10:20")
	if err != nil {
		t.Fatalf("NewRange error: %v", err)
	}
	if !r.Inside {
		t.Error("Inside = false, want true")
	}
	if r.Start != 10 || r.End != 20 {
		t.Errorf("Start/End = %v/%v, want 10/20", r.Start, r.End)
	}
}

func TestNewRangeTooManyParts(t *testing.T) {
	if _, err := NewRange("1:2:3"); err == nil {
		t.Fatal("NewRange with 3 parts: want error, got nil")
	}
}

func TestNewRangeStartAfterEnd(t *testing.T) {
	if _, err := NewRange("500:100"); err == nil {
		t.Fatal("NewRange with start > end: want error, got nil")
	}
}

func TestRangeCheck(t *testing.T) {
	r, err := NewRange("10:20")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		value float64
		want  bool
	}{
		{5, true},   // below range
		{10, false}, // at start, inclusive
		{15, false}, // inside
		{20, false}, // at end, inclusive
		{25, true},  // above range
	}
	for _, c := range cases {
		if got := r.Check(c.value); got != c.want {
			t.Errorf("Check(%v) = %v, want %v", c.value, got, c.want)
		}
	}
}

func TestRangeCheckWarnCritical(t *testing.T) {
	r, err := NewRange("10:20")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		value float64
		want  int
	}{
		{5, OK},
		{10, OK},
		{15, WARNING},
		{20, WARNING},
		{25, CRITICAL},
	}
	for _, c := range cases {
		if got := r.CheckWarnCritical(c.value); got != c.want {
			t.Errorf("CheckWarnCritical(%v) = %v, want %v", c.value, got, c.want)
		}
	}
}

// ===========================================================================
//   Status maps / MonitoringResult
// ===========================================================================

func TestErrstatMapsRoundTrip(t *testing.T) {
	for str, code := range StrToErrstat {
		if ErrstatToStr[code] == "" {
			t.Errorf("ErrstatToStr[%d] is empty for status %q", code, str)
		}
	}
	for code, str := range ErrstatToStr {
		if got, ok := StrToErrstat[strings.ToLower(str)]; !ok || got != code {
			t.Errorf("StrToErrstat[%q] = %v, ok=%v, want %v", strings.ToLower(str), got, ok, code)
		}
	}
}

func TestNewResultDefaultsUnknown(t *testing.T) {
	r := NewResult()
	if r.Status != UNKNOWN {
		t.Errorf("NewResult().Status = %v, want UNKNOWN", r.Status)
	}
}

func TestResultSetStatusOnlyRaises(t *testing.T) {
	r := NewResult() // starts at UNKNOWN (3)
	r.SetStatus(OK)  // 0, lower, should not lower it
	if r.Status != UNKNOWN {
		t.Errorf("SetStatus(OK) after UNKNOWN = %v, want UNKNOWN unchanged", r.Status)
	}
	r.Status = OK
	r.SetStatus(WARNING)
	if r.Status != WARNING {
		t.Errorf("SetStatus(WARNING) = %v, want WARNING", r.Status)
	}
	r.SetStatus(OK) // should not lower back down
	if r.Status != WARNING {
		t.Errorf("SetStatus(OK) after WARNING = %v, want WARNING unchanged", r.Status)
	}
	r.SetStatus(CRITICAL)
	if r.Status != CRITICAL {
		t.Errorf("SetStatus(CRITICAL) = %v, want CRITICAL", r.Status)
	}
}

func TestResultAddDetail(t *testing.T) {
	r := NewResult()
	r.AddDetail("first")
	r.AddDetail("second")
	if len(r.Details) != 2 || r.Details[0] != "first" || r.Details[1] != "second" {
		t.Errorf("Details = %v, want [first second]", r.Details)
	}
}

// ===========================================================================
//   LoadConfiguration
// ===========================================================================

func TestLoadConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abmon.yaml")
	yaml := `
zones:
  example.com:
    customer: cust1
    primary: ns1.example.com
    dnsnode: true
    dns_check_propagation: true
  _internal:
    customer: cust1
    primary: ns2.example.com
  other.com:
    customer: cust2
    primary: ns1.other.com

dhcp_scopes:
  office:
    oid: "123"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfiguration(path)
	if err != nil {
		t.Fatalf("LoadConfiguration error: %v", err)
	}

	zone, ok := config.Zones["example.com"]
	if !ok {
		t.Fatal("zone example.com not loaded")
	}
	if zone.Name != "example.com" {
		t.Errorf("zone.Name = %q, want example.com", zone.Name)
	}
	if !zone.DnsNode || !zone.DnsCheckPropagation {
		t.Errorf("zone flags = %+v, want DnsNode/DnsCheckPropagation true", zone)
	}

	// "_internal" is excluded from the per-customer grouping
	if len(config.Customers["cust1"]) != 1 || config.Customers["cust1"][0].Name != "example.com" {
		t.Errorf("Customers[cust1] = %+v, want only example.com", config.Customers["cust1"])
	}
	if _, found := config.Zones["_internal"]; !found {
		t.Error("_internal zone should still be present in Zones, just not grouped under a customer")
	}

	wantCustomers := []string{"cust1", "cust2"}
	if len(config.SortedCustomers) != len(wantCustomers) {
		t.Fatalf("SortedCustomers = %v, want %v", config.SortedCustomers, wantCustomers)
	}
	for i, name := range wantCustomers {
		if config.SortedCustomers[i] != name {
			t.Errorf("SortedCustomers[%d] = %q, want %q", i, config.SortedCustomers[i], name)
		}
	}

	scope, ok := config.DhcpScopes["office"]
	if !ok {
		t.Fatal("dhcp scope office not loaded")
	}
	if scope.Name != "office" || scope.OID != "123" {
		t.Errorf("scope = %+v, want Name=office OID=123", scope)
	}
}

func TestLoadConfigurationMissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadConfiguration(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Fatal("LoadConfiguration with missing file: want error, got nil")
	}
}

func TestLoadConfigurationBadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("zones: [this is not a map"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfiguration(path); err == nil {
		t.Fatal("LoadConfiguration with malformed yaml: want error, got nil")
	}
}

// ===========================================================================
//   RunExternalCommand
// ===========================================================================

func TestRunExternalCommand(t *testing.T) {
	out, err := RunExternalCommand("echo", []string{"hello"})
	if err != nil {
		t.Fatalf("RunExternalCommand error: %v", err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Errorf("output = %q, want %q", out, "hello")
	}
}

func TestRunExternalCommandNotFound(t *testing.T) {
	if _, err := RunExternalCommand("/no/such/binary-abmon-test", nil); err == nil {
		t.Fatal("RunExternalCommand with missing binary: want error, got nil")
	}
}

// ===========================================================================
//   MonitoringCheck.Exit (calls os.Exit, so it is re-exec'd like cmd_base_run_test.go)
// ===========================================================================

func TestExitHelperProcess(t *testing.T) {
	if os.Getenv("ABMON_EXIT_HELPER") != "1" {
		return
	}
	check := &MonitoringCheck{Result: NewResult()}
	switch os.Getenv("ABMON_EXIT_MODE") {
	case "critical_with_perfdata":
		check.Result.Perfdata = []string{"'metric'=5;;;;"}
		check.Result.AddDetail("detail line")
		check.Exit(CRITICAL, "something is wrong")
	case "keep_status":
		check.Result.Status = WARNING
		check.Exit(-1, "keep current status")
	}
}

func runExitHelper(t *testing.T, mode string) (stdout string, exitCode int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestExitHelperProcess$")
	cmd.Env = append(os.Environ(), "ABMON_EXIT_HELPER=1", "ABMON_EXIT_MODE="+mode)
	var outBuf strings.Builder
	cmd.Stdout = &outBuf
	err := cmd.Run()
	switch e := err.(type) {
	case nil:
		exitCode = 0
	case *exec.ExitError:
		exitCode = e.ExitCode()
	default:
		t.Fatalf("failed to run helper process: %v", err)
	}
	return outBuf.String(), exitCode
}

func TestCheckExitCriticalWithPerfdata(t *testing.T) {
	stdout, code := runExitHelper(t, "critical_with_perfdata")
	if code != CRITICAL {
		t.Fatalf("exit code = %d, want %d", code, CRITICAL)
	}
	want := fmt.Sprintf("%s something is wrong|'metric'=5;;;;\ndetail line", ErrstatToStr[CRITICAL])
	if !strings.Contains(stdout, want) {
		t.Fatalf("stdout = %q, want it to contain %q", stdout, want)
	}
}

func TestCheckExitKeepsExistingStatus(t *testing.T) {
	stdout, code := runExitHelper(t, "keep_status")
	if code != WARNING {
		t.Fatalf("exit code = %d, want %d", code, WARNING)
	}
	if !strings.Contains(stdout, "WARNING keep current status") {
		t.Fatalf("stdout = %q, want it to contain %q", stdout, "WARNING keep current status")
	}
}
