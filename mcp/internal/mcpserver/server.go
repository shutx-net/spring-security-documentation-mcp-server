package mcpserver

import (
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

// BuildServer creates the MCP server with all tools registered.
func BuildServer(st store.Store) *gomcp.Server {
	s := gomcp.NewServer(&gomcp.Implementation{
		Name:    "spring-security-docs",
		Version: "0.1.0",
	}, nil)

	gomcp.AddTool(s, &gomcp.Tool{
		Name:        "search_spring_security_docs",
		Description: "Search Spring Security reference documentation by keyword or concept. Returns matching documentation chunks with titles, content, and source URLs.",
	}, newSearchHandler(st))

	gomcp.AddTool(s, &gomcp.Tool{
		Name:        "get_spring_security_doc",
		Description: "Get a single Spring Security documentation chunk by its ID. Returns the full content including markdown and metadata.",
	}, newGetHandler(st))

	gomcp.AddTool(s, &gomcp.Tool{
		Name:        "list_spring_security_doc_sets",
		Description: "List available Spring Security documentation sets showing all indexed versions (e.g. 6.5.x, 7.0.x) with commit SHA and build date.",
	}, newListDocSetsHandler(st))

	gomcp.AddTool(s, &gomcp.Tool{
		Name:        "get_spring_security_docs_status",
		Description: "Get the status of the Spring Security documentation index including total chunk count and indexed versions.",
	}, newStatusHandler(st))

	return s
}
