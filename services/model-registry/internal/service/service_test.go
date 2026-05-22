package service_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/aegisvision/pkg/store/sqldb"
	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"

	"github.com/aegisvision/services/model-registry/internal/service"
	"github.com/aegisvision/services/model-registry/internal/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newSvc(t *testing.T) *service.Server {
	t.Helper()
	db, err := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return service.New(store.New(db, false), slog.New(slog.NewTextHandler(os.Stderr, nil)))
}

func TestCreateAndGetModel(t *testing.T) {
	svc := newSvc(t)
	resp, err := svc.CreateModel(context.Background(), &controlv1.CreateModelRequest{
		Tenant: &commonv1.TenantRef{Id: "t-1"},
		Name:   "yolo11n",
		Kind:   controlv1.ModelKind_MODEL_KIND_DETECTOR,
		Family: "yolo11",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetModel().GetId() == "" {
		t.Fatal("empty id")
	}

	got, err := svc.GetModel(context.Background(), &controlv1.GetModelRequest{Id: resp.GetModel().GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetModel().GetName() != "yolo11n" {
		t.Fatalf("name: %q", got.GetModel().GetName())
	}
}

func TestPromote_RefusesSTABLEWithoutVerifiedCostProfile(t *testing.T) {
	svc := newSvc(t)
	m, _ := svc.CreateModel(context.Background(), &controlv1.CreateModelRequest{
		Tenant: &commonv1.TenantRef{Id: "t-1"}, Name: "yolo", Kind: controlv1.ModelKind_MODEL_KIND_DETECTOR,
	})
	v, _ := svc.CreateModelVersion(context.Background(), &controlv1.CreateModelVersionRequest{
		ModelId: m.GetModel().GetId(), Semver: "0.1.0",
		ArtifactUri: "oci://example/yolo:v1",
	})
	_, err := svc.PromoteModelVersion(context.Background(), &controlv1.PromoteModelVersionRequest{
		ModelVersionId: v.GetVersion().GetId(),
		Channel:        controlv1.ModelChannel_MODEL_CHANNEL_STABLE,
	})
	if code := status.Code(err); code != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", code)
	}
}

func TestPromote_AcceptsSTABLEWithVerifiedCostProfile(t *testing.T) {
	svc := newSvc(t)
	m, _ := svc.CreateModel(context.Background(), &controlv1.CreateModelRequest{
		Tenant: &commonv1.TenantRef{Id: "t-1"}, Name: "yolo", Kind: controlv1.ModelKind_MODEL_KIND_DETECTOR,
	})
	v, _ := svc.CreateModelVersion(context.Background(), &controlv1.CreateModelVersionRequest{
		ModelId: m.GetModel().GetId(), Semver: "0.1.0",
		ArtifactUri: "oci://example/yolo:v1",
		CostProfiles: []*controlv1.CostProfile{{
			GpuSku: "L4", MigProfile: "1g.10gb", Precision: "fp16",
			InputHeight: 640, InputWidth: 640, Batch: 1,
			VramBytes: 1 << 30, InferenceMs: 8.5, Verified: true,
		}},
	})
	resp, err := svc.PromoteModelVersion(context.Background(), &controlv1.PromoteModelVersionRequest{
		ModelVersionId: v.GetVersion().GetId(),
		Channel:        controlv1.ModelChannel_MODEL_CHANNEL_STABLE,
	})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if resp.GetVersion().GetChannel() != controlv1.ModelChannel_MODEL_CHANNEL_STABLE {
		t.Fatalf("channel: %v", resp.GetVersion().GetChannel())
	}
}

func TestPromote_FaceRequiresFairnessEval(t *testing.T) {
	svc := newSvc(t)
	m, _ := svc.CreateModel(context.Background(), &controlv1.CreateModelRequest{
		Tenant: &commonv1.TenantRef{Id: "t-1"}, Name: "face", Kind: controlv1.ModelKind_MODEL_KIND_FACE,
	})
	v, _ := svc.CreateModelVersion(context.Background(), &controlv1.CreateModelVersionRequest{
		ModelId: m.GetModel().GetId(), Semver: "0.1.0",
		ArtifactUri: "oci://example/face:v1",
		CostProfiles: []*controlv1.CostProfile{{Verified: true}},
		// no FairnessEvalUri
	})
	_, err := svc.PromoteModelVersion(context.Background(), &controlv1.PromoteModelVersionRequest{
		ModelVersionId: v.GetVersion().GetId(),
		Channel:        controlv1.ModelChannel_MODEL_CHANNEL_STABLE,
	})
	if code := status.Code(err); code != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", code)
	}
}
