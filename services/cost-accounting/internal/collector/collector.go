// Package collector polls OpenCost (Kubernetes resource cost) and the
// DCGM exporter (GPU-second usage) at a fixed interval and projects the
// results onto AegisVision tenants via Kubernetes labels.
//
// The scheduler-side scraper writes its findings back to
// cost-accounting's /v1/costs endpoint as an array of Line rows. The
// Store merges with `quantity = SUM, unit_cost_usd = REPLACE, total =
// SUM` semantics so repeated polls are idempotent within a day.
//
// Tenant projection: each OpenCost / DCGM sample exposes a set of
// Kubernetes labels (tenant_id, pipeline_id) we wired into the pod
// templates. The collector groups samples by those labels and emits
// one Line per (tenant, pipeline, line_item) per day.
//
// In dev (no OpenCost / DCGM endpoints configured) the collector
// emits no lines; the production scrape config provides URLs via env.
package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Line mirrors the store.Line shape; we keep it here so the collector
// doesn't import the store package (which would couple it to the SQL
// schema).
type Line struct {
	TenantID    string  `json:"tenant_id"`
	PipelineID  string  `json:"pipeline_id"`
	Day         string  `json:"day"` // YYYY-MM-DD
	LineItem    string  `json:"line_item"`
	Quantity    float64 `json:"quantity"`
	UnitCostUSD float64 `json:"unit_cost_usd"`
	TotalUSD    float64 `json:"total_usd"`
}

// Source represents one upstream that the collector polls. The two
// production sources are OpenCost (HTTP JSON) and DCGM (Prometheus
// text exposition format). For each source we implement a Fetch.
type Source interface {
	Fetch(ctx context.Context, day string) ([]Line, error)
	Name() string
}

// Collector polls all configured sources on a tick and POSTs the
// aggregated lines to the cost-accounting ingest endpoint.
type Collector struct {
	Sources     []Source
	IngestURL   string // POST endpoint; typically http://cost-accounting:8330/v1/costs
	HTTP        *http.Client
	Every       time.Duration
	TenantToken string // optional bearer header for the ingest call
}

func New(ingestURL string, sources ...Source) *Collector {
	return &Collector{
		Sources:   sources,
		IngestURL: ingestURL,
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		Every:     5 * time.Minute,
	}
}

// Run blocks until ctx is canceled. Each tick: fan out a Fetch per
// source, concatenate, dedupe by (tenant, pipeline, day, line_item),
// and POST once.
func (c *Collector) Run(ctx context.Context) error {
	t := time.NewTicker(c.Every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := c.Tick(ctx); err != nil {
				return err
			}
		}
	}
}

// Tick performs one collection cycle. Exposed so tests can drive
// without depending on real time.
func (c *Collector) Tick(ctx context.Context) error {
	day := time.Now().UTC().Format("2006-01-02")
	var mu sync.Mutex
	merged := map[string]*Line{}
	var wg sync.WaitGroup
	for _, src := range c.Sources {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			lines, err := s.Fetch(ctx, day)
			if err != nil {
				return
			}
			mu.Lock()
			for _, l := range lines {
				k := l.TenantID + "|" + l.PipelineID + "|" + l.Day + "|" + l.LineItem
				if cur, ok := merged[k]; ok {
					cur.Quantity += l.Quantity
					cur.TotalUSD += l.TotalUSD
					if l.UnitCostUSD > 0 {
						cur.UnitCostUSD = l.UnitCostUSD
					}
				} else {
					ll := l
					merged[k] = &ll
				}
			}
			mu.Unlock()
		}(src)
	}
	wg.Wait()
	if len(merged) == 0 || c.IngestURL == "" {
		return nil
	}
	batch := make([]Line, 0, len(merged))
	for _, v := range merged {
		batch = append(batch, *v)
	}
	body, _ := json.Marshal(batch)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.IngestURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Aegis-Tenant", "cost-collector")
	if c.TenantToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.TenantToken)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ingest %s: %s", resp.Status, string(b))
	}
	return nil
}

// OpenCostSource talks to the OpenCost HTTP API. The default endpoint
// returns aggregated allocations broken down by namespace + labels.
type OpenCostSource struct {
	BaseURL string
	HTTP    *http.Client
}

func NewOpenCost(baseURL string) *OpenCostSource {
	return &OpenCostSource{BaseURL: baseURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (s *OpenCostSource) Name() string { return "opencost" }

// openCostResp is the subset of the OpenCost /allocation response we use.
type openCostResp struct {
	Data []map[string]openCostAlloc `json:"data"`
}

type openCostAlloc struct {
	Name       string             `json:"name"`
	Properties openCostProperties `json:"properties"`
	CPUCost    float64            `json:"cpuCost"`
	RAMCost    float64            `json:"ramCost"`
	PVCost     float64            `json:"pvCost"`
}

type openCostProperties struct {
	Labels map[string]string `json:"labels"`
}

func (s *OpenCostSource) Fetch(ctx context.Context, day string) ([]Line, error) {
	if s.BaseURL == "" {
		return nil, nil
	}
	url := s.BaseURL + "/allocation?window=" + day + "T00:00:00Z," + day + "T23:59:59Z&aggregate=label:tenant_id,label:pipeline_id"
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("opencost %s", resp.Status)
	}
	var doc openCostResp
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}
	var out []Line
	for _, page := range doc.Data {
		for _, alloc := range page {
			tenant := alloc.Properties.Labels["tenant_id"]
			pipeline := alloc.Properties.Labels["pipeline_id"]
			if tenant == "" {
				continue
			}
			if alloc.CPUCost > 0 {
				out = append(out, Line{TenantID: tenant, PipelineID: pipeline, Day: day, LineItem: "cpu", TotalUSD: alloc.CPUCost})
			}
			if alloc.RAMCost > 0 {
				out = append(out, Line{TenantID: tenant, PipelineID: pipeline, Day: day, LineItem: "memory", TotalUSD: alloc.RAMCost})
			}
			if alloc.PVCost > 0 {
				out = append(out, Line{TenantID: tenant, PipelineID: pipeline, Day: day, LineItem: "storage", TotalUSD: alloc.PVCost})
			}
		}
	}
	return out, nil
}

// DCGMSource scrapes the DCGM exporter's Prometheus endpoint and
// projects GPU-second usage onto tenants via the `tenant_id` label.
// We parse only the metrics we need; the parser is intentionally a
// small text scanner (Prometheus' format is line-based and stable).
type DCGMSource struct {
	BaseURL       string
	HTTP          *http.Client
	GPUDollarHour float64 // tenant-billed price per GPU-hour
}

func NewDCGM(baseURL string, gpuDollarHour float64) *DCGMSource {
	return &DCGMSource{BaseURL: baseURL, HTTP: &http.Client{Timeout: 30 * time.Second}, GPUDollarHour: gpuDollarHour}
}

func (s *DCGMSource) Name() string { return "dcgm" }

func (s *DCGMSource) Fetch(ctx context.Context, day string) ([]Line, error) {
	if s.BaseURL == "" {
		return nil, nil
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", s.BaseURL+"/metrics", nil)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("dcgm %s", resp.Status)
	}
	body, _ := io.ReadAll(resp.Body)
	utilByTenant := parsePromGPUUtil(string(body))
	out := make([]Line, 0, len(utilByTenant))
	for k, util := range utilByTenant {
		// Approximate: gpu_seconds_used = util_fraction * sample_interval
		// We treat one tick as 5 minutes (= 300s). Production scrapers
		// should pass the *delta* of DCGM_FI_DEV_GPU_UTIL between samples
		// instead — that's a refinement for the operator runbook.
		seconds := util * 300
		hours := seconds / 3600.0
		out = append(out, Line{
			TenantID: k.tenant, PipelineID: k.pipeline, Day: day,
			LineItem: "gpu_seconds", Quantity: seconds,
			UnitCostUSD: s.GPUDollarHour / 3600.0,
			TotalUSD:    hours * s.GPUDollarHour,
		})
	}
	return out, nil
}

// parsePromGPUUtil scans the DCGM exporter output for
// `DCGM_FI_DEV_GPU_UTIL{...} <float>` rows and aggregates utilization
// fractions (0..1) by (tenant_id, pipeline_id) labels. Anything without
// a tenant_id label is dropped.
func parsePromGPUUtil(s string) map[promKey]float64 {
	out := map[promKey]float64{}
	for _, line := range splitLines(s) {
		line = trimRight(line)
		if line == "" || line[0] == '#' {
			continue
		}
		if !startsWith(line, "DCGM_FI_DEV_GPU_UTIL") {
			continue
		}
		// Expect: name{labels} value
		brace := indexByte(line, '{')
		end := indexByte(line, '}')
		if brace < 0 || end < 0 || end <= brace {
			continue
		}
		labels := parseLabels(line[brace+1 : end])
		tenant := labels["tenant_id"]
		if tenant == "" {
			continue
		}
		pipeline := labels["pipeline_id"]
		rest := line[end+1:]
		valStr := firstField(rest)
		val, ok := parseFloat(valStr)
		if !ok {
			continue
		}
		// Util reported as percent (0..100) in DCGM; normalize.
		out[promKey{tenant: tenant, pipeline: pipeline}] += val / 100.0
	}
	return out
}

type promKey struct{ tenant, pipeline string }

func parseLabels(s string) map[string]string {
	out := map[string]string{}
	for _, kv := range splitOn(s, ',') {
		eq := indexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		key := trim(kv[:eq])
		val := trim(kv[eq+1:])
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		out[key] = val
	}
	return out
}

// Minimal string helpers (avoid pulling in strings for this small file).
func splitLines(s string) []string { return splitOn(s, '\n') }
func splitOn(s string, c byte) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
func trimRight(s string) string {
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
func firstField(s string) string {
	s = trim(s)
	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '\t' {
		i++
	}
	return s[:i]
}
func parseFloat(s string) (float64, bool) {
	var n float64
	if _, err := fmt.Sscanf(s, "%f", &n); err != nil {
		return 0, false
	}
	return n, true
}
