//go:build unix && !linux

package main

import (
	"errors"

	"golang.org/x/sys/unix"
)

// makeRaw is only implemented for linux, where this daemon actually runs.
// On other platforms connectToTelnetServer falls back to unbuffered,
// non-raw stdin handling rather than failing to build.
func makeRaw(fd int) (*unix.Termios, error) {
	return nil, errors.New("raw terminal mode not supported on this platform")
}

func restoreTermios(fd int, state *unix.Termios) error {
	return nil
}
