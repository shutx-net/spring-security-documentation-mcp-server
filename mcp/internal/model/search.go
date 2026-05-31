package model

// SearchParams holds parameters for searching documentation chunks.
type SearchParams struct {
	Query string
	Ref   string // optional filter
	Area  string // optional filter
	Limit int    // 0 means default (10); max 50
}

// SearchResult holds the result of a search.
type SearchResult struct {
	Chunks []DocChunk
}
