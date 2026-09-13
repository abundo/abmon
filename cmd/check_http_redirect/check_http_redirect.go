//
// Check if an URL does a redirect (code 301 or 302), and that the
// redirect goes to the correct URL
//
// This is an active check
//
// Author: Anders Lowinger, anders@abundo.se
//

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alecthomas/kong"

	cmdbase "github.com/abundo/abmon/cmd"
	abmon "github.com/abundo/abmon/internal"
)

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
	Host    string `help:"Host to check" short:"H" required:""`
	IPv4    bool   `help:"Connect using IPv4" short:"4"`
	IPv6    bool   `help:"Connect using IPv6" short:"6"`
	URL     string `help:"URL to retrieve" short:"U" required:""`
	Redir   string `help:"Expected redirect URL" short:"R" required:""`
	Timeout int    `help:"Timeout waiting for a response in seconds" short:"t" default:"10"`
}

var opts Opts

func checkHTTPRedirect(check *abmon.MonitoringCheck) error {
	u, err := url.Parse(opts.URL)
	if err != nil {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Invalid URL: %s", err))
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Cannot handle scheme %s", u.Scheme))
	}
	if u.Host == "" {
		check.Exit(check.UNKNOWN, "No network location specified")
	}

	// Force IPv4 or IPv6 for the connection. If neither is specified, it is
	// up to the OS to choose
	network := "tcp"
	switch {
	case opts.IPv4:
		network = "tcp4"
	case opts.IPv6:
		network = "tcp6"
	}

	timeout := time.Duration(opts.Timeout) * time.Second
	dialer := &net.Dialer{Timeout: timeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{ServerName: u.Hostname()},
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		// Don't follow the redirect ourselves, we want to inspect it
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Connect to --host (which may be a bare IP), but send the Host header
	// (and TLS SNI) for --url's network location, so a specific backend can
	// be probed for a name-based virtual host
	hostport := opts.Host
	if _, _, err := net.SplitHostPort(hostport); err != nil {
		port := "80"
		if u.Scheme == "https" {
			port = "443"
		}
		hostport = net.JoinHostPort(hostport, port)
	}
	reqURL := *u
	reqURL.Host = hostport

	req, err := http.NewRequest(http.MethodHead, reqURL.String(), nil)
	if err != nil {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Cannot build request: %s", err))
	}
	req.Host = u.Host

	resp, err := client.Do(req)
	if err != nil {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Exception: %s", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently && resp.StatusCode != http.StatusFound {
		check.Exit(check.CRITICAL, fmt.Sprintf("No redirect returned, got status %d", resp.StatusCode))
	}

	location := resp.Header.Get("Location")
	if location == "" {
		check.Exit(check.CRITICAL, "No redirect header")
	}
	if location != opts.Redir {
		check.Exit(check.CRITICAL, fmt.Sprintf("Redirect to wrong URL: got '%s' expected '%s'", location, opts.Redir))
	}

	msg := fmt.Sprintf("%s OK: HTTP/%d.%d %d", strings.ToUpper(u.Scheme), resp.ProtoMajor, resp.ProtoMinor, resp.StatusCode)
	switch resp.StatusCode {
	case http.StatusMovedPermanently:
		msg += " Moved permanently"
	case http.StatusFound:
		msg += " Found/Moved temporarily"
	}
	check.Exit(abmon.OK, fmt.Sprintf("%s. Redirect to %s", msg, location))
	return nil
}

func main() {
	kong.Parse(&opts, kong.Name("check_http_redirect"), kong.Description("Check that an URL redirects to the expected location"), kong.Configuration(cmdbase.ConfigLoader), kong.Vars{"version": abmon.VersionString()})
	cmdbase.Run(func() error {
		check, err := abmon.NewCheck(opts.CheckOpts)
		if err != nil {
			return err
		}
		return checkHTTPRedirect(check)
	})
}
