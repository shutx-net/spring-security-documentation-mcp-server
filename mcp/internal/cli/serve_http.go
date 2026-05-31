package cli

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/mcpserver"
)

func newServeHTTPCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "serve-http",
		Short: "Start MCP server (Streamable HTTP transport)",
		Long: `Start the MCP server over Streamable HTTP.

Reads from CHUNKS_TABLE for the DynamoDB chunks table name.
Standard AWS credential/region environment variables are also respected.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openAWSStore(cmd.Context())
			if err != nil {
				return err
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
	return cmd
}
