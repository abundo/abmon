package main

import (
	"bytes"
	"net"
)

// crlfConn wraps a net.Conn and rewrites bare '\n' to '\r\n' on write.
// The telnet NVT protocol requires CR LF as the line terminator; ordinary
// interactive telnet clients translate a lone '\n' locally so this often
// goes unnoticed, but a raw vty/console does not, producing a staircase
// effect. Wrapping the connection once here fixes every Fprintln/Fprintf
// call site without having to touch each one individually.
type crlfConn struct {
	net.Conn
}

func newCRLFConn(conn net.Conn) net.Conn {
	return &crlfConn{Conn: conn}
}

func (c *crlfConn) Write(p []byte) (int, error) {
	if !bytes.Contains(p, []byte{'\n'}) {
		return c.Conn.Write(p)
	}
	out := make([]byte, 0, len(p)+16)
	for i, b := range p {
		if b == '\n' && (i == 0 || p[i-1] != '\r') {
			out = append(out, '\r')
		}
		out = append(out, b)
	}
	n, err := c.Conn.Write(out)
	if n > len(p) {
		n = len(p)
	}
	if err == nil {
		n = len(p)
	}
	return n, err
}
