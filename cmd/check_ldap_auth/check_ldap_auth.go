//
// Check that authentication against an LDAP server works, by connecting and
// performing a bind with a username/password
//
// The password is read from the environment variable LDAP_PASSWORD, never
// from the command line, so it doesn't leak through the process list or
// shell history
//
// This is an active check
//
// Author: Anders Lowinger, anders@abundo.se
//

package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/alecthomas/kong"

	cmdbase "github.com/abundo/abmon/cmd"
	abmon "github.com/abundo/abmon/internal"
)

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
	Host     string `help:"LDAP server, as an URL (ldap://host:389 or ldaps://host:636)" short:"H" required:""`
	Username string `help:"LDAP bind username (DN)" short:"u" required:""`
	Timeout  int    `help:"Timeout waiting for a response in seconds" short:"t" default:"10"`
}

var opts Opts

func checkLdapAuth(check *abmon.MonitoringCheck) error {
	password := os.Getenv("LDAP_PASSWORD")
	if password == "" {
		check.Exit(check.UNKNOWN, "No environment variable LDAP_PASSWORD set")
	}

	timeout := time.Duration(opts.Timeout) * time.Second
	l, err := ldap.DialURL(opts.Host, ldap.DialWithDialer(&net.Dialer{Timeout: timeout}))
	if err != nil {
		check.Exit(check.CRITICAL, fmt.Sprintf("LDAP check error: %s", err))
	}
	defer l.Close()
	l.SetTimeout(timeout)

	if err := l.Bind(opts.Username, password); err != nil {
		check.Exit(check.CRITICAL, fmt.Sprintf("LDAP authentication failed: %s", err))
	}

	check.Exit(abmon.OK, "LDAP authentication successful")
	return nil
}

func main() {
	kong.Parse(&opts, kong.Name("check_ldap_auth"), kong.Description("Check LDAP authentication"), kong.Configuration(cmdbase.ConfigLoader), kong.Vars{"version": abmon.VersionString()})
	cmdbase.Run(func() error {
		check, err := abmon.NewCheck(opts.CheckOpts)
		if err != nil {
			return err
		}
		return checkLdapAuth(check)
	})
}
