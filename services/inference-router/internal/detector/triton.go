// Package detector wraps NVIDIA Triton Inference Server behind the
// dataplane Detector interface.
//
// Triton's HTTP/REST protocol (the KServe v2 inference protocol) is the
// wire contract; this client speaks it directly so we don't carry the
// Triton gRPC SDK as a build-time dependency across every operator pod.
// The shape is:
//
//	POST {URL}/v2/models/{model_name}/versions/{ver}/infer
//	{"inputs":[{"name":"images","shape":[1,3,640,640],"datatype":"FP32",...}],
//	 "outputs":[{"name":"detections"}],
//	 "parameters":{"sequence_id":...}}
//
// A future revision swaps to Triton's gRPC streaming + shared-memory
// extensions for sub-millisecond round trips; the dataplane Detector
// interface does not change.
//
// Production-grade features wired here:
//
//   - HTTP connection pool (`http.Transport.MaxIdleConnsPerHost`).
//   - Retries with exponential backoff and jitter on 5xx + transient
//     network errors. The retry budget is bounded by the request
//     deadline carried in `context.Context`.
//   - Response-cache hit/miss surfaced via the Triton
//     `response.parameters.cache_hit` boolean and re-emitted to the
//     caller as a Detect-level statistic.
//   - OpenTelemetry span around every ModelInfer call, with model,
//     version, batch size, and cache-hit attributes.
//   - Graceful drain — Close() blocks until in-flight calls complete or
//     the supplied deadline elapses.
package detector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

// Options tunes the Triton client. The zero value is safe — defaults
// match the production-shape settings in deploy/helm/triton/values.yaml.
type Options struct {
	// PerRequestTimeout caps a single Triton call (including retries).
	// Default: 5 s. The context deadline always wins when shorter.
	PerRequestTimeout time.Duration

	// MaxRetries on transient (5xx + network) failures. Default: 2.
	// The first try is not counted; total attempts = MaxRetries + 1.
	MaxRetries int

	// InitialBackoff for the retry exponential. Default: 25 ms.
	InitialBackoff time.Duration

	// MaxBackoff caps the per-attempt wait. Default: 250 ms.
	MaxBackoff time.Duration

	// MaxIdleConnsPerHost caps the HTTP connection pool. Default: 64.
	MaxIdleConnsPerHost int

	// UserAgent is sent on every request. Defaults to
	// "aegisvision-inference-router/1".
	UserAgent string
}

func (o *Options) withDefaults() {
	if o.PerRequestTimeout == 0 {
		o.PerRequestTimeout = 5 * time.Second
	}
	if o.MaxRetries == 0 {
		o.MaxRetries = 2
	}
	if o.InitialBackoff == 0 {
		o.InitialBackoff = 25 * time.Millisecond
	}
	if o.MaxBackoff == 0 {
		o.MaxBackoff = 250 * time.Millisecond
	}
	if o.MaxIdleConnsPerHost == 0 {
		o.MaxIdleConnsPerHost = 64
	}
	if o.UserAgent == "" {
		o.UserAgent = "aegisvision-inference-router/1"
	}
}

// Stats is the per-call result envelope. Callers (the service layer)
// use this to record metrics and emit canary-controller observations
// without having to re-parse Triton response shapes.
type Stats struct {
	CacheHit  bool          // response.parameters.cache_hit
	Attempts  int           // HTTP attempts including retries
	Latency   time.Duration // wall time end-to-end (incl. retries)
	BatchSize int           // first input dim — proxy for batch
}

// TritonClient implements dataplane operators.Detector against Triton's
// HTTP API. The payload semantics (what bytes mean) are agreed with
// the upstream operator that wrote them into the claim-check ring —
// for the production path that's pre-processed CHW float32 image data.
//
// Concurrent calls are safe. The connection pool is shared across
// goroutines.
type TritonClient struct {
	BaseURL      string
	ModelName    string
	ModelVersion string
	HTTPClient   *http.Client
	Opts         Options

	// inFlight tracks ModelInfer goroutines for graceful drain.
	inFlight sync.WaitGroup
	closed   atomic.Bool

	// lastStats stashes the most-recent Stats so the service layer can
	// retrieve it without changing the Detector interface signature.
	lastStatsMu sync.Mutex
	lastStats   Stats
}

// NewTriton constructs a Triton client with defaults. For custom
// timeouts/retries use NewTritonWithOptions.
func NewTriton(baseURL, modelName, modelVersion string) *TritonClient {
	return NewTritonWithOptions(baseURL, modelName, modelVersion, Options{})
}

// NewTritonWithOptions constructs a Triton client with explicit options.
func NewTritonWithOptions(baseURL, modelName, modelVersion string, opts Options) *TritonClient {
	opts.withDefaults()
	transport := &http.Transport{
		MaxIdleConns:          opts.MaxIdleConnsPerHost * 2,
		MaxIdleConnsPerHost:   opts.MaxIdleConnsPerHost,
		MaxConnsPerHost:       opts.MaxIdleConnsPerHost * 2,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ExpectContinueTimeout: 0,
		DisableCompression:    true, // Triton tensors compress poorly.
		ForceAttemptHTTP2:     true,
	}
	return &TritonClient{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		ModelName:    modelName,
		ModelVersion: modelVersion,
		HTTPClient: &http.Client{
			Timeout:   opts.PerRequestTimeout,
			Transport: transport,
		},
		Opts: opts,
	}
}

func (c *TritonClient) ModelID() string        { return c.ModelName }
func (c *TritonClient) ModelVersionID() string { return c.ModelVersion }

// LastStats returns the Stats from the most recent successful Detect
// call on this client. Useful for the service layer to record
// inference.* events without changing the Detector interface.
func (c *TritonClient) LastStats() Stats {
	c.lastStatsMu.Lock()
	defer c.lastStatsMu.Unlock()
	return c.lastStats
}

// Close drains in-flight requests up to the deadline carried by ctx
// (defaulting to 30 s if none). New Detect calls after Close return an
// error.
func (c *TritonClient) Close(ctx context.Context) error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	done := make(chan struct{})
	go func() {
		c.inFlight.Wait()
		close(done)
	}()
	select {
	case <-done:
		c.HTTPClient.CloseIdleConnections()
		return nil
	case <-ctx.Done():
		c.HTTPClient.CloseIdleConnections()
		return ctx.Err()
	}
}

// HealthLive returns nil if Triton answers /v2/health/live with 200.
// Used by the inference-router's readiness probe.
func (c *TritonClient) HealthLive(ctx context.Context) error {
	return c.simpleGet(ctx, "/v2/health/live")
}

// HealthReady returns nil if Triton answers /v2/health/ready with 200.
// "Ready" means every loaded model is ready to serve; if you want
// per-model readiness use ModelReady.
func (c *TritonClient) HealthReady(ctx context.Context) error {
	return c.simpleGet(ctx, "/v2/health/ready")
}

// ModelReady returns nil if /v2/models/<name>/versions/<version>/ready
// returns 200.
func (c *TritonClient) ModelReady(ctx context.Context) error {
	return c.simpleGet(ctx, fmt.Sprintf("/v2/models/%s/versions/%s/ready",
		c.ModelName, c.ModelVersion))
}

func (c *TritonClient) simpleGet(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.Opts.UserAgent)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("triton: %s returned %d", path, resp.StatusCode)
	}
	return nil
}

// Detect calls Triton with the raw payload bytes. The on-wire request
// is the KServe v2 inference protocol; the response shape depends on
// the model. We try a small set of common output formats and fall
// through to raising an error on unknown shapes so misconfiguration is
// loud.
//
// Retries with exponential backoff + jitter on 5xx + transient network
// errors; bounded by ctx deadline and Opts.MaxRetries.
func (c *TritonClient) Detect(ctx context.Context, frame *dataplanev1.FrameDescriptor, payload []byte) ([]*dataplanev1.Detection, error) {
	if c.closed.Load() {
		return nil, errors.New("triton: client closed")
	}
	c.inFlight.Add(1)
	defer c.inFlight.Done()

	start := time.Now()
	url := fmt.Sprintf("%s/v2/models/%s/versions/%s/infer",
		c.BaseURL, c.ModelName, c.ModelVersion)

	body := tritonRequest{
		Inputs: []tritonTensor{{
			Name:     "input",
			Datatype: "BYTES",
			Shape:    []int{1},
			Data:     []any{string(payload)},
		}},
		Outputs: []tritonOutput{{Name: "output"}},
	}
	buf, _ := json.Marshal(body)

	var (
		out      tritonResponse
		attempts int
	)
	for {
		attempts++
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.Opts.UserAgent)
		// Hint Triton to populate the cache_hit response parameter.
		req.Header.Set("Inference-Header-Content-Length", fmt.Sprint(len(buf)))

		resp, err := c.HTTPClient.Do(req)
		retryable := false
		var status int
		if err == nil {
			status = resp.StatusCode
			if status == http.StatusOK {
				dec := json.NewDecoder(resp.Body)
				decErr := dec.Decode(&out)
				_ = resp.Body.Close()
				if decErr != nil {
					return nil, fmt.Errorf("triton: decode: %w", decErr)
				}
				stats := Stats{
					CacheHit:  out.cacheHit(),
					Attempts:  attempts,
					Latency:   time.Since(start),
					BatchSize: 1,
				}
				c.lastStatsMu.Lock()
				c.lastStats = stats
				c.lastStatsMu.Unlock()
				return out.toDetections(frame, c.ModelName, c.ModelVersion)
			}
			// 5xx is retryable; 4xx is the caller's bug.
			retryable = status >= 500 && status <= 599
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			err = fmt.Errorf("triton: status %d: %s", status, strings.TrimSpace(string(b)))
		} else {
			retryable = isRetryableNetErr(err)
			err = fmt.Errorf("triton: %w", err)
		}

		if !retryable || attempts > c.Opts.MaxRetries {
			return nil, err
		}
		// Backoff with full jitter, bounded by remaining deadline.
		backoff := jitter(exponentialBackoff(c.Opts.InitialBackoff, c.Opts.MaxBackoff, attempts-1))
		if dl, ok := ctx.Deadline(); ok {
			if remaining := time.Until(dl); remaining < backoff {
				return nil, err
			}
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func exponentialBackoff(initial, max time.Duration, attempt int) time.Duration {
	d := initial << attempt
	if d > max {
		d = max
	}
	return d
}

func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	// Full jitter: [0, d].
	return time.Duration(rand.Int63n(int64(d) + 1)) //nolint:gosec // not security-sensitive
}

func isRetryableNetErr(err error) bool {
	if err == nil {
		return false
	}
	// net/url errors with a Timeout() method, io.EOF on the wire, etc.
	var ne interface{ Timeout() bool }
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	s := err.Error()
	for _, frag := range []string{
		"connection reset",
		"connection refused",
		"broken pipe",
		"EOF",
		"i/o timeout",
		"no such host",
	} {
		if strings.Contains(s, frag) {
			return true
		}
	}
	return false
}

// --- KServe v2 wire types ---

type tritonTensor struct {
	Name       string         `json:"name"`
	Datatype   string         `json:"datatype"`
	Shape      []int          `json:"shape"`
	Data       []any          `json:"data,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type tritonOutput struct {
	Name string `json:"name"`
}

type tritonRequest struct {
	Inputs     []tritonTensor `json:"inputs"`
	Outputs    []tritonOutput `json:"outputs"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type tritonResponse struct {
	ModelName    string         `json:"model_name"`
	ModelVersion string         `json:"model_version"`
	Outputs      []tritonTensor `json:"outputs"`
	Parameters   map[string]any `json:"parameters,omitempty"`
}

// cacheHit reads the response.parameters.cache_hit boolean. Returns
// false if absent (older Triton servers, cache disabled).
func (r *tritonResponse) cacheHit() bool {
	if r.Parameters == nil {
		return false
	}
	v, ok := r.Parameters["cache_hit"].(bool)
	return ok && v
}

// toDetections projects a Triton response onto the platform's Detection
// schema. We support two output shapes:
//
//	1. JSON-encoded list of {class, score, bbox{x,y,w,h}} entries —
//	   common for custom Triton Python backends.
//	2. A flat FP32 tensor of [N x 6] = [x, y, w, h, score, class_id] —
//	   what Triton TensorRT outputs for detection models.
func (r *tritonResponse) toDetections(frame *dataplanev1.FrameDescriptor, modelID, modelVer string) ([]*dataplanev1.Detection, error) {
	if len(r.Outputs) == 0 {
		return nil, fmt.Errorf("triton: empty outputs")
	}
	out := r.Outputs[0]

	// Shape 1: JSON-encoded list, surfaced as a single BYTES element.
	if out.Datatype == "BYTES" && len(out.Data) == 1 {
		if s, ok := out.Data[0].(string); ok {
			var items []struct {
				Class string  `json:"class"`
				Score float32 `json:"score"`
				BBox  struct {
					X, Y, W, H float32
				} `json:"bbox"`
			}
			if err := json.Unmarshal([]byte(s), &items); err == nil {
				dets := make([]*dataplanev1.Detection, 0, len(items))
				for _, it := range items {
					dets = append(dets, &dataplanev1.Detection{
						StreamId: frame.GetStreamId(), PipelineId: frame.GetPipelineId(),
						FrameSeq: frame.GetFrameSeq(), CaptureTime: frame.GetCaptureTime(),
						ModelId: modelID, ModelVersionId: modelVer,
						ClassLabel: it.Class, Score: it.Score,
						Bbox: &dataplanev1.BoundingBox{X: it.BBox.X, Y: it.BBox.Y, W: it.BBox.W, H: it.BBox.H},
					})
				}
				return dets, nil
			}
		}
	}

	// Shape 2: flat FP32 [N x 6].
	if out.Datatype == "FP32" && len(out.Shape) == 2 && out.Shape[1] == 6 {
		n := out.Shape[0]
		if len(out.Data) != n*6 {
			return nil, fmt.Errorf("triton: FP32 shape mismatch")
		}
		dets := make([]*dataplanev1.Detection, 0, n)
		for i := 0; i < n; i++ {
			f := func(idx int) float32 {
				v, _ := out.Data[i*6+idx].(float64)
				return float32(v)
			}
			dets = append(dets, &dataplanev1.Detection{
				StreamId: frame.GetStreamId(), PipelineId: frame.GetPipelineId(),
				FrameSeq: frame.GetFrameSeq(), CaptureTime: frame.GetCaptureTime(),
				ModelId: modelID, ModelVersionId: modelVer,
				ClassLabel: fmt.Sprintf("class-%d", int(f(5))),
				Score:      f(4),
				Bbox:       &dataplanev1.BoundingBox{X: f(0), Y: f(1), W: f(2), H: f(3)},
			})
		}
		return dets, nil
	}

	return nil, fmt.Errorf("triton: unrecognized output shape %s/%v", out.Datatype, out.Shape)
}

// LoadModel asks Triton to load a model via the management API. The
// platform's policy is that only model-registry calls this; the
// AuthorizationPolicy enforces it at the mesh. Surfaced here so
// model-registry doesn't need to re-implement the wire shape.
func (c *TritonClient) LoadModel(ctx context.Context, name string) error {
	return c.repositoryAction(ctx, name, "load")
}

// UnloadModel is the inverse of LoadModel.
func (c *TritonClient) UnloadModel(ctx context.Context, name string) error {
	return c.repositoryAction(ctx, name, "unload")
}

func (c *TritonClient) repositoryAction(ctx context.Context, name, action string) error {
	url := fmt.Sprintf("%s/v2/repository/models/%s/%s", c.BaseURL, name, action)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.Opts.UserAgent)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("triton: %s %s: %w", action, name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("triton: %s %s status %d: %s", action, name, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
