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

// Sample lines as produced by `chronyc -w sources`, e.g.:
//
//	MS Name/IP address                       Stratum Poll Reach LastRx Last sample
//	=============================================================================================
//	^- 172-232-157-27.ip.linodeusercontent.com     2   9   377   208  +2794us[+2877us] +/-   27ms
//	^- h-155-4-55-243.A444.priv.bahnhof.se         2  10   377   454   -296us[ -216us] +/- 5569us
//	^+ ntp3.flashdance.cx                          2  10   377   359    +57us[ +138us] +/- 1751us
//	^* sth3.ntp.netnod.se                          1   9   377    23    -31us[  +55us] +/-  726us

func TestParseChronySourceSelected(t *testing.T) {
	p, err := ParseChronySource("^* sth3.ntp.netnod.se                          1   9   377    23    -31us[  +55us] +/-  726us")
	if err != nil {
		t.Fatalf("ParseChronySource error: %v", err)
	}
	if p.Tally != "*" {
		t.Errorf("Tally = %q, want *", p.Tally)
	}
	if p.Remote != "sth3.ntp.netnod.se" {
		t.Errorf("Remote = %q, want sth3.ntp.netnod.se", p.Remote)
	}
	if p.Stratum != 1 {
		t.Errorf("Stratum = %d, want 1", p.Stratum)
	}
	if p.Poll != 9 {
		t.Errorf("Poll = %d, want 9", p.Poll)
	}
	if p.Reach != 255 {
		t.Errorf("Reach = %d, want 255 (octal 377)", p.Reach)
	}
	if p.When != "23" {
		t.Errorf("When = %q, want 23", p.When)
	}
	if p.Offset != -0.031 {
		t.Errorf("Offset = %v, want -0.031", p.Offset)
	}
	if p.Jitter != 0.726 {
		t.Errorf("Jitter = %v, want 0.726", p.Jitter)
	}
	if p.Down {
		t.Errorf("Down = true, want false")
	}
}

func TestParseChronySourceNotCombined(t *testing.T) {
	// The stray space right after "[" in this line (misaligned raw sample)
	// must not throw off parsing of the surrounding fields.
	p, err := ParseChronySource("^- h-155-4-55-243.A444.priv.bahnhof.se         2  10   377   454   -296us[ -216us] +/- 5569us")
	if err != nil {
		t.Fatalf("ParseChronySource error: %v", err)
	}
	if p.Tally != "-" {
		t.Errorf("Tally = %q, want -", p.Tally)
	}
	if p.Remote != "h-155-4-55-243.A444.priv.bahnhof.se" {
		t.Errorf("Remote = %q, want h-155-4-55-243.A444.priv.bahnhof.se", p.Remote)
	}
	if p.Offset != -0.296 {
		t.Errorf("Offset = %v, want -0.296", p.Offset)
	}
	if p.Jitter != 5.569 {
		t.Errorf("Jitter = %v, want 5.569", p.Jitter)
	}
	if p.Down {
		t.Errorf("Down = true, want false")
	}
}

func TestParseChronySourceUnreachable(t *testing.T) {
	p, err := ParseChronySource("^? ntp.example.com                             0  10     0     -    +0ns[   +0ns] +/-    0ns")
	if err != nil {
		t.Fatalf("ParseChronySource error: %v", err)
	}
	if !p.Down {
		t.Errorf("Down = false, want true for unreachable (?) source")
	}
	if p.Reach != 0 {
		t.Errorf("Reach = %d, want 0", p.Reach)
	}
}

func TestParseChronySourceEmptyLine(t *testing.T) {
	p, err := ParseChronySource("   ")
	if err != nil {
		t.Fatalf("ParseChronySource error: %v", err)
	}
	if p.Tally != "" || p.Remote != "" {
		t.Errorf("ParseChronySource on blank line = %+v, want zero-value Peer", p)
	}
}

// Sample lines as produced by `chronyc -w sourcestats`, e.g.:
//
//	Name/IP Address            NP  NR  Span  Frequency  Freq Skew  Offset  Std Dev
//	==============================================================================
//	sth3.ntp.netnod.se          30  17   60m      0.002      0.031    +12us    97us

func TestParseChronySourceStats(t *testing.T) {
	name, stddev, err := ParseChronySourceStats("sth3.ntp.netnod.se          30  17   60m      0.002      0.031    +12us    97us")
	if err != nil {
		t.Fatalf("ParseChronySourceStats error: %v", err)
	}
	if name != "sth3.ntp.netnod.se" {
		t.Errorf("name = %q, want sth3.ntp.netnod.se", name)
	}
	if stddev != 0.097 {
		t.Errorf("stddev = %v, want 0.097", stddev)
	}
}

func TestParseChronySourceStatsEmptyLine(t *testing.T) {
	name, stddev, err := ParseChronySourceStats("   ")
	if err != nil {
		t.Fatalf("ParseChronySourceStats error: %v", err)
	}
	if name != "" || stddev != 0 {
		t.Errorf("ParseChronySourceStats on blank line = (%q, %v), want (\"\", 0)", name, stddev)
	}
}

func TestParseChronyDuration(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"27ms", 27},
		{"2794us", 2.794},
		{"-296us", -0.296},
		{"5569us", 5.569},
		{"1ns", 0.000001},
		{"1s", 1000},
	}
	for _, c := range cases {
		got, err := parseChronyDuration(c.in)
		if err != nil {
			t.Fatalf("parseChronyDuration(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("parseChronyDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
