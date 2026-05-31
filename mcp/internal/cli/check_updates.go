package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCheckUpdatesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check-updates",
		Short: "Check for newer Spring Security documentation index releases",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(Phase 3): compare local index commitSha against GitHub latest.
			return fmt.Errorf("check-updates is not yet implemented (Phase 3)")
		},
	}
}
