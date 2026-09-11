//
// Check that authentication against a RADIUS server works, by sending an
// Access-Request (PAP) with a username/password and verifying the server
// replies with Access-Accept
//
// The password is read from the environment variable RADIUS_PASSWORD, never
// from the command line, so it doesn't leak through the process list or
// shell history
//
// This is an active check
//
// Author: Anders Lowinger, anders@abundo.se
//

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"layeh.com/radius"
	"layeh.com/radius/rfc2865"

	"github.com/alecthomas/kong"

	cmdbase "github.com/abundo/abmon/cmd"
	abmon "github.com/abundo/abmon/internal"
)

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
	Host     string `help:"RADIUS server hostname or IP" short:"H" required:""`
	Port     int    `help:"RADIUS server port" short:"P" default:"1812"`
	Username string `help:"RADIUS username" short:"u" required:""`
	Secret   string `help:"RADIUS shared secret" short:"s" required:""`
	Timeout  int    `help:"Timeout waiting for a response in seconds" short:"t" default:"10"`
}

var opts Opts

func checkRadiusAuth(check *abmon.MonitoringCheck) error {
	password := os.Getenv("RADIUS_PASSWORD")
	if password == "" {
		check.Exit(check.UNKNOWN, "No environment variable RADIUS_PASSWORD set")
	}

	packet := radius.New(radius.CodeAccessRequest, []byte(opts.Secret))
	if err := rfc2865.UserName_SetString(packet, opts.Username); err != nil {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Failed to set RADIUS username: %s", err))
	}
	if err := rfc2865.UserPassword_SetString(packet, password); err != nil {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Failed to set RADIUS password: %s", err))
	}

	timeout := time.Duration(opts.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	addr := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))
	response, err := radius.Exchange(ctx, packet, addr)
	if err != nil {
		check.Exit(check.CRITICAL, fmt.Sprintf("RADIUS authentication failed for user %s: %s", opts.Username, err))
	}

	if response.Code != radius.CodeAccessAccept {
		check.Exit(check.CRITICAL, fmt.Sprintf("RADIUS authentication failed for user %s: received %s", opts.Username, response.Code))
	}

	check.Exit(abmon.OK, fmt.Sprintf("RADIUS check OK for user %s", opts.Username))
	return nil
}

func main() {
	kong.Parse(&opts, kong.Name("check_radius_auth"), kong.Description("Check RADIUS authentication"))
	cmdbase.Run(func() error {
		check, err := abmon.NewCheck(opts.CheckOpts)
		if err != nil {
			return err
		}
		return checkRadiusAuth(check)
	})
}
