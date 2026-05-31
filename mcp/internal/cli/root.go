package cli

import "github.com/spf13/cobra"

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
	return root
}
