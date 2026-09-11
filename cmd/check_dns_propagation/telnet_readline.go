package main

import (
	"bufio"
	"errors"
	"net"
	"sort"
	"strings"
)

// errQuit is returned by lineEditor.ReadLine when the client sent Ctrl-D
// (EOF) on an empty line, requesting the session be closed.
var errQuit = errors.New("telnet: client requested quit (Ctrl-D)")

// Minimal telnet option negotiation, just enough to switch the client into
// character-at-a-time mode with server-side echo, so Tab completion and
// history can be implemented here instead of relying on the client's own
// line editor.
const (
	tnIAC  = 255
	tnWILL = 251
	tnWONT = 252
	tnDO   = 253
	tnDONT = 254
	tnSB   = 250
	tnSE   = 240

	tnOptEcho = 1
	tnOptSGA  = 3
)

const cliPrompt = "> "

// negotiateTelnet asks the client to stop local echo/line-editing and
// stream raw keystrokes instead.
func negotiateTelnet(conn net.Conn) {
	conn.Write([]byte{tnIAC, tnWILL, tnOptEcho})
	conn.Write([]byte{tnIAC, tnWILL, tnOptSGA})
	conn.Write([]byte{tnIAC, tnDO, tnOptSGA})
}

// lineEditor is a minimal server-side line editor over a raw (character
// mode) telnet connection: backspace, Tab completion and Up/Down history.
// The cursor is always kept at the end of the line; left/right movement
// within the line is not supported.
type lineEditor struct {
	conn    net.Conn
	reader  *bufio.Reader
	buf     []byte
	history []string
	histPos int // index into history while browsing; len(history) == "not browsing"
}

func newLineEditor(conn net.Conn) *lineEditor {
	return &lineEditor{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

// ReadLine blocks until the client submits a line (Enter) and returns it.
// It returns an error when the connection is closed or broken.
func (le *lineEditor) ReadLine() (string, error) {
	le.buf = le.buf[:0]
	le.histPos = len(le.history)
	le.conn.Write([]byte(cliPrompt))
	for {
		b, err := le.reader.ReadByte()
		if err != nil {
			return "", err
		}
		switch {
		case b == tnIAC:
			if err := le.handleTelnetCommand(); err != nil {
				return "", err
			}
		case b == '\r':
			// Swallow a following '\n' or NUL, per telnet NVT newline rules,
			// but only if it's already buffered: a raw-mode client (e.g. the
			// local dev console) sends a bare '\r' with nothing after it, and
			// Peek would otherwise block on the network waiting for a byte
			// that isn't coming until the next keystroke.
			if le.reader.Buffered() > 0 {
				if next, err := le.reader.Peek(1); err == nil && len(next) == 1 && (next[0] == '\n' || next[0] == 0) {
					le.reader.ReadByte()
				}
			}
			return le.submit(), nil
		case b == '\n':
			return le.submit(), nil
		case b == 0x7f || b == 0x08: // DEL or Backspace
			le.backspace()
		case b == 0x04: // Ctrl-D (EOF): quit if the line is empty, like a shell
			if len(le.buf) == 0 {
				return "", errQuit
			}
		case b == '\t', b == '?':
			le.complete()
		case b == 0x1b: // ESC - possible arrow key sequence
			if err := le.handleEscape(); err != nil {
				return "", err
			}
		case b < 0x20:
			// ignore other control characters
		default:
			le.buf = append(le.buf, b)
			le.conn.Write([]byte{b})
		}
	}
}

func (le *lineEditor) submit() string {
	line := string(le.buf)
	le.conn.Write([]byte("\r\n"))
	if s := strings.TrimSpace(line); s != "" {
		if n := len(le.history); n == 0 || le.history[n-1] != line {
			le.history = append(le.history, line)
		}
	}
	return line
}

func (le *lineEditor) backspace() {
	if len(le.buf) == 0 {
		return
	}
	le.buf = le.buf[:len(le.buf)-1]
	le.conn.Write([]byte{0x08, ' ', 0x08})
}

// handleEscape consumes a (possible) ANSI arrow-key sequence: ESC [ A/B/C/D.
func (le *lineEditor) handleEscape() error {
	b1, err := le.reader.ReadByte()
	if err != nil {
		return err
	}
	if b1 != '[' {
		return nil // not an arrow sequence, ignore
	}
	b2, err := le.reader.ReadByte()
	if err != nil {
		return err
	}
	switch b2 {
	case 'A': // Up
		le.historyMove(-1)
	case 'B': // Down
		le.historyMove(1)
	default: // Left/Right/others: unsupported, ignore
	}
	return nil
}

func (le *lineEditor) historyMove(delta int) {
	if len(le.history) == 0 {
		return
	}
	newPos := le.histPos + delta
	if newPos < 0 {
		newPos = 0
	}
	if newPos > len(le.history) {
		newPos = len(le.history)
	}
	le.histPos = newPos
	var newLine string
	if le.histPos < len(le.history) {
		newLine = le.history[le.histPos]
	}
	le.setLine(newLine)
}

// setLine replaces the current line content on screen and in the buffer.
func (le *lineEditor) setLine(s string) {
	oldLen := len(le.buf)
	le.buf = []byte(s)
	pad := ""
	if oldLen > len(s) {
		pad = strings.Repeat(" ", oldLen-len(s))
	}
	le.conn.Write([]byte("\r" + cliPrompt + s + pad + "\r" + cliPrompt + s))
}

// complete performs Tab completion on the current line, using the CLI
// command table in telnet_completion.go.
func (le *lineEditor) complete() {
	line := string(le.buf)
	newLine, candidates := cliComplete(line)
	if len(candidates) > 1 {
		le.conn.Write([]byte("\r\n" + strings.Join(candidates, "  ") + "\r\n" + cliPrompt + newLine))
		le.buf = []byte(newLine)
		return
	}
	if len(candidates) == 0 {
		le.conn.Write([]byte{0x07}) // bell: no match
		return
	}
	if newLine != line {
		le.setLine(newLine)
	}
}

// handleTelnetCommand consumes (and minimally responds to) an IAC sequence
// whose leading IAC byte has already been read.
func (le *lineEditor) handleTelnetCommand() error {
	cmd, err := le.reader.ReadByte()
	if err != nil {
		return err
	}
	switch cmd {
	case tnWILL, tnWONT, tnDO, tnDONT:
		opt, err := le.reader.ReadByte()
		if err != nil {
			return err
		}
		switch cmd {
		case tnDO:
			if opt == tnOptEcho || opt == tnOptSGA {
				le.conn.Write([]byte{tnIAC, tnWILL, opt})
			} else {
				le.conn.Write([]byte{tnIAC, tnWONT, opt})
			}
		case tnWILL:
			le.conn.Write([]byte{tnIAC, tnDONT, opt})
		}
	case tnSB:
		// Discard subnegotiation payload up to IAC SE.
		for {
			bb, err := le.reader.ReadByte()
			if err != nil {
				return err
			}
			if bb != tnIAC {
				continue
			}
			se, err := le.reader.ReadByte()
			if err != nil {
				return err
			}
			if se == tnSE {
				break
			}
		}
	case tnIAC:
		// Escaped 0xFF data byte: treat as a literal character.
		le.buf = append(le.buf, 0xff)
	default:
		// Other single-byte commands (NOP, AYT, ...): ignore.
	}
	return nil
}

// --- Tab completion ---------------------------------------------------

// cliTopCommands and cliSubCommands mirror the command switch in CLI()
// (check_dns_propagation.go) and must be kept in sync with it.
var cliTopCommands = []string{"check", "debug", "help", "loglevel", "quit", "show", "undebug"}

var cliSubCommands = map[string][]string{
	"debug":    {"all", "check", "dnsnode"},
	"undebug":  {"all", "check", "dnsnode"},
	"loglevel": {"error", "warning", "info", "debug"},
	"show":     {"clients", "jobs", "status", "zone", "zones"},
}

func zoneNames() []string {
	if config == nil {
		return nil
	}
	names := make([]string, 0, len(config.Zones))
	for name := range config.Zones {
		if !strings.HasPrefix(name, "__") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func matchPrefix(options []string, prefix string) []string {
	var out []string
	for _, o := range options {
		if strings.HasPrefix(o, prefix) {
			out = append(out, o)
		}
	}
	sort.Strings(out)
	return out
}

func commonPrefix(items []string) string {
	if len(items) == 0 {
		return ""
	}
	p := items[0]
	for _, s := range items[1:] {
		for !strings.HasPrefix(s, p) {
			p = p[:len(p)-1]
			if p == "" {
				return ""
			}
		}
	}
	return p
}

// cliComplete completes the last (possibly empty) token on line as far as
// possible and reports the remaining candidates. len(candidates) > 1 means
// the completion is ambiguous; the caller should list them.
func cliComplete(line string) (string, []string) {
	trailingSpace := strings.HasSuffix(line, " ")
	fields := strings.Fields(line)
	prefix := ""
	if !trailingSpace && len(fields) > 0 {
		prefix = fields[len(fields)-1]
		fields = fields[:len(fields)-1]
	}

	var candidates []string
	switch len(fields) {
	case 0:
		candidates = matchPrefix(cliTopCommands, prefix)
	case 1:
		if fields[0] == "check" {
			candidates = matchPrefix(append([]string{"all"}, zoneNames()...), prefix)
		} else {
			candidates = matchPrefix(cliSubCommands[fields[0]], prefix)
		}
	case 2:
		if fields[0] == "show" && fields[1] == "zone" {
			candidates = matchPrefix(zoneNames(), prefix)
		}
	}

	if len(candidates) == 0 {
		return line, nil
	}
	token := commonPrefix(candidates)
	newFields := append(append([]string{}, fields...), token)
	newLine := strings.Join(newFields, " ")
	if len(candidates) == 1 {
		newLine += " "
	}
	return newLine, candidates
}
