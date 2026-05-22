// Package backend abstracts the model runtime the gateway delegates to.
//
// In production this is an OpenAI-compatible endpoint — vLLM, TGI, Triton
// with the TensorRT-LLM backend, llama.cpp's server, hosted Bedrock/OpenAI
// behind an upstream shim. The gateway speaks the same wire format on its
// north side and translates back-and-forth on its south side.
//
// We deliberately do NOT take a hard dependency on any specific SDK
// (openai-go, vllm-go, etc.). The wire is JSON and we already speak it
// natively in pkg/llm.HTTPClient — the gateway is essentially a hub that
// adds tenant accounting, safety, rate limiting, and prompt-injection
// defense in front of any of those backends.
package backend

import (
	"context"

	"github.com/aegisvision/pkg/llm"
)

// Backend is the south-side interface to a model runtime.
type Backend interface {
	Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error)
	Embed(ctx context.Context, req llm.EmbeddingRequest) (*llm.EmbeddingResponse, error)
	Describe(ctx context.Context, req llm.VisionDescribeRequest) (*llm.VisionDescribeResponse, error)
}

// HTTPBackend is an OpenAI-compatible backend (vLLM/TGI/Triton+TRT-LLM/...).
// We re-use pkg/llm.HTTPClient — it already speaks the wire.
type HTTPBackend struct {
	Client *llm.HTTPClient
}

func NewHTTP(base string) *HTTPBackend { return &HTTPBackend{Client: llm.NewHTTPClient(base)} }

func (b *HTTPBackend) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return b.Client.Chat(ctx, req)
}

func (b *HTTPBackend) Embed(ctx context.Context, req llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return b.Client.Embed(ctx, req)
}

func (b *HTTPBackend) Describe(ctx context.Context, req llm.VisionDescribeRequest) (*llm.VisionDescribeResponse, error) {
	return b.Client.Describe(ctx, req)
}

// FakeBackend re-uses pkg/llm.FakeClient. Only active when env=dev or no
// LLM_BACKEND_URL is configured.
type FakeBackend struct{ Inner *llm.FakeClient }

func NewFake() *FakeBackend { return &FakeBackend{Inner: llm.NewFakeClient()} }

func (b *FakeBackend) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return b.Inner.Chat(ctx, req)
}

func (b *FakeBackend) Embed(ctx context.Context, req llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return b.Inner.Embed(ctx, req)
}

func (b *FakeBackend) Describe(ctx context.Context, req llm.VisionDescribeRequest) (*llm.VisionDescribeResponse, error) {
	return b.Inner.Describe(ctx, req)
}
