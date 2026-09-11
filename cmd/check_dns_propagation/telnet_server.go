package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
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
	defer func() {
		TelnetClientsMutex.Lock()
		delete(TelnetClients, conn)
		TelnetClientsMutex.Unlock()
		conn.Close()
	}()
	client := &telnetClient{loglevel: logrus.InfoLevel}
	TelnetClientsMutex.Lock()
	TelnetClients[conn] = client
	TelnetClientsMutex.Unlock()

	reader := bufio.NewReader(conn)
	conn.Write([]byte("-------------------------------------------------------------------\n"))
	conn.Write([]byte("! Welcome to check_dns_propagation. Type help for some light help !\n"))
	conn.Write([]byte("-------------------------------------------------------------------\n"))
	for {
		message, err := reader.ReadString('\n')
		message = strings.TrimSpace(message)
		if err != nil {
			log.Info("Telnet - client disconnected")
			return
		}
		if message == "quit" || message == "q" {
			log.Info("Telnet - client used command quit")
			fmt.Fprintln(conn, "Disconnecting, bye!")
			return
		}
		chanEvent <- Event{Name: message,
			Type:   MessageCLI,
			Client: client,
			Conn:   conn,
		}
	}
}

// Goroutine
// Listens for telnet clients, and starts a goroutine for each
func TelnetServer() {
	// listener, err := net.Listen("tcp", "127.0.0.1:23")
	listener, err := net.Listen("tcp", ":10023")
	if err != nil {
		log.Error("Telnet - Error starting telnet server:", err)
		return
	}
	defer listener.Close()
	log.Info("Telnet server started on port 10023")
	for {
		// Accept a new connection
		conn, err := listener.Accept()
		if err != nil {
			log.Error("Telnet - Error accepting connection:", err)
			continue
		}
		log.Infof("Telnet - new client connected, %s", conn.RemoteAddr())
		go HandleTelnetClient(conn)
	}
}
