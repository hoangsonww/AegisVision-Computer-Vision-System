// SQL-backed persistent store for events.v1. The in-memory Ring stays as
// the live-fanout layer; this store gives us the durable read path the
// production console + audit tooling query.
//
// Schema portability: we ship one base migration for SQLite/Postgres and a
// parallel ClickHouse DDL that uses MergeTree + TTL. The persistent path
// auto-applies the right one based on the configured driver.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/aegisvision/pkg/store/migrate"
)

//go:embed *.sql
var migrationsFS embed.FS

// clickhouseDDL is the production schema used when the driver is ClickHouse.
// We keep it inline so we don't need a separate FS — the application can
// switch dialects without touching the file layout.
const clickhouseDDL = `
CREATE TABLE IF NOT EXISTS events (
    id            String,
    tenant_id     String,
    project_id    String,
    pipeline_id   String,
    stream_id     String,
    kind          UInt8,
    severity      UInt8,
    frame_seq     Int64,
    capture_time  DateTime64(9),
    emitted_at    DateTime64(9),
    summary       String,
    rule_id       String,
    zone_id       String,
    track_id      String,
    class_label   String,
    score         Float32,
    bbox_x        Float32,
    bbox_y        Float32,
    bbox_w        Float32,
    bbox_h        Float32,
    traceparent   String
) ENGINE = MergeTree()
PARTITION BY (tenant_id, toDate(emitted_at))
ORDER BY (tenant_id, stream_id, emitted_at, id)
TTL emitted_at + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;
`

// MigratePersistent applies the right DDL for the active driver. Call once
// on event-service startup before serving traffic.
func MigratePersistent(ctx context.Context, db *sql.DB, isClickHouse bool) error {
	if isClickHouse {
		_, err := db.ExecContext(ctx, clickhouseDDL)
		return err
	}
	ms, err := migrate.LoadFS(migrationsFS)
	if err != nil {
		return err
	}
	return migrate.Apply(ctx, db, ms)
}

// PersistentStore is a sql.DB-backed write+read interface for events.v1.
type PersistentStore struct {
	DB           *sql.DB
	IsPostgres   bool
	IsClickHouse bool
}

// NewPersistent constructs a PersistentStore.
func NewPersistent(db *sql.DB, isPostgres, isClickHouse bool) *PersistentStore {
	return &PersistentStore{DB: db, IsPostgres: isPostgres, IsClickHouse: isClickHouse}
}

// Append persists one event. Append failures are logged by the caller but do
// not block the live SSE fan-out.
func (s *PersistentStore) Append(ctx context.Context, ev *dataplanev1.Event) error {
	q := s.ph(`INSERT INTO events
      (id, tenant_id, project_id, pipeline_id, stream_id, kind, severity,
       frame_seq, capture_time, emitted_at, summary, rule_id, zone_id,
       track_id, class_label, score, bbox_x, bbox_y, bbox_w, bbox_h,
       traceparent)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q,
		ev.GetId(), ev.GetTenantId(), ev.GetProjectId(), ev.GetPipelineId(),
		ev.GetStreamId(), int(ev.GetKind()), int(ev.GetSeverity()),
		ev.GetFrameSeq(),
		toTime(ev.GetCaptureTime()), toTime(ev.GetEmittedAt()),
		ev.GetSummary(), ev.GetRuleId(), ev.GetZoneId(),
		ev.GetTrackId(), ev.GetClassLabel(), ev.GetScore(),
		ev.GetBboxX(), ev.GetBboxY(), ev.GetBboxW(), ev.GetBboxH(),
		ev.GetTraceparent())
	return err
}

// List returns the most recent events matching the filter. SQL ORDER BY
// emitted_at DESC is fast both on B-tree (Postgres) and the MergeTree
// primary key (ClickHouse).
func (s *PersistentStore) List(ctx context.Context, tenantID, streamID string, limit int) ([]*dataplanev1.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	if tenantID == "" {
		return nil, errors.New("events: tenant required")
	}
	var sb strings.Builder
	sb.WriteString(`SELECT id, tenant_id, project_id, pipeline_id, stream_id,
        kind, severity, frame_seq, capture_time, emitted_at, summary,
        rule_id, zone_id, track_id, class_label, score, bbox_x, bbox_y,
        bbox_w, bbox_h, traceparent FROM events WHERE tenant_id = ?`)
	args := []any{tenantID}
	if streamID != "" {
		sb.WriteString(` AND stream_id = ?`)
		args = append(args, streamID)
	}
	sb.WriteString(` ORDER BY emitted_at DESC LIMIT ?`)
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, s.ph(sb.String()), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*dataplanev1.Event
	for rows.Next() {
		ev, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func scanEventRow(rows *sql.Rows) (*dataplanev1.Event, error) {
	var (
		id, tenant, project, pipeline, stream, summary, ruleID, zoneID,
		trackID, cls, traceparent string
		kind, sev                  int
		frameSeq                   int64
		captureT, emittedAt        time.Time
		score, bx, by, bw, bh      float32
	)
	if err := rows.Scan(&id, &tenant, &project, &pipeline, &stream,
		&kind, &sev, &frameSeq, &captureT, &emittedAt, &summary,
		&ruleID, &zoneID, &trackID, &cls, &score, &bx, &by, &bw, &bh,
		&traceparent); err != nil {
		return nil, err
	}
	return &dataplanev1.Event{
		Id: id, TenantId: tenant, ProjectId: project,
		PipelineId: pipeline, StreamId: stream,
		Kind:        dataplanev1.EventKind(kind),
		Severity:    commonv1.Severity(sev),
		FrameSeq:    frameSeq,
		CaptureTime: timestamppb.New(captureT),
		EmittedAt:   timestamppb.New(emittedAt),
		Summary:     summary, RuleId: ruleID, ZoneId: zoneID,
		TrackId: trackID, ClassLabel: cls,
		Score: score, BboxX: bx, BboxY: by, BboxW: bw, BboxH: bh,
		Traceparent: traceparent,
	}, nil
}

func toTime(t interface{ AsTime() time.Time }) time.Time {
	if t == nil {
		return time.Now().UTC()
	}
	return t.AsTime().UTC()
}

// ph renders dialect placeholders. Both ClickHouse and Postgres expect $N;
// SQLite uses ?.
func (s *PersistentStore) ph(q string) string {
	if !s.IsPostgres && !s.IsClickHouse {
		return q
	}
	var out strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			fmt.Fprintf(&out, "$%d", n)
		} else {
			out.WriteByte(q[i])
		}
	}
	return out.String()
}
