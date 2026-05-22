package pagination_test

import (
	"errors"
	"testing"

	"github.com/aegisvision/pkg/platform/pagination"
)

func TestEncodeDecode_RoundTrip(t *testing.T) {
	enc := pagination.NewEncoder("test-key")
	token := enc.Encode("pipelines", "p-42")
	if token == "" {
		t.Fatal("empty token from non-empty id")
	}
	c, err := enc.Decode("pipelines", token)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c.LastID != "p-42" {
		t.Fatalf("want last_id p-42, got %q", c.LastID)
	}
}

func TestDecode_RejectsCrossResourceCursor(t *testing.T) {
	enc := pagination.NewEncoder("test-key")
	token := enc.Encode("pipelines", "p-42")

	_, err := enc.Decode("streams", token)
	if !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("want ErrInvalidCursor, got %v", err)
	}
}

func TestDecode_RejectsTamperedCursor(t *testing.T) {
	enc := pagination.NewEncoder("test-key")
	token := enc.Encode("pipelines", "p-42")
	// Flip the last char to break the HMAC.
	tampered := token[:len(token)-1] + "A"
	if tampered == token {
		tampered = token[:len(token)-1] + "B"
	}
	_, err := enc.Decode("pipelines", tampered)
	if !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("want ErrInvalidCursor, got %v", err)
	}
}

func TestDecode_EmptyTokenIsZero(t *testing.T) {
	enc := pagination.NewEncoder("test-key")
	c, err := enc.Decode("pipelines", "")
	if err != nil {
		t.Fatalf("empty token should not error: %v", err)
	}
	if c.LastID != "" {
		t.Fatalf("empty token should produce zero cursor, got %+v", c)
	}
}

func TestClampPageSize(t *testing.T) {
	cases := map[int32]int{
		-5: pagination.DefaultPageSize,
		0:  pagination.DefaultPageSize,
		25: 25,
		// Above MaxPageSize clamps down.
		9999: pagination.MaxPageSize,
	}
	for in, want := range cases {
		if got := pagination.ClampPageSize(in); got != want {
			t.Fatalf("ClampPageSize(%d) = %d, want %d", in, got, want)
		}
	}
}
