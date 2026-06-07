package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

// RunOptions controls how topics are executed against a search backend.
type RunOptions struct {
	RunID string
	Limit int

	// OnTopic, if set, is called immediately before each topic's search runs,
	// with the topic's 1-based position and the total topic count. It lets
	// the CLI report per-topic progress (e.g. via log.Printf) for long runs
	// without internal/eval taking a dependency on a logger.
	OnTopic func(i, total int, topic Topic)
}

// SearchTopics runs each topic's query against st using the existing search
// usecase and returns ranked run entries compatible with eval score's RunEntry.
//
// Topics are processed sequentially: the production Search already fans out to
// multiple backends (keyword/vector/keywords-table) and calls Bedrock per query,
// so adding topic-level concurrency would raise throttling risk and make
// failures harder to attribute to a specific topic.
//
// Score is intentionally left unset: Search does not expose a per-chunk
// relevance score (vector similarity scores are discarded once chunks are
// rehydrated from the store), and eval score does not use RunEntry.Score for
// scoring. A synthetic rank-derived value would be misleading.
func SearchTopics(ctx context.Context, st store.Store, topics []Topic, opts RunOptions) ([]RunEntry, error) {
	var entries []RunEntry
	for topicIdx, t := range topics {
		if opts.OnTopic != nil {
			opts.OnTopic(topicIdx+1, len(topics), t)
		}
		result, err := st.Search(ctx, model.SearchParams{
			Query: t.Query,
			Ref:   t.Ref,
			Area:  t.Area,
			Limit: opts.Limit,
		})
		if err != nil {
			return nil, fmt.Errorf("search topic %q: %w", t.TopicID, err)
		}
		for i, c := range result.Chunks {
			entries = append(entries, RunEntry{
				RunID:   opts.RunID,
				TopicID: t.TopicID,
				Rank:    i + 1,
				ChunkID: c.ID,
			})
		}
	}
	return entries, nil
}

// WriteRun writes run entries as JSONL (one JSON object per line) to w.
func WriteRun(w io.Writer, entries []RunEntry) error {
	enc := json.NewEncoder(w)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("write run entry: %w", err)
		}
	}
	return nil
}
