// Package service implements the PipelineService gRPC contract.
//
// Per ADR-001, this service only owns the *control-plane* view of a pipeline:
// the declarative DAG and its lifecycle state. Deploying a pipeline triggers
// a Temporal workflow that compiles the DAG into both a workflow definition
// and a data-plane operator graph. The Temporal client is wired into the
// service struct so the handler stays thin.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"

	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"
	"github.com/aegisvision/services/pipeline-service/internal/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Deployer abstracts the Temporal client so this package compiles without
// pulling in the Temporal SDK in the walking-skeleton phase. A real
// implementation calls client.ExecuteWorkflow against the deploy workflow.
type Deployer interface {
	StartDeploy(ctx context.Context, pipelineID string) (workflowID string, err error)
}

// Server implements controlv1.PipelineServiceServer.
type Server struct {
	controlv1.UnimplementedPipelineServiceServer
	Store    store.Store
	Deployer Deployer
	Logger   *slog.Logger
}

// New constructs a Server with safe defaults. Pass a real Deployer once
// Temporal is wired in; until then NoopDeployer is used.
func New(s store.Store, d Deployer, logger *slog.Logger) *Server {
	if d == nil {
		d = NoopDeployer{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{Store: s, Deployer: d, Logger: logger}
}

func (s *Server) GetPipeline(ctx context.Context, req *controlv1.GetPipelineRequest) (*controlv1.GetPipelineResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	p, err := s.Store.Get(req.GetId())
	if errors.Is(err, store.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "pipeline %q not found", req.GetId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "lookup failed: %v", err)
	}
	return &controlv1.GetPipelineResponse{Pipeline: p}, nil
}

func (s *Server) ListPipelines(ctx context.Context, req *controlv1.ListPipelinesRequest) (*controlv1.ListPipelinesResponse, error) {
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	pageSize := clamp(int(req.GetPage().GetPageSize()), 50, 1, 200)
	q := store.PageQuery{
		PageSize: pageSize,
		AfterID:  req.GetPage().GetPageToken(),
	}
	res, err := s.Store.List(store.ListFilter{
		TenantID:     req.GetTenantId(),
		ProjectID:    req.GetProjectId(),
		NameContains: req.GetNameContains(),
		StatusIn:     req.GetStatusIn(),
	}, q)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list failed: %v", err)
	}
	return &controlv1.ListPipelinesResponse{
		Pipelines: res.Items,
		Page: &commonv1.PageResponse{
			NextPageToken:    res.NextAfter,
			ApproximateTotal: res.Total,
		},
	}, nil
}

func (s *Server) CreatePipeline(ctx context.Context, req *controlv1.CreatePipelineRequest) (*controlv1.CreatePipelineResponse, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetTenant().GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant.id is required")
	}
	if req.GetProject().GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "project.id is required")
	}

	p := &controlv1.Pipeline{
		Id:              "p-" + newID(),
		Tenant:          req.GetTenant(),
		Project:         req.GetProject(),
		Name:            req.GetName(),
		Description:     req.GetDescription(),
		Labels:          req.GetLabels(),
		DagJson:         req.GetDagJson(),
		Status:          controlv1.PipelineStatus_PIPELINE_STATUS_DRAFT,
		CurrentRevision: 1,
	}
	created, err := s.Store.Create(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create failed: %v", err)
	}
	s.Logger.InfoContext(ctx, "pipeline created",
		slog.String("pipeline_id", created.GetId()),
		slog.String("tenant_id", created.GetTenant().GetId()),
	)
	return &controlv1.CreatePipelineResponse{Pipeline: created}, nil
}

func (s *Server) DeployPipeline(ctx context.Context, req *controlv1.DeployPipelineRequest) (*controlv1.DeployPipelineResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if _, err := s.Store.Get(req.GetId()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "pipeline %q not found", req.GetId())
		}
		return nil, status.Errorf(codes.Internal, "lookup failed: %v", err)
	}

	workflowID, err := s.Deployer.StartDeploy(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "deploy workflow start failed: %v", err)
	}
	updated, err := s.Store.UpdateStatus(req.GetId(), controlv1.PipelineStatus_PIPELINE_STATUS_DEPLOYING)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "status update failed: %v", err)
	}

	s.Logger.InfoContext(ctx, "pipeline deploy started",
		slog.String("pipeline_id", req.GetId()),
		slog.String("workflow_id", workflowID),
	)

	return &controlv1.DeployPipelineResponse{
		WorkflowId: workflowID,
		Status:     updated.GetStatus(),
	}, nil
}

// NoopDeployer is a stand-in for the Temporal client; it mints a synthetic
// workflow id so the contract is satisfied during Phase 0.
type NoopDeployer struct{}

func (NoopDeployer) StartDeploy(_ context.Context, pipelineID string) (string, error) {
	return "wf-" + pipelineID + "-" + newID(), nil
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func clamp(v, def, lo, hi int) int {
	if v <= 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
