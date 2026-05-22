package collector_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/aegisvision/services/cost-accounting/internal/collector"
)

type stubSource struct {
	name  string
	lines []collector.Line
}

func (s *stubSource) Name() string { return s.name }
func (s *stubSource) Fetch(_ context.Context, day string) ([]collector.Line, error) {
	out := make([]collector.Line, len(s.lines))
	for i, l := range s.lines {
		l.Day = day
		out[i] = l
	}
	return out, nil
}

func TestCollector_MergesAndPostsBatch(t *testing.T) {
	var posted atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []collector.Line
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
			w.WriteHeader(500)
			return
		}
		posted.Add(int64(len(body)))
		w.WriteHeader(204)
	}))
	defer srv.Close()

	a := &stubSource{name: "a", lines: []collector.Line{
		{TenantID: "t-1", PipelineID: "p-1", LineItem: "cpu", TotalUSD: 1.0},
		{TenantID: "t-1", PipelineID: "p-1", LineItem: "memory", TotalUSD: 0.4},
	}}
	b := &stubSource{name: "b", lines: []collector.Line{
		{TenantID: "t-1", PipelineID: "p-1", LineItem: "cpu", TotalUSD: 0.2},
	}}
	c := collector.New(srv.URL, a, b)
	if err := c.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 3 lines emitted; 2 distinct keys after merge (cpu merged across sources).
	if posted.Load() != 2 {
		t.Fatalf("expected 2 distinct keys, got %d", posted.Load())
	}
}

func TestCollector_EmptyBatchSkipsPost(t *testing.T) {
	var posted atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted.Add(1)
	}))
	defer srv.Close()
	c := collector.New(srv.URL)
	if err := c.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if posted.Load() != 0 {
		t.Fatalf("expected no posts on empty batch, got %d", posted.Load())
	}
}

func TestOpenCostSource_ParsesAllocation(t *testing.T) {
	body := `{"data":[{"alloc-a":{"name":"alloc-a","properties":{"labels":{"tenant_id":"t-1","pipeline_id":"p-1"}},"cpuCost":2.50,"ramCost":0.75,"pvCost":0.10}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	s := collector.NewOpenCost(srv.URL)
	lines, err := s.Fetch(context.Background(), "2026-05-21")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (cpu/memory/storage), got %d", len(lines))
	}
}

func TestDCGMSource_ParsesGPUUtil(t *testing.T) {
	body := `# HELP DCGM_FI_DEV_GPU_UTIL GPU utilization
# TYPE DCGM_FI_DEV_GPU_UTIL gauge
DCGM_FI_DEV_GPU_UTIL{tenant_id="t-1",pipeline_id="p-1",gpu="0"} 80
DCGM_FI_DEV_GPU_UTIL{tenant_id="t-1",pipeline_id="p-1",gpu="1"} 20
DCGM_FI_DEV_GPU_UTIL{gpu="2"} 50
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	s := collector.NewDCGM(srv.URL, 4.00) // $4/GPU-hour
	lines, err := s.Fetch(context.Background(), "2026-05-21")
	if err != nil {
		t.Fatal(err)
	}
	// Two labeled GPUs, one un-labeled (dropped). 80% + 20% = 100% of one GPU.
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %+v", len(lines), lines)
	}
	if lines[0].LineItem != "gpu_seconds" {
		t.Fatalf("got line item: %s", lines[0].LineItem)
	}
	// 100% of one GPU for 5 minutes = 300 seconds.
	if lines[0].Quantity < 299 || lines[0].Quantity > 301 {
		t.Fatalf("quantity: %f", lines[0].Quantity)
	}
}
