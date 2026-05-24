package main

import (
	"fmt"
	"log"
	"os"

	"github.com/shutx-net/spring-security-documentation-mcp-server"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: spring-security-docs-mcp <command>")
		fmt.Fprintln(os.Stderr, "Commands: serve")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		if err := mcp.ServeStdio(); err != nil {
			log.Printf("Server stopped: %v", err)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
