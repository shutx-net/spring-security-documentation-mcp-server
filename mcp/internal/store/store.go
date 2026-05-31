package store

import (
	"context"
	"time"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
)

// Status holds the current state of the documentation index.
type Status struct {
	TotalChunks int
	DocSets     []model.DocSet
	LastBuiltAt time.Time
}

// Store is the interface for reading and writing documentation chunks.
type Store interface {
	// Search returns chunks matching the given params.
	Search(ctx context.Context, params model.SearchParams) (model.SearchResult, error)

	// GetChunk returns a single chunk by ID.
	GetChunk(ctx context.Context, id string) (model.DocChunk, error)

	// ListDocSets returns all indexed documentation sets.
	ListDocSets(ctx context.Context) ([]model.DocSet, error)

	// Status returns the current state of the index.
	Status(ctx context.Context) (Status, error)

	// UpsertChunks writes chunks, replacing any existing chunk with the same ID.
	UpsertChunks(ctx context.Context, chunks []model.DocChunk) error

	// Close releases resources.
	Close() error
}
