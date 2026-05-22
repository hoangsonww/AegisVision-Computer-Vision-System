package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aegisvision/pkg/platform/idempotency"
	"github.com/aegisvision/pkg/platform/metrics"
	"github.com/aegisvision/pkg/platform/middleware"
	"github.com/aegisvision/pkg/platform/pagination"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/aegisvision/services/api-gateway/internal/handlers"
	"github.com/aegisvision/services/api-gateway/internal/server"
	"github.com/aegisvision/services/api-gateway/internal/upstream"

	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakePipelineSrv implements controlv1.PipelineServiceServer in-process. It
// gives the gateway tests an end-to-end gRPC hop (router → grpc client →
// grpc server → handler) without crossing module boundaries. The behavior
// mirrors the production pipeline-service for the cases the gateway cares
// about.
type fakePipelineSrv struct {
	controlv1.UnimplementedPipelineServiceServer

	mu        sync.Mutex
	pipelines map[string]*controlv1.Pipeline
}

func newFake() *fakePipelineSrv {
	return &fakePipelineSrv{pipelines: map[string]*controlv1.Pipeline{}}
}

func (f *fakePipelineSrv) GetPipeline(ctx context.Context, req *controlv1.GetPipelineRequest) (*controlv1.GetPipelineResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pipelines[req.GetId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "not found")
	}
	return &controlv1.GetPipelineResponse{Pipeline: p}, nil
}

func (f *fakePipelineSrv) ListPipelines(ctx context.Context, req *controlv1.ListPipelinesRequest) (*controlv1.ListPipelinesResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*controlv1.Pipeline{}
	for _, p := range f.pipelines {
		if p.GetTenant().GetId() == req.GetTenantId() {
			out = append(out, p)
		}
	}
	return &controlv1.ListPipelinesResponse{
		Pipelines: out,
		Page:      &commonv1.PageResponse{NextPageToken: "", ApproximateTotal: int64(len(out))},
	}, nil
}

func (f *fakePipelineSrv) CreatePipeline(ctx context.Context, req *controlv1.CreatePipelineRequest) (*controlv1.CreatePipelineResponse, error) {
	if req.GetName() == "" || req.GetTenant().GetId() == "" || req.GetProject().GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "missing required fields")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	id := "p-" + strings.ToLower(req.GetName())
	p := &controlv1.Pipeline{
		Id:              id,
		Tenant:          req.GetTenant(),
		Project:         req.GetProject(),
		Name:            req.GetName(),
		Description:     req.GetDescription(),
		Labels:          req.GetLabels(),
		Status:          controlv1.PipelineStatus_PIPELINE_STATUS_DRAFT,
		CurrentRevision: 1,
		DagJson:         req.GetDagJson(),
	}
	f.pipelines[id] = p
	return &controlv1.CreatePipelineResponse{Pipeline: p}, nil
}

func (f *fakePipelineSrv) DeployPipeline(ctx context.Context, req *controlv1.DeployPipelineRequest) (*controlv1.DeployPipelineResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pipelines[req.GetId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "not found")
	}
	p.Status = controlv1.PipelineStatus_PIPELINE_STATUS_DEPLOYING
	return &controlv1.DeployPipelineResponse{
		WorkflowId: "wf-" + req.GetId(),
		Status:     p.Status,
	}, nil
}

// seed mirrors store.Seed so tests don't need a real pipeline-service.
func (f *fakePipelineSrv) seed(tenantID, projectID, name string) string {
	id := "p-" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pipelines[id] = &controlv1.Pipeline{
		Id:              id,
		Tenant:          &commonv1.TenantRef{Id: tenantID},
		Project:         &commonv1.ProjectRef{Id: projectID, TenantId: tenantID},
		Name:            name,
		Status:          controlv1.PipelineStatus_PIPELINE_STATUS_VALIDATED,
		CurrentRevision: 1,
	}
	return id
}

// newEnd2End brings up an in-process gRPC server hosting fakePipelineSrv and
// a fully-wired api-gateway router that talks to it.
func newEnd2End(t *testing.T) (*httptest.Server, *fakePipelineSrv) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	fake := newFake()
	grpcSrv := grpc.NewServer()
	controlv1.RegisterPipelineServiceServer(grpcSrv, fake)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = grpcSrv.Serve(lis) }()
	t.Cleanup(grpcSrv.GracefulStop)

	client, err := upstream.Dial(lis.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	red := metrics.New("api-gateway-test", prometheus.NewRegistry())

	router := server.NewRouter(server.Deps{
		Logger:           logger,
		Metrics:          red,
		AuthZ:            middleware.AuthZDeciderFunc(func(context.Context, middleware.AuthZInput) (bool, error) { return true, nil }),
		IdempotencyStore: idempotency.NewMemoryStore(0),
		IdempotencyTTL:   time.Hour,
		Pipelines: &handlers.PipelineHandlers{
			Pipelines: client,
			Cursors:   pagination.NewEncoder("test-key"),
		},
	})

	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)
	return ts, fake
}

func doJSON(t *testing.T, method, url string, body any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = strings.NewReader(string(buf))
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func TestCreateListGet_RoundTrip(t *testing.T) {
	ts, _ := newEnd2End(t)

	resp, body := doJSON(t, "POST", ts.URL+"/v1/pipelines",
		map[string]any{"name": "dock-watch", "project_id": "proj-1"},
		map[string]string{"X-Tenant-Id": "t-1", "Idempotency-Key": "k1"},
	)
	if resp.StatusCode != 201 {
		t.Fatalf("create: want 201, got %d body=%s", resp.StatusCode, body)
	}
	var created map[string]any
	_ = json.Unmarshal(body, &created)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("missing id in %s", body)
	}
	if created["status"] != "draft" {
		t.Fatalf("status: %v", created["status"])
	}

	resp, body = doJSON(t, "GET", ts.URL+"/v1/pipelines", nil, map[string]string{"X-Tenant-Id": "t-1"})
	if resp.StatusCode != 200 {
		t.Fatalf("list: want 200, got %d body=%s", resp.StatusCode, body)
	}
	var list map[string]any
	_ = json.Unmarshal(body, &list)
	items := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}

	resp, body = doJSON(t, "GET", ts.URL+"/v1/pipelines/"+id, nil, map[string]string{"X-Tenant-Id": "t-1"})
	if resp.StatusCode != 200 {
		t.Fatalf("get: want 200, got %d body=%s", resp.StatusCode, body)
	}
}

func TestIdempotency_ReplayThroughGateway(t *testing.T) {
	ts, _ := newEnd2End(t)

	hdr := map[string]string{"X-Tenant-Id": "t-1", "Idempotency-Key": "abc"}
	resp1, body1 := doJSON(t, "POST", ts.URL+"/v1/pipelines",
		map[string]any{"name": "a", "project_id": "p"}, hdr,
	)
	if resp1.StatusCode != 201 {
		t.Fatalf("first: %d %s", resp1.StatusCode, body1)
	}
	resp2, body2 := doJSON(t, "POST", ts.URL+"/v1/pipelines",
		map[string]any{"name": "a", "project_id": "p"}, hdr,
	)
	if resp2.StatusCode != 201 {
		t.Fatalf("replay: %d %s", resp2.StatusCode, body2)
	}
	if resp2.Header.Get("Idempotent-Replay") != "true" {
		t.Fatal("missing Idempotent-Replay marker on replay")
	}
	if string(body1) != string(body2) {
		t.Fatalf("replay body diverged:\n%s\nvs\n%s", body1, body2)
	}
}

func TestCrossTenant_GetIs404(t *testing.T) {
	ts, _ := newEnd2End(t)

	resp, body := doJSON(t, "POST", ts.URL+"/v1/pipelines",
		map[string]any{"name": "x", "project_id": "p"},
		map[string]string{"X-Tenant-Id": "t-owner", "Idempotency-Key": "k"},
	)
	if resp.StatusCode != 201 {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var created map[string]any
	_ = json.Unmarshal(body, &created)
	id, _ := created["id"].(string)

	resp, body = doJSON(t, "GET", ts.URL+"/v1/pipelines/"+id, nil,
		map[string]string{"X-Tenant-Id": "t-attacker"})
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant: want 404, got %d body=%s", resp.StatusCode, body)
	}
}

func TestUnprocessable_OnInvalidCreate(t *testing.T) {
	ts, _ := newEnd2End(t)
	resp, body := doJSON(t, "POST", ts.URL+"/v1/pipelines",
		map[string]any{}, // no name, no project
		map[string]string{"X-Tenant-Id": "t-1", "Idempotency-Key": "z"},
	)
	if resp.StatusCode != 422 {
		t.Fatalf("want 422, got %d body=%s", resp.StatusCode, body)
	}
	var p map[string]any
	_ = json.Unmarshal(body, &p)
	if p["code"] != "unprocessable_entity" {
		t.Fatalf("unexpected error code: %v", p)
	}
	v := p["violations"].([]any)
	if len(v) != 2 {
		t.Fatalf("want 2 violations, got %d", len(v))
	}
}

func TestMissingTenant_Is401(t *testing.T) {
	ts, _ := newEnd2End(t)
	resp, _ := doJSON(t, "GET", ts.URL+"/v1/pipelines", nil, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestDeploy_Returns202WithWorkflowID(t *testing.T) {
	ts, fake := newEnd2End(t)
	id := fake.seed("t-1", "p-1", "to-deploy")

	resp, body := doJSON(t, "POST", ts.URL+"/v1/pipelines/"+id+":deploy", nil,
		map[string]string{"X-Tenant-Id": "t-1", "Idempotency-Key": "d-1"})
	if resp.StatusCode != 202 {
		t.Fatalf("want 202, got %d body=%s", resp.StatusCode, body)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if out["workflow_id"] == "" {
		t.Fatal("missing workflow_id")
	}
	if out["status"] != "deploying" {
		t.Fatalf("status: %v", out["status"])
	}
}

func TestAuthZDenied_Is403(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	red := metrics.New("api-gateway-test-denied", prometheus.NewRegistry())
	router := server.NewRouter(server.Deps{
		Logger:           logger,
		Metrics:          red,
		AuthZ:            middleware.AuthZDeciderFunc(func(context.Context, middleware.AuthZInput) (bool, error) { return false, nil }),
		IdempotencyStore: idempotency.NewMemoryStore(0),
		IdempotencyTTL:   time.Hour,
		Pipelines:        &handlers.PipelineHandlers{Cursors: pagination.NewEncoder("k")},
	})
	ts := httptest.NewServer(router)
	defer ts.Close()
	resp, _ := doJSON(t, "GET", ts.URL+"/v1/pipelines", nil, map[string]string{"X-Tenant-Id": "t-1"})
	if resp.StatusCode != 403 {
		t.Fatalf("want 403, got %d", resp.StatusCode)
	}
}

func TestAuthZUnavailable_Is503(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	red := metrics.New("api-gateway-test-unavail", prometheus.NewRegistry())
	router := server.NewRouter(server.Deps{
		Logger:  logger,
		Metrics: red,
		AuthZ: middleware.AuthZDeciderFunc(func(context.Context, middleware.AuthZInput) (bool, error) {
			return false, errors.Join(middleware.ErrAuthZUnavailable, errors.New("dial tcp: refused"))
		}),
		IdempotencyStore: idempotency.NewMemoryStore(0),
		IdempotencyTTL:   time.Hour,
		Pipelines:        &handlers.PipelineHandlers{Cursors: pagination.NewEncoder("k")},
	})
	ts := httptest.NewServer(router)
	defer ts.Close()
	resp, _ := doJSON(t, "GET", ts.URL+"/v1/pipelines", nil, map[string]string{"X-Tenant-Id": "t-1"})
	if resp.StatusCode != 503 {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}
