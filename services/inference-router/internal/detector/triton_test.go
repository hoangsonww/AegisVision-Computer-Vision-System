package detector_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
		// One detection: [x, y, w, h, score, class_id]
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
		http.Error(w, "model not loaded", http.StatusServiceUnavailable)
	}))
	defer ts.Close()
	c := detector.NewTriton(ts.URL, "yolo", "1")
	_, err := c.Detect(context.Background(), &dataplanev1.FrameDescriptor{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
