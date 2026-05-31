package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
)

const schema = `
CREATE TABLE IF NOT EXISTS chunks (
	id               TEXT PRIMARY KEY,
	project          TEXT NOT NULL,
	ref              TEXT NOT NULL,
	commit_sha       TEXT NOT NULL,
	built_at         TEXT NOT NULL,
	source_type      TEXT NOT NULL,
	source_path      TEXT NOT NULL,
	source_file      TEXT NOT NULL DEFAULT '',
	canonical_url    TEXT NOT NULL,
	title            TEXT NOT NULL,
	heading_path     TEXT NOT NULL,
	area             TEXT NOT NULL,
	content_html TEXT NOT NULL,
	content_text     TEXT NOT NULL,
	indexed_at       TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
	title,
	content_text,
	heading_path,
	content='chunks',
	content_rowid='rowid'
);

CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
	INSERT INTO chunks_fts(rowid, title, content_text, heading_path)
	VALUES (new.rowid, new.title, new.content_text, new.heading_path);
END;

CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
	INSERT INTO chunks_fts(chunks_fts, rowid, title, content_text, heading_path)
	VALUES ('delete', old.rowid, old.title, old.content_text, old.heading_path);
END;

CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
	INSERT INTO chunks_fts(chunks_fts, rowid, title, content_text, heading_path)
	VALUES ('delete', old.rowid, old.title, old.content_text, old.heading_path);
	INSERT INTO chunks_fts(rowid, title, content_text, heading_path)
	VALUES (new.rowid, new.title, new.content_text, new.heading_path);
END;
`

// SQLiteStore implements Store using a local SQLite database with FTS5.
type SQLiteStore struct {
	db *sql.DB
}

// Open opens (or creates) a SQLite store at the given path.
func Open(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) UpsertChunks(ctx context.Context, chunks []model.DocChunk) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO chunks
			(id, project, ref, commit_sha, built_at, source_type, source_path, source_file,
			 canonical_url, title, heading_path, area, content_html, content_text, indexed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			project=excluded.project, ref=excluded.ref, commit_sha=excluded.commit_sha,
			built_at=excluded.built_at, source_type=excluded.source_type,
			source_path=excluded.source_path, source_file=excluded.source_file,
			canonical_url=excluded.canonical_url, title=excluded.title,
			heading_path=excluded.heading_path, area=excluded.area,
			content_html=excluded.content_html, content_text=excluded.content_text,
			indexed_at=excluded.indexed_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range chunks {
		hp, _ := json.Marshal(c.HeadingPath)
		_, err := stmt.ExecContext(ctx,
			c.ID, c.Project, c.Ref, c.CommitSha,
			c.BuiltAt.UTC().Format(time.RFC3339),
			string(c.SourceType), c.SourcePath, c.SourceFile,
			c.CanonicalURL, c.Title, string(hp), string(c.Area),
			c.ContentHtml, c.ContentText,
			c.IndexedAt.UTC().Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("upsert chunk %s: %w", c.ID, err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) Search(ctx context.Context, params model.SearchParams) (model.SearchResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	query := `
		SELECT c.id, c.project, c.ref, c.commit_sha, c.built_at, c.source_type,
		       c.source_path, c.source_file, c.canonical_url, c.title,
		       c.heading_path, c.area, c.content_html, c.content_text, c.indexed_at
		FROM chunks c
		JOIN chunks_fts fts ON c.rowid = fts.rowid
		WHERE chunks_fts MATCH ?
		  AND (? = '' OR c.ref = ?)
		  AND (? = '' OR c.area = ?)
		ORDER BY rank
		LIMIT ?
	`
	rows, err := s.db.QueryContext(ctx, query,
		params.Query,
		params.Ref, params.Ref,
		params.Area, params.Area,
		limit,
	)
	if err != nil {
		return model.SearchResult{}, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var result model.SearchResult
	for rows.Next() {
		c, err := scanChunk(rows)
		if err != nil {
			return model.SearchResult{}, err
		}
		result.Chunks = append(result.Chunks, c)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) GetChunk(ctx context.Context, id string) (model.DocChunk, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project, ref, commit_sha, built_at, source_type,
		       source_path, source_file, canonical_url, title,
		       heading_path, area, content_html, content_text, indexed_at
		FROM chunks WHERE id = ?
	`, id)
	c, err := scanChunk(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.DocChunk{}, fmt.Errorf("chunk %q not found", id)
	}
	return c, err
}

func (s *SQLiteStore) ListDocSets(ctx context.Context) ([]model.DocSet, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ref, commit_sha, MAX(built_at) as built_at, COUNT(*) as chunk_count, source_type
		FROM chunks
		GROUP BY ref
		ORDER BY ref
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sets []model.DocSet
	for rows.Next() {
		var ds model.DocSet
		var builtAtStr string
		if err := rows.Scan(&ds.Ref, &ds.CommitSha, &builtAtStr, &ds.ChunkCount, &ds.SourceType); err != nil {
			return nil, err
		}
		ds.BuiltAt, _ = time.Parse(time.RFC3339, builtAtStr)
		sets = append(sets, ds)
	}
	return sets, rows.Err()
}

func (s *SQLiteStore) Status(ctx context.Context) (Status, error) {
	var st Status
	var lastStr sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), MAX(built_at) FROM chunks
	`).Scan(&st.TotalChunks, &lastStr)
	if err != nil {
		return st, err
	}
	if lastStr.Valid {
		st.LastBuiltAt, _ = time.Parse(time.RFC3339, lastStr.String)
	}
	st.DocSets, err = s.ListDocSets(ctx)
	return st, err
}

// scanner abstracts *sql.Row and *sql.Rows for scanChunk.
type scanner interface {
	Scan(dest ...any) error
}

func scanChunk(s scanner) (model.DocChunk, error) {
	var c model.DocChunk
	var builtAtStr, indexedAtStr, headingPathJSON string
	err := s.Scan(
		&c.ID, &c.Project, &c.Ref, &c.CommitSha, &builtAtStr,
		&c.SourceType, &c.SourcePath, &c.SourceFile,
		&c.CanonicalURL, &c.Title, &headingPathJSON, &c.Area,
		&c.ContentHtml, &c.ContentText, &indexedAtStr,
	)
	if err != nil {
		return c, err
	}
	c.BuiltAt, _ = time.Parse(time.RFC3339, builtAtStr)
	c.IndexedAt, _ = time.Parse(time.RFC3339, indexedAtStr)
	_ = json.Unmarshal([]byte(headingPathJSON), &c.HeadingPath)
	return c, nil
}
