package main

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
)

var TelnetClients = make(map[net.Conn]*telnetClient)
var TelnetClientsMutex sync.Mutex

func writeToAllTelnetClients(message string) {
	TelnetClientsMutex.Lock()
	defer TelnetClientsMutex.Unlock()
	for conn, client := range TelnetClients {
		if client.debugFlag && DebugCheck > 0 {
			conn.Write([]byte(message))
		}
	}
}

func HandleTelnetClient(conn net.Conn) {
	conn = newCRLFConn(conn)
	defer func() {
		TelnetClientsMutex.Lock()
		delete(TelnetClients, conn)
		TelnetClientsMutex.Unlock()
		conn.Close()
	}()
	client := &telnetClient{loglevel: slog.LevelInfo}
	TelnetClientsMutex.Lock()
	TelnetClients[conn] = client
	TelnetClientsMutex.Unlock()

	negotiateTelnet(conn)
	le := newLineEditor(conn)
	conn.Write([]byte("-------------------------------------------------------------------\n"))
	conn.Write([]byte("! Welcome to check_dns_propagation. Type help for some light help !\n"))
	conn.Write([]byte("! Tab/? completes commands, Up/Down history, Ctrl-D quits         !\n"))
	conn.Write([]byte("-------------------------------------------------------------------\n"))
	for {
		message, err := le.ReadLine()
		message = strings.TrimSpace(message)
		if err == errQuit {
			slog.Info("Telnet - client used Ctrl-D to quit")
			fmt.Fprintln(conn, "Disconnecting, bye!")
			return
		}
		if err != nil {
			slog.Info("Telnet - client disconnected")
			return
		}
		if message == "quit" || message == "q" {
			slog.Info("Telnet - client used command quit")
			fmt.Fprintln(conn, "Disconnecting, bye!")
			return
		}
		done := make(chan struct{})
		chanEvent <- Event{Name: message,
			Type:   MessageCLI,
			Client: client,
			Conn:   conn,
			Done:   done,
		}
		// Wait for CLI() to finish writing its output before looping back
		// to ReadLine, so the next prompt isn't printed ahead of it.
		<-done
	}
}

// Listens for telnet clients, and starts a goroutine for each
func TelnetServer() {
	// listener, err := net.Listen("tcp", "127.0.0.1:23")
	listener, err := net.Listen("tcp", ":10023")
	if err != nil {
		slog.Error(fmt.Sprint("Telnet - Error starting telnet server:", err))
		return
	}
	defer listener.Close()
	slog.Info("Telnet server started on port 10023")
	for {
		// Accept a new connection
		conn, err := listener.Accept()
		if err != nil {
			slog.Error(fmt.Sprint("Telnet - Error accepting connection:", err))
			continue
		}
		slog.Info(fmt.Sprintf("Telnet - new client connected, %s", conn.RemoteAddr()))
		go HandleTelnetClient(conn)
	}
}
