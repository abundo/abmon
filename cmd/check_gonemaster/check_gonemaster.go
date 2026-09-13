//
// Check a zone for errors using gonemaster
// This is an passive check, triggered by cron
//
// This is a passive check.
// The result is sent to icinga (REST API) and/or nagios (Command file)
//
// https://codeberg.org/pawal/gonemaster
//

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	cmdbase "github.com/abundo/abmon/cmd"
	abmon "github.com/abundo/abmon/internal"

	"github.com/alecthomas/kong"
)

// gonemaster severity levels, least to most severe
var StrToLevel = map[string]int{
	"INFO":     0,
	"NOTICE":   1,
	"WARNING":  2,
	"ERROR":    3,
	"CRITICAL": 4,
}

// Maps the worst gonemaster level seen to a nagios/icinga plugin return code
// (0=OK, 1=WARNING, 2=CRITICAL); gonemaster has more levels than nagios has
// return codes, so INFO/NOTICE both count as OK and ERROR/CRITICAL both
// count as CRITICAL.
var levelToReturnCode = map[string]int{
	"INFO":     0,
	"NOTICE":   0,
	"WARNING":  1,
	"ERROR":    2,
	"CRITICAL": 2,
}

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
	Zones   []string `help:"Zones to check. If not specified, checks all" name:"zone" short:"z"`
	Exclude []string `help:"Zones to exclude" name:"exclude" short:"e" default:"[]"`
}

var config *abmon.ConfigFile

var opts Opts

type GonemasterResultTest struct {
	Args      map[string]any `json:"args"`
	Level     string
	Message   string
	Module    string
	Tag       string
	Testcase  string
	Timestamp float32
}

func GonemasterCheck(check *abmon.MonitoringCheck) error {
	// Fail fast with one clear message instead of repeating an "executable
	// not found" error for every zone below.
	if _, err := exec.LookPath("gonemaster"); err != nil {
		return fmt.Errorf("gonemaster not found: %w", err)
	}

	// Check that all specified zones are valid
	if opts.Zones != nil {
		for _, zone := range opts.Zones {
			_, found := config.Zones[zone]
			if !found {
				fmt.Printf("Unknown zone %s\n", zone)
				os.Exit(1)
			}
		}
	}

	var checkErr error
	for zonename, zone := range config.Zones {
		if !strings.HasPrefix(zonename, "_") {
			if !zone.DnsCheckGonemaster {
				continue
			}
			if opts.Zones != nil {
				var found bool
				for _, name := range opts.Zones {
					if zonename == name {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			fmt.Println("-------------------------------------------------------------------------------")
			fmt.Printf("Checking zone %s\n", zonename)

			// map of tests/modules to exclude, populated verbatim from the
			// zone config: either a full "module/testcase" name or just a
			// module name to exclude all its testcases
			testToExclude := make(map[string]bool)
			for test := range zone.Exclude {
				testToExclude[test] = true
			}

			cmdArgs := []string{"--json", zonename}
			slog.Debug(fmt.Sprint("cmdArgs:", cmdArgs))
			cmd := exec.Command("gonemaster", cmdArgs...)
			cmdOutput, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Error running gonemaster for zone %s: %s, %s\n", zonename, err, cmdOutput)
				checkErr = err
				continue
			}
			var results []GonemasterResultTest
			err = json.Unmarshal(cmdOutput, &results)
			if err != nil {
				fmt.Printf("Error decoding gonemaster JSON response for zone %s: %s\n", zonename, err)
				checkErr = err
				continue
			}

			var worstLevel string
			var worstOrdinal = -1
			var output []string
			status := abmon.IcingaStatus{
				Timestamp:          time.Now().Unix(),
				Hostname:           "Zone - " + zonename,
				ServiceDescription: "DNS-Gonemaster",
			}

			for _, result := range results {
				if testToExclude[result.Module] || testToExclude[result.Module+"/"+result.Testcase] {
					continue
				}
				ordinal, found := StrToLevel[result.Level]
				if found {
					if ordinal > worstOrdinal {
						worstOrdinal = ordinal
						worstLevel = result.Level
					}
				} else {
					slog.Info(fmt.Sprintf("Unknown level %s", result.Level))
				}
				r := fmt.Sprintf("level %s, module %s, tag %s", result.Level, result.Testcase, result.Tag)
				if len(result.Args) > 0 {
					var args []string
					for key, val := range result.Args {
						args = append(args, fmt.Sprintf("%s=%s", key, val))
					}
					r = r + " " + strings.Join(args, ", ")
				}
				output = append(output, r)
			}
			if len(output) > 1 {
				output[0] += "  More..."
			}
			status.Output = strings.Join(output, "\\n")
			status.ReturnCode = levelToReturnCode[worstLevel]

			abmon.Notify(config, status)
		}
	}
	return checkErr
}

func main() {
	kong.Parse(&opts, kong.Name("check_gonemaster"), kong.Description("Check a zone for errors using gonemaster"), kong.Configuration(cmdbase.ConfigLoader), kong.Vars{"version": abmon.VersionString()})
	check, err := abmon.NewCheck(opts.CheckOpts)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	slog.Debug(fmt.Sprintf("opts: %+v", opts))
	config = check.Config
	if err := GonemasterCheck(check); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	fmt.Println("\n-------------------------------------------------------------------------------")
	fmt.Println("All done")
}
