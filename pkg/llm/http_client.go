// HTTPClient talks to the platform's llm-gateway over JSON. The wire shape
// is OpenAI-compatible, so the same client also works against vLLM, Ollama,
// llama.cpp, or hosted Bedrock/OpenAI/Vertex sitting behind an
// OpenAI-compatible shim.
//
// Retries: bounded exponential backoff for 5xx + 429 only. 4xx is fatal
// (caller has malformed input). Context cancellation always wins.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient is a thread-safe Client.
type HTTPClient struct {
	BaseURL    string
	HTTP       *http.Client
	// Authorization header forwarded to gateway (optional; mTLS handles the
	// service-to-service auth in-cluster).
	AuthHeader string
	// Number of retry attempts on transient errors. 0 = no retries.
	MaxRetries int
	// Initial backoff; doubles each retry, capped at 30s.
	BackoffStart time.Duration
}

// NewHTTPClient constructs a sensible default client. baseURL points at
// llm-gateway; e.g. http://llm-gateway.aegisvision.svc:8400.
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		BaseURL:      baseURL,
		HTTP:         &http.Client{Timeout: 120 * time.Second},
		MaxRetries:   3,
		BackoffStart: 250 * time.Millisecond,
	}
}

func (c *HTTPClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	var out ChatResponse
	if err := c.do(ctx, "POST", "/v1/chat/completions", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *HTTPClient) Embed(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	var out EmbeddingResponse
	if err := c.do(ctx, "POST", "/v1/embeddings", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *HTTPClient) Describe(ctx context.Context, req VisionDescribeRequest) (*VisionDescribeResponse, error) {
	var out VisionDescribeResponse
	if err := c.do(ctx, "POST", "/v1/vision/describe", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *HTTPClient) do(ctx context.Context, method, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	backoff := c.BackoffStart
	if backoff == 0 {
		backoff = 250 * time.Millisecond
	}
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff *= 2; backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
		hreq, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		hreq.Header.Set("Content-Type", "application/json")
		hreq.Header.Set("Accept", "application/json")
		if c.AuthHeader != "" {
			hreq.Header.Set("Authorization", c.AuthHeader)
		}
		resp, err := c.HTTP.Do(hreq)
		if err != nil {
			lastErr = err
			continue
		}
		body2, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if rerr != nil {
			lastErr = rerr
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("llm gateway %s: %s", resp.Status, string(body2))
			continue
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("llm gateway %s: %s", resp.Status, string(body2))
		}
		if err := json.Unmarshal(body2, out); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("exhausted retries")
	}
	return lastErr
}
