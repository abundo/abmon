//go:build linux

package main

import "golang.org/x/sys/unix"

// makeRaw puts fd into character-at-a-time, unechoed mode (like
// golang.org/x/term.MakeRaw) and returns the previous state so it can be
// restored. Needed so connectToTelnetServer can forward every keystroke to
// the telnet server immediately instead of only after Enter, matching what
// a real raw-mode telnet client does.
func makeRaw(fd int) (*unix.Termios, error) {
	oldState, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}
	newState := *oldState
	newState.Iflag &^= unix.BRKINT | unix.ICRNL | unix.INPCK | unix.ISTRIP | unix.IXON
	newState.Oflag &^= unix.OPOST
	newState.Lflag &^= unix.ECHO | unix.ICANON | unix.IEXTEN | unix.ISIG
	newState.Cflag = (newState.Cflag &^ unix.CSIZE) | unix.CS8
	newState.Cc[unix.VMIN] = 1
	newState.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &newState); err != nil {
		return nil, err
	}
	return oldState, nil
}

func restoreTermios(fd int, state *unix.Termios) error {
	return unix.IoctlSetTermios(fd, unix.TCSETS, state)
}
