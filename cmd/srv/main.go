package main

import (
	"flag"
	"fmt"
	"os"

	"srv.exe.dev/srv"
)

var flagPort = flag.String("port", "8080", "port to listen on")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func run() error {
	flag.Parse()
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	server, err := srv.New("db.sqlite3", hostname)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}
	return server.Serve(*flagPort)
}
