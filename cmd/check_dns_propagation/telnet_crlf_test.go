package main

import (
	"testing"
	"time"
)

func TestCRLFConnTranslatesBareLF(t *testing.T) {
	server, client := tcpPipe(t)
	defer server.Close()
	defer client.Close()

	wrapped := newCRLFConn(server)

	tests := []struct {
		in   string
		want string
	}{
		{"127.0.0.1, &{debugFlag:false}\n", "127.0.0.1, &{debugFlag:false}\r\n"},
		{"already\r\nfine\r\n", "already\r\nfine\r\n"},
		{"no newline here", "no newline here"},
		{"multi\nline\nmessage\n", "multi\r\nline\r\nmessage\r\n"},
	}

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4096)
	for _, tt := range tests {
		if _, err := wrapped.Write([]byte(tt.in)); err != nil {
			t.Fatalf("Write(%q): %v", tt.in, err)
		}
		n, err := client.Read(buf)
		if err != nil {
			t.Fatalf("Read after Write(%q): %v", tt.in, err)
		}
		got := string(buf[:n])
		if got != tt.want {
			t.Errorf("Write(%q) produced %q, want %q", tt.in, got, tt.want)
		}
	}
}
