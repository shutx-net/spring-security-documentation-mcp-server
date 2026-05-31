package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/mcpserver"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

func newServeCmd() *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start MCP server (stdio transport)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbPath == "" {
				dbPath = defaultDBPath()
			}
			if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
				return fmt.Errorf("create db dir: %w", err)
			}
			st, err := store.Open(dbPath)
			if err != nil {
				return fmt.Errorf("open store at %s: %w", dbPath, err)
			}
			defer st.Close()

			s := mcpserver.BuildServer(st)
			return s.Run(context.Background(), &gomcp.StdioTransport{})
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "path to SQLite database (default: $HOME/.local/share/spring-security-docs-mcp/index.db)")
	return cmd
}

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "spring-security-docs.db"
	}
	return filepath.Join(home, ".local", "share", "spring-security-docs-mcp", "index.db")
}
