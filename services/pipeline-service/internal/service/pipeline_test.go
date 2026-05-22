package service_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"
	"github.com/aegisvision/services/pipeline-service/internal/service"
	"github.com/aegisvision/services/pipeline-service/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newSvc(t *testing.T) *service.Server {
	t.Helper()
	return service.New(store.NewMemoryStore(), service.NoopDeployer{}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
}

func TestCreateAndGetPipeline(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	resp, err := svc.CreatePipeline(ctx, &controlv1.CreatePipelineRequest{
		Tenant:  &commonv1.TenantRef{Id: "t-1"},
		Project: &commonv1.ProjectRef{Id: "proj-1", TenantId: "t-1"},
		Name:    "dock-watch",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.GetPipeline().GetId() == "" {
		t.Fatal("create returned empty id")
	}
	if got := resp.GetPipeline().GetStatus(); got != controlv1.PipelineStatus_PIPELINE_STATUS_DRAFT {
		t.Fatalf("want DRAFT, got %v", got)
	}

	got, err := svc.GetPipeline(ctx, &controlv1.GetPipelineRequest{Id: resp.GetPipeline().GetId()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetPipeline().GetName() != "dock-watch" {
		t.Fatalf("name mismatch: %q", got.GetPipeline().GetName())
	}
}

func TestCreatePipeline_ValidatesArguments(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	cases := []*controlv1.CreatePipelineRequest{
		{Tenant: &commonv1.TenantRef{Id: "t"}, Project: &commonv1.ProjectRef{Id: "p"}}, // no name
		{Name: "x", Project: &commonv1.ProjectRef{Id: "p"}},                            // no tenant
		{Name: "x", Tenant: &commonv1.TenantRef{Id: "t"}},                              // no project
	}
	for i, req := range cases {
		_, err := svc.CreatePipeline(ctx, req)
		if code := status.Code(err); code != codes.InvalidArgument {
			t.Fatalf("case %d: want InvalidArgument, got %v", i, code)
		}
	}
}

func TestListPipelines_PaginatesDeterministically(t *testing.T) {
	memstore := store.NewMemoryStore()
	memstore.Seed("t-1", "proj-1", "A")
	memstore.Seed("t-1", "proj-1", "B")
	memstore.Seed("t-1", "proj-1", "C")
	svc := service.New(memstore, service.NoopDeployer{}, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	ctx := context.Background()
	resp1, err := svc.ListPipelines(ctx, &controlv1.ListPipelinesRequest{
		TenantId: "t-1",
		Page:     &commonv1.PageRequest{PageSize: 2},
	})
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if got := len(resp1.GetPipelines()); got != 2 {
		t.Fatalf("page 1: want 2 items, got %d", got)
	}
	if resp1.GetPage().GetNextPageToken() == "" {
		t.Fatal("page 1: expected next_page_token")
	}

	resp2, err := svc.ListPipelines(ctx, &controlv1.ListPipelinesRequest{
		TenantId: "t-1",
		Page:     &commonv1.PageRequest{PageSize: 2, PageToken: resp1.GetPage().GetNextPageToken()},
	})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if got := len(resp2.GetPipelines()); got != 1 {
		t.Fatalf("page 2: want 1 item, got %d", got)
	}
	if resp2.GetPage().GetNextPageToken() != "" {
		t.Fatal("page 2: expected terminal cursor")
	}
}

func TestDeployPipeline_NotFound(t *testing.T) {
	svc := newSvc(t)
	_, err := svc.DeployPipeline(context.Background(), &controlv1.DeployPipelineRequest{Id: "missing"})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("want NotFound, got %v", got)
	}
}

func TestDeployPipeline_Transitions(t *testing.T) {
	memstore := store.NewMemoryStore()
	id := memstore.Seed("t-1", "proj-1", "deploy-test")
	svc := service.New(memstore, fakeDeployer{wf: "wf-fixed"}, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	resp, err := svc.DeployPipeline(context.Background(), &controlv1.DeployPipelineRequest{Id: id})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if resp.GetWorkflowId() != "wf-fixed" {
		t.Fatalf("workflow id mismatch: %q", resp.GetWorkflowId())
	}
	if resp.GetStatus() != controlv1.PipelineStatus_PIPELINE_STATUS_DEPLOYING {
		t.Fatalf("status: want DEPLOYING, got %v", resp.GetStatus())
	}
}

type fakeDeployer struct{ wf string }

func (f fakeDeployer) StartDeploy(context.Context, string) (string, error) { return f.wf, nil }

func TestDeployPipeline_DeployerErrorIsInternal(t *testing.T) {
	memstore := store.NewMemoryStore()
	id := memstore.Seed("t-1", "proj-1", "deploy-fail")
	svc := service.New(memstore, errDeployer{}, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	_, err := svc.DeployPipeline(context.Background(), &controlv1.DeployPipelineRequest{Id: id})
	if code := status.Code(err); code != codes.Internal {
		t.Fatalf("want Internal, got %v", code)
	}
}

type errDeployer struct{}

func (errDeployer) StartDeploy(context.Context, string) (string, error) {
	return "", errors.New("temporal unavailable")
}
