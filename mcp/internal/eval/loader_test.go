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

func TestLoadTopics_basic(t *testing.T) {
	input := `{"topicId":"SS-001","query":"How do I configure SecurityFilterChain in Spring Security?","ref":"6.5.x","area":"servlet"}
{"topicId":"SS-002","query":"When should CSRF protection be disabled in Spring Security?"}
`
	topics, err := LoadTopics(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(topics) != 2 {
		t.Fatalf("got %d topics, want 2", len(topics))
	}
	if topics[0].TopicID != "SS-001" || topics[0].Ref != "6.5.x" || topics[0].Area != "servlet" {
		t.Errorf("unexpected topic[0]: %+v", topics[0])
	}
	if topics[1].TopicID != "SS-002" || topics[1].Ref != "" || topics[1].Area != "" {
		t.Errorf("unexpected topic[1]: %+v", topics[1])
	}
}

func TestLoadTopics_missingTopicId(t *testing.T) {
	input := `{"query":"How do I configure SecurityFilterChain in Spring Security?"}`
	_, err := LoadTopics(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing topicId")
	}
}

func TestLoadTopics_missingQuery(t *testing.T) {
	input := `{"topicId":"SS-001"}`
	_, err := LoadTopics(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestLoadTopics_duplicateTopicId(t *testing.T) {
	input := `{"topicId":"SS-001","query":"first"}
{"topicId":"SS-001","query":"second"}
`
	_, err := LoadTopics(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for duplicate topicId")
	}
}
