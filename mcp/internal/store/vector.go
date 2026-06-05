package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors"
	s3vdoc "github.com/aws/aws-sdk-go-v2/service/s3vectors/document"
	s3vtypes "github.com/aws/aws-sdk-go-v2/service/s3vectors/types"
)

type vectorMatch struct {
	Key      string
	Distance float32
}

func queryVectors(
	ctx context.Context,
	sv *s3vectors.Client,
	indexArn string,
	vector []float32,
	topK int,
	ref, area string,
) ([]vectorMatch, error) {
	input := &s3vectors.QueryVectorsInput{
		IndexArn: aws.String(indexArn),
		QueryVector: &s3vtypes.VectorDataMemberFloat32{
			Value: vector,
		},
		TopK:           aws.Int32(int32(topK)),
		ReturnMetadata: false,
		ReturnDistance: true,
	}
	if f := buildVectorFilter(ref, area); f != nil {
		input.Filter = f
	}

	out, err := sv.QueryVectors(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("query vectors: %w", err)
	}

	matches := make([]vectorMatch, len(out.Vectors))
	for i, v := range out.Vectors {
		dist := float32(0)
		if v.Distance != nil {
			dist = *v.Distance
		}
		matches[i] = vectorMatch{Key: aws.ToString(v.Key), Distance: dist}
	}
	return matches, nil
}

// buildVectorFilter builds a metadata filter document for S3 Vectors.
// Returns nil when no filter fields are set (no filtering applied).
func buildVectorFilter(ref, area string) s3vdoc.Interface {
	m := map[string]interface{}{}
	if ref != "" {
		m["ref"] = map[string]interface{}{"$eq": ref}
	}
	if area != "" {
		m["area"] = map[string]interface{}{"$eq": area}
	}
	if len(m) == 0 {
		return nil
	}
	return s3vdoc.NewLazyDocument(m)
}
