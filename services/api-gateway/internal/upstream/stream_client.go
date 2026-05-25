// StreamClient mirrors PipelineClient but for the StreamService.
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

type StreamClient struct {
	conn *grpc.ClientConn
	cli  controlv1.StreamServiceClient
}

func DialStream(target string) (*StreamClient, error) {
	if target == "" {
		return nil, errors.New("upstream: empty target")
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("upstream: dial %q: %w", target, err)
	}
	return &StreamClient{conn: conn, cli: controlv1.NewStreamServiceClient(conn)}, nil
}

func (c *StreamClient) Close() error { return c.conn.Close() }

func (c *StreamClient) Get(ctx context.Context, id string) (*controlv1.Stream, error) {
	resp, err := c.cli.GetStream(ctx, &controlv1.GetStreamRequest{Id: id})
	if isStreamNotFound(err) {
		return nil, nil //nolint:nilnil // documented (nil, nil) sentinel — handler maps to 404
	}
	if err != nil {
		return nil, err
	}
	return resp.GetStream(), nil
}

func (c *StreamClient) List(ctx context.Context, req *controlv1.ListStreamsRequest) (*controlv1.ListStreamsResponse, error) {
	return c.cli.ListStreams(ctx, req)
}

func (c *StreamClient) Create(ctx context.Context, req *controlv1.CreateStreamRequest) (*controlv1.Stream, error) {
	resp, err := c.cli.CreateStream(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetStream(), nil
}

func (c *StreamClient) Delete(ctx context.Context, id string) error {
	_, err := c.cli.DeleteStream(ctx, &controlv1.DeleteStreamRequest{Id: id})
	return err
}

func isStreamNotFound(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.NotFound
}
