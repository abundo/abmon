package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
)

// connectToTelnetServer is the "local CLI" used during development: when
// stdout is a TTY, main() dials the daemon's own telnet server instead of
// requiring a separate telnet client in another terminal.
//
// The server (telnet_readline.go) negotiates character-at-a-time mode and
// does its own line editing (echo, backspace, Tab/? completion, history),
// so this bridge puts the local terminal into raw mode and forwards every
// keystroke immediately, and strips telnet IAC negotiation bytes coming
// back from the server before printing to stdout.
func connectToTelnetServer() {
	conn, err := net.Dial("tcp", "localhost:10023")
	if err != nil {
		fmt.Println("Error connecting to local telnet server:", err)
		return
	}
	defer conn.Close()

	fd := int(os.Stdin.Fd())
	oldState, err := makeRaw(fd)
	if err != nil {
		fmt.Println("Warning: could not set local terminal to raw mode, falling back to line mode:", err)
	} else {
		defer restoreTermios(fd, oldState)
	}

	go relayStdinToConn(conn)
	relayConnToStdout(conn)
}

// relayStdinToConn forwards raw stdin bytes to conn one at a time, so
// keystrokes (Tab, ?, arrows, backspace) reach the server's line editor
// immediately instead of only after Enter.
func relayStdinToConn(conn net.Conn) {
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if _, werr := conn.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// relayConnToStdout copies conn to stdout, discarding telnet IAC
// negotiation sequences so they don't show up as garbage on the console.
func relayConnToStdout(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			fmt.Println("Disconnected from server.")
			return
		}
		if b != tnIAC {
			os.Stdout.Write([]byte{b})
			continue
		}
		if err := skipTelnetIACSequence(reader); err != nil {
			return
		}
	}
}

// skipTelnetIACSequence consumes a telnet command sequence whose leading
// IAC byte has already been read, without writing anything to stdout.
func skipTelnetIACSequence(r *bufio.Reader) error {
	cmd, err := r.ReadByte()
	if err != nil {
		return err
	}
	switch cmd {
	case tnWILL, tnWONT, tnDO, tnDONT:
		_, err := r.ReadByte() // option byte
		return err
	case tnSB:
		for {
			bb, err := r.ReadByte()
			if err != nil {
				return err
			}
			if bb != tnIAC {
				continue
			}
			se, err := r.ReadByte()
			if err != nil {
				return err
			}
			if se == tnSE {
				return nil
			}
		}
	case tnIAC:
		// Escaped 0xFF data byte: pass it through as a literal character.
		os.Stdout.Write([]byte{0xff})
		return nil
	default:
		// Other single-byte commands (NOP, AYT, ...): nothing more to read.
		return nil
	}
}
