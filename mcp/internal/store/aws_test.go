package store

import (
	"os"
	"testing"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
)

func chunk(id string) model.DocChunk { return model.DocChunk{ID: id} }

func TestMergeSearchResults_PriorityOrder(t *testing.T) {
	vector  := []model.DocChunk{chunk("v1"), chunk("v2")}
	kwTable := []model.DocChunk{chunk("kt1"), chunk("v1")} // v1 duplicate
	kw      := []model.DocChunk{chunk("k1"), chunk("kt1")} // kt1 duplicate

	got := mergeSearchResults(5, vector, kwTable, kw)
	want := []string{"v1", "v2", "kt1", "k1"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i, c := range got {
		if c.ID != want[i] {
			t.Errorf("pos %d: got %q want %q", i, c.ID, want[i])
		}
	}
}

func TestMergeSearchResults_RespectsLimit(t *testing.T) {
	src := []model.DocChunk{chunk("a"), chunk("b"), chunk("c")}
	got := mergeSearchResults(2, src)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
}

func TestMergeSearchResults_Empty(t *testing.T) {
	got := mergeSearchResults(10)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d", len(got))
	}
}

func TestMergeSearchResults_AllDuplicates(t *testing.T) {
	src := []model.DocChunk{chunk("a"), chunk("a"), chunk("a")}
	got := mergeSearchResults(10, src)
	if len(got) != 1 {
		t.Fatalf("expected 1 unique, got %d", len(got))
	}
}

func TestLooksLikeIdentifier(t *testing.T) {
	cases := []struct {
		query string
		want  bool
	}{
		{"SecurityFilterChain", true},
		{"SecurityWebFilterChain", true},
		{"@PreAuthorize", true},
		{"@WithMockUser", true},
		{"JwtDecoder", true},
		{"oauth2ResourceServer", true},
		{"csrf", true},
		{"how to configure method security", false},
		{"how to disable csrf", false},
		{"spring security testing mock user", false},
	}
	for _, tc := range cases {
		if got := looksLikeIdentifier(tc.query); got != tc.want {
			t.Errorf("looksLikeIdentifier(%q) = %v, want %v", tc.query, got, tc.want)
		}
	}
}

func TestMergeSearchResults_IdentifierPriority(t *testing.T) {
	vr  := []model.DocChunk{chunk("v1"), chunk("v2")}
	ktr := []model.DocChunk{chunk("kt1"), chunk("v1")} // v1 duplicate
	kr  := []model.DocChunk{chunk("k1")}

	// identifier query: ktr before vr
	got := mergeSearchResults(5, ktr, vr, kr)
	want := []string{"kt1", "v1", "v2", "k1"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i, c := range got {
		if c.ID != want[i] {
			t.Errorf("pos %d: got %q want %q", i, c.ID, want[i])
		}
	}
}

func TestAWSConfigFromEnv_KeywordsTable(t *testing.T) {
	t.Setenv("CHUNKS_TABLE", "chunks")
	t.Setenv("KEYWORDS_TABLE", "keywords")
	cfg := AWSConfigFromEnv()
	if cfg.KeywordsTable != "keywords" {
		t.Errorf("KeywordsTable=%q want %q", cfg.KeywordsTable, "keywords")
	}
}

func TestAWSConfigFromEnv_KeywordsTableEmpty(t *testing.T) {
	os.Unsetenv("KEYWORDS_TABLE")
	cfg := AWSConfigFromEnv()
	if cfg.KeywordsTable != "" {
		t.Errorf("KeywordsTable=%q want empty", cfg.KeywordsTable)
	}
}
