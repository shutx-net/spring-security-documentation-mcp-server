package eval

import (
	"strings"
	"testing"
)

func TestLoadQrels_basic(t *testing.T) {
	input := `{"topicId":"SS-001","chunkId":"chunk-a","grade":3}
{"topicId":"SS-001","chunkId":"chunk-b","grade":2}
`
	qrels, err := LoadQrels(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(qrels) != 2 {
		t.Fatalf("got %d qrels, want 2", len(qrels))
	}
	if qrels[0].Grade != 3 || qrels[1].Grade != 2 {
		t.Errorf("unexpected grades: %v", qrels)
	}
}

func TestLoadQrels_invalidGrade(t *testing.T) {
	input := `{"topicId":"SS-001","chunkId":"chunk-a","grade":4}`
	_, err := LoadQrels(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for grade 4")
	}
}

func TestLoadQrels_negativeGrade(t *testing.T) {
	input := `{"topicId":"SS-001","chunkId":"chunk-a","grade":-1}`
	_, err := LoadQrels(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for grade -1")
	}
}

func TestLoadRun_basic(t *testing.T) {
	input := `{"runId":"hybrid","topicId":"SS-001","rank":1,"chunkId":"chunk-a","score":12.45}
{"runId":"hybrid","topicId":"SS-001","rank":2,"chunkId":"chunk-b","score":10.23}
`
	entries, err := LoadRun(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Rank != 1 || entries[1].Rank != 2 {
		t.Errorf("unexpected ranks: %v", entries)
	}
}

func TestLoadRun_invalidRank(t *testing.T) {
	input := `{"runId":"hybrid","topicId":"SS-001","rank":0,"chunkId":"chunk-a"}`
	_, err := LoadRun(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for rank 0")
	}
}

func TestLoadQrels_emptyTopicId(t *testing.T) {
	input := `{"chunkId":"chunk-a","grade":2}`
	_, err := LoadQrels(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing topicId")
	}
}

func TestLoadQrels_emptyChunkId(t *testing.T) {
	input := `{"topicId":"SS-001","grade":2}`
	_, err := LoadQrels(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing chunkId")
	}
}

func TestLoadQrels_duplicateEntry(t *testing.T) {
	input := `{"topicId":"SS-001","chunkId":"chunk-a","grade":3}
{"topicId":"SS-001","chunkId":"chunk-a","grade":1}
`
	_, err := LoadQrels(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for duplicate (topicId, chunkId)")
	}
}

func TestLoadRun_emptyTopicId(t *testing.T) {
	input := `{"runId":"r","chunkId":"chunk-a","rank":1}`
	_, err := LoadRun(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing topicId")
	}
}

func TestLoadRun_emptyChunkId(t *testing.T) {
	input := `{"runId":"r","topicId":"SS-001","rank":1}`
	_, err := LoadRun(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing chunkId")
	}
}
