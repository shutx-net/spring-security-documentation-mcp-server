package cli

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/mcpserver"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

func newServeHTTPCmd() *cobra.Command {
	var (
		port   int
		dbPath string
	)

	cmd := &cobra.Command{
		Use:   "serve-http",
		Short: "Start MCP server (Streamable HTTP transport)",
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
			handler := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server {
				return s
			}, nil)

			mux := http.NewServeMux()
			mux.Handle("/mcp", handler)
			mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			})

			addr := fmt.Sprintf(":%d", port)
			log.Printf("MCP HTTP server listening at %s", addr)
			return http.ListenAndServe(addr, mux)
		},
	}
	cmd.Flags().IntVar(&port, "port", 8080, "HTTP port to listen on")
	cmd.Flags().StringVar(&dbPath, "db", "", "path to SQLite database")
	return cmd
}
