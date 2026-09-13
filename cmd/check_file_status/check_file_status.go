//
// Check a status file written by another check (or any periodic job),
// typically run by cron.
//
//  1. Check the age of the file. If older than the configured limits,
//     return WARNING/CRITICAL
//  2. Inspect the first line of the file, looking for the text OK, WARNING,
//     CRITICAL or UNKNOWN, and return that status
//  3. Include the file content in the check output, so nagios/icinga can
//     show it
//
// This is a passive check helper: it reports on the *result* of another
// job, it does not probe a live target itself.
//
// Author: Anders Lowinger, anders@abundo.se
//

package main

import (
	"bufio"
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
	File        string  `help:"File to check" short:"f" required:""`
	AgeWarning  float64 `help:"Max age on file in hours, before WARNING" name:"age_warning" default:"48"`
	AgeCritical float64 `help:"Max age on file in hours, before CRITICAL" name:"age_critical" default:"36"`
}

var opts Opts

func checkFileStatus(check *abmon.MonitoringCheck) error {
	fi, err := os.Stat(opts.File)
	if err != nil {
		if os.IsNotExist(err) {
			check.Exit(check.UNKNOWN, fmt.Sprintf("File '%s' does not exist", opts.File))
		}
		check.Exit(check.UNKNOWN, fmt.Sprintf("Cannot get modified time for file %s: %s", opts.File, err))
	}

	age := time.Since(fi.ModTime())
	ageHours := age.Hours()
	if ageHours > opts.AgeCritical {
		check.Exit(check.CRITICAL, fmt.Sprintf("File %s last modified %0.2f hours ago, older than limit %0.2f hours",
			opts.File, ageHours, opts.AgeCritical))
	}
	if ageHours > opts.AgeWarning {
		check.Exit(check.WARNING, fmt.Sprintf("File %s last modified %0.2f hours ago, older than limit %0.2f hours",
			opts.File, ageHours, opts.AgeWarning))
	}

	f, err := os.Open(opts.File)
	if err != nil {
		check.Exit(check.UNKNOWN, fmt.Sprintf("Cannot open file %s, is the path correct?", opts.File))
	}
	defer f.Close()

	// Look at the first line of the file, and use it as the returned status,
	// if it starts with one of "ok", "warning", "critical" or "unknown"
	status := check.UNKNOWN
	var firstLine string
	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		firstLine = strings.TrimSpace(scanner.Text())
		tmp := strings.ToLower(firstLine)
		for key, val := range abmon.StrToErrstat {
			if strings.HasPrefix(tmp, key) {
				status = val
				break
			}
		}
	}

	msg := fmt.Sprintf("File %s last modified %0.2f hours ago", opts.File, ageHours)
	if firstLine != "" {
		msg += "\n" + firstLine
	}
	check.Exit(status, msg)
	return nil
}

func main() {
	kong.Parse(&opts, kong.Name("check_file_status"), kong.Description("Check the age and status of a file written by another job"), kong.Configuration(cmdbase.ConfigLoader), kong.Vars{"version": abmon.VersionString()})
	cmdbase.Run(func() error {
		check, err := abmon.NewCheck(opts.CheckOpts)
		if err != nil {
			return err
		}
		return checkFileStatus(check)
	})
}
