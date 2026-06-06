package indexer

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
)

// ParseOptions controls how a single HTML page is parsed.
type ParseOptions struct {
	Ref          string
	CommitSha    string
	BuiltAt      time.Time
	SourceType   model.SourceType
	SourcePath   string // relative path within the site
	CanonicalURL string
}

// ParseHTML parses a single HTML document and returns its chunks.
//
// The Antora-generated HTML is treated as the source of truth.
// Each chunk stores the raw HTML fragment (ContentHtml) and plain text
// extracted for FTS / Embedding (ContentText). Markdown conversion is
// deferred to response time and is not performed here.
func ParseHTML(r io.Reader, opts ParseOptions) ([]model.DocChunk, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	// Remove navigation, header, footer, sidebar noise.
	doc.Find("nav, header, footer, .sidebar, .nav, #toc, script, style").Remove()

	// Find main content area, preferring specific containers over body.
	var content *goquery.Selection
	for _, sel := range []string{"main", "article", ".content", "body"} {
		if c := doc.Find(sel); c.Length() > 0 {
			content = c.First()
			break
		}
	}
	if content == nil || content.Length() == 0 {
		return nil, nil
	}

	area := inferArea(opts.SourcePath)
	docType := inferDocType(opts.SourcePath)

	var chunks []model.DocChunk
	var currentHeadings []string // [h1, h2, h3] stack
	var htmlParts []string       // raw HTML of body elements
	var textParts []string       // plain text for FTS

	flush := func(headingPath []string) {
		if len(htmlParts) == 0 {
			return
		}
		contentHtml := strings.TrimSpace(strings.Join(htmlParts, "\n"))
		contentText := strings.TrimSpace(strings.Join(textParts, "\n"))
		htmlParts = nil
		textParts = nil
		if contentHtml == "" {
			return
		}
		title := ""
		if len(headingPath) > 0 {
			title = headingPath[len(headingPath)-1]
		}
		if title == "" {
			return
		}
		id := chunkID(opts.Ref, opts.CommitSha, opts.CanonicalURL, headingPath)
		chunks = append(chunks, model.DocChunk{
			ID:          id,
			Project:     "spring-security",
			Ref:         opts.Ref,
			CommitSha:   opts.CommitSha,
			BuiltAt:     opts.BuiltAt,
			SourceType:  opts.SourceType,
			SourcePath:  opts.SourcePath,
			CanonicalURL: opts.CanonicalURL,
			Title:       title,
			HeadingPath: append([]string{}, headingPath...),
			Area:        area,
			DocType:     docType,
			ContentHtml: contentHtml,
			ContentText: contentText,
			IndexedAt:   time.Now().UTC(),
		})
	}

	content.Children().Each(func(_ int, sel *goquery.Selection) {
		tag := goquery.NodeName(sel)
		switch tag {
		case "h1":
			flush(currentHeadings)
			currentHeadings = []string{strings.TrimSpace(sel.Text())}
		case "h2":
			flush(currentHeadings)
			h := strings.TrimSpace(sel.Text())
			if len(currentHeadings) >= 1 {
				currentHeadings = []string{currentHeadings[0], h}
			} else {
				currentHeadings = []string{h}
			}
		case "h3":
			flush(currentHeadings)
			h := strings.TrimSpace(sel.Text())
			switch len(currentHeadings) {
			case 0:
				currentHeadings = []string{h}
			case 1:
				currentHeadings = []string{currentHeadings[0], h}
			default:
				currentHeadings = []string{currentHeadings[0], currentHeadings[1], h}
			}
		default:
			if len(currentHeadings) == 0 {
				return
			}
			outer, err := goquery.OuterHtml(sel)
			if err != nil || strings.TrimSpace(outer) == "" {
				return
			}
			text := strings.TrimSpace(sel.Text())
			htmlParts = append(htmlParts, outer)
			if text != "" {
				textParts = append(textParts, text)
			}
		}
	})
	flush(currentHeadings)

	return chunks, nil
}
