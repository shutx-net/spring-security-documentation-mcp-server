package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
)

// MemoryStore is an in-memory Store implementation for testing.
type MemoryStore struct {
	chunks []model.DocChunk // ordered slice for deterministic iteration
	index  map[string]int   // id → position in chunks
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{index: make(map[string]int)}
}

func (s *MemoryStore) Close() error { return nil }

func (s *MemoryStore) UpsertChunks(_ context.Context, chunks []model.DocChunk) error {
	for _, c := range chunks {
		if pos, ok := s.index[c.ID]; ok {
			s.chunks[pos] = c
		} else {
			s.index[c.ID] = len(s.chunks)
			s.chunks = append(s.chunks, c)
		}
	}
	return nil
}

func (s *MemoryStore) GetChunk(_ context.Context, id string) (model.DocChunk, error) {
	if pos, ok := s.index[id]; ok {
		return s.chunks[pos], nil
	}
	return model.DocChunk{}, fmt.Errorf("chunk %q not found", id)
}

func (s *MemoryStore) Search(_ context.Context, params model.SearchParams) (model.SearchResult, error) {
	limit := params.Limit
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	q := strings.ToLower(params.Query)
	var result model.SearchResult
	for _, c := range s.chunks {
		if !strings.Contains(strings.ToLower(c.ContentText), q) &&
			!strings.Contains(strings.ToLower(c.Title), q) {
			continue
		}
		if params.Ref != "" && c.Ref != params.Ref {
			continue
		}
		if params.Area != "" && string(c.Area) != params.Area {
			continue
		}
		result.Chunks = append(result.Chunks, c)
		if len(result.Chunks) >= limit {
			break
		}
	}
	return result, nil
}

func (s *MemoryStore) ListDocSets(_ context.Context) ([]model.DocSet, error) {
	type key struct{ ref, sha string }
	sets := map[key]*model.DocSet{}
	counts := map[key]int{}
	for _, c := range s.chunks {
		k := key{c.Ref, c.CommitSha}
		counts[k]++
		if _, ok := sets[k]; !ok {
			sets[k] = &model.DocSet{
				Ref:        c.Ref,
				CommitSha:  c.CommitSha,
				BuiltAt:    c.BuiltAt,
				SourceType: c.SourceType,
			}
		}
	}
	result := make([]model.DocSet, 0, len(sets))
	for k, ds := range sets {
		ds.ChunkCount = counts[k]
		result = append(result, *ds)
	}
	return result, nil
}

func (s *MemoryStore) Status(ctx context.Context) (Status, error) {
	sets, _ := s.ListDocSets(ctx)
	var st Status
	st.DocSets = sets
	for _, ds := range sets {
		st.TotalChunks += ds.ChunkCount
		if ds.BuiltAt.After(st.LastBuiltAt) {
			st.LastBuiltAt = ds.BuiltAt
		}
	}
	return st, nil
}
