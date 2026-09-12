//
// Check status on NTP / Chrony
// - One peer must be selected
// - Offset must be less than allowed max offset
// - Jitter must be less than allowed max jitter
// - All peers must be up
//
// Verify that both LB are active
// Verify that lb1 is the active and lb2 is standby
//
// This is an active check
//
// Author: Anders Lowinger, anders@abundo.se
//

package main

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"

	cmdbase "github.com/abundo/abmon/cmd"
	abmon "github.com/abundo/abmon/internal"
)

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
	Source    string `help:"NTP daemon to query" name:"source" short:"s" enum:"ntpq,chronyc" default:"ntpq"`
	MaxOffset string `help:"Max offset in milliseconds before warning/critical. Specify as warning:critical" name:"maxoffset" short:"o" default:"250:500"`
	MaxJitter string `help:"Max jitter in milliseconds before warning/critical. Specify as warning:critical" name:"maxjitter" short:"j" default:"250:500"`
}

var MaxOffset *abmon.Range
var MaxJitter *abmon.Range

type Peer struct {
	/*
	   Represents one NTP peer

	   tally is one of
	     + = denotes symmetric active
	     - = indicates symmetric passive
	     a = the remote server is being polled in client mode
	     ^ = indicates that the server is broadcasting to this address,
	     ~ = denotes that the remote peer is sending broadcasts
	     * = the peer the server is rently synchronizing to.

	   t is one of
	     u: unicast or manycast client
	     b: broadcast or multicast client
	     l: local (reference clock)
	     s: symmetric (peer)
	     A: manycast server
	     B: broadcast server
	     M: multicast server
	*/
	Tally   string
	Remote  string
	RefID   string
	Stratum int64
	T       string
	When    string
	Poll    int64
	Reach   int64
	Delay   float64
	Offset  float64
	Jitter  float64
	Down    bool
}

func ParsePeer(line string) (*Peer, error) {
	p := new(Peer)
	line = strings.TrimSpace(line)
	if line != "" {
		// Parse the string into the struct
		p.Tally = string(line[0])
		// tmp := strings.Split(line[1:], " ")
		tmp := strings.Fields(line[1:])
		// abmon.Pprint(tmp)
		p.Remote = tmp[0]
		p.RefID = tmp[1]
		p.Stratum, _ = strconv.ParseInt(tmp[2], 10, 64)
		p.T = tmp[3]
		p.When = tmp[4]
		p.Poll, _ = strconv.ParseInt(tmp[5], 10, 64)
		p.Reach, _ = strconv.ParseInt(tmp[6], 8, 64) // octal
		p.Delay, _ = strconv.ParseFloat(tmp[7], 64)
		//p.Delay *= 1000
		p.Offset, _ = strconv.ParseFloat(tmp[8], 64)
		//p.Offset *= 1000
		p.Jitter, _ = strconv.ParseFloat(tmp[9], 64)
		//p.Jitter *= 1000
		p.Down = p.Stratum == 16
	}
	return p, nil
}

// chronySourceRe matches one data line from `chronyc -w sources`, e.g.:
//
//	MS Name/IP address                       Stratum Poll Reach LastRx Last sample
//	=============================================================================================
//	^- 172-232-157-27.ip.linodeusercontent.com     2   9   377   208  +2794us[+2877us] +/-   27ms
//	^* sth3.ntp.netnod.se                          1   9   377    23    -31us[  +55us] +/-  726us
//
// The "Last sample" column right-aligns the bracketed raw measurement, which
// can introduce a stray space right after "[" (see the second example line
// above), so the tail of the line is matched with a dedicated pattern rather
// than split on whitespace.
var chronySourceRe = regexp.MustCompile(
	`^(\S)(\S)\s+(\S+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\S+)\s+` +
		`([+-]?\d+(?:\.\d+)?(?:ns|us|ms|s))\[\s*[+-]?\d+(?:\.\d+)?(?:ns|us|ms|s)\]\s*\+/-\s*` +
		`([+-]?\d+(?:\.\d+)?(?:ns|us|ms|s))\s*$`,
)

// ParseChronySource parses one data line from `chronyc -w sources`.
//
// The first two characters give the mode (^ server, = peer, # reference
// clock) and state (* current sync source, + combined, - not combined,
// ? unreachable, x falseticker, ~ too variable). Peer.Tally is set to the
// state character, so callers can check for "*" the same way as for ntpq.
//
// chronyc sources has no jitter column, so Peer.Jitter is set here from the
// "Last sample" +/- error estimate as a fallback approximation; CheckNTPPeers
// overrides it with the true standard deviation from `chronyc sourcestats`
// when one is available for the same source.
func ParseChronySource(line string) (*Peer, error) {
	p := new(Peer)
	line = strings.TrimSpace(line)
	if line == "" {
		return p, nil
	}
	m := chronySourceRe.FindStringSubmatch(line)
	if m == nil {
		return nil, fmt.Errorf("unrecognized chronyc sources line: %q", line)
	}
	p.Tally = m[2]
	p.Remote = m[3]
	p.Stratum, _ = strconv.ParseInt(m[4], 10, 64)
	p.Poll, _ = strconv.ParseInt(m[5], 10, 64)
	p.Reach, _ = strconv.ParseInt(m[6], 8, 64) // octal
	p.When = m[7]

	offset, err := parseChronyDuration(m[8])
	if err != nil {
		return nil, err
	}
	p.Offset = offset

	jitter, err := parseChronyDuration(m[9])
	if err != nil {
		return nil, err
	}
	p.Jitter = jitter

	p.Down = p.Tally == "?"

	return p, nil
}

// parseChronyDuration converts a chronyc duration value such as "27ms",
// "2794us" or "-31us" into milliseconds.
func parseChronyDuration(s string) (float64, error) {
	for _, unit := range []string{"ns", "us", "ms", "s"} {
		numPart, ok := strings.CutSuffix(s, unit)
		if !ok {
			continue
		}
		val, err := strconv.ParseFloat(numPart, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		switch unit {
		case "ns":
			return val / 1e6, nil
		case "us":
			return val / 1e3, nil
		case "ms":
			return val, nil
		default: // "s"
			return val * 1e3, nil
		}
	}
	return 0, fmt.Errorf("unknown unit in duration %q", s)
}

// ParseChronySourceStats parses one data line from `chronyc -w sourcestats`,
// e.g.:
//
//	Name/IP Address            NP  NR  Span  Frequency  Freq Skew  Offset  Std Dev
//	==============================================================================
//	sth3.ntp.netnod.se          30  17   60m      0.002      0.031    +12us    97us
//
// It returns the source name and its Std Dev column converted to
// milliseconds, the true jitter figure that `chronyc sources` itself lacks.
func ParseChronySourceStats(line string) (name string, stddevMs float64, err error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", 0, nil
	}
	tmp := strings.Fields(line)
	if len(tmp) < 8 {
		return "", 0, fmt.Errorf("unrecognized chronyc sourcestats line: %q", line)
	}
	name = tmp[0]
	stddevMs, err = parseChronyDuration(tmp[7])
	if err != nil {
		return "", 0, err
	}
	return name, stddevMs, nil
}

var opts Opts

// applyChronySourceStats runs `chronyc -w sourcestats` and overrides each
// peer's Jitter with the true standard deviation reported there, matched by
// source name.
func applyChronySourceStats(peers []*Peer) error {
	extcmd, err := abmon.RunExternalCommand("/usr/bin/chronyc", []string{"-w", "sourcestats"})
	if err != nil {
		return err
	}

	stats := make(map[string]float64)
	skiplines := true
	for _, line := range strings.Split(extcmd, "\n") {
		if len(line) == 0 {
			continue
		}
		if string(line[0]) == "=" {
			skiplines = false
			continue
		}
		if skiplines {
			continue
		}
		name, stddevMs, err := ParseChronySourceStats(line)
		if err != nil {
			return err
		}
		if name == "" {
			continue
		}
		stats[name] = stddevMs
	}

	for _, p := range peers {
		if stddevMs, ok := stats[p.Remote]; ok {
			p.Jitter = stddevMs
		}
	}
	return nil
}

func CheckNTPPeers(check *abmon.MonitoringCheck) error {
	var peers []*Peer

	cmdPath := "/usr/bin/ntpq"
	cmdArgs := []string{"-p"}
	parseLine := ParsePeer
	if opts.Source == "chronyc" {
		cmdPath = "/usr/bin/chronyc"
		cmdArgs = []string{"-w", "sources"}
		parseLine = ParseChronySource
	}

	extcmd, err := abmon.RunExternalCommand(cmdPath, cmdArgs)
	if err != nil {
		return err
	}
	// fmt.Println(extcmd)
	// Get remote peer and all the values
	skiplines := true
	// for _, line := range extcmd {
	for _, line := range strings.Split(extcmd, "\n") {
		if len(line) == 0 {
			continue
		}
		slog.Debug(line)
		if string(line[0]) == "=" {
			skiplines = false // next row is list of peers
			continue
		}
		if skiplines {
			continue
		}
		p, err := parseLine(line)
		if err != nil {
			return err
		}
		peers = append(peers, p)
	}

	if opts.Source == "chronyc" {
		if err := applyChronySourceStats(peers); err != nil {
			return err
		}
	}

	// Check peer status
	var selectedPeer *Peer
	var countPeersUp int
	for _, peer := range peers {
		if peer.Tally == "*" {
			selectedPeer = peer
		}
		s := fmt.Sprintf("Peer %s", peer.Remote)

		if !peer.Down {
			countPeersUp += 1
			s += fmt.Sprintf(", offset %f ms, jitter %f ms", peer.Offset, peer.Jitter)
			// stat = self.max_offset.check_warn_crit(abs(peer.offset))
			stat := MaxOffset.CheckWarnCritical(math.Abs(peer.Offset))
			if stat != abmon.OK {
				check.Result.SetStatus(stat)
				s += fmt.Sprintf(", Offset %s over maximum", abmon.ErrstatToStr[stat])
			} else {
				s += ", Offset OK"
			}

			stat = MaxJitter.CheckWarnCritical(math.Abs(peer.Jitter))
			if stat != abmon.OK {
				check.Result.SetStatus(stat)
				s += fmt.Sprintf(", Jitter %s over maximum", abmon.ErrstatToStr[stat])
			} else {
				s += ", Jitter OK"
			}
		} else {
			s += ", peer is DOWN"
		}

		check.Result.AddDetail(s)
	}

	if selectedPeer == nil {
		check.Exit(abmon.CRITICAL, "No NTP peer is selected")
	}
	if countPeersUp < 1 {
		check.Exit(abmon.CRITICAL, "All NTP peers down")
	}
	if countPeersUp < len(peers) {
		check.Exit(abmon.WARNING,
			fmt.Sprintf("Of total %d NTP peers is %d down", len(peers), len(peers)-countPeersUp),
		)
	}
	check.Exit(
		-1,
		fmt.Sprintf("Peer '%s' is selected, offset '%f' ms, jitter %f ms",
			selectedPeer.Remote, selectedPeer.Offset, selectedPeer.Jitter),
	)

	return nil
}

func main() {
	var err error

	kong.Parse(&opts, kong.Name("check_ntp_peers"), kong.Description("Check status on NTP / Chrony peers"), kong.Configuration(cmdbase.ConfigLoader))

	// Load configuration
	check, err := abmon.NewCheck(opts.CheckOpts)
	if err != nil {
		os.Exit(1)
	}
	MaxOffset, err = abmon.NewRange(opts.MaxOffset)
	if err != nil {
		panic(err)
	}
	MaxJitter, err = abmon.NewRange(opts.MaxJitter)
	if err != nil {
		panic(err)
	}
	slog.Debug(fmt.Sprintf("MaxOffset %+v", MaxOffset))
	slog.Debug(fmt.Sprintf("MaxJitter %+v", MaxJitter))
	err = CheckNTPPeers(check)
	//lint:ignore SA4023 CheckNTPPeers only returns without an error when check.Exit has already terminated the process; this guards the remaining case where it returns early with a real error
	if err != nil {
		fmt.Printf("%s\n", err)
	}
}
