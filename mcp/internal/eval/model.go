package eval

// Qrel is a single relevance judgment: topic × chunk → grade.
type Qrel struct {
	TopicID string `json:"topicId"`
	ChunkID string `json:"chunkId"`
	Grade   int    `json:"grade"` // 0..3
}

// RunEntry is a single search result from an IR system.
type RunEntry struct {
	RunID   string  `json:"runId"`
	TopicID string  `json:"topicId"`
	Rank    int     `json:"rank"` // 1-based
	ChunkID string  `json:"chunkId"`
	Score   float64 `json:"score,omitempty"`
}

// TopicMetric holds per-topic evaluation scores.
type TopicMetric struct {
	TopicID     string             `json:"topicId"`
	NDCG        map[string]float64 `json:"ndcg"`        // e.g. {"ndcg@5": 0.9}
	UnjudgedAtK map[string]int     `json:"unjudgedAtK"` // e.g. {"5": 1, "10": 2}
}

// RunReport is the overall evaluation report for a run.
type RunReport struct {
	RunID       string             `json:"runId"`
	Metrics     map[string]float64 `json:"metrics"`     // mean nDCG across topics
	TopicCount  int                `json:"topicCount"`
	Topics      []TopicMetric      `json:"topics"`
	UnjudgedAtK map[string]float64 `json:"unjudgedAtK"` // mean unjudged per topic at k
}

// ScoreOptions controls nDCG computation.
type ScoreOptions struct {
	Ks []int // k values to compute; defaults to [5, 10]
}
