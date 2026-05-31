package cli

import (
	"context"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/mcpserver"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start MCP server (stdio transport)",
		Long: `Start the MCP server over stdio.

Reads from CHUNKS_TABLE for the DynamoDB chunks table name.
Standard AWS credential/region environment variables are also respected.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openAWSStore(cmd.Context())
			if err != nil {
				return err
			}
			defer st.Close()
			s := mcpserver.BuildServer(st)
			return s.Run(context.Background(), &gomcp.StdioTransport{})
		},
	}
}
