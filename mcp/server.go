package mcp

import (
	"context"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/mcpserver"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

// ServeStdio opens the DynamoDB store and runs the MCP server over stdio.
// The DynamoDB table name is read from the SPRING_SEC_MCP_CHUNKS_TABLE environment variable.
func ServeStdio(ctx context.Context) error {
	st, err := store.NewAWSStore(ctx, store.AWSConfigFromEnv())
	if err != nil {
		return err
	}
	defer st.Close()
	s := mcpserver.BuildServer(st)
	return s.Run(ctx, &gomcp.StdioTransport{})
}
