package backend_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/llm"
	"github.com/aegisvision/services/llm-gateway/internal/backend"
)

func TestFakeBackend_RoundTrip(t *testing.T) {
	b := backend.NewFake()
	b.Inner.ScriptFinal("hello")
	resp, err := b.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "hello" {
		t.Fatalf("got %q", resp.Message.Content)
	}
}

func TestFakeBackend_EmbedDeterministic(t *testing.T) {
	b := backend.NewFake()
	r1, _ := b.Embed(context.Background(), llm.EmbeddingRequest{Input: []string{"a"}})
	r2, _ := b.Embed(context.Background(), llm.EmbeddingRequest{Input: []string{"a"}})
	if len(r1.Data) != 1 || len(r2.Data) != 1 {
		t.Fatal("need 1 embed each")
	}
	for i := range r1.Data[0].Vector {
		if r1.Data[0].Vector[i] != r2.Data[0].Vector[i] {
			t.Fatal("not deterministic")
		}
	}
}
