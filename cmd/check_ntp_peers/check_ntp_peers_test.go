package main

import "testing"

// Sample lines as produced by `ntpq -p`, e.g.:
//
//	     remote           refid      st t when poll reach   delay   offset  jitter
//	==============================================================================
//	*ntp1.example.com 192.0.2.1        2 u   45   64  377    0.512   -0.031   0.045
//	+ntp2.example.com .INIT.          16 u    -   64    0    0.000    0.000   0.000

func TestParsePeerSelected(t *testing.T) {
	p, err := ParsePeer("*ntp1.example.com 192.0.2.1        2 u   45   64  377    0.512   -0.031   0.045")
	if err != nil {
		t.Fatalf("ParsePeer error: %v", err)
	}
	if p.Tally != "*" {
		t.Errorf("Tally = %q, want *", p.Tally)
	}
	if p.Remote != "ntp1.example.com" {
		t.Errorf("Remote = %q, want ntp1.example.com", p.Remote)
	}
	if p.RefID != "192.0.2.1" {
		t.Errorf("RefID = %q, want 192.0.2.1", p.RefID)
	}
	if p.Stratum != 2 {
		t.Errorf("Stratum = %d, want 2", p.Stratum)
	}
	if p.T != "u" {
		t.Errorf("T = %q, want u", p.T)
	}
	if p.Poll != 64 {
		t.Errorf("Poll = %d, want 64", p.Poll)
	}
	if p.Delay != 0.512 {
		t.Errorf("Delay = %v, want 0.512", p.Delay)
	}
	if p.Offset != -0.031 {
		t.Errorf("Offset = %v, want -0.031", p.Offset)
	}
	if p.Jitter != 0.045 {
		t.Errorf("Jitter = %v, want 0.045", p.Jitter)
	}
}

func TestParsePeerReachOctal(t *testing.T) {
	// reach 377 is octal for 255 (all polls answered)
	p, err := ParsePeer("+ntp2.example.com .INIT.          16 u    2   64  377    0.000    0.000   0.000")
	if err != nil {
		t.Fatalf("ParsePeer error: %v", err)
	}
	if p.Reach != 255 {
		t.Errorf("Reach = %d, want 255 (octal 377)", p.Reach)
	}
	if p.Stratum != 16 {
		t.Errorf("Stratum = %d, want 16 (down/unreachable)", p.Stratum)
	}
}

func TestParsePeerEmptyLine(t *testing.T) {
	p, err := ParsePeer("   ")
	if err != nil {
		t.Fatalf("ParsePeer error: %v", err)
	}
	if p.Tally != "" || p.Remote != "" {
		t.Errorf("ParsePeer on blank line = %+v, want zero-value Peer", p)
	}
}
