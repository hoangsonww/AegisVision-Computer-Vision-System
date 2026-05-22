package evidence_test

import (
	"context"
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/aegisvision/services/compliance-evidence-service/internal/evidence"
)

type stubCollector struct {
	name     string
	controls []evidence.Control
	recs     []evidence.Record
}

func (s stubCollector) Name() string                    { return s.name }
func (s stubCollector) Controls() []evidence.Control    { return s.controls }
func (s stubCollector) Collect(_ context.Context, _ evidence.Request) ([]evidence.Record, error) {
	return s.recs, nil
}

func TestRequest_ValidateDefaults(t *testing.T) {
	r := evidence.Request{TenantID: "t-1"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.To.IsZero() || r.From.IsZero() {
		t.Fatal("expected defaults")
	}
	if !r.From.Before(r.To) {
		t.Fatal("from should be before to")
	}
	if r.Limit != 1000 {
		t.Fatalf("limit: %d", r.Limit)
	}
}

func TestRequest_RequiresTenant(t *testing.T) {
	r := evidence.Request{}
	if err := r.Validate(); err == nil {
		t.Fatal("expected tenant_id required")
	}
}

func TestService_MergesAndSortsDescending(t *testing.T) {
	now := time.Now().UTC()
	a := stubCollector{
		name: "a", controls: []evidence.Control{evidence.CC6_1},
		recs: []evidence.Record{
			{Control: evidence.CC6_1, TenantID: "t-1", OccurredAt: now.Add(-1 * time.Hour), Reference: "r1"},
		},
	}
	b := stubCollector{
		name: "b", controls: []evidence.Control{evidence.CC6_6},
		recs: []evidence.Record{
			{Control: evidence.CC6_6, TenantID: "t-1", OccurredAt: now.Add(-2 * time.Hour), Reference: "r2"},
			{Control: evidence.CC6_6, TenantID: "t-1", OccurredAt: now,                       Reference: "r3"},
		},
	}
	svc := evidence.New(a, b)
	out, err := svc.Collect(context.Background(), evidence.Request{
		TenantID: "t-1",
		From:     now.Add(-3 * time.Hour),
		To:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 records, got %d", len(out))
	}
	// Descending by OccurredAt — r3 first, then r1, then r2.
	if out[0].Reference != "r3" || out[1].Reference != "r1" || out[2].Reference != "r2" {
		t.Fatalf("unexpected order: %+v", out)
	}
}

func TestService_FiltersByControl(t *testing.T) {
	now := time.Now().UTC()
	a := stubCollector{
		name: "a", controls: []evidence.Control{evidence.CC6_1},
		recs: []evidence.Record{{Control: evidence.CC6_1, TenantID: "t-1", OccurredAt: now}},
	}
	b := stubCollector{
		name: "b", controls: []evidence.Control{evidence.CC6_6},
		recs: []evidence.Record{{Control: evidence.CC6_6, TenantID: "t-1", OccurredAt: now}},
	}
	svc := evidence.New(a, b)
	out, _ := svc.Collect(context.Background(), evidence.Request{
		TenantID: "t-1", Control: evidence.CC6_6,
	})
	if len(out) != 1 || out[0].Control != evidence.CC6_6 {
		t.Fatalf("filter broke: %+v", out)
	}
}

func TestService_ClampsLimit(t *testing.T) {
	now := time.Now().UTC()
	many := make([]evidence.Record, 100)
	for i := range many {
		many[i] = evidence.Record{Control: evidence.CC7_3, TenantID: "t-1", OccurredAt: now.Add(time.Duration(-i) * time.Second)}
	}
	a := stubCollector{name: "a", controls: []evidence.Control{evidence.CC7_3}, recs: many}
	svc := evidence.New(a)
	out, _ := svc.Collect(context.Background(), evidence.Request{
		TenantID: "t-1", Limit: 10,
	})
	if len(out) != 10 {
		t.Fatalf("expected 10, got %d", len(out))
	}
}

func TestWriteCSV_HeaderAndRows(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	recs := []evidence.Record{
		{Control: evidence.CC6_1, TenantID: "t-1", OccurredAt: now, Source: "audit",
			Summary: "GET /v1/x → 200", Reference: "r-1", Outcome: "ok"},
	}
	var buf bytes.Buffer
	if err := evidence.WriteCSV(&buf, recs); err != nil {
		t.Fatal(err)
	}
	cr := csv.NewReader(strings.NewReader(buf.String()))
	rows, err := cr.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0][0] != "control" || rows[1][0] != "CC6.1" {
		t.Fatalf("csv: %+v", rows)
	}
}

func TestWriteJSON_LinesPerRecord(t *testing.T) {
	now := time.Now().UTC()
	recs := []evidence.Record{
		{Control: evidence.CC6_1, TenantID: "t-1", OccurredAt: now, Reference: "r1"},
		{Control: evidence.CC6_6, TenantID: "t-1", OccurredAt: now, Reference: "r2"},
	}
	var buf bytes.Buffer
	if err := evidence.WriteJSON(&buf, recs); err != nil {
		t.Fatal(err)
	}
	lines := strings.Count(buf.String(), "\n")
	if lines != 2 {
		t.Fatalf("expected 2 jsonl lines, got %d", lines)
	}
}
