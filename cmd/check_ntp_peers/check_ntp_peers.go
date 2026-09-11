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
	"math"
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/alecthomas/kong"

	abmon "github.com/abundo/abmon/internal"
)

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
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
}

func ParsePeer(line string) (*Peer, error) {
	p := new(Peer)
	// self.tally = None
	// self.remote = remote
	// self.refid = None
	// self.stratum = None
	// self.t = None
	// self.when = None
	// self.poll = None
	// self.reach = None
	// self.delay = None
	// self.offset = None
	// self.jitter = None
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
	}
	return p, nil
}

var opts Opts

func CheckNTPPeers(check *abmon.MonitoringCheck) error {
	var peers []*Peer

	extcmd, err := abmon.RunExternalCommand("/usr/bin/ntpq", []string{"-p"})
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
		log.Debugf("%s", line)
		if string(line[0]) == "=" {
			skiplines = false // next row is list of peers
			continue
		}
		if skiplines {
			continue
		}
		p, err := ParsePeer(line)
		if err != nil {
			return err
		}
		peers = append(peers, p)
	}

	// Check peer status
	var selectedPeer *Peer
	var countPeersUp int
	for _, peer := range peers {
		if peer.Tally == "*" {
			selectedPeer = peer
		}
		s := fmt.Sprintf("Peer %s", peer.Remote)

		if peer.Stratum != 16 {
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

	kong.Parse(&opts, kong.Name("check_ntp_peers"), kong.Description("Check status on NTP / Chrony peers"))

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
	log.Debugf("MaxOffset %+v", MaxOffset)
	log.Debugf("MaxJitter %+v", MaxJitter)
	err = CheckNTPPeers(check)
	//lint:ignore SA4023 CheckNTPPeers only returns without an error when check.Exit has already terminated the process; this guards the remaining case where it returns early with a real error
	if err != nil {
		fmt.Printf("%s\n", err)
	}
}
