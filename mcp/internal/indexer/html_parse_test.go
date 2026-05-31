package indexer

import (
	"strings"
	"testing"
	"time"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
)

const sampleHTML = `<!DOCTYPE html>
<html>
<head><title>Authentication</title></head>
<body>
<nav>Nav noise</nav>
<main>
  <h1>Authentication</h1>
  <p>Spring Security supports many authentication mechanisms.</p>
  <h2>Username/Password</h2>
  <p>The most common way is username and password authentication using SecurityFilterChain.</p>
  <pre><code>http.formLogin(Customizer.withDefaults());</code></pre>
  <h2>OAuth2 Login</h2>
  <p>Configure oauth2Login with JwtDecoder.</p>
  <h3>Client Registration</h3>
  <p>Register your OAuth2 client.</p>
</main>
</body>
</html>`

func TestParseHTML(t *testing.T) {
	opts := ParseOptions{
		Ref:          "6.5.x",
		CommitSha:    "abc123",
		BuiltAt:      time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		SourceType:   model.SourceTypeOfficialZIP,
		SourcePath:   "servlet/authentication.html",
		CanonicalURL: "https://docs.spring.io/spring-security/reference/servlet/authentication.html",
	}

	chunks, err := ParseHTML(strings.NewReader(sampleHTML), opts)
	if err != nil {
		t.Fatalf("ParseHTML: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}

	// Check that nav noise is excluded from both HTML and text.
	for _, c := range chunks {
		if strings.Contains(c.ContentHtml, "Nav noise") || strings.Contains(c.ContentText, "Nav noise") {
			t.Errorf("chunk %q contains nav noise", c.Title)
		}
	}

	// ContentHtml should contain raw HTML elements, not Markdown syntax.
	for _, c := range chunks {
		if strings.Contains(c.ContentHtml, "## ") {
			t.Errorf("chunk %q ContentHtml contains Markdown syntax", c.Title)
		}
	}

	// Check area inference.
	for _, c := range chunks {
		if c.Area != model.AreaServlet {
			t.Errorf("chunk %q: got area %q, want servlet", c.Title, c.Area)
		}
	}

	// IDs must be stable across calls.
	chunks2, _ := ParseHTML(strings.NewReader(sampleHTML), opts)
	for i := range chunks {
		if chunks[i].ID != chunks2[i].ID {
			t.Errorf("chunk ID not stable: %q vs %q", chunks[i].ID, chunks2[i].ID)
		}
	}
}

func TestChunkID(t *testing.T) {
	id1 := chunkID("6.5.x", "abc", "https://example.com/page", []string{"Auth", "Form Login"})
	id2 := chunkID("6.5.x", "abc", "https://example.com/page", []string{"Auth", "Form Login"})
	if id1 != id2 {
		t.Errorf("IDs not stable: %q vs %q", id1, id2)
	}

	id3 := chunkID("7.0.x", "abc", "https://example.com/page", []string{"Auth", "Form Login"})
	if id1 == id3 {
		t.Error("different refs must produce different IDs")
	}
}

func TestInferArea(t *testing.T) {
	cases := []struct {
		path string
		want model.Area
	}{
		{"servlet/authentication.html", model.AreaServlet},
		{"reactive/getting-started.html", model.AreaReactive},
		{"oauth2/resource-server.html", model.AreaOAuth2},
		{"saml2/login.html", model.AreaSAML2},
		{"method-security/index.html", model.AreaMethodSecurity},
		{"testing/index.html", model.AreaTesting},
		{"index.html", model.AreaOther},
	}
	for _, tc := range cases {
		got := inferArea(tc.path)
		if got != tc.want {
			t.Errorf("inferArea(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
