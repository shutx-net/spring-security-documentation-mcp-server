package eval

import (
	"math"
	"testing"
)

func TestScoreRun_perfectRanking(t *testing.T) {
	qrels := []Qrel{
		{TopicID: "SS-001", ChunkID: "chunk-a", Grade: 3},
		{TopicID: "SS-001", ChunkID: "chunk-b", Grade: 2},
	}
	run := []RunEntry{
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"},
		{RunID: "test", TopicID: "SS-001", Rank: 2, ChunkID: "chunk-b"},
	}
	report, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	v := report.Metrics["ndcg@5"]
	if math.Abs(v-1.0) > 1e-9 {
		t.Errorf("nDCG@5 = %f, want 1.0", v)
	}
}

func TestScoreRun_degradedRankingLowersScore(t *testing.T) {
	qrels := []Qrel{
		{TopicID: "SS-001", ChunkID: "chunk-a", Grade: 3},
		{TopicID: "SS-001", ChunkID: "chunk-b", Grade: 2},
	}
	// chunk-b (grade 2) at rank 1, chunk-a (grade 3) at rank 2 — degraded order
	run := []RunEntry{
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-b"},
		{RunID: "test", TopicID: "SS-001", Rank: 2, ChunkID: "chunk-a"},
	}
	report, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	v := report.Metrics["ndcg@5"]
	if v >= 1.0 {
		t.Errorf("nDCG@5 = %f, want < 1.0 for degraded ranking", v)
	}
}

func TestScoreRun_duplicateChunkID(t *testing.T) {
	qrels := []Qrel{
		{TopicID: "SS-001", ChunkID: "chunk-a", Grade: 3},
	}
	run := []RunEntry{
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"},
		{RunID: "test", TopicID: "SS-001", Rank: 2, ChunkID: "chunk-a"},
	}
	_, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}})
	if err == nil {
		t.Fatal("expected error for duplicate chunkId")
	}
}

func TestScoreRun_unjudgedChunkCountedAsGradeZero(t *testing.T) {
	qrels := []Qrel{
		{TopicID: "SS-001", ChunkID: "chunk-a", Grade: 3},
	}
	run := []RunEntry{
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"},
		{RunID: "test", TopicID: "SS-001", Rank: 2, ChunkID: "chunk-unknown"}, // unjudged
	}
	report, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Topics[0].UnjudgedAtK["5"] != 1 {
		t.Errorf("unjudged@5 = %d, want 1", report.Topics[0].UnjudgedAtK["5"])
	}
}

func TestScoreRun_kLargerThanResults(t *testing.T) {
	qrels := []Qrel{
		{TopicID: "SS-001", ChunkID: "chunk-a", Grade: 3},
	}
	run := []RunEntry{
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"},
	}
	_, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{10}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScoreRun_defaultKs(t *testing.T) {
	qrels := []Qrel{
		{TopicID: "SS-001", ChunkID: "chunk-a", Grade: 3},
	}
	run := []RunEntry{
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"},
	}
	report, err := ScoreRun(qrels, run, ScoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := report.Metrics["ndcg@5"]; !ok {
		t.Error("expected ndcg@5 in metrics when Ks is empty")
	}
	if _, ok := report.Metrics["ndcg@10"]; !ok {
		t.Error("expected ndcg@10 in metrics when Ks is empty")
	}
}

func TestScoreRun_topicSetFromQrels(t *testing.T) {
	qrels := []Qrel{
		{TopicID: "SS-001", ChunkID: "chunk-a", Grade: 3},
	}
	// SS-002 exists only in run, not qrels — should be ignored
	run := []RunEntry{
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"},
		{RunID: "test", TopicID: "SS-002", Rank: 1, ChunkID: "chunk-b"},
	}
	report, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	if report.TopicCount != 1 {
		t.Errorf("TopicCount = %d, want 1", report.TopicCount)
	}
}

func TestScoreRun_duplicateRank(t *testing.T) {
	qrels := []Qrel{
		{TopicID: "SS-001", ChunkID: "chunk-a", Grade: 3},
		{TopicID: "SS-001", ChunkID: "chunk-b", Grade: 2},
	}
	run := []RunEntry{
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"},
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-b"}, // same rank
	}
	_, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}})
	if err == nil {
		t.Fatal("expected error for duplicate rank within topic")
	}
}

func TestScoreRun_unjudgedUsesArrayIndex(t *testing.T) {
	// Non-contiguous ranks [1, 2, 6] with k=5:
	// rank-6 entry is at array index 2 (< k=5), so DCG@5 includes it.
	// With the old rank-based check (e.Rank <= k), rank 6 would not be counted.
	// With the correct index-based check, it must be counted as unjudged@5.
	qrels := []Qrel{
		{TopicID: "SS-001", ChunkID: "chunk-a", Grade: 3},
		{TopicID: "SS-001", ChunkID: "chunk-b", Grade: 2},
	}
	run := []RunEntry{
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"},
		{RunID: "test", TopicID: "SS-001", Rank: 2, ChunkID: "chunk-b"},
		{RunID: "test", TopicID: "SS-001", Rank: 6, ChunkID: "chunk-unknown"}, // idx=2, rank>k
	}
	report, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Topics[0].UnjudgedAtK["5"] != 1 {
		t.Errorf("unjudged@5 = %d, want 1 (rank 6 is at array index 2 < k=5)", report.Topics[0].UnjudgedAtK["5"])
	}
}

func TestScoreRun_emptyQrelsReturnsEmptyTopics(t *testing.T) {
	report, err := ScoreRun(nil, nil, ScoreOptions{Ks: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Topics == nil {
		t.Error("Topics should be an empty slice, not nil")
	}
	if len(report.Topics) != 0 {
		t.Errorf("Topics length = %d, want 0", len(report.Topics))
	}
}
