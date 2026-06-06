package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

// NewRootCmd creates the root cobra command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "spring-security-docs-mcp",
		Short: "MCP server for Spring Security reference documentation",
	}
	root.AddCommand(newServeCmd())
	root.AddCommand(newServeHTTPCmd())
	root.AddCommand(newIndexCmd())
	root.AddCommand(newInstallIndexCmd())
	root.AddCommand(newCheckUpdatesCmd())
	root.AddCommand(newEvalCmd())
	return root
}

// openAWSStore opens the DynamoDB-backed store using environment variables.
func openAWSStore(ctx context.Context) (*store.AWSStore, error) {
	cfg := store.AWSConfigFromEnv()
	st, err := store.NewAWSStore(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open AWS store: %w\n\nSet CHUNKS_TABLE to the DynamoDB table name", err)
	}
	return st, nil
}
