package store

import (
	"context"
	"testing"
	"time"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
)

func openTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func fixtureChunk(id, ref, area, title, text string) model.DocChunk {
	return model.DocChunk{
		ID:              id,
		Project:         "spring-security",
		Ref:             ref,
		CommitSha:       "abc123",
		BuiltAt:         time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		SourceType:      model.SourceTypeOfficialZIP,
		SourcePath:      "/servlet/authentication.html",
		CanonicalURL:    "https://docs.spring.io/spring-security/reference/" + ref + "/servlet/authentication.html",
		Title:           title,
		HeadingPath:     []string{"Authentication", title},
		Area:            model.Area(area),
		ContentHtml:     "<p>" + text + "</p>",
		ContentText:     text,
		IndexedAt:       time.Now().UTC(),
	}
}

func TestUpsertAndGet(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	chunk := fixtureChunk("id1", "6.5.x", "servlet", "Form Login", "SecurityFilterChain configuration")
	if err := s.UpsertChunks(ctx, []model.DocChunk{chunk}); err != nil {
		t.Fatalf("UpsertChunks: %v", err)
	}

	got, err := s.GetChunk(ctx, "id1")
	if err != nil {
		t.Fatalf("GetChunk: %v", err)
	}
	if got.Title != "Form Login" {
		t.Errorf("got title %q, want %q", got.Title, "Form Login")
	}
	if got.Ref != "6.5.x" {
		t.Errorf("got ref %q, want %q", got.Ref, "6.5.x")
	}
}

func TestGetChunkNotFound(t *testing.T) {
	s := openTestStore(t)
	_, err := s.GetChunk(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent chunk")
	}
}

func TestSearch(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	chunks := []model.DocChunk{
		fixtureChunk("id1", "6.5.x", "servlet", "Form Login", "Configure SecurityFilterChain for form login"),
		fixtureChunk("id2", "6.5.x", "oauth2", "OAuth2 Login", "Configure oauth2ResourceServer and JwtDecoder"),
		fixtureChunk("id3", "7.0.x", "servlet", "CSRF Protection", "Enable csrf protection"),
	}
	if err := s.UpsertChunks(ctx, chunks); err != nil {
		t.Fatalf("UpsertChunks: %v", err)
	}

	result, err := s.Search(ctx, model.SearchParams{Query: "SecurityFilterChain", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Chunks) != 1 {
		t.Errorf("got %d results, want 1", len(result.Chunks))
	}

	// ref フィルタ: "csrf" は 7.0.x のチャンクのみに含まれる
	result, err = s.Search(ctx, model.SearchParams{Query: "csrf", Ref: "7.0.x", Limit: 10})
	if err != nil {
		t.Fatalf("Search with ref filter: %v", err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0].Ref != "7.0.x" {
		t.Errorf("ref filter failed: got %d results", len(result.Chunks))
	}
}

func TestListDocSets(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	chunks := []model.DocChunk{
		fixtureChunk("id1", "6.5.x", "servlet", "Form Login", "text"),
		fixtureChunk("id2", "7.0.x", "servlet", "CSRF", "text"),
	}
	if err := s.UpsertChunks(ctx, chunks); err != nil {
		t.Fatalf("UpsertChunks: %v", err)
	}

	sets, err := s.ListDocSets(ctx)
	if err != nil {
		t.Fatalf("ListDocSets: %v", err)
	}
	if len(sets) != 2 {
		t.Errorf("got %d docsets, want 2", len(sets))
	}
}

func TestStatus(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	st, err := s.Status(ctx)
	if err != nil {
		t.Fatalf("Status on empty: %v", err)
	}
	if st.TotalChunks != 0 {
		t.Errorf("expected 0 chunks, got %d", st.TotalChunks)
	}

	chunk := fixtureChunk("id1", "6.5.x", "servlet", "Form Login", "text")
	_ = s.UpsertChunks(ctx, []model.DocChunk{chunk})

	st, err = s.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.TotalChunks != 1 {
		t.Errorf("expected 1 chunk, got %d", st.TotalChunks)
	}
}
