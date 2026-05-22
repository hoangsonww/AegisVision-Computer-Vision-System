package claimcheck_test

import (
	"errors"
	"testing"

	"github.com/aegisvision/pkg/dataplane/claimcheck"
)

func TestRing_PutGetRoundTrip(t *testing.T) {
	r, err := claimcheck.NewRing("stream-1", 4)
	if err != nil {
		t.Fatal(err)
	}
	loc := r.Put([]byte("frame-0"))
	got, err := r.Get(loc)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != "frame-0" {
		t.Fatalf("payload mismatch: %q", got)
	}
}

func TestRing_StaleLocatorAfterOverwrite(t *testing.T) {
	r, err := claimcheck.NewRing("stream-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	first := r.Put([]byte("a"))
	r.Put([]byte("b"))
	// Wrap around — slot 0 gets rewritten on the third put.
	r.Put([]byte("c"))
	if _, err := r.Get(first); !errors.Is(err, claimcheck.ErrStale) {
		t.Fatalf("expected ErrStale, got %v", err)
	}
}

func TestLocator_RoundTripEncoding(t *testing.T) {
	l := claimcheck.Locator{Ring: "stream-1", Slot: 7, Gen: 42}
	parsed, err := claimcheck.ParseLocator(l.Encode())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed != l {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", parsed, l)
	}
}

func TestRegistry_EnsureIsIdempotent(t *testing.T) {
	reg := claimcheck.NewRegistry()
	a, _ := reg.Ensure("stream-1", 4)
	b, _ := reg.Ensure("stream-1", 99) // capacity ignored on second call
	if a != b {
		t.Fatal("Ensure should return the same ring instance")
	}
}

func TestRing_ConcurrentSafe(t *testing.T) {
	r, _ := claimcheck.NewRing("s", 16)
	done := make(chan struct{}, 4)
	for i := 0; i < 4; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				loc := r.Put([]byte("x"))
				_, _ = r.Get(loc) // may be stale; that's OK
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 4; i++ {
		<-done
	}
}
