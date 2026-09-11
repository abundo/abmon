package main

// checkZone and the DNS/telnet servers need real network I/O (a live
// nameserver, a bound UDP/TCP port), so they aren't exercised here. What is
// testable in isolation is the pure bookkeeping (setSitePropagation,
// debugFlagsToString) and the CLI command dispatcher, which only writes to
// a net.Conn and touches in-memory state.

import (
	"net"
	"strings"
	"testing"
	"time"

	abmon "github.com/abundo/abmon/internal"
)

// ===========================================================================
//   setSitePropagation
// ===========================================================================

func TestSetSitePropagationNegativeInitializesOnce(t *testing.T) {
	sites := make(map[string]int)
	setSitePropagation(sites, "site1", -1)
	if got, ok := sites["site1"]; !ok || got != 0 {
		t.Fatalf("sites[site1] = %v, ok=%v, want 0/true", got, ok)
	}
	// a later negative call must not clobber a real value already recorded
	sites["site1"] = 5
	setSitePropagation(sites, "site1", -1)
	if sites["site1"] != 5 {
		t.Fatalf("sites[site1] = %v, want unchanged 5", sites["site1"])
	}
}

func TestSetSitePropagationPositiveOverwrites(t *testing.T) {
	sites := make(map[string]int)
	setSitePropagation(sites, "site1", -1)
	setSitePropagation(sites, "site1", 7)
	if sites["site1"] != 7 {
		t.Fatalf("sites[site1] = %v, want 7", sites["site1"])
	}
}

// ===========================================================================
//   debugFlagsToString
// ===========================================================================

func TestDebugFlagsToString(t *testing.T) {
	orig := debugFlag
	defer func() { debugFlag = orig }()

	debugFlag = 0
	if got := debugFlagsToString(); got != "" {
		t.Errorf("debugFlagsToString() = %q, want empty", got)
	}

	debugFlag = DebugCheck
	if got := debugFlagsToString(); got != "check" {
		t.Errorf("debugFlagsToString() = %q, want %q", got, "check")
	}

	debugFlag = DebugDnsnode
	if got := debugFlagsToString(); got != "dnsnode" {
		t.Errorf("debugFlagsToString() = %q, want %q", got, "dnsnode")
	}

	debugFlag = DebugCheck | DebugDnsnode
	if got := debugFlagsToString(); got != "check,dnsnode" {
		t.Errorf("debugFlagsToString() = %q, want %q", got, "check,dnsnode")
	}
}

// ===========================================================================
//   CLI
// ===========================================================================

// runCLIAndRead runs CLI(...) in a goroutine (it writes synchronously to
// conn, and net.Pipe is unbuffered - a write only completes once someone is
// reading) and returns everything written to conn by the time CLI returns.
//
// Each Read gets a short deadline so a read that would otherwise block
// forever (nothing left to read, CLI still hasn't returned) instead loops
// back around to check whether CLI is done yet.
func runCLIAndRead(t *testing.T, client *telnetClient, cmd string) string {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		CLI(serverConn, client, cmd)
		close(done)
	}()

	var out strings.Builder
	buf := make([]byte, 4096)
	deadline := time.Now().Add(2 * time.Second)
	for {
		clientConn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err := clientConn.Read(buf)
		out.Write(buf[:n])
		if err != nil {
			select {
			case <-done:
				return out.String()
			default:
			}
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for CLI(%q) to finish; got so far: %q", cmd, out.String())
			}
			continue
		}
	}
}

func TestCLIHelp(t *testing.T) {
	config = &abmon.ConfigFile{Zones: map[string]*abmon.ConfigZone{}}
	client := &telnetClient{}
	out := runCLIAndRead(t, client, "help")
	if !strings.Contains(out, "Commands:") || !strings.Contains(out, "check <zone>") {
		t.Fatalf("help output = %q, missing expected content", out)
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	config = &abmon.ConfigFile{Zones: map[string]*abmon.ConfigZone{}}
	client := &telnetClient{}
	out := runCLIAndRead(t, client, "bogus")
	if !strings.Contains(out, "Unknown command: bogus") {
		t.Fatalf("output = %q, want it to mention the unknown command", out)
	}
}

func TestCLIShowZoneUnknown(t *testing.T) {
	config = &abmon.ConfigFile{Zones: map[string]*abmon.ConfigZone{}}
	client := &telnetClient{}
	out := runCLIAndRead(t, client, "show zone no-such-zone")
	if !strings.Contains(out, "No zone no-such-zone defined") {
		t.Fatalf("output = %q, want it to mention no such zone", out)
	}
}

func TestCLIShowZoneKnown(t *testing.T) {
	config = &abmon.ConfigFile{Zones: map[string]*abmon.ConfigZone{
		"example.com": {Name: "example.com", Customer: "acme"},
	}}
	client := &telnetClient{}
	out := runCLIAndRead(t, client, "show zone example.com")
	if !strings.Contains(out, "example.com") || !strings.Contains(out, "acme") {
		t.Fatalf("output = %q, want zone details", out)
	}
}

func TestCLIDebugUndebug(t *testing.T) {
	orig := debugFlag
	defer func() { debugFlag = orig }()

	config = &abmon.ConfigFile{Zones: map[string]*abmon.ConfigZone{}}
	client := &telnetClient{}

	debugFlag = 0
	out := runCLIAndRead(t, client, "debug all")
	if debugFlag != DebugCheck|DebugDnsnode {
		t.Fatalf("debugFlag = %d after 'debug all', want %d", debugFlag, DebugCheck|DebugDnsnode)
	}
	if !strings.Contains(out, "Debug set to:") {
		t.Fatalf("output = %q, want confirmation", out)
	}

	out = runCLIAndRead(t, client, "undebug all")
	if debugFlag != 0 {
		t.Fatalf("debugFlag = %d after 'undebug all', want 0", debugFlag)
	}
	_ = out
}

func TestCLIEmptyCommandNoop(t *testing.T) {
	config = &abmon.ConfigFile{Zones: map[string]*abmon.ConfigZone{}}
	client := &telnetClient{}
	// CLI("") returns immediately without writing anything; make sure it
	// doesn't block or panic.
	CLI(nil, client, "")
}
