package eval

import (
	"strings"
	"testing"
)

// A stable-key qrel must keep matching after a reindex changes every chunkId.
func TestScoreRun_stableKeySurvivesReindex(t *testing.T) {
	qrels := []Qrel{
		{TopicID: "T", CanonicalURL: "https://x/csrf.html", HeadingPath: []string{"CSRF"}, Grade: 3},
	}
	// Same document/section, but a different (reindexed) chunkId hash.
	run := []RunEntry{
		{RunID: "hybrid", TopicID: "T", Rank: 1, ChunkID: "brand-new-hash", CanonicalURL: "https://x/csrf.html", HeadingPath: []string{"CSRF"}},
	}

	report, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Metrics["ndcg@5"]; got != 1.0 {
		t.Errorf("ndcg@5 = %v, want 1.0 (stable key should match despite new chunkId)", got)
	}
	if report.JudgedEntries != 1 || report.ScoredEntries != 1 {
		t.Errorf("coverage = %d/%d, want 1/1", report.JudgedEntries, report.ScoredEntries)
	}
}

// Legacy chunkId-only qrels must still work (backward compatibility).
func TestScoreRun_legacyChunkIDStillMatches(t *testing.T) {
	qrels := []Qrel{{TopicID: "T", ChunkID: "abc", Grade: 3}}
	run := []RunEntry{{RunID: "hybrid", TopicID: "T", Rank: 1, ChunkID: "abc"}}

	report, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Metrics["ndcg@5"]; got != 1.0 {
		t.Errorf("ndcg@5 = %v, want 1.0", got)
	}
	if report.JudgedEntries != 1 {
		t.Errorf("judgedEntries = %d, want 1", report.JudgedEntries)
	}
}

// The exact incident we hit: results retrieved, but none in the qrels pool.
// Coverage must expose it (JudgedEntries == 0 while ScoredEntries > 0).
func TestScoreRun_staleFixtureHasZeroJudged(t *testing.T) {
	qrels := []Qrel{{TopicID: "T", ChunkID: "old-hash", Grade: 3}}
	run := []RunEntry{
		{RunID: "hybrid", TopicID: "T", Rank: 1, ChunkID: "new-hash-1"},
		{RunID: "hybrid", TopicID: "T", Rank: 2, ChunkID: "new-hash-2"},
	}

	report, err := ScoreRun(qrels, run, ScoreOptions{Ks: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Metrics["ndcg@5"] != 0 {
		t.Errorf("ndcg@5 = %v, want 0", report.Metrics["ndcg@5"])
	}
	if report.ScoredEntries != 2 || report.JudgedEntries != 0 {
		t.Errorf("coverage = %d/%d, want 0/2", report.JudgedEntries, report.ScoredEntries)
	}
}

func TestLoadQrels_acceptsStableKeyAndRejectsNeither(t *testing.T) {
	// Stable-key-only qrel (no chunkId) is valid.
	ok := `{"topicId":"T","canonicalUrl":"https://x/a.html","headingPath":["A"],"grade":2}`
	qrels, err := LoadQrels(strings.NewReader(ok))
	if err != nil {
		t.Fatalf("stable-key qrel should load: %v", err)
	}
	if len(qrels) != 1 || qrels[0].judgedKey() != "u:"+StableKey("https://x/a.html", []string{"A"}) {
		t.Errorf("unexpected qrel: %+v", qrels)
	}

	// Neither chunkId nor canonicalUrl is invalid.
	if _, err := LoadQrels(strings.NewReader(`{"topicId":"T","grade":2}`)); err == nil {
		t.Error("qrel with neither chunkId nor canonicalUrl should error")
	}
}
