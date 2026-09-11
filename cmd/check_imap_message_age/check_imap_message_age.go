//
// Connect to an IMAP server, check the age of the oldest message
// Return warning/critical if older than configured value
//
// Can be used to check if a system is polling for email. If it
// stops polling, email piles up on the IMAP server
//
// This is an active check
//
// Author: Anders Lowinger, anders@abundo.se
//
// https://github.com/emersion/go-imap
//

package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"

	"github.com/alecthomas/kong"

	cmdbase "github.com/abundo/abmon/cmd"
	abmon "github.com/abundo/abmon/internal"
)

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
	Host        string `help:"IMAP server" short:"H" required:""`
	Port        int    `help:"Port on IMAP server (default 143, or 993 with --ssl)" short:"p"`
	SSL         bool   `help:"Use IMAPS (default port 993)" short:"S"`
	Username    string `help:"IMAP account username" short:"U"`
	Password    string `help:"IMAP account password" short:"P"`
	Credentials string `help:".INI filename with IMAP account username and password"`
	Folder      string `help:"IMAP folder to check" short:"f" default:"INBOX"`
	// No short flag: -c collides with the shared --config-file flag's -c.
	// (In the original go-flags version, go-flags silently resolved the
	// collision by binding -c to --critical, meaning -c could never be
	// used to set --config-file for this one binary. kong instead rejects
	// the duplicate short flag outright, so -c now unambiguously means
	// --config-file, matching every other abmon check.)
	Critical float64 `help:"Return CRITICAL if oldest message is older than this many seconds" required:""`
	Warning  float64 `help:"Return WARNING if oldest message is older than this many seconds" short:"w" required:""`
	Timeout  int     `help:"Timeout waiting for a response in seconds" short:"t" default:"10"`
}

var opts Opts

// loadCredentials reads username/password from a simple ".INI" style file
// (optional "[section]" header, "key = value" lines)
func loadCredentials(path string) (username, password string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(parts[0])) {
		case "username":
			username = strings.TrimSpace(parts[1])
		case "password":
			password = strings.TrimSpace(parts[1])
		}
	}
	return username, password, nil
}

func checkImapMessageAge(check *abmon.MonitoringCheck) error {
	if opts.Credentials != "" {
		username, password, err := loadCredentials(opts.Credentials)
		if err != nil {
			check.Exit(check.UNKNOWN, fmt.Sprintf("Cannot read credentials file %s: %s", opts.Credentials, err))
		}
		opts.Username = username
		opts.Password = password
	}

	port := opts.Port
	if port == 0 {
		port = 143
		if opts.SSL {
			port = 993
		}
	}
	addr := net.JoinHostPort(opts.Host, strconv.Itoa(port))
	timeout := time.Duration(opts.Timeout) * time.Second
	dialer := &net.Dialer{Timeout: timeout}

	var c *client.Client
	var err error
	if opts.SSL {
		c, err = client.DialWithDialerTLS(dialer, addr, &tls.Config{ServerName: opts.Host})
	} else {
		c, err = client.DialWithDialer(dialer, addr)
	}
	if err != nil {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Could not connect to IMAP server %s: %s", addr, err))
	}
	c.Timeout = timeout
	defer c.Logout()

	if err := c.Login(opts.Username, opts.Password); err != nil {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Authentication failure with username '%s': %s", opts.Username, err))
	}

	mbox, err := c.Select(opts.Folder, true)
	if err != nil {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Could not select folder '%s': %s", opts.Folder, err))
	}

	msg := fmt.Sprintf("IMAP account '%s' folder '%s'", opts.Username, opts.Folder)
	if mbox.Messages == 0 {
		check.Exit(abmon.OK, fmt.Sprintf("%s: No messages found", msg))
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(1, mbox.Messages)

	messages := make(chan *imap.Message, 32)
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchInternalDate}, messages)
	}()

	var oldest time.Time
	count := 0
	for m := range messages {
		count++
		if oldest.IsZero() || m.InternalDate.Before(oldest) {
			oldest = m.InternalDate
		}
	}
	if err := <-done; err != nil {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Could not fetch messages: %s", err))
	}

	if count == 0 || oldest.IsZero() {
		check.Exit(abmon.OK, fmt.Sprintf("%s: No messages", msg))
	}

	age := time.Since(oldest).Seconds()
	msg = fmt.Sprintf("%s: Oldest message is %.0f seconds old", msg, age)
	if age > opts.Critical {
		check.Exit(check.CRITICAL, fmt.Sprintf("%s, > critical limit %.0f seconds", msg, opts.Critical))
	}
	if age > opts.Warning {
		check.Exit(check.WARNING, fmt.Sprintf("%s, > warning limit %.0f seconds", msg, opts.Warning))
	}
	check.Exit(abmon.OK, msg)
	return nil
}

func main() {
	kong.Parse(&opts, kong.Name("check_imap_message_age"), kong.Description("Check the age of the oldest message in an IMAP folder"), kong.Configuration(cmdbase.ConfigLoader))
	cmdbase.Run(func() error {
		check, err := abmon.NewCheck(opts.CheckOpts)
		if err != nil {
			return err
		}
		return checkImapMessageAge(check)
	})
}
