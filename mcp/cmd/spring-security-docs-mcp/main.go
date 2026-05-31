package main

import (
	"os"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
