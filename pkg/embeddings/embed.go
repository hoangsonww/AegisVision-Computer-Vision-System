// Embed: helper that converts a Client embedding result into a single
// vector. The convenience here is that callers can pass a single string
// instead of marshalling EmbeddingRequest themselves.
package embeddings

import (
	"context"
	"errors"

	"github.com/aegisvision/pkg/llm"
)

// One returns the embedding vector for a single string. The model is
// caller-chosen.
func One(ctx context.Context, c llm.Client, model, tenant, text string) ([]float32, error) {
	resp, err := c.Embed(ctx, llm.EmbeddingRequest{
		TenantID: tenant,
		Model:    model,
		Input:    []string{text},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, errors.New("empty embedding response")
	}
	return resp.Data[0].Vector, nil
}
