package eval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// scanJSONL iterates over non-empty lines of a JSONL reader, calling fn for each.
// Uses a 1 MB buffer to handle large lines (fixes default 64 KB limit).
func scanJSONL(r io.Reader, fn func([]byte) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if err := fn(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// LoadQrels reads JSONL-formatted relevance judgments from r.
func LoadQrels(r io.Reader) ([]Qrel, error) {
	type key struct{ topicID, chunkID string }
	seen := make(map[key]struct{})
	var qrels []Qrel
	err := scanJSONL(r, func(line []byte) error {
		var q Qrel
		if err := json.Unmarshal(line, &q); err != nil {
			return fmt.Errorf("parse qrel: %w", err)
		}
		if q.TopicID == "" {
			return fmt.Errorf("missing topicId")
		}
		if q.ChunkID == "" {
			return fmt.Errorf("missing chunkId")
		}
		if q.Grade < 0 || q.Grade > 3 {
			return fmt.Errorf("invalid grade %d for chunkId %q", q.Grade, q.ChunkID)
		}
		k := key{q.TopicID, q.ChunkID}
		if _, dup := seen[k]; dup {
			return fmt.Errorf("duplicate (topicId, chunkId): (%q, %q)", q.TopicID, q.ChunkID)
		}
		seen[k] = struct{}{}
		qrels = append(qrels, q)
		return nil
	})
	return qrels, err
}

// LoadRun reads JSONL-formatted search results from r.
func LoadRun(r io.Reader) ([]RunEntry, error) {
	var entries []RunEntry
	err := scanJSONL(r, func(line []byte) error {
		var e RunEntry
		if err := json.Unmarshal(line, &e); err != nil {
			return fmt.Errorf("parse run entry: %w", err)
		}
		if e.TopicID == "" {
			return fmt.Errorf("missing topicId")
		}
		if e.ChunkID == "" {
			return fmt.Errorf("missing chunkId")
		}
		if e.Rank < 1 {
			return fmt.Errorf("invalid rank %d for chunkId %q", e.Rank, e.ChunkID)
		}
		entries = append(entries, e)
		return nil
	})
	return entries, err
}
