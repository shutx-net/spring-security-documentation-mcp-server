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
	m := vectorFilterDoc(ref, area)
	if m == nil {
		return nil
	}
	return s3vdoc.NewLazyDocument(m)
}

// vectorFilterDoc builds the metadata filter as a plain map.
// S3 Vectors rejects implicit AND across multiple top-level keys with
// "Invalid filter"; multiple field conditions must be wrapped in an
// explicit $and. Returns nil when no filter fields are set.
func vectorFilterDoc(ref, area string) map[string]interface{} {
	var conds []map[string]interface{}
	if ref != "" {
		conds = append(conds, map[string]interface{}{"ref": map[string]interface{}{"$eq": ref}})
	}
	if area != "" {
		conds = append(conds, map[string]interface{}{"area": map[string]interface{}{"$eq": area}})
	}
	switch len(conds) {
	case 0:
		return nil
	case 1:
		return conds[0]
	default:
		return map[string]interface{}{"$and": conds}
	}
}
