package main

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestCliComplete(t *testing.T) {
	oldConfig := config
	config = nil
	defer func() { config = oldConfig }()

	tests := []struct {
		in         string
		wantLine   string
		wantNCands int
	}{
		{"sh", "show ", 1},
		{"show ", "show ", 5},
		{"show z", "show zone", 2},
		{"debug ch", "debug check ", 1},
		{"quit", "quit ", 1},
		{"check a", "check all ", 1},
		{"bogus", "bogus", 0},
	}
	for _, tt := range tests {
		gotLine, gotCands := cliComplete(tt.in)
		if gotLine != tt.wantLine {
			t.Errorf("cliComplete(%q) line = %q, want %q", tt.in, gotLine, tt.wantLine)
		}
		if len(gotCands) != tt.wantNCands {
			t.Errorf("cliComplete(%q) candidates = %v, want %d of them", tt.in, gotCands, tt.wantNCands)
		}
	}
}

// tcpPipe returns a pair of connected TCP loopback connections, so writes
// don't need to interleave exactly with the peer's reads (unlike net.Pipe).
func tcpPipe(t *testing.T) (server, client net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	acceptCh := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		acceptCh <- c
	}()

	client, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	server = <-acceptCh
	if server == nil {
		t.Fatal("accept failed")
	}
	return server, client
}

func TestLineEditorReadLine(t *testing.T) {
	server, client := tcpPipe(t)
	defer server.Close()
	defer client.Close()

	le := newLineEditor(server)
	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := le.ReadLine()
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- line
	}()

	// "sh" + Tab completes to "show ", then Enter submits it.
	client.Write([]byte("sh\t\r\n"))

	select {
	case line := <-resultCh:
		if line != "show " {
			t.Fatalf("ReadLine() = %q, want %q", line, "show ")
		}
	case err := <-errCh:
		t.Fatalf("ReadLine error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ReadLine result")
	}

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4096)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("reading server output: %v", err)
	}
	out := string(buf[:n])
	if !strings.Contains(out, "show ") {
		t.Errorf("server output %q does not contain completed line %q", out, "show ")
	}
}

func TestLineEditorCtrlDOnEmptyLineQuits(t *testing.T) {
	server, client := tcpPipe(t)
	defer server.Close()
	defer client.Close()

	le := newLineEditor(server)
	errCh := make(chan error, 1)
	go func() {
		_, err := le.ReadLine()
		errCh <- err
	}()

	client.Write([]byte{0x04}) // Ctrl-D on an empty line

	select {
	case err := <-errCh:
		if err != errQuit {
			t.Fatalf("ReadLine() error = %v, want errQuit", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ReadLine result")
	}
}

func TestLineEditorCtrlDMidLineIsIgnored(t *testing.T) {
	server, client := tcpPipe(t)
	defer server.Close()
	defer client.Close()

	le := newLineEditor(server)
	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := le.ReadLine()
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- line
	}()

	// Ctrl-D with text already on the line should not quit; it's ignored
	// and the line can still be completed normally.
	client.Write([]byte{'h', 'e', 0x04, 'l', 'p', '\r', '\n'})

	select {
	case line := <-resultCh:
		if line != "help" {
			t.Fatalf("ReadLine() = %q, want %q", line, "help")
		}
	case err := <-errCh:
		t.Fatalf("ReadLine error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ReadLine result")
	}
}

func TestLineEditorQuestionMarkCompletes(t *testing.T) {
	server, client := tcpPipe(t)
	defer server.Close()
	defer client.Close()

	le := newLineEditor(server)
	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := le.ReadLine()
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- line
	}()

	// "sh" + ? completes to "show ", same as Tab, then Enter submits it.
	client.Write([]byte("sh?\r\n"))

	select {
	case line := <-resultCh:
		if line != "show " {
			t.Fatalf("ReadLine() = %q, want %q", line, "show ")
		}
	case err := <-errCh:
		t.Fatalf("ReadLine error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ReadLine result")
	}
}

func TestLineEditorBackspaceAndHistory(t *testing.T) {
	server, client := tcpPipe(t)
	defer server.Close()
	defer client.Close()

	le := newLineEditor(server)

	// First line: "help", submitted normally, becomes history[0].
	firstDone := make(chan struct{})
	go func() {
		le.ReadLine()
		close(firstDone)
	}()
	client.Write([]byte("help\r\n"))
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	drain(t, client)
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first ReadLine to finish")
	}

	// Second line: type "xyz", backspace it all away, then Up-arrow recalls
	// "help" from history, then Enter submits it.
	resultCh := make(chan string, 1)
	go func() {
		line, _ := le.ReadLine()
		resultCh <- line
	}()
	client.Write([]byte("xyz"))
	drain(t, client)
	client.Write([]byte{0x7f, 0x7f, 0x7f}) // backspace x3
	drain(t, client)
	client.Write([]byte{0x1b, '[', 'A'}) // Up arrow
	drain(t, client)
	client.Write([]byte("\r\n"))

	select {
	case line := <-resultCh:
		if line != "help" {
			t.Fatalf("ReadLine() after history recall = %q, want %q", line, "help")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ReadLine result")
	}
}

func TestNegotiateTelnetBytes(t *testing.T) {
	server, client := tcpPipe(t)
	defer server.Close()
	defer client.Close()

	negotiateTelnet(server)

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("reading negotiation bytes: %v", err)
	}
	want := []byte{tnIAC, tnWILL, tnOptEcho, tnIAC, tnWILL, tnOptSGA, tnIAC, tnDO, tnOptSGA}
	got := buf[:n]
	if string(got) != string(want) {
		t.Fatalf("negotiateTelnet bytes = % x, want % x", got, want)
	}
}

// A real telnet client replies to our option offers with its own IAC
// sequences (e.g. IAC DO ECHO, IAC WONT SUPPRESS-GO-AHEAD) interleaved with
// typed characters. handleTelnetCommand must consume those in-band without
// corrupting the line buffer or hanging.
func TestLineEditorConsumesClientNegotiationReplies(t *testing.T) {
	server, client := tcpPipe(t)
	defer server.Close()
	defer client.Close()

	le := newLineEditor(server)
	resultCh := make(chan string, 1)
	go func() {
		line, _ := le.ReadLine()
		resultCh <- line
	}()

	client.Write([]byte{tnIAC, tnDO, tnOptEcho})
	client.Write([]byte("he"))
	client.Write([]byte{tnIAC, tnWONT, tnOptSGA})
	client.Write([]byte("lp"))
	client.Write([]byte{tnIAC, tnSB, 24, 0, 'x', 'x', tnIAC, tnSE}) // discardable subnegotiation
	client.Write([]byte("\r\n"))

	select {
	case line := <-resultCh:
		if line != "help" {
			t.Fatalf("ReadLine() = %q, want %q", line, "help")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ReadLine result")
	}
}

func drain(t *testing.T, conn net.Conn) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 4096)
	conn.Read(buf)
}
