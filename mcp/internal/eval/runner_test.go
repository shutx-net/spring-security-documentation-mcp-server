package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

func seedChunk(id, title, text, ref string, area model.Area) model.DocChunk {
	return model.DocChunk{
		ID:          id,
		Title:       title,
		ContentText: text,
		Ref:         ref,
		Area:        area,
	}
}

func TestSearchTopics_basic(t *testing.T) {
	st := store.NewMemoryStore()
	if err := st.UpsertChunks(context.Background(), []model.DocChunk{
		seedChunk("chunk-a", "SecurityFilterChain", "Configures the security filter chain.", "6.5.x", model.AreaServlet),
		seedChunk("chunk-b", "SecurityFilterChain basics", "Basics of the security filter chain.", "6.5.x", model.AreaServlet),
		seedChunk("chunk-c", "CsrfFilter", "Cross-site request forgery protection.", "6.5.x", model.AreaServlet),
	}); err != nil {
		t.Fatal(err)
	}

	topics := []Topic{{TopicID: "SS-001", Query: "securityfilterchain"}}
	entries, err := SearchTopics(context.Background(), st, topics, RunOptions{RunID: "hybrid", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	want := []RunEntry{
		{RunID: "hybrid", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"},
		{RunID: "hybrid", TopicID: "SS-001", Rank: 2, ChunkID: "chunk-b"},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i, e := range entries {
		if e != want[i] {
			t.Errorf("entry[%d] = %+v, want %+v", i, e, want[i])
		}
	}
}

func TestSearchTopics_multipleTopics(t *testing.T) {
	st := store.NewMemoryStore()
	if err := st.UpsertChunks(context.Background(), []model.DocChunk{
		seedChunk("chunk-a", "SecurityFilterChain", "filterchain content", "6.5.x", model.AreaServlet),
		seedChunk("chunk-b", "CsrfFilter", "csrffilter content", "6.5.x", model.AreaServlet),
	}); err != nil {
		t.Fatal(err)
	}

	topics := []Topic{
		{TopicID: "SS-001", Query: "securityfilterchain"},
		{TopicID: "SS-002", Query: "csrffilter"},
	}
	entries, err := SearchTopics(context.Background(), st, topics, RunOptions{RunID: "hybrid", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	want := []RunEntry{
		{RunID: "hybrid", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"},
		{RunID: "hybrid", TopicID: "SS-002", Rank: 1, ChunkID: "chunk-b"},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i, e := range entries {
		if e != want[i] {
			t.Errorf("entry[%d] = %+v, want %+v", i, e, want[i])
		}
	}
}

func TestSearchTopics_topicFiltersPropagate(t *testing.T) {
	st := store.NewMemoryStore()
	if err := st.UpsertChunks(context.Background(), []model.DocChunk{
		seedChunk("chunk-old-ref", "SecurityFilterChain", "filterchain content", "6.4.x", model.AreaServlet),
		seedChunk("chunk-new", "SecurityFilterChain", "filterchain content", "6.5.x", model.AreaServlet),
		seedChunk("chunk-other-area", "SecurityFilterChain", "filterchain content", "6.5.x", model.AreaReactive),
	}); err != nil {
		t.Fatal(err)
	}

	topics := []Topic{{TopicID: "SS-001", Query: "securityfilterchain", Ref: "6.5.x", Area: "servlet"}}
	entries, err := SearchTopics(context.Background(), st, topics, RunOptions{RunID: "hybrid", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 || entries[0].ChunkID != "chunk-new" {
		t.Fatalf("got %+v, want a single entry for chunk-new (topic ref/area should filter out the others)", entries)
	}
}

func TestSearchTopics_callsOnTopic(t *testing.T) {
	st := store.NewMemoryStore()
	if err := st.UpsertChunks(context.Background(), []model.DocChunk{
		seedChunk("chunk-a", "SecurityFilterChain", "filterchain content", "6.5.x", model.AreaServlet),
	}); err != nil {
		t.Fatal(err)
	}

	topics := []Topic{
		{TopicID: "SS-001", Query: "securityfilterchain"},
		{TopicID: "SS-002", Query: "securityfilterchain"},
	}

	type call struct {
		i, total int
		topicID  string
	}
	var calls []call
	_, err := SearchTopics(context.Background(), st, topics, RunOptions{
		RunID: "hybrid",
		Limit: 10,
		OnTopic: func(i, total int, topic Topic) {
			calls = append(calls, call{i, total, topic.TopicID})
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []call{
		{1, 2, "SS-001"},
		{2, 2, "SS-002"},
	}
	if len(calls) != len(want) {
		t.Fatalf("got %d OnTopic calls, want %d: %+v", len(calls), len(want), calls)
	}
	for i, c := range calls {
		if c != want[i] {
			t.Errorf("call[%d] = %+v, want %+v", i, c, want[i])
		}
	}
}

// erroringStore is a minimal store.Store fake that fails every Search call.
// Embedding the interface satisfies store.Store without implementing the
// other methods, which SearchTopics never calls.
type erroringStore struct {
	store.Store
	err error
}

func (s erroringStore) Search(context.Context, model.SearchParams) (model.SearchResult, error) {
	return model.SearchResult{}, s.err
}

func TestSearchTopics_wrapsSearchError(t *testing.T) {
	wantErr := errors.New("boom")
	st := erroringStore{err: wantErr}

	_, err := SearchTopics(context.Background(), st, []Topic{{TopicID: "SS-001", Query: "q"}}, RunOptions{RunID: "hybrid", Limit: 10})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want it to wrap %v", err, wantErr)
	}
	if !strings.Contains(err.Error(), "SS-001") {
		t.Errorf("error = %v, want it to mention topicId SS-001", err)
	}
}

func TestWriteRun_jsonl(t *testing.T) {
	entries := []RunEntry{
		{RunID: "hybrid", TopicID: "SS-001", Rank: 1, ChunkID: "chunk-a"},
		{RunID: "hybrid", TopicID: "SS-001", Rank: 2, ChunkID: "chunk-b"},
	}
	var buf bytes.Buffer
	if err := WriteRun(&buf, entries); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(entries) {
		t.Fatalf("got %d lines, want %d: %q", len(lines), len(entries), buf.String())
	}
	for i, line := range lines {
		if strings.Contains(line, "score") {
			t.Errorf("line %d should omit score: %s", i, line)
		}
		var got RunEntry
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		if got != entries[i] {
			t.Errorf("line %d decoded = %+v, want %+v", i, got, entries[i])
		}
	}
}
