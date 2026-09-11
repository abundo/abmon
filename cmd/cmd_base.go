// Package cmdbase provides shared helpers for abmon's check commands.
//
// Author: Anders Lowinger, anders@abundo.se
package cmdbase

import (
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
	kongyaml "github.com/alecthomas/kong-yaml"
)

// ConfigLoader is a kong.ConfigurationLoader for the --config flag shared by
// every check (abmon.CheckOpts.ConfigFile). It wraps kongyaml.Loader so that
// an already-set environment variable wins over a value from the config
// file: kong's own resolver mechanism only skips a flag that was set on the
// command line, not one set from an envar during Reset(), so plain
// kongyaml.Loader would let the config file silently override an envar.
// Kong flag > envar > config file > default is preserved by having this
// resolver decline to answer for any flag whose envar is set.
func ConfigLoader(r io.Reader) (kong.Resolver, error) {
	inner, err := kongyaml.Loader(r)
	if err != nil {
		return nil, err
	}
	return kong.ResolverFunc(func(ctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
		for _, env := range flag.Envs {
			if _, ok := os.LookupEnv(env); ok {
				return nil, nil
			}
		}
		return inner.Resolve(ctx, parent, flag)
	}), nil
}

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
