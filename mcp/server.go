package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/mcpserver"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

// DefaultDBPath returns the default SQLite database path.
func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "spring-security-docs.db"
	}
	return filepath.Join(home, ".local", "share", "spring-security-docs-mcp", "index.db")
}

// ServeStdio opens the default store and runs the MCP server over stdio.
func ServeStdio() error {
	dbPath := DefaultDBPath()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	s := mcpserver.BuildServer(st)
	return s.Run(context.Background(), &gomcp.StdioTransport{})
}
