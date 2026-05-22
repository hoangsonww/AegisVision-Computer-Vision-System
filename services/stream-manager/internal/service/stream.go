// Package service implements StreamService.
//
// On Create, after persisting the Stream record, the service publishes an
// operator.control.<pipeline_id> message so a dataplane-runner can pick up
// the new stream and bind the pipeline's operator DAG against it. The
// publish is fire-and-forget; the runner is reconcile-loop driven and will
// see the stream again on its own next poll if the message is lost.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/services/stream-manager/internal/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements controlv1.StreamServiceServer.
type Server struct {
	controlv1.UnimplementedStreamServiceServer
	Store     store.Store
	Publisher bus.Publisher // optional; may be nil in tests
	Logger    *slog.Logger
}

func New(s store.Store, pub bus.Publisher, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{Store: s, Publisher: pub, Logger: logger}
}

func (s *Server) GetStream(ctx context.Context, req *controlv1.GetStreamRequest) (*controlv1.GetStreamResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	st, err := s.Store.Get(req.GetId())
	if errors.Is(err, store.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "stream %q not found", req.GetId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "lookup failed: %v", err)
	}
	return &controlv1.GetStreamResponse{Stream: st}, nil
}

func (s *Server) ListStreams(ctx context.Context, req *controlv1.ListStreamsRequest) (*controlv1.ListStreamsResponse, error) {
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	pageSize := clamp(int(req.GetPage().GetPageSize()), 50, 1, 200)
	items, next, err := s.Store.List(req.GetTenantId(), req.GetProjectId(), req.GetSiteId(),
		req.GetHealthIn(), pageSize, req.GetPage().GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list failed: %v", err)
	}
	return &controlv1.ListStreamsResponse{
		Streams: items,
		Page:    &commonv1.PageResponse{NextPageToken: next, ApproximateTotal: -1},
	}, nil
}

func (s *Server) CreateStream(ctx context.Context, req *controlv1.CreateStreamRequest) (*controlv1.CreateStreamResponse, error) {
	st, err := s.Store.Create(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "create failed: %v", err)
	}
	s.Logger.InfoContext(ctx, "stream created",
		slog.String("stream_id", st.GetId()),
		slog.String("tenant_id", st.GetTenant().GetId()),
	)
	if s.Publisher != nil && req.GetPipelineId() != "" {
		s.dispatch(ctx, st, req.GetPipelineId())
	}
	return &controlv1.CreateStreamResponse{Stream: st}, nil
}

func (s *Server) DeleteStream(ctx context.Context, req *controlv1.DeleteStreamRequest) (*controlv1.DeleteStreamResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := s.Store.Delete(req.GetId()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "stream %q not found", req.GetId())
		}
		return nil, status.Errorf(codes.Internal, "delete failed: %v", err)
	}
	return &controlv1.DeleteStreamResponse{}, nil
}

// dispatch publishes an operator.control.<pipeline_id> directive. The payload
// is JSON so dataplane-runner can deserialize without taking a strict
// dependency on this service's protobuf — different runners may speak
// different operator vocabularies later.
func (s *Server) dispatch(ctx context.Context, st *controlv1.Stream, pipelineID string) {
	type cmd struct {
		Op         string `json:"op"`
		StreamID   string `json:"stream_id"`
		TenantID   string `json:"tenant_id"`
		PipelineID string `json:"pipeline_id"`
		Protocol   string `json:"protocol"`
		URL        string `json:"url_redacted"`
	}
	body, _ := json.Marshal(cmd{
		Op:         "start",
		StreamID:   st.GetId(),
		TenantID:   st.GetTenant().GetId(),
		PipelineID: pipelineID,
		Protocol:   st.GetProtocol().String(),
		URL:        st.GetUrlRedacted(),
	})
	subject := "operator.control." + pipelineID
	if err := s.Publisher.Publish(ctx, bus.Message{Subject: subject, Data: body}); err != nil {
		s.Logger.WarnContext(ctx, "dispatch failed",
			slog.String("subject", subject),
			slog.String("stream_id", st.GetId()),
			slog.String("err", err.Error()),
		)
	}
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
