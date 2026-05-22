package store_test

import (
	"context"
	"testing"

	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/store/sqldb"
	"github.com/aegisvision/services/autonomy-orchestrator/internal/store"
)

func TestStore_ScheduleAndRunRoundtrip(t *testing.T) {
	db, _ := sqldb.Open(context.Background(), sqldb.Config{Driver: sqldb.DriverSQLite, DSN: ":memory:"})
	defer db.Close()
	if err := store.MigrateUp(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	s := store.New(db, false)
	sc := intelligence.Schedule{
		ID: "sch-1", TenantID: "t-1",
		Role: intelligence.AutonomyRoleMonitor, Trigger: intelligence.TriggerCron,
		CronExpression: "0 */5 * * * *", MaxSteps: 6, MaxAutonomyTier: 2,
	}
	if err := s.UpsertSchedule(context.Background(), sc); err != nil {
		t.Fatal(err)
	}
	all, _ := s.ListSchedules(context.Background())
	if len(all) != 1 {
		t.Fatalf("schedules: %d", len(all))
	}
	r := intelligence.AutonomyRun{
		ID: "ar-1", ScheduleID: "sch-1", TenantID: "t-1",
		Role: intelligence.AutonomyRoleMonitor, Trigger: intelligence.TriggerCron,
		Status: intelligence.RunStatusRunning, TriggerReason: "cron tick",
	}
	if err := s.InsertRun(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRunStatus(context.Background(), "ar-1", intelligence.RunStatusAwaitingGate, "sess-1", "gate-1"); err != nil {
		t.Fatal(err)
	}
	runs, _ := s.ListRuns(context.Background(), "t-1", 10)
	if len(runs) != 1 || runs[0].Status != intelligence.RunStatusAwaitingGate || runs[0].GateID != "gate-1" {
		t.Fatalf("runs: %+v", runs)
	}
}
