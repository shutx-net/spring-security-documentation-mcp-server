package store

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type embeddingCache struct {
	ddb   *dynamodb.Client
	table string
}

func queryHash(query string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(query))))
	return hex.EncodeToString(h[:])
}

func float32sToBytes(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func bytesToFloat32s(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func (c *embeddingCache) get(ctx context.Context, hash string) ([]float32, bool) {
	out, err := c.ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(c.table),
		Key: map[string]types.AttributeValue{
			"queryHash": &types.AttributeValueMemberS{Value: hash},
		},
	})
	if err != nil || out.Item == nil {
		return nil, false
	}
	b, ok := out.Item["vector"].(*types.AttributeValueMemberB)
	if !ok {
		return nil, false
	}
	return bytesToFloat32s(b.Value), true
}

func (c *embeddingCache) put(ctx context.Context, hash string, vector []float32) {
	ttl := time.Now().Add(24 * time.Hour).Unix()
	c.ddb.PutItem(ctx, &dynamodb.PutItemInput{ //nolint:errcheck
		TableName: aws.String(c.table),
		Item: map[string]types.AttributeValue{
			"queryHash": &types.AttributeValueMemberS{Value: hash},
			"vector":    &types.AttributeValueMemberB{Value: float32sToBytes(vector)},
			"ttl":       &types.AttributeValueMemberN{Value: strconv.FormatInt(ttl, 10)},
		},
	})
}
