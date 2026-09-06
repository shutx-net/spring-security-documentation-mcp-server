package mcpserver

import (
	"encoding/json"
	"net/http"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

// NewHTTPHandler builds the HTTP handler serving MCP over Streamable HTTP at
// /mcp plus a /healthz probe.
//
// Stateless is required because consecutive requests may reach different Lambda
// execution environments, so sessions cannot be kept in process. JSONResponse
// makes responses application/json instead of SSE, so a buffered invoke mode is
// enough.
func NewHTTPHandler(st store.Store) http.Handler {
	s := BuildServer(st)
	handler := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server {
		return s
	}, &gomcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})

	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	return mux
}
