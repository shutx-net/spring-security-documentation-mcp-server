package eval

import "strings"

// Separators for StableKey. Use ASCII unit/record separators so they never
// collide with characters that appear in URLs or heading text.
const (
	stableFieldSep   = "\x1f"
	stableHeadingSep = "\x1e"
)

// StableKey returns a reindex-stable identity for a chunk, independent of the
// commitSha that is baked into ChunkID. Two chunks with the same canonical URL
// and heading path are the same judged item across re-indexing, so keying
// relevance judgments on this survives mechanical reindexing (which changes
// every ChunkID hash).
func StableKey(canonicalURL string, headingPath []string) string {
	return canonicalURL + stableFieldSep + strings.Join(headingPath, stableHeadingSep)
}

// Topic is a single evaluation information need to run searches against.
type Topic struct {
	TopicID string `json:"topicId"`
	Query   string `json:"query"`
	Ref     string `json:"ref,omitempty"`
	Area    string `json:"area,omitempty"`
}

// Qrel is a single relevance judgment: topic × chunk → grade.
//
// A judgment is identified by a stable key (CanonicalURL + HeadingPath) when
// present, falling back to the legacy commit-dependent ChunkID otherwise. Pin
// judgments to the stable key so they do not silently break on reindex.
type Qrel struct {
	TopicID      string   `json:"topicId"`
	ChunkID      string   `json:"chunkId,omitempty"`
	CanonicalURL string   `json:"canonicalUrl,omitempty"`
	HeadingPath  []string `json:"headingPath,omitempty"`
	Grade        int      `json:"grade"` // 0..3
}

// judgedKey returns the key this qrel is judged under: the stable key when a
// canonical URL is present, otherwise the legacy chunkId. The "u:"/"c:" prefixes
// keep the two key spaces disjoint.
func (q Qrel) judgedKey() string {
	if q.CanonicalURL != "" {
		return "u:" + StableKey(q.CanonicalURL, q.HeadingPath)
	}
	return "c:" + q.ChunkID
}

// RunEntry is a single search result from an IR system.
type RunEntry struct {
	RunID        string   `json:"runId"`
	TopicID      string   `json:"topicId"`
	Rank         int      `json:"rank"` // 1-based
	ChunkID      string   `json:"chunkId"`
	CanonicalURL string   `json:"canonicalUrl,omitempty"`
	HeadingPath  []string `json:"headingPath,omitempty"`
	Score        float64  `json:"score,omitempty"`
}

// lookupKeys returns the keys this entry may be judged under, most stable first,
// so scoring matches a stable-key qrel when possible and a legacy chunkId qrel
// otherwise.
func (e RunEntry) lookupKeys() []string {
	keys := make([]string, 0, 2)
	if e.CanonicalURL != "" {
		keys = append(keys, "u:"+StableKey(e.CanonicalURL, e.HeadingPath))
	}
	if e.ChunkID != "" {
		keys = append(keys, "c:"+e.ChunkID)
	}
	return keys
}

// TopicMetric holds per-topic evaluation scores.
type TopicMetric struct {
	TopicID     string             `json:"topicId"`
	NDCG        map[string]float64 `json:"ndcg"`        // e.g. {"ndcg@5": 0.9}
	Precision   map[string]float64 `json:"precision"`   // e.g. {"precision@5": 0.4}
	UnjudgedAtK map[string]int     `json:"unjudgedAtK"` // e.g. {"5": 1, "10": 2}
}

// RunReport is the overall evaluation report for a run.
type RunReport struct {
	RunID       string             `json:"runId"`
	Metrics     map[string]float64 `json:"metrics"` // mean nDCG across topics
	TopicCount  int                `json:"topicCount"`
	Topics      []TopicMetric      `json:"topics"`
	UnjudgedAtK map[string]float64 `json:"unjudgedAtK"` // mean unjudged per topic at k

	// Coverage: how many scored run entries (in qrel topics) matched a qrel.
	// JudgedEntries == 0 while ScoredEntries > 0 means the run and the qrels
	// share no items — the fixture is almost certainly stale.
	ScoredEntries int `json:"scoredEntries"`
	JudgedEntries int `json:"judgedEntries"`
}

// ScoreOptions controls metric computation.
type ScoreOptions struct {
	Ks                 []int // k values to compute; defaults to [5, 10]
	RelevanceThreshold int   // minimum grade treated as relevant for Precision@k; defaults to 2
}

func (o ScoreOptions) relevanceThreshold() int {
	if o.RelevanceThreshold <= 0 {
		return 2
	}
	return o.RelevanceThreshold
}
