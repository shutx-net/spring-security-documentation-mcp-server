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
		ref     string
		siteDir string
		workDir string
		dbPath  string
	)

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Build or update the documentation index from an Antora build",
		Long: `Index Spring Security documentation from an Antora-built site.

There are two modes:

1. Pre-built site directory (recommended for CI):
   Run the Antora build externally, then pass the output directory:

     ./gradlew -PbuildSrc.skipTests=true :spring-security-docs:antora
     spring-security-docs-mcp index --ref 6.5.x --site-dir docs/build/site

2. Automatic clone + build:
   The spring-security repository is cloned and the Antora build is executed
   automatically. Requires git, JDK, Gradle, and Node.js.

     spring-security-docs-mcp index --ref 6.5.x`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ref == "" {
				return fmt.Errorf("--ref is required (e.g. 6.5.x, 7.0.x, main)")
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

			log.Printf("Indexing Spring Security docs (ref=%s)...", ref)
			n, err := indexer.IndexFromAntora(cmd.Context(), indexer.AntoraIndexOptions{
				Ref:     ref,
				Store:   st,
				SiteDir: siteDir,
				WorkDir: workDir,
			})
			if err != nil {
				return fmt.Errorf("index failed: %w", err)
			}
			log.Printf("Done: indexed %d chunks (ref=%s, db=%s)", n, ref, dbPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&ref, "ref", "", "Spring Security version ref to index (e.g. 6.5.x, 7.0.x, main) [required]")
	cmd.Flags().StringVar(&siteDir, "site-dir", "", "Path to a pre-built Antora site directory (docs/build/site). If set, clone and build are skipped.")
	cmd.Flags().StringVar(&workDir, "work-dir", "", "Directory for cloning the repository. If empty, a temporary directory is used.")
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the SQLite database (default: $HOME/.local/share/spring-security-docs-mcp/index.db)")
	_ = cmd.MarkFlagRequired("ref")
	return cmd
}
