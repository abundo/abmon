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
	"os"
	"time"

	"github.com/miekg/dns"

	log "github.com/sirupsen/logrus"

	"github.com/alecthomas/kong"

	abmon "github.com/abundo/abmon/internal"
)

const (
	RRSIG_DFORMAT = "%Y%m%d%H%M%S" // date+time format on RRSIG Resource Record
)

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
	Zone string `help:"Zone to transfer" short:"z"`
	// Zonefile string  `help:"read zone from file instead of AXFR"`
	// No short flag: -c collides with the shared --config-file flag's -c.
	// (In the original go-flags version, go-flags silently resolved the
	// collision by binding -c to --critical, meaning -c could never be
	// used to set --config-file for this one binary. kong instead rejects
	// the duplicate short flag outright, so -c now unambiguously means
	// --config-file, matching every other abmon check.)
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
	kong.Parse(&opts, kong.Name("check_rrsig_expiry"), kong.Description("Check age of RRSIG records in a zone fetched via AXFR"))

	// Load configuration
	check, err := abmon.NewCheck(opts.CheckOpts)
	if err != nil {
		log.Debug(err)
		os.Exit(abmon.UNKNOWN)
	}
	config = check.Config // shortcut
	zone, ok := config.Zones[opts.Zone]
	if !ok {
		check.Exit(check.UNKNOWN, "No or unknown zone")
	}
	err = CheckRRSIGExpiry(check, zone)
	if err != nil {
		log.Error(err)
	}
}

/*
class Check_Rrsig_Expiry(m_util.Plugin_Check):

    def check(self, args):
        oldest_rrsig_expiration = datetime.timedelta(days=999999)
        now = datetime.datetime.now().replace(microsecond=0)

        cmd = 'dig'
        cmd += ' +nottlid'                          # Exclude TTL
        if self.args.tsig:
            cmd += " -k %s" % self.args.tsig
        cmd += " @%s" % self.args.host
        cmd += " -q %s" % self.args.zone
        cmd += " -t AXFR"
        if self.args.zonefile:
            cmd = 'zcat %s' % self.args.zonefile
        cmd += ' | grep -i "IN[[:space:]]RRSIG"'    # filter out RRSIG RR
        cmd += ' | tr "\t" " "'                       # replace all tabs->spaces
        cmd += ' | tr -s " "'                       # replace repeated spaces with one
        cmd += ' | m_util. -d " " -f 1,8,9'             # extract name and two date fields
        if self.args.verbose: print("cmd :", cmd)
        p = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, bufsize=65536, shell=True)
        rrsig_count = 0
        for line in p.stdout:
            line = line.decode().lower()
            tmp = line.split()
            if len(tmp) != 3:
                print("Unknown RRSIG format in line:", file=sys.stderr)
                print("  %s" % line, file=sys.stderr)
                continue
            rrsig_count += 1
            try:
                expiration = datetime.datetime.strptime(tmp[1], RRSIG_DFORMAT)
                inception = datetime.datetime.strptime(tmp[2], RRSIG_DFORMAT)
            except ValueError:
                print("Unknown date format in line:", file=sys.stderr)
                print("  %s" % line, file=sys.stderr)
                continue

            len_before_expire = expiration - now
            if len_before_expire < oldest_rrsig_expiration:
                oldest_rrsig_expiration = len_before_expire
                # print("%s | %s" % (tmp[0], oldest_rrsig_expiration), file=sys.stderr)
            time.sleep(PACING_SLEEP)

        if self.args.verbose: print("Found %i RRSIG records" % rrsig_count)
        if rrsig_count < 1:
            abmon.Reply.exit(m_util.CRITICAL, "no signatures found")

        oldest_rrsig_expiration_sec = oldest_rrsig_expiration.days * 86400 + oldest_rrsig_expiration.seconds
        oldest_rrsig_expiration_days = oldest_rrsig_expiration_sec / 86400

        if oldest_rrsig_expiration_days < 0:
            abmon.Reply.exit(m_util.CRITICAL, "signatures has expired")

        if oldest_rrsig_expiration_days <= args.critical:
            abmon.Reply.exit(m_util.CRITICAL, "some signatures will expire in %0.1f days" % oldest_rrsig_expiration_days)

        if oldest_rrsig_expiration_days < args.warning:
            abmon.Reply.exit(m_util.WARNING, "some signatures will expire in %.1f days" % oldest_rrsig_expiration_days)

        abmon.Reply.exit(m_util.OK, "minimum signature expire in %.1f days\n" % oldest_rrsig_expiration_days)

*/
