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

type Server struct {
	GRPC *grpc.Server
	lis  net.Listener
	addr string
}

func New(addr string, svc controlv1.StreamServiceServer, opts ...grpc.ServerOption) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := grpc.NewServer(opts...)
	controlv1.RegisterStreamServiceServer(s, svc)

	h := health.NewServer()
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	h.SetServingStatus("aegisvision.control.v1.StreamService", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(s, h)
	reflection.Register(s)

	return &Server{GRPC: s, lis: lis, addr: lis.Addr().String()}, nil
}

func (s *Server) Addr() string  { return s.addr }
func (s *Server) Serve() error  { return s.GRPC.Serve(s.lis) }
func (s *Server) Shutdown(ctx context.Context) error {
	stopped := make(chan struct{})
	go func() { s.GRPC.GracefulStop(); close(stopped) }()
	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		s.GRPC.Stop()
		return ctx.Err()
	}
}
