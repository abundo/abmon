//
// Fetch a zone using AXFR.
// Go through all RRSIG records and verify that none of them are too old or expired
//
// This is a active check
//
// Author: Anders Lowinger, anders@abundo.se
//

package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/miekg/dns"

	"github.com/alecthomas/kong"

	cmdbase "github.com/abundo/abmon/cmd"
	abmon "github.com/abundo/abmon/internal"
)

const (
	RRSIG_DFORMAT = "%Y%m%d%H%M%S" // date+time format on RRSIG Resource Record
)

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
	Zone     string  `help:"Zone to transfer" short:"z"`
	Critical float64 `help:"Minimim age in days on RRSIG before critical" default:"5.0"`
	Warning  float64 `help:"Minimim age in days on RRSIG before warning" short:"w" default:"8.0"`
}

var config *abmon.ConfigFile
var opts Opts

// Fetch zone using AXFR
// Return all RRSIG
func get_rrsig(zone *abmon.ConfigZone) (*[]dns.RR, error) {
	t := new(dns.Transfer)
	if zone.TSIG != nil {
		t.TsigSecret = map[string]string{zone.TSIG.Key: zone.TSIG.Secret}
	}
	m := new(dns.Msg)
	m.SetAxfr(zone.Name + ".")
	if zone.TSIG != nil {
		m.SetTsig(zone.TSIG.Key, zone.TSIG.Algorithm+".", 300, time.Now().Unix())
	}
	c, err := t.In(m, zone.Primary+":53")
	if err != nil {
		return nil, err
	}
	rrsigs := new([]dns.RR)
	for channel := range c {
		_ = err
		for _, rr := range channel.RR {
			_ = err
			if rr.Header().Rrtype == dns.TypeRRSIG {
				*rrsigs = append(*rrsigs, rr)
			}
		}
	}
	return rrsigs, nil
}

// Read the RRSIG from a zonefile
// Return array of RRISG
//
// Format on RRSIG
// example.com. IN RRSIG DS 5 2 3600 20150124203956 20150112001201 44410 se. <snip keys>
func get_rrsig_zonefile(zone *abmon.ConfigZone) (*[]dns.RR, error) {
	return nil, nil
}

// Get the RRSIGs for a zone. Check age of all RRSIGs
func CheckRRSIGExpiry(check *abmon.MonitoringCheck, zone *abmon.ConfigZone) error {
	now := time.Now().Unix()
	warning := now + int64(opts.Warning*86400)
	critical := now + int64(opts.Critical*86400)
	// validate parameters
	if check.Opts.WarningAs <= check.Opts.CriticalAs {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Makes no sense with warning %f days <= critical %f days",
			opts.Warning, opts.Critical))
	}
	rrsigs, err := get_rrsig(zone)
	if err != nil {
		return err
	}
	for _, rr := range *rrsigs {
		rrsig := rr.(*dns.RRSIG)
		fmt.Printf("%s %s %d %d\n", rrsig.Hdr.Name, dns.TypeToString[rrsig.Hdr.Rrtype], rrsig.Inception, rrsig.Expiration)
		expiration := int64(rrsig.Expiration)
		fmt.Printf("    time to expire %d, time to critical %d, time to warning %d\n",
			(expiration-now)/3600, (expiration-critical)/3600, (expiration-warning)/3600)
		if expiration < critical {
			fmt.Println("critical")
		} else {
			if expiration < warning {
				fmt.Println("warning")
			}
		}

	}
	return nil
}

func main() {
	kong.Parse(&opts, kong.Name("check_rrsig_expiry"), kong.Description("Check age of RRSIG records in a zone fetched via AXFR"), kong.Configuration(cmdbase.ConfigLoader))

	// Load configuration
	check, err := abmon.NewCheck(opts.CheckOpts)
	if err != nil {
		slog.Debug(err.Error())
		os.Exit(abmon.UNKNOWN)
	}
	config = check.Config // shortcut
	zone, ok := config.Zones[opts.Zone]
	if !ok {
		check.Exit(check.UNKNOWN, "No or unknown zone")
	}
	err = CheckRRSIGExpiry(check, zone)
	if err != nil {
		slog.Error(err.Error())
	}
}
