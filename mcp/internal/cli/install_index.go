package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInstallIndexCmd() *cobra.Command {
	var version string

	cmd := &cobra.Command{
		Use:   "install-index",
		Short: "Download and install a prebuilt documentation index from GitHub Releases",
		RunE: func(cmd *cobra.Command, args []string) error {
			if version == "" {
				return fmt.Errorf("--version is required (e.g. 6.5.x)")
			}
			// TODO(Phase 2): download from GitHub Releases and place in defaultDBPath().
			return fmt.Errorf("install-index is not yet implemented (Phase 2)")
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Spring Security version to install (e.g. 6.5.x)")
	return cmd
}
