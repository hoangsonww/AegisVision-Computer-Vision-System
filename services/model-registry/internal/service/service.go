// Package service implements ModelRegistryService.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"

	"github.com/aegisvision/services/model-registry/internal/store"

	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	controlv1.UnimplementedModelRegistryServiceServer
	Store  *store.SQLStore
	Logger *slog.Logger
}

func New(s *store.SQLStore, l *slog.Logger) *Server {
	if l == nil {
		l = slog.Default()
	}
	return &Server{Store: s, Logger: l}
}

func (s *Server) GetModel(ctx context.Context, req *controlv1.GetModelRequest) (*controlv1.GetModelResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	m, err := s.Store.GetModel(req.GetId())
	if errors.Is(err, store.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "model %q not found", req.GetId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "lookup: %v", err)
	}
	return &controlv1.GetModelResponse{Model: m}, nil
}

func (s *Server) ListModels(ctx context.Context, req *controlv1.ListModelsRequest) (*controlv1.ListModelsResponse, error) {
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	ps := int(req.GetPage().GetPageSize())
	if ps <= 0 {
		ps = 50
	}
	if ps > 200 {
		ps = 200
	}
	items, next, err := s.Store.ListModels(req.GetTenantId(), req.GetKind(), ps, req.GetPage().GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list: %v", err)
	}
	return &controlv1.ListModelsResponse{
		Models: items,
		Page:   &commonv1.PageResponse{NextPageToken: next, ApproximateTotal: -1},
	}, nil
}

func (s *Server) CreateModel(ctx context.Context, req *controlv1.CreateModelRequest) (*controlv1.CreateModelResponse, error) {
	if req.GetName() == "" || req.GetTenant().GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant and name are required")
	}
	m := &controlv1.Model{
		Id:     "model-" + slug(req.GetName()) + "-" + newID(4),
		Tenant: req.GetTenant(),
		Name:   req.GetName(),
		Kind:   req.GetKind(),
		Family: req.GetFamily(),
		Labels: req.GetLabels(),
	}
	if err := s.Store.CreateModel(m); err != nil {
		return nil, status.Errorf(codes.Internal, "create: %v", err)
	}
	s.Logger.InfoContext(ctx, "model created",
		slog.String("model_id", m.GetId()), slog.String("kind", m.GetKind().String()))
	return &controlv1.CreateModelResponse{Model: m}, nil
}

func (s *Server) CreateModelVersion(ctx context.Context, req *controlv1.CreateModelVersionRequest) (*controlv1.CreateModelVersionResponse, error) {
	if req.GetModelId() == "" || req.GetSemver() == "" || req.GetArtifactUri() == "" {
		return nil, status.Error(codes.InvalidArgument, "model_id, semver, artifact_uri are required")
	}
	v := &controlv1.ModelVersion{
		Id:              "mv-" + newID(8),
		ModelId:         req.GetModelId(),
		Semver:          req.GetSemver(),
		Channel:         controlv1.ModelChannel_MODEL_CHANNEL_CANARY,
		ArtifactUri:     req.GetArtifactUri(),
		Signature:       req.GetSignature(),
		SbomUri:         req.GetSbomUri(),
		FairnessEvalUri: req.GetFairnessEvalUri(),
		CostProfiles:    req.GetCostProfiles(),
	}
	if err := s.Store.CreateVersion(v); err != nil {
		return nil, status.Errorf(codes.Internal, "create_version: %v", err)
	}
	return &controlv1.CreateModelVersionResponse{Version: v}, nil
}

func (s *Server) PromoteModelVersion(ctx context.Context, req *controlv1.PromoteModelVersionRequest) (*controlv1.PromoteModelVersionResponse, error) {
	if req.GetModelVersionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "model_version_id is required")
	}
	v, err := s.Store.PromoteVersion(req.GetModelVersionId(), req.GetChannel())
	if errors.Is(err, store.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "model version not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
	}
	s.Logger.InfoContext(ctx, "model version promoted",
		slog.String("mv_id", v.GetId()), slog.String("channel", v.GetChannel().String()))
	return &controlv1.PromoteModelVersionResponse{Version: v}, nil
}

func slug(s string) string {
	r := strings.NewReplacer(" ", "-", "_", "-")
	return strings.ToLower(r.Replace(s))
}
func newID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
