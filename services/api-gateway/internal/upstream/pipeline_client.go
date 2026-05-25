// Package upstream wraps the gRPC clients api-gateway uses to call control-
// plane services. Wrapping keeps the HTTP handlers free of generated-code
// imports and provides one place to add tracing/metrics/retries.
package upstream

import (
	"context"
	"errors"
	"fmt"

	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// PipelineClient is the api-gateway-facing surface of pipeline-service. The
// signatures lean on the protobuf types so we don't lose schema fidelity at
// the gateway boundary.
type PipelineClient struct {
	conn *grpc.ClientConn
	cli  controlv1.PipelineServiceClient
}

// Dial constructs a PipelineClient. Inside the mesh (Istio Ambient), upstream
// addresses are reached over plaintext gRPC and the mesh upgrades to mTLS on
// the wire; for direct dial in dev, plaintext is the same code path.
func Dial(target string) (*PipelineClient, error) {
	if target == "" {
		return nil, errors.New("upstream: empty target")
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("upstream: dial %q: %w", target, err)
	}
	return &PipelineClient{conn: conn, cli: controlv1.NewPipelineServiceClient(conn)}, nil
}

// Close releases the connection at shutdown.
func (c *PipelineClient) Close() error { return c.conn.Close() }

// Get proxies a get-by-id call. Returns (nil, nil) when the upstream returns
// NotFound so the handler can translate to a 404 problem+json.
func (c *PipelineClient) Get(ctx context.Context, id string) (*controlv1.Pipeline, error) {
	resp, err := c.cli.GetPipeline(ctx, &controlv1.GetPipelineRequest{Id: id})
	if isNotFound(err) {
		return nil, nil //nolint:nilnil // documented (nil, nil) sentinel — handler maps to 404
	}
	if err != nil {
		return nil, err
	}
	return resp.GetPipeline(), nil
}

// List proxies a list call.
func (c *PipelineClient) List(ctx context.Context, req *controlv1.ListPipelinesRequest) (*controlv1.ListPipelinesResponse, error) {
	return c.cli.ListPipelines(ctx, req)
}

// Create proxies a create call.
func (c *PipelineClient) Create(ctx context.Context, req *controlv1.CreatePipelineRequest) (*controlv1.Pipeline, error) {
	resp, err := c.cli.CreatePipeline(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetPipeline(), nil
}

// Deploy proxies a deploy call.
func (c *PipelineClient) Deploy(ctx context.Context, id string) (*controlv1.DeployPipelineResponse, error) {
	return c.cli.DeployPipeline(ctx, &controlv1.DeployPipelineRequest{Id: id})
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.NotFound
}
