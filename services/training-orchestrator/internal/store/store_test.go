package store_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/training-orchestrator/internal/store"
)

func TestJob_Lifecycle(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	_ = store.MigrateUp(context.Background(), db)
	s := store.New(db, false)
	_ = s.Submit(context.Background(), store.Job{
		ID: "j-1", TenantID: "t-1", Name: "yolo-finetune",
		BaseModel: "yolo11n", DatasetID: "ds-1", DatasetVer: "v-1",
		EpochsTotal: 50,
	})
	_ = s.Update(context.Background(), "j-1", store.StateRunning, 5, `{"loss":0.42}`, "", "")
	_ = s.Update(context.Background(), "j-1", store.StateSucceeded, 50, `{"map50":0.78}`, "oci://reg/yolo:ft-1", "")
	j, err := s.Get(context.Background(), "j-1")
	if err != nil {
		t.Fatal(err)
	}
	if j.State != store.StateSucceeded || !j.FinishedAt.Valid {
		t.Fatalf("state/finished: %+v", j)
	}
}
