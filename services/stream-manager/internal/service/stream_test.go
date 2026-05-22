package service_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aegisvision/pkg/bus"
	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"

	"github.com/aegisvision/services/stream-manager/internal/service"
	"github.com/aegisvision/services/stream-manager/internal/store"
)

func TestCreateStream_RedactsCredentials(t *testing.T) {
	svc := service.New(store.NewMemoryStore(), nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	resp, err := svc.CreateStream(context.Background(), &controlv1.CreateStreamRequest{
		Tenant:   &commonv1.TenantRef{Id: "t-1"},
		Project:  &commonv1.ProjectRef{Id: "p-1", TenantId: "t-1"},
		Name:     "dock-1",
		Protocol: controlv1.StreamProtocol_STREAM_PROTOCOL_RTSP,
		Url:      "rtsp://user:secret@192.168.10.5:554/dock",
	})
	if err != nil {
		t.Fatal(err)
	}
	url := resp.GetStream().GetUrlRedacted()
	if url == "" {
		t.Fatal("empty url_redacted")
	}
	if want := "rtsp://192.168.10.5:554/dock"; url != want {
		t.Fatalf("want %q, got %q", want, url)
	}
}

func TestCreateStream_DispatchesToBus(t *testing.T) {
	b := bus.NewInMemory()
	var mu sync.Mutex
	var lastSubject string
	var lastCmd map[string]any

	_, _ = b.Subscribe(context.Background(), "operator.control.*", "", func(_ context.Context, m bus.Message) error {
		mu.Lock()
		defer mu.Unlock()
		lastSubject = m.Subject
		var cmd map[string]any
		_ = json.Unmarshal(m.Data, &cmd)
		lastCmd = cmd
		return nil
	})

	svc := service.New(store.NewMemoryStore(), b, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	_, err := svc.CreateStream(context.Background(), &controlv1.CreateStreamRequest{
		Tenant:     &commonv1.TenantRef{Id: "t-1"},
		Project:    &commonv1.ProjectRef{Id: "p-1", TenantId: "t-1"},
		Name:       "dock-1",
		Protocol:   controlv1.StreamProtocol_STREAM_PROTOCOL_RTSP,
		Url:        "rtsp://192.168.10.5/dock",
		PipelineId: "p-deploy-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if lastSubject != "operator.control.p-deploy-1" {
		t.Fatalf("subject: %q", lastSubject)
	}
	if lastCmd["op"] != "start" {
		t.Fatalf("op: %v", lastCmd["op"])
	}
	if lastCmd["pipeline_id"] != "p-deploy-1" {
		t.Fatalf("pipeline: %v", lastCmd["pipeline_id"])
	}
}
