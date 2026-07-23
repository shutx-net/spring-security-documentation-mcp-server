package mcpserver

import (
	"strings"
	"testing"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
)

func TestToSnippet_shortTextUntouched(t *testing.T) {
	c := model.DocChunk{ContentText: "short body"}
	got := toSnippet(c, "anything")
	if got.ContentText != "short body" {
		t.Errorf("ContentText = %q, want unchanged", got.ContentText)
	}
	if got.Truncated {
		t.Error("Truncated = true, want false for short text")
	}
}

func TestToSnippet_windowsAroundMatch(t *testing.T) {
	// Term sits well past searchSnippetMaxChars, so head truncation would miss it.
	term := "OAuth2LoginConfigurer"
	body := strings.Repeat("a", 600) + term + strings.Repeat("b", 600)
	got := toSnippet(model.DocChunk{ContentText: body}, term)

	if !got.Truncated {
		t.Error("Truncated = false, want true")
	}
	if !strings.Contains(got.ContentText, term) {
		t.Errorf("snippet does not contain the matched term %q: %q", term, got.ContentText)
	}
	if !strings.HasPrefix(got.ContentText, "...") {
		t.Error("expected leading ellipsis for a mid-document match")
	}
	// Window must not blow past the cap (allow ellipses on both ends).
	if n := len([]rune(got.ContentText)); n > searchSnippetMaxChars+6 {
		t.Errorf("snippet length %d exceeds cap", n)
	}
}

func TestToSnippet_caseInsensitiveMatch(t *testing.T) {
	term := "JwtDecoder"
	body := strings.Repeat("x", 700) + term + strings.Repeat("y", 200)
	got := toSnippet(model.DocChunk{ContentText: body}, "jwtdecoder")
	if !strings.Contains(got.ContentText, term) {
		t.Errorf("case-insensitive query did not surface %q: %q", term, got.ContentText)
	}
}

func TestToSnippet_fallsBackToHeadWhenAbsent(t *testing.T) {
	// A vector-only hit: the query substring is not in the body.
	body := strings.Repeat("a", 800)
	got := toSnippet(model.DocChunk{ContentText: body}, "not-present-term")
	if !got.Truncated {
		t.Error("Truncated = false, want true")
	}
	if !strings.HasPrefix(got.ContentText, "aaa") {
		t.Errorf("expected head fallback, got %q", got.ContentText[:10])
	}
	if strings.HasPrefix(got.ContentText, "...") {
		t.Error("head window should not have a leading ellipsis")
	}
	if !strings.HasSuffix(got.ContentText, "...") {
		t.Error("head window should have a trailing ellipsis")
	}
}

func TestToSnippet_emptyQueryFallsBackToHead(t *testing.T) {
	body := strings.Repeat("a", 800)
	got := toSnippet(model.DocChunk{ContentText: body}, "")
	if strings.HasPrefix(got.ContentText, "...") {
		t.Error("empty query should fall back to head window (no leading ellipsis)")
	}
}
