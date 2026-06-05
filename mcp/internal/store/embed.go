package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"golang.org/x/time/rate"
)

type titanEmbedReq struct {
	InputText  string `json:"inputText"`
	Dimensions int    `json:"dimensions"`
	Normalize  bool   `json:"normalize"`
}

type titanEmbedResp struct {
	Embedding []float32 `json:"embedding"`
}

type bedrockEmbedder struct {
	client  *bedrockruntime.Client
	modelID string
	limiter *rate.Limiter
}

func newBedrockEmbedder(client *bedrockruntime.Client, modelID string) *bedrockEmbedder {
	return &bedrockEmbedder{
		client:  client,
		modelID: modelID,
		limiter: rate.NewLimiter(rate.Limit(20), 5),
	}
}

func (e *bedrockEmbedder) embed(ctx context.Context, text string) ([]float32, error) {
	if err := e.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	body, err := json.Marshal(titanEmbedReq{InputText: text, Dimensions: 1024, Normalize: true})
	if err != nil {
		return nil, fmt.Errorf("marshal embed req: %w", err)
	}
	out, err := e.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(e.modelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        body,
	})
	if err != nil {
		return nil, fmt.Errorf("invoke model: %w", err)
	}
	var resp titanEmbedResp
	if err := json.Unmarshal(out.Body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal embed resp: %w", err)
	}
	return resp.Embedding, nil
}
