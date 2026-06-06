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
	if v := report.Metrics["precision@5"]; math.Abs(v-0.2) > 1e-9 {
		t.Errorf("precision@5 = %f, want 0.2", v)
	}
	if v := report.Metrics["precision@10"]; math.Abs(v-0.1) > 1e-9 {
		t.Errorf("precision@10 = %f, want 0.1", v)
	}
}

func TestScoreRun_precisionDefaultThreshold(t *testing.T) {
	// grade 3 and grade 2 are relevant (threshold=2 default)
	// grade 1 is non-relevant
	qrels := []Qrel{
		{TopicID: "SS-001", ChunkID: "chunk-a", Grade: 3},
		{TopicID: "SS-001", ChunkID: "chunk-b", Grade: 2},
		{TopicID: "SS-001", ChunkID: "chunk-c", Grade: 1},
	}
	run := []RunEntry{
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"}, // relevant
		{RunID: "test", TopicID: "SS-001", Rank: 2, ChunkID: "chunk-b"}, // relevant
		{RunID: "test", TopicID: "SS-001", Rank: 3, ChunkID: "chunk-c"}, // non-relevant
	}
	report, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	// 2 relevant in top-5, denominator=5 → 0.4
	got := report.Metrics["precision@5"]
	want := 0.4
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("precision@5 = %f, want %f", got, want)
	}
	// TopicMetric also contains precision
	if _, ok := report.Topics[0].Precision["precision@5"]; !ok {
		t.Error("expected precision@5 in topic metrics")
	}
}

func TestScoreRun_precisionCustomThreshold(t *testing.T) {
	// threshold=3: only grade 3 is relevant
	qrels := []Qrel{
		{TopicID: "SS-001", ChunkID: "chunk-a", Grade: 3},
		{TopicID: "SS-001", ChunkID: "chunk-b", Grade: 2},
	}
	run := []RunEntry{
		{RunID: "test", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"}, // relevant
		{RunID: "test", TopicID: "SS-001", Rank: 2, ChunkID: "chunk-b"}, // non-relevant at threshold=3
	}
	report, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}, RelevanceThreshold: 3})
	if err != nil {
		t.Fatal(err)
	}
	// 1 relevant in top-5, denominator=5 → 0.2
	got := report.Metrics["precision@5"]
	want := 0.2
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("precision@5 = %f, want %f", got, want)
	}
}

func TestScoreRun_precisionShortResultsUseKAsDenominator(t *testing.T) {
	// Only 2 results, k=5: denominator must be 5
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
	// 2 relevant / 5 = 0.4
	got := report.Metrics["precision@5"]
	want := 0.4
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("precision@5 = %f, want %f", got, want)
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
