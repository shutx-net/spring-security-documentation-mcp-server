package cli

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/indexer"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

func newIndexCmd() *cobra.Command {
	var (
		source string
		ref    string
		dbPath string
	)

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Build or update the documentation index",
		RunE: func(cmd *cobra.Command, args []string) error {
			if ref == "" {
				return fmt.Errorf("--ref is required (e.g. 6.5.x)")
			}
			if dbPath == "" {
				dbPath = defaultDBPath()
			}
			if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
				return fmt.Errorf("create db dir: %w", err)
			}
			st, err := store.Open(dbPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer st.Close()

			switch source {
			case "zip":
				log.Printf("Indexing Spring Security %s from GitHub ZIP...", ref)
				n, err := indexer.IndexFromZIP(cmd.Context(), indexer.ZipIndexOptions{
					Ref:   ref,
					Store: st,
				})
				if err != nil {
					return fmt.Errorf("index from zip: %w", err)
				}
				log.Printf("Indexed %d chunks for ref=%s", n, ref)

			case "antora":
				return fmt.Errorf("antora source is not yet implemented (Phase 3)")

			default:
				return fmt.Errorf("unknown source %q: use 'zip' or 'antora'", source)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&source, "source", "zip", "documentation source: zip|antora")
	cmd.Flags().StringVar(&ref, "ref", "", "Spring Security version ref (e.g. 6.5.x, 7.0.x, main)")
	cmd.Flags().StringVar(&dbPath, "db", "", "path to SQLite database")
	return cmd
}
