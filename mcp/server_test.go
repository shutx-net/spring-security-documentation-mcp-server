package mcp

import (
	"context"
	"slices"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/mcpserver"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

func newTestStore(t *testing.T) *store.MemoryStore {
	t.Helper()
	return store.NewMemoryStore()
}

func seedChunks(t *testing.T, st store.Store, chunks []model.DocChunk) {
	t.Helper()
	if err := st.UpsertChunks(context.Background(), chunks); err != nil {
		t.Fatalf("UpsertChunks: %v", err)
	}
}

func connectSession(t *testing.T, st store.Store) *gomcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := gomcp.NewInMemoryTransports()
	s := mcpserver.BuildServer(st)
	go s.Connect(context.Background(), serverTransport, nil)

	client := gomcp.NewClient(&gomcp.Implementation{Name: "test", Version: "0.1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestServerTools(t *testing.T) {
	session := connectSession(t, newTestStore(t))

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	want := []string{
		"search_spring_security_docs",
		"get_spring_security_doc",
		"list_spring_security_doc_sets",
		"get_spring_security_docs_status",
	}
	if len(result.Tools) != len(want) {
		t.Fatalf("got %d tools, want %d", len(result.Tools), len(want))
	}
	for _, name := range want {
		if !slices.ContainsFunc(result.Tools, func(tool *gomcp.Tool) bool { return tool.Name == name }) {
			t.Errorf("tool %q not found", name)
		}
	}
}

func TestSearchTool(t *testing.T) {
	st := newTestStore(t)
	seedChunks(t, st, []model.DocChunk{
		{
			ID: "id1", Project: "spring-security", Ref: "6.5.x", CommitSha: "abc",
			BuiltAt: time.Now(), SourceType: model.SourceTypeAntoraBuild,
			SourcePath:   "servlet/authentication.html",
			CanonicalURL: "https://docs.spring.io/spring-security/reference/servlet/authentication.html",
			Title:        "Form Login", HeadingPath: []string{"Authentication", "Form Login"},
			Area:        model.AreaServlet,
			ContentHtml: "<p>Configure SecurityFilterChain.</p>",
			ContentText: "Form Login Configure SecurityFilterChain.",
			IndexedAt:   time.Now(),
		},
	})

	session := connectSession(t, st)
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      "search_spring_security_docs",
		Arguments: map[string]any{"query": "SecurityFilterChain"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("got tool error: %v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
}

func TestSearchToolEmptyQuery(t *testing.T) {
	session := connectSession(t, newTestStore(t))
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      "search_spring_security_docs",
		Arguments: map[string]any{"query": ""},
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for empty query")
	}
}

func TestGetToolNotFound(t *testing.T) {
	session := connectSession(t, newTestStore(t))
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      "get_spring_security_doc",
		Arguments: map[string]any{"id": "nonexistent"},
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for nonexistent ID")
	}
}

func TestListDocSetsTool(t *testing.T) {
	session := connectSession(t, newTestStore(t))
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      "list_spring_security_doc_sets",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}
}

func TestStatusTool(t *testing.T) {
	session := connectSession(t, newTestStore(t))
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      "get_spring_security_docs_status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}
}
