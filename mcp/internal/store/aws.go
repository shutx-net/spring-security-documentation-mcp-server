package store

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
)

// AWSConfig holds configuration for the DynamoDB-backed store.
type AWSConfig struct {
	Region         string // defaults to AWS_REGION env var
	ChunksTable    string // DynamoDB table for doc chunks
	ChunksTableGsi string // GSI name for ref-commitSha queries

	// Keyword index (optional — enables O(1) lookup for known class names)
	KeywordsTable string // DynamoDB table for keyword index

	// Semantic search (optional — if unset, falls back to keyword-only search)
	VectorBucket        string // S3 Vectors bucket ARN
	VectorIndex         string // S3 Vectors index ARN
	EmbeddingModelID    string // Bedrock model ID (e.g. amazon.titan-embed-text-v2:0)
	EmbeddingCacheTable string // DynamoDB table for embedding cache
}

// AWSConfigFromEnv reads configuration from environment variables.
//
// Required:
//   CHUNKS_TABLE  — DynamoDB table name for doc chunks (injected by service-stack.ts)
//
// Optional (standard AWS SDK env vars are also honoured):
//   AWS_REGION / AWS_DEFAULT_REGION
//   VECTOR_BUCKET, VECTOR_INDEX, EMBEDDING_MODEL_ID, EMBEDDING_CACHE_TABLE
func AWSConfigFromEnv() AWSConfig {
	return AWSConfig{
		Region:              envOr("AWS_REGION", envOr("AWS_DEFAULT_REGION", "us-east-1")),
		ChunksTable:         os.Getenv("CHUNKS_TABLE"),
		ChunksTableGsi:      envOr("CHUNKS_TABLE_GSI", "ref-commitSha-index"),
		KeywordsTable:       os.Getenv("KEYWORDS_TABLE"),
		VectorBucket:        os.Getenv("VECTOR_BUCKET"),
		VectorIndex:         os.Getenv("VECTOR_INDEX"),
		EmbeddingModelID:    os.Getenv("EMBEDDING_MODEL_ID"),
		EmbeddingCacheTable: os.Getenv("EMBEDDING_CACHE_TABLE"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// AWSStore implements Store using Amazon DynamoDB, Bedrock, and S3 Vectors.
type AWSStore struct {
	ddb      *dynamodb.Client
	embedder *bedrockEmbedder  // nil when semantic search is not configured
	sv       *s3vectors.Client // nil when semantic search is not configured
	cache    *embeddingCache   // nil when embedding cache table is not configured
	config   AWSConfig
}

// NewAWSStore creates a DynamoDB-backed Store.
// Bedrock and S3 Vectors clients are initialised only when the required env vars are set.
func NewAWSStore(ctx context.Context, cfg AWSConfig) (*AWSStore, error) {
	if cfg.ChunksTable == "" {
		return nil, fmt.Errorf("CHUNKS_TABLE is not set")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	store := &AWSStore{
		ddb:    dynamodb.NewFromConfig(awsCfg),
		config: cfg,
	}

	if cfg.VectorIndex != "" && cfg.EmbeddingModelID != "" {
		store.embedder = newBedrockEmbedder(bedrockruntime.NewFromConfig(awsCfg), cfg.EmbeddingModelID)
		store.sv = s3vectors.NewFromConfig(awsCfg)
	}
	if cfg.EmbeddingCacheTable != "" && store.embedder != nil {
		store.cache = &embeddingCache{ddb: store.ddb, table: cfg.EmbeddingCacheTable}
	}

	return store, nil
}

func (s *AWSStore) Close() error { return nil }

// ChunksTable returns the DynamoDB table name used for doc chunks.
func (s *AWSStore) ChunksTable() string { return s.config.ChunksTable }

// UpsertChunks writes chunks to DynamoDB in batches of 25 (API limit).
func (s *AWSStore) UpsertChunks(ctx context.Context, chunks []model.DocChunk) error {
	const maxBatch = 25
	for i := 0; i < len(chunks); i += maxBatch {
		end := i + maxBatch
		if end > len(chunks) {
			end = len(chunks)
		}
		var reqs []types.WriteRequest
		for _, c := range chunks[i:end] {
			item, err := attributevalue.MarshalMap(toItem(c))
			if err != nil {
				return fmt.Errorf("marshal chunk %s: %w", c.ID, err)
			}
			reqs = append(reqs, types.WriteRequest{
				PutRequest: &types.PutRequest{Item: item},
			})
		}
		if _, err := s.ddb.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{s.config.ChunksTable: reqs},
		}); err != nil {
			return fmt.Errorf("batch write chunks: %w", err)
		}
	}
	return nil
}

// GetChunk fetches a single chunk by its PK (chunkId).
func (s *AWSStore) GetChunk(ctx context.Context, id string) (model.DocChunk, error) {
	out, err := s.ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.config.ChunksTable),
		Key: map[string]types.AttributeValue{
			"chunkId": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return model.DocChunk{}, fmt.Errorf("get chunk: %w", err)
	}
	if out.Item == nil {
		return model.DocChunk{}, fmt.Errorf("chunk %q not found", id)
	}
	var item chunkItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return model.DocChunk{}, fmt.Errorf("unmarshal chunk: %w", err)
	}
	return fromItem(item), nil
}

// Search returns chunks matching the query.
// Runs up to three searches concurrently and merges results in priority order:
//  1. vector search (semantic, via Bedrock + S3 Vectors)
//  2. keywords table search (exact keyword lookup, O(1))
//  3. keyword scan (contains() filter on contentText, fallback)
func (s *AWSStore) Search(ctx context.Context, params model.SearchParams) (model.SearchResult, error) {
	limit := params.Limit
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	// Fast path: neither semantic nor keyword-table search configured.
	if s.embedder == nil && s.config.KeywordsTable == "" {
		chunks, err := s.keywordSearch(ctx, params, limit)
		return model.SearchResult{Chunks: chunks}, err
	}

	type searchResult struct {
		chunks []model.DocChunk
		err    error
	}
	kCh := make(chan searchResult, 1)
	vCh := make(chan searchResult, 1)
	ktCh := make(chan searchResult, 1)

	go func() {
		chunks, err := s.keywordSearch(ctx, params, limit)
		kCh <- searchResult{chunks, err}
	}()
	go func() {
		if s.embedder != nil {
			chunks, err := s.vectorSearch(ctx, params, limit*2)
			vCh <- searchResult{chunks, err}
		} else {
			vCh <- searchResult{}
		}
	}()
	go func() {
		if s.config.KeywordsTable != "" {
			chunks, err := s.keywordsTableSearch(ctx, params, limit)
			ktCh <- searchResult{chunks, err}
		} else {
			ktCh <- searchResult{}
		}
	}()

	kr, vr, ktr := <-kCh, <-vCh, <-ktCh
	if kr.err != nil {
		log.Printf("keyword search error: %v", kr.err)
	}
	if vr.err != nil {
		log.Printf("vector search error: %v", vr.err)
	}
	if ktr.err != nil {
		log.Printf("keywords table search error: %v", ktr.err)
	}
	if kr.err != nil && vr.err != nil && ktr.err != nil {
		return model.SearchResult{}, fmt.Errorf("search failed: %w", kr.err)
	}

	return model.SearchResult{Chunks: mergeSearchResults(limit, vr.chunks, ktr.chunks, kr.chunks)}, nil
}

// keywordSearch performs a DynamoDB Scan with a contains() filter on contentText.
func (s *AWSStore) keywordSearch(ctx context.Context, params model.SearchParams, limit int) ([]model.DocChunk, error) {
	var filters []string
	names := map[string]string{"#ct": "contentText"}
	values := map[string]types.AttributeValue{
		":q": &types.AttributeValueMemberS{Value: params.Query},
	}
	filters = append(filters, "contains(#ct, :q)")

	if params.Ref != "" {
		filters = append(filters, "#ref = :ref")
		names["#ref"] = "ref"
		values[":ref"] = &types.AttributeValueMemberS{Value: params.Ref}
	}
	if params.Area != "" {
		filters = append(filters, "#area = :area")
		names["#area"] = "area"
		values[":area"] = &types.AttributeValueMemberS{Value: params.Area}
	}

	input := &dynamodb.ScanInput{
		TableName:                 aws.String(s.config.ChunksTable),
		FilterExpression:          aws.String(strings.Join(filters, " AND ")),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
	}

	var chunks []model.DocChunk
	paginator := dynamodb.NewScanPaginator(s.ddb, input)
	for paginator.HasMorePages() && len(chunks) < limit {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("scan page: %w", err)
		}
		for _, raw := range page.Items {
			if len(chunks) >= limit {
				break
			}
			var item chunkItem
			if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
				continue
			}
			chunks = append(chunks, fromItem(item))
		}
	}
	return chunks, nil
}

// vectorSearch embeds the query with Bedrock, queries S3 Vectors, and fetches
// the matching chunks from DynamoDB via BatchGetItem.
func (s *AWSStore) vectorSearch(ctx context.Context, params model.SearchParams, topK int) ([]model.DocChunk, error) {
	vec, err := s.getOrEmbedQuery(ctx, params.Query)
	if err != nil {
		return nil, err
	}

	matches, err := queryVectors(ctx, s.sv, s.config.VectorIndex, vec, topK, params.Ref, params.Area)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}

	ids := make([]string, len(matches))
	for i, m := range matches {
		ids[i] = m.Key
	}
	return s.batchGetChunks(ctx, ids)
}

// getOrEmbedQuery returns a cached embedding or calls Bedrock to compute one.
func (s *AWSStore) getOrEmbedQuery(ctx context.Context, query string) ([]float32, error) {
	if s.cache != nil {
		hash := queryHash(query)
		if v, ok := s.cache.get(ctx, hash); ok {
			return v, nil
		}
		v, err := s.embedder.embed(ctx, query)
		if err != nil {
			return nil, err
		}
		s.cache.put(ctx, hash, v)
		return v, nil
	}
	return s.embedder.embed(ctx, query)
}

// batchGetChunks fetches chunks by ID in batches of 100 (BatchGetItem limit).
func (s *AWSStore) batchGetChunks(ctx context.Context, ids []string) ([]model.DocChunk, error) {
	const maxBatch = 100
	var result []model.DocChunk

	for i := 0; i < len(ids); i += maxBatch {
		end := i + maxBatch
		if end > len(ids) {
			end = len(ids)
		}
		keys := make([]map[string]types.AttributeValue, end-i)
		for j, id := range ids[i:end] {
			keys[j] = map[string]types.AttributeValue{
				"chunkId": &types.AttributeValueMemberS{Value: id},
			}
		}

		remaining := map[string]types.KeysAndAttributes{
			s.config.ChunksTable: {Keys: keys},
		}
		for len(remaining) > 0 {
			out, err := s.ddb.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
				RequestItems: remaining,
			})
			if err != nil {
				return nil, fmt.Errorf("batch get chunks: %w", err)
			}
			for _, raw := range out.Responses[s.config.ChunksTable] {
				var item chunkItem
				if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
					continue
				}
				result = append(result, fromItem(item))
			}
			remaining = out.UnprocessedKeys
			if len(remaining) > 0 {
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
	return result, nil
}

// keywordsTableSearch queries the keywords index table for an exact keyword match.
// Uses begins_with on the sort key (refAreaChunkId) to filter by ref and area efficiently.
func (s *AWSStore) keywordsTableSearch(ctx context.Context, params model.SearchParams, limit int) ([]model.DocChunk, error) {
	keyCond := "#kw = :kw"
	names := map[string]string{"#kw": "keyword"}
	values := map[string]types.AttributeValue{
		":kw": &types.AttributeValueMemberS{Value: params.Query},
	}

	if params.Ref != "" && params.Area != "" {
		keyCond += " AND begins_with(refAreaChunkId, :prefix)"
		values[":prefix"] = &types.AttributeValueMemberS{Value: params.Ref + "#" + params.Area + "#"}
	} else if params.Ref != "" {
		keyCond += " AND begins_with(refAreaChunkId, :prefix)"
		values[":prefix"] = &types.AttributeValueMemberS{Value: params.Ref + "#"}
	}

	out, err := s.ddb.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.config.KeywordsTable),
		KeyConditionExpression:    aws.String(keyCond),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
	})
	if err != nil {
		return nil, fmt.Errorf("keywords table query: %w", err)
	}
	if len(out.Items) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{})
	ids := make([]string, 0, len(out.Items))
	for _, item := range out.Items {
		v, ok := item["chunkId"].(*types.AttributeValueMemberS)
		if !ok {
			continue
		}
		if _, dup := seen[v.Value]; dup {
			continue
		}
		seen[v.Value] = struct{}{}
		ids = append(ids, v.Value)
		if len(ids) >= limit {
			break
		}
	}
	return s.batchGetChunks(ctx, ids)
}

// mergeSearchResults deduplicates and merges results from multiple sources in priority order.
func mergeSearchResults(limit int, sources ...[]model.DocChunk) []model.DocChunk {
	seen := make(map[string]struct{})
	out := make([]model.DocChunk, 0, limit)
	for _, source := range sources {
		for _, c := range source {
			if len(out) >= limit {
				return out
			}
			if _, dup := seen[c.ID]; !dup {
				seen[c.ID] = struct{}{}
				out = append(out, c)
			}
		}
	}
	return out
}

// ListDocSets returns unique documentation sets by paginating the full chunks table.
func (s *AWSStore) ListDocSets(ctx context.Context) ([]model.DocSet, error) {
	input := &dynamodb.ScanInput{
		TableName:            aws.String(s.config.ChunksTable),
		ProjectionExpression: aws.String("#ref, #commitSha, #builtAt, #sourceType"),
		ExpressionAttributeNames: map[string]string{
			"#ref":        "ref",
			"#commitSha":  "commitSha",
			"#builtAt":    "builtAt",
			"#sourceType": "sourceType",
		},
	}

	type key struct{ ref, sha string }
	sets := map[key]*model.DocSet{}
	counts := map[key]int{}

	paginator := dynamodb.NewScanPaginator(s.ddb, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("scan page for docsets: %w", err)
		}
		for _, raw := range page.Items {
			var item chunkItem
			if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
				continue
			}
			k := key{item.Ref, item.CommitSha}
			counts[k]++
			if _, ok := sets[k]; !ok {
				t, _ := time.Parse(time.RFC3339, item.BuiltAt)
				sets[k] = &model.DocSet{
					Ref:        item.Ref,
					CommitSha:  item.CommitSha,
					BuiltAt:    t,
					SourceType: model.SourceType(item.SourceType),
				}
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

// DeleteDocSet deletes all chunks for the given ref + commitSha using the GSI.
// Returns the number of deleted chunks.
func (s *AWSStore) DeleteDocSet(ctx context.Context, ref, commitSha string) (int, error) {
	var ids []string

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.config.ChunksTable),
		IndexName:              aws.String(s.config.ChunksTableGsi),
		KeyConditionExpression: aws.String("#ref = :ref AND commitSha = :commitSha"),
		ExpressionAttributeNames: map[string]string{
			"#ref": "ref",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":ref":       &types.AttributeValueMemberS{Value: ref},
			":commitSha": &types.AttributeValueMemberS{Value: commitSha},
		},
		ProjectionExpression: aws.String("chunkId"),
	}

	paginator := dynamodb.NewQueryPaginator(s.ddb, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return 0, fmt.Errorf("query gsi: %w", err)
		}
		for _, item := range page.Items {
			if v, ok := item["chunkId"].(*types.AttributeValueMemberS); ok {
				ids = append(ids, v.Value)
			}
		}
	}

	const maxBatch = 25
	for i := 0; i < len(ids); i += maxBatch {
		end := i + maxBatch
		if end > len(ids) {
			end = len(ids)
		}
		var reqs []types.WriteRequest
		for _, id := range ids[i:end] {
			reqs = append(reqs, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{
						"chunkId": &types.AttributeValueMemberS{Value: id},
					},
				},
			})
		}
		if _, err := s.ddb.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{s.config.ChunksTable: reqs},
		}); err != nil {
			return 0, fmt.Errorf("batch delete chunks: %w", err)
		}
	}

	return len(ids), nil
}

// Status returns the current state of the index.
func (s *AWSStore) Status(ctx context.Context) (Status, error) {
	sets, err := s.ListDocSets(ctx)
	if err != nil {
		return Status{}, err
	}
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

// chunkItem maps DocChunk fields to DynamoDB attribute names.
// PK is "chunkId" (defined in storage-stack.ts).
type chunkItem struct {
	ID           string   `dynamodbav:"chunkId"`
	Project      string   `dynamodbav:"project"`
	Ref          string   `dynamodbav:"ref"`
	CommitSha    string   `dynamodbav:"commitSha"`
	BuiltAt      string   `dynamodbav:"builtAt"`
	SourceType   string   `dynamodbav:"sourceType"`
	SourcePath   string   `dynamodbav:"sourcePath"`
	SourceFile   string   `dynamodbav:"sourceFile"`
	CanonicalURL string   `dynamodbav:"canonicalUrl"`
	Title        string   `dynamodbav:"title"`
	HeadingPath  []string `dynamodbav:"headingPath"`
	Area         string   `dynamodbav:"area"`
	ContentHtml  string   `dynamodbav:"contentHtml"`
	ContentText  string   `dynamodbav:"contentText"`
	IndexedAt    string   `dynamodbav:"indexedAt"`
}

func toItem(c model.DocChunk) chunkItem {
	return chunkItem{
		ID:           c.ID,
		Project:      c.Project,
		Ref:          c.Ref,
		CommitSha:    c.CommitSha,
		BuiltAt:      c.BuiltAt.UTC().Format(time.RFC3339),
		SourceType:   string(c.SourceType),
		SourcePath:   c.SourcePath,
		SourceFile:   c.SourceFile,
		CanonicalURL: c.CanonicalURL,
		Title:        c.Title,
		HeadingPath:  c.HeadingPath,
		Area:         string(c.Area),
		ContentHtml:  c.ContentHtml,
		ContentText:  c.ContentText,
		IndexedAt:    c.IndexedAt.UTC().Format(time.RFC3339),
	}
}

func fromItem(ci chunkItem) model.DocChunk {
	builtAt, _ := time.Parse(time.RFC3339, ci.BuiltAt)
	indexedAt, _ := time.Parse(time.RFC3339, ci.IndexedAt)
	return model.DocChunk{
		ID:           ci.ID,
		Project:      ci.Project,
		Ref:          ci.Ref,
		CommitSha:    ci.CommitSha,
		BuiltAt:      builtAt,
		SourceType:   model.SourceType(ci.SourceType),
		SourcePath:   ci.SourcePath,
		SourceFile:   ci.SourceFile,
		CanonicalURL: ci.CanonicalURL,
		Title:        ci.Title,
		HeadingPath:  ci.HeadingPath,
		Area:         model.Area(ci.Area),
		ContentHtml:  ci.ContentHtml,
		ContentText:  ci.ContentText,
		IndexedAt:    indexedAt,
	}
}
