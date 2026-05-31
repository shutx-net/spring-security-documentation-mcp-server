package indexer

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

const (
	githubArchiveURL = "https://github.com/spring-projects/spring-security/archive/refs/heads/%s.zip"
	// HTML files are inside the docs/build/site/ subtree within the archive.
	// For the ZIP archive of the branch, files start with spring-security-<ref>/.
	docsPrefix = "docs/build/site/"
)

// ZipIndexOptions configures indexing from the GitHub archive ZIP.
type ZipIndexOptions struct {
	Ref       string
	CommitSha string // can be empty for ZIP source (SHA not available without git)
	Store     store.Store
}

// IndexFromZIP downloads the GitHub archive for the given ref and indexes all HTML files.
func IndexFromZIP(ctx context.Context, opts ZipIndexOptions) (int, error) {
	url := fmt.Sprintf(githubArchiveURL, opts.Ref)

	data, err := downloadURL(ctx, url)
	if err != nil {
		return 0, fmt.Errorf("download zip: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("open zip: %w", err)
	}

	builtAt := time.Now().UTC()
	var total int

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Strip the top-level directory (spring-security-<branch>/).
		name := f.Name
		idx := strings.Index(name, "/")
		if idx < 0 {
			continue
		}
		rel := name[idx+1:] // e.g. "docs/build/site/servlet/authentication.html"

		if !strings.HasPrefix(rel, docsPrefix) {
			continue
		}
		if filepath.Ext(rel) != ".html" {
			continue
		}

		sourcePath := strings.TrimPrefix(rel, docsPrefix) // e.g. "servlet/authentication.html"
		canonicalURL := canonicalURLFor(opts.Ref, sourcePath)

		rc, err := f.Open()
		if err != nil {
			continue
		}

		chunks, err := ParseHTML(rc, ParseOptions{
			Ref:          opts.Ref,
			CommitSha:    opts.CommitSha,
			BuiltAt:      builtAt,
			SourceType:   model.SourceTypeOfficialZIP,
			SourcePath:   sourcePath,
			CanonicalURL: canonicalURL,
		})
		rc.Close()
		if err != nil || len(chunks) == 0 {
			continue
		}

		if err := opts.Store.UpsertChunks(ctx, chunks); err != nil {
			return total, fmt.Errorf("upsert chunks for %s: %w", sourcePath, err)
		}
		total += len(chunks)
	}

	return total, nil
}

func downloadURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func canonicalURLFor(ref, sourcePath string) string {
	// e.g. "servlet/authentication.html" → "https://docs.spring.io/spring-security/reference/servlet/authentication.html"
	path := strings.TrimSuffix(sourcePath, ".html")
	return "https://docs.spring.io/spring-security/reference/" + path + ".html"
}
