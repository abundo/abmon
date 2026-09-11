//
// Listen on DNS notify from primary/hidden/dist DNS nodes
// Measure propagation time, notifications from secondary DNS servers
// If zone is provided to DnsNode, check if the zone is distributed to all anycast nodes
//
// This is a passive check, running as a daemon
//

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	cmdbase "github.com/abundo/abmon/cmd"
	abmon "github.com/abundo/abmon/internal"
	dnsnode "github.com/abundo/dnsnode"
	"github.com/alecthomas/kong"
	"github.com/mattn/go-isatty"
	"github.com/miekg/dns"
)

type telnetClient struct {
	debugFlag bool
	loglevel  slog.Level
}

// Check CLI Options
type Opts struct {
	abmon.CheckOpts
}

var opts Opts
var check *abmon.MonitoringCheck
var config *abmon.ConfigFile
var debug bool

// var telnetClients = make(map[net.Conn]*telnetClient)
// var telnetClientsMutex sync.Mutex
var debugFlag uint32

// Info of running job
type RunningJob struct {
	Name string
}

// Tracks running jobs/goroutines
var jobs = map[string]*RunningJob{}

const (
	DebugCheck   = 1 << 0
	DebugDnsnode = 1 << 1
)

// Event loop message types
const (
	MessageNew = iota
	MessageDone
	MessageCLI
)

type Event struct {
	Name   string
	Serial uint32
	Type   int    // new, done, cli
	Error  string // if non-empty, error message
	Client *telnetClient
	Conn   net.Conn
	Done   chan struct{} // for MessageCLI, closed once CLI() has finished writing its output
}

// todo, handle huge amount of events better, limit number of workers?
var chanEvent = make(chan Event, 500)

// Fetch SOA serial for a zone
// Returns 0 if error
func get_soa_serial(zonename string, master string) (uint32, error) {
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(zonename), dns.TypeSOA)
	resp, _, err := c.Exchange(m, master+":53")
	if err != nil {
		return 0, err
	}
	for _, answer := range resp.Answer {
		if soa, ok := answer.(*dns.SOA); ok {
			return soa.Serial, nil
		}
	}
	return 0, errors.New("No SOA serial number foudn")
}

// Fetch NS records for a zone
func get_ns(zonename string, master string) ([]*dns.NS, error) {
	var ns_list []*dns.NS
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(zonename), dns.TypeNS)
	resp, _, err := c.Exchange(m, master+":53")
	if err != nil {
		slog.Warn(fmt.Sprint("Failed to query SOA:", err))
		return nil, err
	}
	for _, answer := range resp.Answer {
		if ns, ok := answer.(*dns.NS); ok {
			ns_list = append(ns_list, ns)
			slog.Debug(fmt.Sprintf("Found NS %s", ns))
		}
	}
	return ns_list, nil
}

func setSitePropagation(sites map[string]int, key string, val int) {
	if val < 0 {
		_, ok := sites[key]
		if !ok {
			sites[key] = 0
		}
	} else {
		sites[key] = val
	}
}

// Goroutine
// Called when a zone has a new serial number
// Get target serial number from the primary nameserver
// Poll all nameservers. If it is a dnsnode nameserver, use the Dnsnode API
// Polls until all nameservers has the correct serial number
// Report errors to Icinga/Nagios
func checkZone(event Event) {
	var err error
	sites := make(map[string]int)

	zonename := event.Name
	slog.Info(fmt.Sprintf("%s start checkZone", zonename))
	event.Type = MessageDone

	zone, ok := config.Zones[zonename]
	if !ok {
		event.Error = "Unknown zone"
		chanEvent <- event
		return
	}
	primary := zone.Primary
	event.Serial, err = get_soa_serial(event.Name, primary)
	if err != nil {
		slog.Warn(err.Error())
		return
	}
	slog.Info(fmt.Sprintf("%s Got SOA serial %d from %s\n", zonename, event.Serial, primary))

	ns_list, err := get_ns(zonename, primary)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	var dnsnodeClient *dnsnode.DnsNodeClient
	if zone.DnsNode {
		var param = dnsnode.DnsNodeParam{
			Token: config.Dnsnode.Token,
			URL:   config.Dnsnode.URL,
			Debug: debugFlag&DebugDnsnode > 0,
		}
		dnsnodeClient = dnsnode.New(param)
	}

	// check for 60 seconds, every 4th second, if the new serial has propagated
	var wrongSerial bool
	icingaStatus := abmon.IcingaStatus{
		Timestamp:          time.Now().Unix(),
		Hostname:           "Zone - " + event.Name,
		ServiceDescription: "DNS-propagation",
	}
	var output []string
	var ix int
	var status *dnsnode.ResponseType // .DnsNodeResponse
	for ix = 1; ix <= 75; ix++ {
		if debugFlag&DebugCheck > 0 {
			slog.Debug(fmt.Sprintf("%s Checking try %d\n", zonename, ix))
			slog.Debug(abmon.StructToString(output))
		}
		output = []string{}

		m := fmt.Sprintf("Master %s SOA serial %d", primary, event.Serial)
		output = append(output, m)

		wrongSerial = false

		if zone.DnsNode {
			status, err = dnsnodeClient.Status(event.Name)
			if err != nil {
				event.Error = err.Error()
				chanEvent <- event
				return
			}
			for _, distmaster := range status.DistPrimaries {
				setSitePropagation(sites, distmaster.IPv4Address, -1)
				if distmaster.Serial != event.Serial {
					m := fmt.Sprintf("Distmaster %s serial %d", distmaster.IPv4Address, distmaster.Serial)
					output = append(output, m)
					wrongSerial = true
					setSitePropagation(sites, distmaster.IPv4Address, ix)
				}
			}
			for _, site := range status.Sites {
				setSitePropagation(sites, site.Name, -1)
				if site.Serial != event.Serial {
					m := fmt.Sprintf("Site %s serial %d", site.Name, site.Serial)
					output = append(output, m)
					wrongSerial = true
					setSitePropagation(sites, site.Name, ix)
				}
			}
		} else {
			// Zone is not hosted at DnsNode, poll each nameserver directly instead
			for _, ns := range ns_list {
				nsHost := strings.TrimSuffix(ns.Ns, ".")
				setSitePropagation(sites, nsHost, -1)
				serial, err := get_soa_serial(zonename, nsHost)
				if err != nil {
					m := fmt.Sprintf("NS %s query error: %s", nsHost, err)
					output = append(output, m)
					wrongSerial = true
					setSitePropagation(sites, nsHost, ix)
					continue
				}
				if serial != event.Serial {
					m := fmt.Sprintf("NS %s serial %d", nsHost, serial)
					output = append(output, m)
					wrongSerial = true
					setSitePropagation(sites, nsHost, ix)
				}
			}
		}

		if !wrongSerial {
			break
		}
		time.Sleep(4 * time.Second)
	}

	if ix > 1 {
		// We only send performance data if the first try didn't already have the correct serial
		// It's probably a "check <zone_name>" command, instead of reception of a Notify
		if zone.DnsNode && status != nil {
			for _, site := range status.Sites {
				var m string
				if event.Serial == site.Serial {
					m = fmt.Sprintf("'%s delay'=%d;;;;", site.Name, site.Timestamp-status.CurrentTimestamp)
					icingaStatus.PerformanceData = append(icingaStatus.PerformanceData, m)
				} else {
					m = fmt.Sprintf("%s incorrect serial %d\n", site.Name, site.Serial)
					_ = m
				}
			}
		}

		for site, val := range sites {
			m := fmt.Sprintf("'%s API-ResponseTime'=%d;;;;", site, val*4)
			icingaStatus.PerformanceData = append(icingaStatus.PerformanceData, m)
		}
	}
	if debugFlag&DebugCheck > 0 {
		slog.Info(abmon.StructToString(output))
	}
	icingaStatus.Output = strings.Join(output, "\n")
	if wrongSerial {
		icingaStatus.ReturnCode = check.WARNING
	}

	abmon.Notify(config, icingaStatus)
	slog.Info(fmt.Sprintf("%s end of checkZone, return code %d\n", zonename, icingaStatus.ReturnCode))
	chanEvent <- event
}

func debugFlagsToString() string {
	var s []string
	if debugFlag&DebugCheck > 0 {
		s = append(s, "check")
	}
	if debugFlag&DebugDnsnode > 0 {
		s = append(s, "dnsnode")
	}
	return strings.Join(s, ",")
}

// Process CLI commands
// Called from eventloop. Do not run long tasks here
func CLI(conn net.Conn, client *telnetClient, cmd string) {
	if cmd == "" {
		return
	}
	args := strings.Split(cmd, " ")
	switch args[0] {
	case "check":
		if len(args) == 2 {
			if args[1] == "all" {
				for zonename, zone := range config.Zones {
					if zone.DnsNode {
						fmt.Fprintf(conn, "%s check all\n", zonename)
						chanEvent <- Event{Name: zonename, Type: MessageNew}
					}
				}
			} else {
				chanEvent <- Event{Name: args[1], Type: MessageNew}
			}
		}
	case "debug", "undebug":
		var value uint32
		var bit uint32
		if len(args) < 2 {
			fmt.Fprintln(conn, "ERROR: missing what to (un)debug")
			return
		}
		if args[0] == "debug" {
			value = 1
		}

		switch args[1] {
		case "all":
			if value > 0 {
				debugFlag = DebugCheck | DebugDnsnode
			} else {
				debugFlag = 0
			}
		case "check":
			bit = DebugCheck
		case "dnsnode":
			bit = DebugDnsnode
		}
		if bit > 0 {
			debugFlag = debugFlag &^ bit
			debugFlag = debugFlag | (value << (bit - 1))
		}
		fmt.Fprintf(conn, "Debug set to: %s\n", debugFlagsToString())
	case "loglevel":
		if len(args) < 2 {
			fmt.Fprintln(conn, "ERROR: unknown command")
			return
		}
		level, ok := abmon.LogLevels[args[1]]
		if ok {
			client.loglevel = level
			fmt.Fprintf(conn, "Loglevel set to: %s\n", args[1])
		} else {
			fmt.Fprintf(conn, "Unknown loglevel: %s\n", args[1])
		}

	case "help":
		fmt.Fprintln(conn, "Commands:")
		fmt.Fprintln(conn, "  check <zone>")
		fmt.Fprintln(conn, "  check all")
		fmt.Fprintln(conn, "  debug <all|check|dnsnode>")
		fmt.Fprintln(conn, "  undebug <all|check|dnsnode>")
		fmt.Fprintln(conn, "  help")
		fmt.Fprintln(conn, "  loglevel <error|warning|info|debug")
		fmt.Fprintln(conn, "  show clients")
		fmt.Fprintln(conn, "  show jobs")
		fmt.Fprintln(conn, "  show status")
		fmt.Fprintln(conn, "  show zone <name>")
		fmt.Fprintln(conn, "  show zones")
		fmt.Fprintln(conn, "  quit")
		fmt.Fprintln(conn, "Tab or ? completes commands, Up/Down arrows recall history, Ctrl-D quits")
	case "quit":
		fmt.Fprintln(conn, "ERROR: Not implemented yet")
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(conn, "ERROR: unknown command")
		} else {
			switch args[1] {
			case "clients":
				for tmpConn, tmpClient := range TelnetClients {
					fmt.Fprintf(conn, "%s, %+v\n", tmpConn.RemoteAddr(), tmpClient)
				}
			case "jobs":
				fmt.Fprintln(conn, "----- Current jobs -----")
				for key, job := range jobs {
					fmt.Fprintf(conn, "%s - %+v\n", key, job)
				}
			case "status":
				fmt.Fprintf(conn, "  Loglevel: %s\n", client.loglevel)
				fmt.Fprintf(conn, "  Debug: %s\n", debugFlagsToString())
				fmt.Fprintf(conn, "%s\n", abmon.StructToString(opts))
				fmt.Fprintf(conn, "%d zones loaded\n", len(config.Zones))
			case "zone":
				if len(args) < 3 {
					fmt.Fprintln(conn, "Missing name")
				} else {
					name := args[2]
					zone, ok := config.Zones[name]
					if ok {
						fmt.Fprintf(conn, "%s\n", abmon.StructToString(zone))
					} else {
						fmt.Fprintf(conn, "No zone %s defined\n", name)
					}
				}
			case "zones":
				fmt.Fprintln(conn, "----- Loaded zones -----")
				for _, zone := range config.Zones {
					if !strings.HasPrefix(zone.Name, "__") {
						fmt.Fprintf(conn, "%s\n", abmon.StructToString(zone))
					}
				}
			default:
				fmt.Fprintf(conn, "Error: unknown command %s\n", args[1:])
			}
		}
	default:
		fmt.Fprintf(conn, "Error: Unknown command: %s\n", cmd)
	}
}

// Goroutine
// Receive DNS notify messages, if one we are interested in, send to event loop
func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	if r.Opcode == dns.OpcodeNotify {
		for _, q := range r.Question {
			name := strings.TrimSuffix(q.Name, ".")
			slog.Info(fmt.Sprintf("Zone: %s Received DNS NOTIFY\n", name))
			zone, ok := config.Zones[name]
			if ok && zone.DnsCheckPropagation && zone.DnsNode {
				chanEvent <- Event{Name: name, Type: MessageNew}
			} else {
				slog.Info(fmt.Sprintf("%s Not an DnsNode zone, ignoring", name))
			}
		}
	}

	// Respond to the NOTIFY message
	m := new(dns.Msg)
	m.SetReply(r)
	w.WriteMsg(m)
}

// Goroutine
// Main eventloop
// Create new checks
// Handle result from checks
// Handle CLI
func eventLoop() {
	slog.Debug("Starting Eventloop")
	for {
		event, open := <-chanEvent
		if !open {
			// channel is closed, probably exiting program
			break
		}
		if debug {
			slog.Debug(fmt.Sprintf("Eventloop rx message: %+v\n", event))
		}
		switch event.Type {

		case MessageNew:
			// Is there a check in process?
			checkProcess, ok := jobs[event.Name]
			if ok {
				// abort check? queue check? result can be misleading
				slog.Warn(fmt.Sprintf("%s check is already running\n", event.Name))
			} else {
				_ = checkProcess
				jobs[event.Name] = &RunningJob{Name: event.Name}

				// start new goroutine
				go checkZone(Event{Name: event.Name})
			}

		case MessageDone:
			checkProcess, ok := jobs[event.Name]
			_ = checkProcess
			if ok {
				// Remove
				delete(jobs, event.Name)
			} else {
				slog.Error(fmt.Sprintf("INTERNAL ERROR: 'done' from check %s\n", event.Name))
			}
		case MessageCLI:
			CLI(event.Conn, event.Client, event.Name)
			if event.Done != nil {
				close(event.Done)
			}
		default:
			slog.Error(fmt.Sprintf("Unknown event action: %d", event.Type))
		}
	}
}

// Wraps another slog.Handler, additionally broadcasting every log record to
// each connected telnet client (replaces the logrus AddHook mechanism)
type telnetLogHandler struct {
	out slog.Handler
}

func (h *telnetLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= abmon.LogLevel.Level()
}

func (h *telnetLogHandler) Handle(ctx context.Context, r slog.Record) error {
	if err := h.out.Handle(ctx, r); err != nil {
		return err
	}

	message := fmt.Sprintf("%s %s %s\n", r.Time.Format("2006-01-02 15:04:05"), r.Level, r.Message)
	TelnetClientsMutex.Lock()
	defer TelnetClientsMutex.Unlock()
	for conn, client := range TelnetClients {
		if r.Level >= client.loglevel {
			conn.Write([]byte(message))
		}
	}
	return nil
}

func (h *telnetLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &telnetLogHandler{out: h.out.WithAttrs(attrs)}
}

func (h *telnetLogHandler) WithGroup(name string) slog.Handler {
	return &telnetLogHandler{out: h.out.WithGroup(name)}
}

// connectToTelnetServer is defined in telnet_devclient.go

func main() {
	var err error

	kong.Parse(&opts, kong.Name("check_dns_propagation"), kong.Description("Listen for DNS NOTIFY and measure propagation time"), kong.Configuration(cmdbase.ConfigLoader))

	check, err = abmon.NewCheck(opts.CheckOpts)
	if err != nil {
		os.Exit(1)
	}
	config = check.Config

	// Enable Telnet server
	go TelnetServer()

	// Only enable local CLI if there is an TTY
	localConsole := isatty.IsTerminal(os.Stdout.Fd())
	stdout := io.Writer(os.Stdout)
	if localConsole {
		stdout = io.Discard // avoid duplicate log outputs
	}
	slog.SetDefault(slog.New(&telnetLogHandler{
		out: slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: abmon.LogLevel}),
	}))

	if localConsole {
		go func() {
			connectToTelnetServer()
			// The local dev console disconnected (e.g. Ctrl-D); exit the
			// whole process rather than leaving it running headless.
			os.Exit(0)
		}()
	}

	// Start the eventloop
	go eventLoop()

	// Create a new DNS server, listening on port 1053
	server := &dns.Server{Addr: ":1053", Net: "udp"}
	dns.HandleFunc(".", handleDNSRequest)

	slog.Info("Starting DNS server on port 1053...")
	err = server.ListenAndServe()
	defer server.Shutdown()
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to start server: %s\n", err.Error()))
		os.Exit(1)
	}
}
