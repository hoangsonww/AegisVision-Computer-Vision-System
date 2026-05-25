package detector_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
	"github.com/aegisvision/services/inference-router/internal/detector"
)

func TestTriton_DecodesJSONOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := map[string]any{
			"model_name":    "yolo",
			"model_version": "1",
			"outputs": []map[string]any{{
				"name":     "output",
				"datatype": "BYTES",
				"shape":    []int{1},
				"data":     []any{`[{"class":"person","score":0.92,"bbox":{"X":0.1,"Y":0.2,"W":0.3,"H":0.4}}]`},
			}},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer ts.Close()
	c := detector.NewTriton(ts.URL, "yolo", "1")
	dets, err := c.Detect(context.Background(), &dataplanev1.FrameDescriptor{StreamId: "s", FrameSeq: 7}, []byte(`raw`))
	if err != nil {
		t.Fatal(err)
	}
	if len(dets) != 1 {
		t.Fatalf("want 1 det, got %d", len(dets))
	}
	if dets[0].GetClassLabel() != "person" || dets[0].GetScore() != 0.92 {
		t.Fatalf("bad det: %+v", dets[0])
	}
}

func TestTriton_DecodesFP32Output(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := map[string]any{
			"outputs": []map[string]any{{
				"name":     "output",
				"datatype": "FP32",
				"shape":    []int{1, 6},
				"data":     []any{0.1, 0.2, 0.3, 0.4, 0.87, 2.0},
			}},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer ts.Close()
	c := detector.NewTriton(ts.URL, "yolo", "1")
	dets, err := c.Detect(context.Background(), &dataplanev1.FrameDescriptor{StreamId: "s", FrameSeq: 1}, []byte(`raw`))
	if err != nil {
		t.Fatal(err)
	}
	if len(dets) != 1 {
		t.Fatalf("want 1 det, got %d", len(dets))
	}
	if dets[0].GetScore() < 0.86 || dets[0].GetScore() > 0.88 {
		t.Fatalf("score: %f", dets[0].GetScore())
	}
}

func TestTriton_BubblesNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer ts.Close()
	c := detector.NewTriton(ts.URL, "yolo", "1")
	_, err := c.Detect(context.Background(), &dataplanev1.FrameDescriptor{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTriton_RetriesOn5xxThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, "model not loaded", http.StatusServiceUnavailable)
			return
		}
		body := map[string]any{
			"outputs": []map[string]any{{
				"name":     "output",
				"datatype": "FP32",
				"shape":    []int{1, 6},
				"data":     []any{0.1, 0.2, 0.3, 0.4, 0.5, 0.0},
			}},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer ts.Close()

	c := detector.NewTritonWithOptions(ts.URL, "yolo", "1", detector.Options{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     5 * time.Millisecond,
	})
	dets, err := c.Detect(context.Background(), &dataplanev1.FrameDescriptor{StreamId: "s"}, []byte("raw"))
	if err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
	if len(dets) != 1 {
		t.Fatalf("want 1 detection")
	}
	stats := c.LastStats()
	if stats.Attempts < 3 {
		t.Fatalf("expected >=3 attempts, got %d", stats.Attempts)
	}
}

func TestTriton_GivesUpAfterMaxRetries(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "still down", http.StatusBadGateway)
	}))
	defer ts.Close()

	c := detector.NewTritonWithOptions(ts.URL, "yolo", "1", detector.Options{
		MaxRetries:     2,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     2 * time.Millisecond,
	})
	_, err := c.Detect(context.Background(), &dataplanev1.FrameDescriptor{}, []byte("raw"))
	if err == nil {
		t.Fatal("expected exhausted-retries error")
	}
	got := calls.Load()
	// 1 initial + 2 retries = 3
	if got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestTriton_RespectsContextDeadline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		http.Error(w, "slow", http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	c := detector.NewTritonWithOptions(ts.URL, "yolo", "1", detector.Options{
		MaxRetries:     5,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     200 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := c.Detect(ctx, &dataplanev1.FrameDescriptor{}, []byte("raw"))
	if err == nil {
		t.Fatal("expected context deadline exceeded")
	}
}

func TestTriton_SurfacesCacheHit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := map[string]any{
			"outputs": []map[string]any{{
				"name":     "output",
				"datatype": "FP32",
				"shape":    []int{1, 6},
				"data":     []any{0.1, 0.2, 0.3, 0.4, 0.5, 0.0},
			}},
			"parameters": map[string]any{
				"cache_hit": true,
			},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer ts.Close()

	c := detector.NewTriton(ts.URL, "yolo", "1")
	_, err := c.Detect(context.Background(), &dataplanev1.FrameDescriptor{}, []byte("raw"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.LastStats().CacheHit {
		t.Fatal("expected CacheHit=true")
	}
}

func TestTriton_HealthEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v2/health/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/v2/models/yolo/versions/1/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := detector.NewTriton(ts.URL, "yolo", "1")
	if err := c.HealthLive(context.Background()); err != nil {
		t.Fatalf("HealthLive: %v", err)
	}
	if err := c.HealthReady(context.Background()); err == nil {
		t.Fatal("HealthReady should have errored")
	}
	if err := c.ModelReady(context.Background()); err != nil {
		t.Fatalf("ModelReady: %v", err)
	}
}

func TestTriton_LoadAndUnload(t *testing.T) {
	var loaded, unloaded atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/repository/models/yolo/load", func(w http.ResponseWriter, _ *http.Request) {
		loaded.Store(true)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v2/repository/models/yolo/unload", func(w http.ResponseWriter, _ *http.Request) {
		unloaded.Store(true)
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := detector.NewTriton(ts.URL, "yolo", "1")
	if err := c.LoadModel(context.Background(), "yolo"); err != nil {
		t.Fatal(err)
	}
	if !loaded.Load() {
		t.Fatal("expected load to have been called")
	}
	if err := c.UnloadModel(context.Background(), "yolo"); err != nil {
		t.Fatal(err)
	}
	if !unloaded.Load() {
		t.Fatal("expected unload to have been called")
	}
}

func TestTriton_CloseRefusesNewCalls(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"outputs": []map[string]any{{
				"name": "output", "datatype": "FP32", "shape": []int{1, 6},
				"data": []any{0.1, 0.2, 0.3, 0.4, 0.5, 0.0},
			}},
		})
	}))
	defer ts.Close()

	c := detector.NewTriton(ts.URL, "yolo", "1")
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := c.Detect(context.Background(), &dataplanev1.FrameDescriptor{}, []byte("x"))
	if err == nil || !errors.Is(err, err) {
		t.Fatalf("expected error after Close, got %v", err)
	}
}
