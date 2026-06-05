package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
)

// AWSConfig holds configuration for the DynamoDB-backed store.
type AWSConfig struct {
	Region         string // defaults to AWS_REGION env var
	ChunksTable    string // DynamoDB table for doc chunks
	ChunksTableGsi string // GSI name for ref-commitSha queries
}

// AWSConfigFromEnv reads configuration from environment variables.
//
// Required:
//   CHUNKS_TABLE  — DynamoDB table name for doc chunks (injected by service-stack.ts)
//
// Optional (standard AWS SDK env vars are also honoured):
//   AWS_REGION / AWS_DEFAULT_REGION
func AWSConfigFromEnv() AWSConfig {
	return AWSConfig{
		Region:         envOr("AWS_REGION", envOr("AWS_DEFAULT_REGION", "us-east-1")),
		ChunksTable:    os.Getenv("CHUNKS_TABLE"),
		ChunksTableGsi: envOr("CHUNKS_TABLE_GSI", "ref-commitSha-index"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// AWSStore implements Store using Amazon DynamoDB.
//
// Search (Phase 1): DynamoDB Scan + FilterExpression on contentText.
// Future: replace Search with Bedrock Embeddings + S3 Vectors.
type AWSStore struct {
	ddb    *dynamodb.Client
	config AWSConfig
}

// NewAWSStore creates a DynamoDB-backed Store.
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
	return &AWSStore{ddb: dynamodb.NewFromConfig(awsCfg), config: cfg}, nil
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

// Search scans DynamoDB for chunks matching the query using a paginator.
//
// Phase 1 implementation: FilterExpression with contains() on contentText.
// This performs a full table scan and is suitable for small datasets.
// Phase 2 will replace this with Bedrock Embeddings + S3 Vectors.
func (s *AWSStore) Search(ctx context.Context, params model.SearchParams) (model.SearchResult, error) {
	limit := params.Limit
	if limit <= 0 || limit > 20 {
		limit = 10
	}

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

	var result model.SearchResult
	paginator := dynamodb.NewScanPaginator(s.ddb, input)
	for paginator.HasMorePages() && len(result.Chunks) < limit {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return model.SearchResult{}, fmt.Errorf("scan page: %w", err)
		}
		for _, raw := range page.Items {
			if len(result.Chunks) >= limit {
				break
			}
			var item chunkItem
			if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
				continue
			}
			result.Chunks = append(result.Chunks, fromItem(item))
		}
	}
	return result, nil
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
