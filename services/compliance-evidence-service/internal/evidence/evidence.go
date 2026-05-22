// Package evidence collects audit-relevant records from the platform's
// authoritative stores into a unified per-control evidence report.
//
// Per ADR-0029, this service is the read-only window an auditor consults
// during a SOC 2 / EU AI Act / ISO 27001 review. It doesn't own data —
// it composes queries against:
//
//   - audit-service          (CC6, CC7 — access decisions, gate decisions)
//   - drift-detection-service (CC7.4 — ML monitoring evidence)
//   - slo-watchdog           (CC7.5 — SLO burn evidence)
//   - policy-gate-service    (CC6.6 — consequential-action approvals)
//
// All queries are tenant-scoped at the gateway. The evidence package
// exposes a tiny `Collector` interface so tests can stub the upstream
// services without standing up the whole platform.
package evidence

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// Control is the SOC 2 / NIST 800-53 control identifier this row supports.
type Control string

// Common Trust Service Criteria identifiers used across the platform.
const (
	CC6_1 Control = "CC6.1" // logical-access controls
	CC6_6 Control = "CC6.6" // privileged-action approval (policy-gate)
	CC7_2 Control = "CC7.2" // monitoring of system performance / SLOs
	CC7_3 Control = "CC7.3" // security event detection (audit)
	CC7_4 Control = "CC7.4" // incident response (drift, slo signals)
)

// Record is one evidence row.
type Record struct {
	Control     Control   `json:"control"`
	TenantID    string    `json:"tenant_id"`
	OccurredAt  time.Time `json:"occurred_at"`
	Source      string    `json:"source"`
	Summary     string    `json:"summary"`
	Reference   string    `json:"reference"` // upstream object id
	Outcome     string    `json:"outcome,omitempty"`
	Severity    string    `json:"severity,omitempty"`
	ActorSubject string   `json:"actor_subject,omitempty"`
}

// Collector queries one upstream system and returns records for the
// (tenant, control, time-range) tuple.
type Collector interface {
	Name() string
	// Controls this collector emits. Used by the service to short-circuit
	// when the request asks for a control this collector doesn't cover.
	Controls() []Control
	Collect(ctx context.Context, req Request) ([]Record, error)
}

// Request is the user-facing query.
type Request struct {
	TenantID string
	Control  Control   // optional — empty = all
	From     time.Time
	To       time.Time
	Limit    int
}

// Validate normalizes + sanity-checks. From defaults to 30d ago; To
// defaults to now; Limit clamps to [1, 10000].
func (r *Request) Validate() error {
	if r.TenantID == "" {
		return errors.New("tenant_id required")
	}
	if r.To.IsZero() {
		r.To = time.Now().UTC()
	}
	if r.From.IsZero() {
		r.From = r.To.Add(-30 * 24 * time.Hour)
	}
	if !r.From.Before(r.To) {
		return errors.New("from must be < to")
	}
	if r.Limit <= 0 {
		r.Limit = 1000
	}
	if r.Limit > 10000 {
		r.Limit = 10000
	}
	return nil
}

// Service composes multiple Collectors into one evidence report.
type Service struct {
	Collectors []Collector
}

// New constructs a Service with the provided collectors.
func New(cs ...Collector) *Service {
	return &Service{Collectors: cs}
}

// Collect dispatches the request to every collector that covers the
// requested control (or all collectors if Control is empty), merges +
// sorts by OccurredAt desc, and trims to Limit.
func (s *Service) Collect(ctx context.Context, req Request) ([]Record, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var all []Record
	var errs []string
	for _, c := range s.Collectors {
		if req.Control != "" && !covers(c, req.Control) {
			continue
		}
		recs, err := c.Collect(ctx, req)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", c.Name(), err))
			continue
		}
		all = append(all, recs...)
	}
	if len(all) == 0 && len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].OccurredAt.After(all[j].OccurredAt)
	})
	if len(all) > req.Limit {
		all = all[:req.Limit]
	}
	return all, nil
}

// WriteCSV serializes records as CSV — the auditor's preferred format.
// Header line first; one record per row.
func WriteCSV(w io.Writer, recs []Record) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"control", "tenant_id", "occurred_at", "source",
		"summary", "reference", "outcome", "severity", "actor_subject",
	}); err != nil {
		return err
	}
	for _, r := range recs {
		if err := cw.Write([]string{
			string(r.Control), r.TenantID,
			r.OccurredAt.UTC().Format(time.RFC3339),
			r.Source, r.Summary, r.Reference,
			r.Outcome, r.Severity, r.ActorSubject,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteJSON serializes records as JSON-Lines — one object per line, so
// auditors can stream very large reports without loading them into memory.
func WriteJSON(w io.Writer, recs []Record) error {
	enc := json.NewEncoder(w)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

func covers(c Collector, control Control) bool {
	for _, k := range c.Controls() {
		if k == control {
			return true
		}
	}
	return false
}
