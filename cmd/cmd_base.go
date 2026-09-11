// Package cmdbase provides shared helpers for abmon's check commands.
//
// Author: Anders Lowinger, anders@abundo.se
package cmdbase

import (
	"fmt"
	"os"
)

// Run executes fn and, if it returns an error, prints a single concise
// "Error: ..." message to stderr and exits with status 1.
//
// Mirrors the fix in https://github.com/abundo/dnsmgr2/commit/2f5d1f69152ffdc6d527c242a8eb70c60ee980b2:
// a failing command should print one clear error line, not also dump usage
// or other framework noise. Here that means every check's main() reports
// its startup/runtime errors through Run instead of silently calling
// os.Exit(1) (as several checks used to) or hand-rolling its own
// log+os.Exit pair. --help and CLI parsing errors are unaffected: kong
// prints those itself and exits before Run's function body ever runs.
func Run(fn func() error) {
	if err := fn(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
