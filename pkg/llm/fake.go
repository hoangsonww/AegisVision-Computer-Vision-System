// FakeClient is a deterministic in-process Client for unit tests and the
// `dev`/no-LLM-backend path of the llm-gateway. It is NOT a model — it
// echoes a scripted set of responses and is sufficient for shape testing
// (does the agent loop dispatch tools? does the safety layer truncate? does
// the RAG path embed and search?).
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"sync/atomic"
)

// FakeClient supports:
//   - Scripted Chat responses: callers can push canned ChatResponse values
//     that pop FIFO; if empty, an echo response is produced.
//   - Deterministic embeddings: each input is hashed to a fixed vector,
//     suitable for similarity tests (same string → same vector).
//   - Deterministic vision descriptions: image URN → descriptive sentence.
type FakeClient struct {
	mu          sync.Mutex
	chatScript  []ChatResponse
	chatCalls   atomic.Int64
	embedDims   int
	defaultName string
}

func NewFakeClient() *FakeClient {
	return &FakeClient{embedDims: 16, defaultName: "fake-model"}
}

// Script appends canned chat responses to be returned in FIFO order. After
// the script is exhausted, Chat falls back to echoing the last user message.
func (f *FakeClient) Script(resp ...ChatResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chatScript = append(f.chatScript, resp...)
}

// CallCount returns the number of Chat calls observed.
func (f *FakeClient) CallCount() int64 { return f.chatCalls.Load() }

func (f *FakeClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	f.chatCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.chatScript) > 0 {
		r := f.chatScript[0]
		f.chatScript = f.chatScript[1:]
		if r.Model == "" {
			r.Model = pickModel(req.Model, f.defaultName)
		}
		return &r, nil
	}
	// Echo: take the last user message.
	last := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == RoleUser {
			last = req.Messages[i].Content
			break
		}
	}
	if last == "" {
		last = "ok"
	}
	prompt := countTokens(req.Messages)
	return &ChatResponse{
		ID:           fmt.Sprintf("fake-chat-%d", f.chatCalls.Load()),
		Model:        pickModel(req.Model, f.defaultName),
		Message:      Message{Role: RoleAssistant, Content: "echo: " + last},
		Usage:        TokenUsage{PromptTokens: prompt, CompletionTokens: 2, TotalTokens: prompt + 2},
		FinishReason: "stop",
	}, nil
}

func (f *FakeClient) Embed(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	out := EmbeddingResponse{Data: make([]Embedding, len(req.Input))}
	total := 0
	for i, s := range req.Input {
		out.Data[i] = Embedding{Vector: hashEmbed(s, f.embedDims), Index: i}
		total += countWords(s)
	}
	out.Usage = TokenUsage{PromptTokens: total, TotalTokens: total}
	return &out, nil
}

func (f *FakeClient) Describe(ctx context.Context, req VisionDescribeRequest) (*VisionDescribeResponse, error) {
	desc := fmt.Sprintf("Image %s described per instruction %q.", req.Image.URN, Truncate(req.Instruction, 64))
	return &VisionDescribeResponse{
		Description: desc,
		Usage:       TokenUsage{PromptTokens: 8, CompletionTokens: countWords(desc), TotalTokens: 8 + countWords(desc)},
	}, nil
}

// ScriptToolCall is a convenience: scripts a single assistant turn whose only
// content is a tool call with the given name + arguments.
func (f *FakeClient) ScriptToolCall(name string, args any) {
	b, _ := json.Marshal(args)
	f.Script(ChatResponse{
		Message: Message{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{{
				ID:            fmt.Sprintf("call-%d", len(f.chatScript)+1),
				Name:          name,
				ArgumentsJSON: string(b),
			}},
		},
		FinishReason: "tool_calls",
	})
}

// ScriptFinal scripts a final assistant turn with the given text.
func (f *FakeClient) ScriptFinal(content string) {
	f.Script(ChatResponse{
		Message:      Message{Role: RoleAssistant, Content: content},
		FinishReason: "stop",
	})
}

func pickModel(req, fallback string) string {
	if req != "" {
		return req
	}
	return fallback
}

// countTokens is a deliberately rough proxy (whitespace word count); the
// real gateway uses model-specific tokenizers.
func countTokens(msgs []Message) int {
	n := 0
	for _, m := range msgs {
		n += countWords(m.Content)
	}
	return n
}

func countWords(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Fields(s))
}

// hashEmbed produces a deterministic, unit-norm vector from a string.
// Same input → same vector → testable cosine similarity.
func hashEmbed(s string, dims int) []float32 {
	if dims <= 0 {
		dims = 16
	}
	v := make([]float32, dims)
	for i := 0; i < dims; i++ {
		h := fnv.New64a()
		h.Write([]byte(fmt.Sprintf("%d|%s", i, s)))
		// Map u64 to [-1, 1).
		u := h.Sum64()
		v[i] = float32(int64(u%200000)-100000) / 100000.0
	}
	// Normalize to unit length (so cosine == dot).
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	norm := float32(math.Sqrt(sum))
	for i := range v {
		v[i] /= norm
	}
	return v
}
