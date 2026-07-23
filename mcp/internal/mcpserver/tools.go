package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

const (
	searchSnippetMaxChars = 500
	chunkContentMaxChars  = 20_000
)

// chunkSnippet is a reduced view of DocChunk used in search results.
// ContentHtml is omitted; ContentText is truncated to searchSnippetMaxChars.
type chunkSnippet struct {
	ID           string       `json:"id"`
	Ref          string       `json:"ref"`
	CommitSha    string       `json:"commit_sha"`
	BuiltAt      time.Time    `json:"built_at"`
	SourcePath   string       `json:"source_path"`
	CanonicalURL string       `json:"canonical_url"`
	Title        string       `json:"title"`
	HeadingPath  []string     `json:"heading_path"`
	Area         model.Area    `json:"area"`
	DocType      model.DocType `json:"doc_type"`
	ContentText  string        `json:"content_text"`
	Truncated    bool          `json:"truncated,omitempty"`
}

// toSnippet builds a reduced view of a chunk. When ContentText exceeds
// searchSnippetMaxChars, it returns a window centered on the first occurrence of
// query (KWIC) so the matched term is visible, falling back to the head of the
// text when query is empty or absent (e.g. a vector-only hit).
func toSnippet(c model.DocChunk, query string) chunkSnippet {
	text := c.ContentText
	truncated := false
	if runes := []rune(text); len(runes) > searchSnippetMaxChars {
		text = snippetWindow(runes, query)
		truncated = true
	}
	return chunkSnippet{
		ID:          c.ID,
		Ref:         c.Ref,
		CommitSha:   c.CommitSha,
		BuiltAt:     c.BuiltAt,
		SourcePath:  c.SourcePath,
		CanonicalURL: c.CanonicalURL,
		Title:       c.Title,
		HeadingPath: c.HeadingPath,
		Area:        c.Area,
		DocType:     c.DocType,
		ContentText: text,
		Truncated:   truncated,
	}
}

// snippetWindow returns a searchSnippetMaxChars-rune window of runes centered on
// the first case-insensitive occurrence of query, with "..." added on any
// truncated side. When query is empty or not found it returns the leading
// window (the previous behavior).
func snippetWindow(runes []rune, query string) string {
	start := 0
	if mi := runeIndexFold(runes, query); mi > 0 {
		lead := (searchSnippetMaxChars - len([]rune(query))) / 2
		if lead < 0 {
			lead = 0
		}
		start = mi - lead
		if start < 0 {
			start = 0
		}
	}
	end := start + searchSnippetMaxChars
	if end > len(runes) {
		end = len(runes)
		if start = end - searchSnippetMaxChars; start < 0 {
			start = 0
		}
	}

	var b strings.Builder
	if start > 0 {
		b.WriteString("...")
	}
	b.WriteString(string(runes[start:end]))
	if end < len(runes) {
		b.WriteString("...")
	}
	return b.String()
}

// runeIndexFold returns the rune index of the first case-insensitive occurrence
// of needle in hay, or -1. Rune-based so the offset is safe to slice a []rune.
func runeIndexFold(hay []rune, needle string) int {
	nl := []rune(strings.ToLower(needle))
	if len(nl) == 0 || len(nl) > len(hay) {
		return -1
	}
	for i := 0; i+len(nl) <= len(hay); i++ {
		match := true
		for j, r := range nl {
			if unicode.ToLower(hay[i+j]) != r {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func capChunk(c model.DocChunk) model.DocChunk {
	if r := []rune(c.ContentText); len(r) > chunkContentMaxChars {
		c.ContentText = string(r[:chunkContentMaxChars]) + "..."
	}
	if r := []rune(c.ContentHtml); len(r) > chunkContentMaxChars {
		c.ContentHtml = string(r[:chunkContentMaxChars]) + "..."
	}
	return c
}

// --- search_spring_security_docs ---

type searchArgs struct {
	Query string `json:"query" jsonschema:"Search query (keyword, concept, or Java identifier such as SecurityFilterChain)"`
	Ref   string `json:"ref,omitempty"   jsonschema:"Spring Security version to search (e.g. 6.5.x or 7.0.x)"`
	Area  string `json:"area,omitempty"  jsonschema:"Documentation area: servlet, reactive, oauth2, saml2, method-security, testing, architecture, authorization, or authentication"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of results (default 10, max 20)"`
}

func newSearchHandler(st store.Store) func(context.Context, *gomcp.CallToolRequest, searchArgs) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, args searchArgs) (*gomcp.CallToolResult, any, error) {
		if args.Query == "" {
			return nil, nil, fmt.Errorf("query is required")
		}
		limit := args.Limit
		if limit <= 0 || limit > 20 {
			limit = 10
		}
		result, err := st.Search(ctx, model.SearchParams{
			Query: args.Query,
			Ref:   args.Ref,
			Area:  args.Area,
			Limit: limit,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("search failed: %w", err)
		}
		snippets := make([]chunkSnippet, len(result.Chunks))
		for i, c := range result.Chunks {
			snippets[i] = toSnippet(c, args.Query)
		}
		return textResult(snippets)
	}
}

// --- get_spring_security_doc ---

type getArgs struct {
	ID string `json:"id" jsonschema:"Chunk ID returned by search_spring_security_docs"`
}

func newGetHandler(st store.Store) func(context.Context, *gomcp.CallToolRequest, getArgs) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, args getArgs) (*gomcp.CallToolResult, any, error) {
		if args.ID == "" {
			return nil, nil, fmt.Errorf("id is required")
		}
		chunk, err := st.GetChunk(ctx, args.ID)
		if err != nil {
			return nil, nil, err
		}
		return textResult(capChunk(chunk))
	}
}

// --- list_spring_security_doc_sets ---

func newListDocSetsHandler(st store.Store) func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, _ struct{}) (*gomcp.CallToolResult, any, error) {
		sets, err := st.ListDocSets(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list doc sets failed: %w", err)
		}
		return textResult(sets)
	}
}

// --- get_spring_security_docs_status ---

func newStatusHandler(st store.Store) func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, _ struct{}) (*gomcp.CallToolResult, any, error) {
		status, err := st.Status(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("status failed: %w", err)
		}
		return textResult(status)
	}
}

// textResult marshals v to JSON and wraps it in a TextContent result.
func textResult(v any) (*gomcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal result: %w", err)
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{
			&gomcp.TextContent{Text: string(b)},
		},
	}, nil, nil
}
