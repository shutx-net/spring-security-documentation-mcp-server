package indexer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

const springSecurityRepoURL = "https://github.com/spring-projects/spring-security.git"

// AntoraIndexOptions configures indexing from an Antora build.
type AntoraIndexOptions struct {
	Ref   string
	Store store.Store

	// SiteDir: path to a pre-built Antora site directory (docs/build/site).
	// When set, clone and build are skipped — the directory is indexed directly.
	// This is the primary path for CI: run the Antora build externally, then
	// pass --site-dir to this command.
	SiteDir string

	// WorkDir: directory where the repo is cloned.
	// If empty and SiteDir is not set, a temporary directory is used and
	// removed after indexing.
	WorkDir string
}

// IndexFromAntora indexes Spring Security documentation from an Antora build.
//
// When opts.SiteDir is set the repo clone and Gradle build are skipped and
// the directory is indexed directly. This is the expected path in CI:
//
//	./gradlew -PbuildSrc.skipTests=true :spring-security-docs:antora
//	spring-security-docs-mcp index --source antora --ref 6.5.x \
//	    --site-dir docs/build/site
//
// When opts.SiteDir is empty the repository is cloned and the Antora build
// is executed automatically.
func IndexFromAntora(ctx context.Context, opts AntoraIndexOptions) (int, error) {
	if opts.Ref == "" {
		return 0, fmt.Errorf("ref is required")
	}
	if opts.SiteDir != "" {
		commitSha, _ := gitRevParseDir(ctx, opts.SiteDir)
		return indexSiteDir(ctx, opts.Ref, commitSha, opts.SiteDir, opts.Store)
	}
	return cloneBuildAndIndex(ctx, opts)
}

// cloneBuildAndIndex performs a shallow clone and runs the Antora build.
func cloneBuildAndIndex(ctx context.Context, opts AntoraIndexOptions) (int, error) {
	workDir := opts.WorkDir
	var cleanup func()
	if workDir == "" {
		tmp, err := os.MkdirTemp("", "spring-security-docs-*")
		if err != nil {
			return 0, fmt.Errorf("create temp dir: %w", err)
		}
		workDir = tmp
		cleanup = func() { os.RemoveAll(tmp) }
	}
	if cleanup != nil {
		defer cleanup()
	}

	fmt.Printf("Cloning %s (ref=%s) into %s ...\n", springSecurityRepoURL, opts.Ref, workDir)
	if err := runCmd(ctx, ".", "git", "clone",
		"--depth", "1",
		"--branch", opts.Ref,
		springSecurityRepoURL, workDir,
	); err != nil {
		return 0, fmt.Errorf("git clone (ref=%s): %w", opts.Ref, err)
	}

	commitSha, _ := gitRevParseDir(ctx, workDir)
	fmt.Printf("Commit: %s\n", commitSha)

	// Run the Antora build via the Gradle wrapper.
	gradlew := filepath.Join(workDir, "gradlew")
	fmt.Println("Running Antora build (this may take several minutes)...")
	if err := runCmd(ctx, workDir, gradlew,
		"-PbuildSrc.skipTests=true",
		":spring-security-docs:antora",
	); err != nil {
		return 0, fmt.Errorf("antora build failed: %w", err)
	}

	siteDir := filepath.Join(workDir, "docs", "build", "site")
	return indexSiteDir(ctx, opts.Ref, commitSha, siteDir, opts.Store)
}

// indexSiteDir walks an Antora-built site directory and indexes all HTML files.
func indexSiteDir(ctx context.Context, ref, commitSha, siteDir string, st store.Store) (int, error) {
	if _, err := os.Stat(siteDir); os.IsNotExist(err) {
		return 0, fmt.Errorf(
			"site directory not found: %s\n"+
				"Run the Antora build first:\n"+
				"  ./gradlew -PbuildSrc.skipTests=true :spring-security-docs:antora\n"+
				"Then pass --site-dir docs/build/site",
			siteDir,
		)
	}

	builtAt := time.Now().UTC()
	var total int

	err := filepath.WalkDir(siteDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}

		rel, _ := filepath.Rel(siteDir, path)
		sourcePath := filepath.ToSlash(rel)
		canonicalURL := canonicalURLFor(ref, sourcePath)

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		chunks, err := ParseHTML(f, ParseOptions{
			Ref:          ref,
			CommitSha:    commitSha,
			BuiltAt:      builtAt,
			SourceType:   model.SourceTypeAntoraBuild,
			SourcePath:   sourcePath,
			CanonicalURL: canonicalURL,
		})
		f.Close()
		if err != nil || len(chunks) == 0 {
			return nil
		}

		if err := st.UpsertChunks(ctx, chunks); err != nil {
			return fmt.Errorf("upsert %s: %w", sourcePath, err)
		}
		total += len(chunks)
		return nil
	})

	return total, err
}

func gitRevParseDir(ctx context.Context, dir string) (string, error) {
	// dir may be a git repo root (cloned repo) or a site dir inside one.
	// Walk up to find a .git directory.
	for d := dir; d != filepath.Dir(d); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			cmd := exec.CommandContext(ctx, "git", "-C", d, "rev-parse", "HEAD")
			out, err := cmd.Output()
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(out)), nil
		}
	}
	return "", fmt.Errorf("not a git repository: %s", dir)
}

func runCmd(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" && dir != "." {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
