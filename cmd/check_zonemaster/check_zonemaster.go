//
// Check a zone for errors using zonemaster-cli
//
// This is an passive check, triggered by cron
//
//
// Zonemaster-cli is quite slow, so this is implemented purely as a passive
// check. The result is sent to icinga (REST API) and/or nagios (Command file)
//
// zonemaster-cli is executed using docker, simplifying installation and
// keeping it updated. Each run checks and pulls the latest release
//
// background: how to run zonemaster/cli in docker
// https://github.com/zonemaster/zonemaster/blob/master/docs/public/using/cli.md
//

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	abmon "github.com/abundo/abmon/internal"
	log "github.com/sirupsen/logrus"

	"github.com/alecthomas/kong"
)

// Zonemaster error codes
var StrToLevel = map[string]int{
	"NOTICE":  0,
	"WARNING": 1,
	"ERROR":   2,
}

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
	Zones   []string `help:"Zones to check. If not specified, checks all" name:"zone" short:"z"`
	Exclude []string `help:"Zones to exclude" name:"exclude" short:"e" default:"[]"`
}

var config *abmon.ConfigFile

var opts Opts
var tests []string
var modules map[string][]string

// Get list of all tests avaliable.
func getAllTests() error {
	// Note: --list_tests command doesn't support --json, parse each line
	cmd := exec.Command("/usr/bin/docker", "run", "-t", "--pull", "always", "--network", "host", "--rm", "zonemaster/cli", "--list_tests")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error gettting list of tests:", err, output)
		return err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	tests = make([]string, 0)
	modules = make(map[string][]string)
	var state int
	var module string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		switch state {
		case 0:
			if strings.HasPrefix(line, "Status:") {
				state = 1
			}
		case 1:
			if line[0] != ' ' {
				module = strings.TrimSpace(line)
				modules[module] = make([]string, 1)
			} else {
				line = strings.TrimSpace(line)
				tests = append(tests, module+"/"+line)
				modules[module] = append(modules[module], module+"/"+line)
			}
		}
	}
	return nil
}

type ZonemasterResultTest struct {
	Args      map[string]any `json:"args"`
	Level     string
	Message   string
	Module    string
	Tag       string
	Testcase  string
	Timestamp float32
}
type ZonemasterResult struct {
	Results []ZonemasterResultTest `json:"results"`
}

func ZonemasterCheck(check *abmon.MonitoringCheck) error {
	var err error
	var zoneResult ZonemasterResult

	err = getAllTests()
	if err != nil {
		log.Fatal(err)
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

	for zonename, zone := range config.Zones {
		if !strings.HasPrefix(zonename, "_") {
			// fmt.Println(zonename, zone)
			if !zone.DnsCheckZonemaster {
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
			cmdArgs := []string{"run", "-t", "--network", "host", "--rm", "zonemaster/cli", "--json", "--show-testcase", "--show-module"}

			// create map of tests to exclude
			testToExclude := make(map[string]int)
			for test := range zone.Exclude {
				if strings.Contains(test, "/") {
					// Exclude one test
					testToExclude[test] = 1

				} else {
					// Exclude all tests in a module
					for _, tmp := range modules[test] {
						testToExclude[tmp] = 1
					}
				}
			}

			var found bool
			for _, test := range tests {
				_, found = testToExclude[test]
				if !found {
					cmdArgs = append(cmdArgs, "--test", test)
				}
			}

			cmdArgs = append(cmdArgs, zonename)
			log.Debug("cmdArgs:", cmdArgs)
			cmd := exec.Command("/usr/bin/docker", cmdArgs...)
			cmdOutput, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Error running zonemaster-cli test: %s, %s", err, cmdOutput)
				return err
			}
			err = json.Unmarshal(cmdOutput, &zoneResult)
			if err != nil {
				fmt.Printf("Error decoding zonemaster-cli JSON response: %s\n", err)
				continue
			}

			var return_code int
			var output []string
			status := abmon.IcingaStatus{
				Timestamp:          time.Now().Unix(),
				Hostname:           "Zone - " + zonename,
				ServiceDescription: "DNS-Zonemaster",
			}

			for _, result := range zoneResult.Results {
				tmp, found := StrToLevel[result.Level]
				if found {
					if tmp > return_code {
						return_code = tmp
					}
				} else {
					log.Printf("Unknown level %s", result.Level)
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
			status.ReturnCode = return_code

			abmon.Notify(config, status)
		}
	}
	return nil
}

func main() {
	kong.Parse(&opts, kong.Name("check_zonemaster"), kong.Description("Check a zone for errors using zonemaster-cli"))
	check, err := abmon.NewCheck(opts.CheckOpts)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	log.Debugf("opts: %+v\n", opts)
	config = check.Config
	ZonemasterCheck(check)

	fmt.Println("\n-------------------------------------------------------------------------------")
	fmt.Println("All done")
}
