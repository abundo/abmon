//
// Check free addresses in BECS DHCP scopes
//
// Optionally (re)writes an icinga2 configuration file with one passive
// service check per scope, and sends the current utilization to icinga
// as a passive check result.
//
// This is a passive check, triggered by cron
//
// Author: Anders Lowinger, anders@abundo.se
//

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"

	cmdbase "github.com/abundo/abmon/cmd"
	abmon "github.com/abundo/abmon/internal"
)

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
	Scopes       []string `help:"DHCP scopes to check, as configured in abmon.yaml. If not specified, checks all" short:"s"`
	Exclude      []string `help:"DHCP scopes to exclude" short:"e"`
	FreeWarning  int      `help:"Generate WARNING if number of free addresses is below" name:"free_warning" default:"20"`
	FreeCritical int      `help:"Generate CRITICAL if number of free addresses is below" name:"free_critical" default:"5"`
	Icinga       bool     `help:"Write icinga2 config file and send result as a passive check"`
	IcingaFile   string   `help:"Path to the generated icinga2 config file" name:"icinga_config_file"`
	IcingaHost   string   `help:"host.name to use in the generated icinga2 'assign where' clause" name:"icinga_host"`
}

var opts Opts

// DHCPScopeUtilization holds the address usage for one DHCP scope
type DHCPScopeUtilization struct {
	Name     string
	Total    int
	Assigned int
	Free     int
}

// BecsClient talks to the BECS EAPI (JSON-RPC) to fetch DHCP scope
// utilization. See becs.go for the implementation.
type BecsClient interface {
	DHCPScopeUtilization(oid string) (*DHCPScopeUtilization, error)
	Logout()
}

// writeIcingaConfig (re)generates the icinga2 configuration file with one
// passive "DHCP scope free addresses" service per scope, and reloads icinga2
// if the file changed
func writeIcingaConfig(filename, icingaHost string, scopes []DHCPScopeUtilization) error {
	var b strings.Builder
	for _, s := range scopes {
		fmt.Fprintf(&b, "apply Service \"DHCP Scope %s\" {\n", s.Name)
		b.WriteString("  import \"dhcp-scope-free-addresses\"\n")
		fmt.Fprintf(&b, "  assign where host.name == \"%s\"\n", icingaHost)
		b.WriteString("}\n\n")
	}

	tmpFile := filename + ".tmp"
	if err := os.WriteFile(tmpFile, []byte(b.String()), 0644); err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	if _, err := os.Stat(filename); err == nil {
		same, err := abmon.FileCmp(tmpFile, filename)
		if err != nil {
			return err
		}
		if same {
			// Nothing changed, no need to install the new file / reload icinga
			return nil
		}
	}

	if err := abmon.CopyFile(tmpFile, filename); err != nil {
		return err
	}
	abmon.ReloadIcinga()
	return nil
}

// sendIcingaResults sends one passive check result per scope to icinga
func sendIcingaResults(config *abmon.ConfigFile, scopes []DHCPScopeUtilization) {
	timestamp := time.Now().Unix()
	for _, s := range scopes {
		if s.Total == 0 {
			// no prefix in scope, ignore
			continue
		}
		status := abmon.OK
		if s.Free < opts.FreeCritical {
			status = abmon.CRITICAL
		} else if s.Free < opts.FreeWarning {
			status = abmon.WARNING
		}
		status_desc := fmt.Sprintf("DHCP Scope %s", s.Name)
		output := fmt.Sprintf("%d free addresses, %d assigned addresses", s.Free, s.Assigned)
		fmt.Printf("[%d] %s;%s;%s\n", timestamp, status_desc, abmon.ErrstatToStr[status], output)
		abmon.Notify(config, abmon.IcingaStatus{
			Timestamp:          timestamp,
			Hostname:           opts.IcingaHost,
			ServiceDescription: status_desc,
			ReturnCode:         status,
			Output:             output,
		})
	}
}

func checkBecsDhcpScope(check *abmon.MonitoringCheck) error {
	config := check.Config

	names := opts.Scopes
	if len(names) == 0 {
		names = abmon.SortedKeys(config.DhcpScopes)
	}
	exclude := make(map[string]bool)
	for _, e := range opts.Exclude {
		exclude[e] = true
	}

	becs, err := newBecsClient(config)
	if err != nil {
		check.Exit(check.UNKNOWN, err.Error())
	}
	defer becs.Logout()

	var results []DHCPScopeUtilization
	overall := abmon.OK
	for _, name := range names {
		if exclude[name] {
			continue
		}
		scope, ok := config.DhcpScopes[name]
		if !ok {
			check.Exit(check.UNKNOWN, fmt.Sprintf("No or unknown DHCP scope '%s'", name))
		}
		util, err := becs.DHCPScopeUtilization(scope.OID)
		if err != nil {
			check.Exit(check.UNKNOWN, fmt.Sprintf("Cannot get DHCP scope utilization for '%s': %s", name, err))
		}
		util.Name = name
		results = append(results, *util)

		status := abmon.OK
		if util.Free < opts.FreeCritical {
			status = check.CRITICAL
		} else if util.Free < opts.FreeWarning {
			status = check.WARNING
		}
		if status > overall {
			overall = status
		}
		check.Result.AddDetail(fmt.Sprintf("%s: %d free, %d assigned, %d total", name, util.Free, util.Assigned, util.Total))
	}

	if opts.Icinga {
		if opts.IcingaFile != "" {
			if err := writeIcingaConfig(opts.IcingaFile, opts.IcingaHost, results); err != nil {
				check.Exit(check.UNKNOWN, fmt.Sprintf("Cannot write icinga config: %s", err))
			}
		}
		sendIcingaResults(config, results)
	}

	check.Exit(overall, fmt.Sprintf("%d DHCP scope(s) checked", len(results)))
	return nil
}

func main() {
	kong.Parse(&opts, kong.Name("check_becs_dhcp_scope"), kong.Description("Check free addresses in BECS DHCP scopes"), kong.Configuration(cmdbase.ConfigLoader))
	cmdbase.Run(func() error {
		check, err := abmon.NewCheck(opts.CheckOpts)
		if err != nil {
			return err
		}
		return checkBecsDhcpScope(check)
	})
}
