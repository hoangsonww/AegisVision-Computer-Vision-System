// Package server hosts the gRPC server for pipeline-service.
//
// gRPC because (a) protobuf is the contract source of truth (ADR-007), (b)
// streaming RPCs are first-class when we later add ListPipelineEvents, and
// (c) the platform mesh upgrades cleartext gRPC to mTLS automatically
// inside the namespace. We expose the standard health service so kubelet
// can probe with grpc_health_probe.
package server

import (
	"context"
	"net"

	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// Server bundles a *grpc.Server and the listener it owns.
type Server struct {
	GRPC   *grpc.Server
	lis    net.Listener
	addr   string
	health *health.Server
}

// New constructs a gRPC server bound to addr. It does NOT call Serve.
func New(addr string, pipelineSvc controlv1.PipelineServiceServer, opts ...grpc.ServerOption) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := grpc.NewServer(opts...)
	controlv1.RegisterPipelineServiceServer(s, pipelineSvc)

	h := health.NewServer()
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	h.SetServingStatus("aegisvision.control.v1.PipelineService", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(s, h)

	// Reflection makes ad-hoc grpcurl debugging work in dev / staging. The
	// production Helm chart strips reflection via an interceptor when env
	// != "dev".
	reflection.Register(s)

	return &Server{GRPC: s, lis: lis, addr: lis.Addr().String(), health: h}, nil
}

// Addr returns the actual listen address (useful when bound to ":0" in tests).
func (s *Server) Addr() string { return s.addr }

// Serve blocks until the underlying gRPC server returns.
func (s *Server) Serve() error { return s.GRPC.Serve(s.lis) }

// Shutdown gracefully stops the server, draining in-flight RPCs.
func (s *Server) Shutdown(ctx context.Context) error {
	stopped := make(chan struct{})
	go func() {
		s.GRPC.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		s.GRPC.Stop()
		return ctx.Err()
	}
}
