// SQL-backed model registry store. Same shape as the pipeline-service /
// stream-manager stores; production = Postgres, tests = SQLite.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aegisvision/pkg/store/migrate"
	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var ErrNotFound = errors.New("model: not found")

//go:embed *.sql
var migrationsFS embed.FS

func MigrateUp(ctx context.Context, db *sql.DB) error {
	ms, err := migrate.LoadFS(migrationsFS)
	if err != nil {
		return err
	}
	return migrate.Apply(ctx, db, ms)
}

type SQLStore struct {
	DB       *sql.DB
	Postgres bool
}

func New(db *sql.DB, postgres bool) *SQLStore { return &SQLStore{DB: db, Postgres: postgres} }

func (s *SQLStore) CreateModel(m *controlv1.Model) error {
	labels, _ := json.Marshal(m.GetLabels())
	now := time.Now().UTC()
	if m.Audit == nil {
		m.Audit = &commonv1.AuditFields{}
	}
	m.Audit.CreatedAt = timestamppb.New(now)
	m.Audit.UpdatedAt = timestamppb.New(now)
	m.Audit.Version = 1
	q := s.ph(`INSERT INTO models(id, tenant_id, name, kind, family, labels_json, version, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)`)
	_, err := s.DB.ExecContext(context.Background(), q,
		m.GetId(), m.GetTenant().GetId(), m.GetName(),
		int(m.GetKind()), m.GetFamily(), string(labels), now, now)
	return err
}

func (s *SQLStore) GetModel(id string) (*controlv1.Model, error) {
	q := s.ph(`SELECT id, tenant_id, name, kind, family, labels_json, version, created_at, updated_at
        FROM models WHERE id = ?`)
	row := s.DB.QueryRowContext(context.Background(), q, id)
	return s.scanModel(row)
}

func (s *SQLStore) ListModels(tenantID string, kind controlv1.ModelKind, pageSize int, after string) ([]*controlv1.Model, string, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	var sb strings.Builder
	sb.WriteString(`SELECT id, tenant_id, name, kind, family, labels_json, version, created_at, updated_at
        FROM models WHERE tenant_id = ?`)
	args := []any{tenantID}
	if kind != controlv1.ModelKind_MODEL_KIND_UNSPECIFIED {
		sb.WriteString(` AND kind = ?`)
		args = append(args, int(kind))
	}
	if after != "" {
		sb.WriteString(` AND id > ?`)
		args = append(args, after)
	}
	sb.WriteString(` ORDER BY id ASC LIMIT ?`)
	args = append(args, pageSize+1)
	rows, err := s.DB.QueryContext(context.Background(), s.ph(sb.String()), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []*controlv1.Model{}
	for rows.Next() {
		m, err := s.scanModelRow(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > pageSize {
		out = out[:pageSize]
		next = out[len(out)-1].GetId()
	}
	return out, next, nil
}

func (s *SQLStore) CreateVersion(v *controlv1.ModelVersion) error {
	// Default channel to CANARY on first create; STABLE requires Promote.
	if v.Channel == controlv1.ModelChannel_MODEL_CHANNEL_UNSPECIFIED {
		v.Channel = controlv1.ModelChannel_MODEL_CHANNEL_CANARY
	}
	cp := []byte("[]")
	if len(v.GetCostProfiles()) > 0 {
		// Serialize protobuf cost profiles as JSON for portability.
		type wire struct {
			GPUSKU         string  `json:"gpu_sku"`
			MIGProfile     string  `json:"mig_profile"`
			Precision      string  `json:"precision"`
			InputHeight    int32   `json:"input_height"`
			InputWidth     int32   `json:"input_width"`
			Batch          int32   `json:"batch"`
			VRAMBytes      int64   `json:"vram_bytes"`
			InferenceMs    float64 `json:"inference_ms"`
			InferenceMsP95 float64 `json:"inference_ms_p95"`
			Watts          float64 `json:"watts"`
			Verified       bool    `json:"verified"`
		}
		items := make([]wire, 0, len(v.GetCostProfiles()))
		for _, p := range v.GetCostProfiles() {
			items = append(items, wire{
				GPUSKU: p.GetGpuSku(), MIGProfile: p.GetMigProfile(),
				Precision: p.GetPrecision(), InputHeight: p.GetInputHeight(),
				InputWidth: p.GetInputWidth(), Batch: p.GetBatch(),
				VRAMBytes:      p.GetVramBytes(),
				InferenceMs:    p.GetInferenceMs(),
				InferenceMsP95: p.GetInferenceMsP95(),
				Watts:          p.GetWatts(), Verified: p.GetVerified(),
			})
		}
		cp, _ = json.Marshal(items)
	}
	now := time.Now().UTC()
	if v.Audit == nil {
		v.Audit = &commonv1.AuditFields{}
	}
	v.Audit.CreatedAt = timestamppb.New(now)
	v.Audit.UpdatedAt = timestamppb.New(now)
	v.Audit.Version = 1
	q := s.ph(`INSERT INTO model_versions(id, model_id, semver, channel, artifact_uri, signature, sbom_uri, fairness_eval_uri, cost_profiles_json, version, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`)
	_, err := s.DB.ExecContext(context.Background(), q,
		v.GetId(), v.GetModelId(), v.GetSemver(), int(v.Channel),
		v.GetArtifactUri(), v.GetSignature(), v.GetSbomUri(),
		v.GetFairnessEvalUri(), string(cp), now, now)
	return err
}

// PromoteVersion enforces promotion gates: STABLE requires a verified
// CostProfile, and FACE/PERSON-class models require a fairness eval URI.
func (s *SQLStore) PromoteVersion(versionID string, ch controlv1.ModelChannel) (*controlv1.ModelVersion, error) {
	v, modelKind, err := s.getVersionAndKind(versionID)
	if err != nil {
		return nil, err
	}

	if ch == controlv1.ModelChannel_MODEL_CHANNEL_STABLE {
		verified := false
		for _, p := range v.GetCostProfiles() {
			if p.GetVerified() {
				verified = true
				break
			}
		}
		if !verified {
			return nil, errors.New("promote: STABLE requires at least one verified CostProfile (ADR-0003 + production lesson)")
		}
		if modelKind == controlv1.ModelKind_MODEL_KIND_FACE && v.GetFairnessEvalUri() == "" {
			return nil, errors.New("promote: face/person models require fairness_eval_uri (EU AI Act release gate)")
		}
	}

	now := time.Now().UTC()
	q := s.ph(`UPDATE model_versions SET channel=?, version=version+1, updated_at=? WHERE id=?`)
	res, err := s.DB.ExecContext(context.Background(), q, int(ch), now, versionID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	v.Channel = ch
	v.Audit.UpdatedAt = timestamppb.New(now)
	v.Audit.Version++
	return v, nil
}

func (s *SQLStore) getVersionAndKind(versionID string) (*controlv1.ModelVersion, controlv1.ModelKind, error) {
	q := s.ph(`SELECT mv.id, mv.model_id, mv.semver, mv.channel, mv.artifact_uri, mv.signature,
            mv.sbom_uri, mv.fairness_eval_uri, mv.cost_profiles_json, mv.version,
            mv.created_at, mv.updated_at, m.kind
        FROM model_versions mv JOIN models m ON m.id = mv.model_id
        WHERE mv.id = ?`)
	row := s.DB.QueryRowContext(context.Background(), q, versionID)
	var (
		id, modelID, semver, artifact, sig, sbom, fair, cpJSON string
		channel, kind                                          int
		version                                                int64
		createdAt, updatedAt                                   time.Time
	)
	err := row.Scan(&id, &modelID, &semver, &channel, &artifact, &sig, &sbom, &fair, &cpJSON, &version, &createdAt, &updatedAt, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	v := &controlv1.ModelVersion{
		Id: id, ModelId: modelID, Semver: semver,
		Channel:         controlv1.ModelChannel(channel),
		ArtifactUri:     artifact,
		Signature:       sig,
		SbomUri:         sbom,
		FairnessEvalUri: fair,
		Audit: &commonv1.AuditFields{
			CreatedAt: timestamppb.New(createdAt),
			UpdatedAt: timestamppb.New(updatedAt),
			Version:   version,
		},
	}
	// Decode cost profiles for the promotion gate.
	if cpJSON != "" && cpJSON != "[]" {
		var items []struct {
			GPUSKU         string  `json:"gpu_sku"`
			MIGProfile     string  `json:"mig_profile"`
			Precision      string  `json:"precision"`
			InputHeight    int32   `json:"input_height"`
			InputWidth     int32   `json:"input_width"`
			Batch          int32   `json:"batch"`
			VRAMBytes      int64   `json:"vram_bytes"`
			InferenceMs    float64 `json:"inference_ms"`
			InferenceMsP95 float64 `json:"inference_ms_p95"`
			Watts          float64 `json:"watts"`
			Verified       bool    `json:"verified"`
		}
		_ = json.Unmarshal([]byte(cpJSON), &items)
		for _, it := range items {
			v.CostProfiles = append(v.CostProfiles, &controlv1.CostProfile{
				GpuSku: it.GPUSKU, MigProfile: it.MIGProfile,
				Precision: it.Precision, InputHeight: it.InputHeight,
				InputWidth: it.InputWidth, Batch: it.Batch,
				VramBytes: it.VRAMBytes, InferenceMs: it.InferenceMs,
				InferenceMsP95: it.InferenceMsP95, Watts: it.Watts,
				Verified: it.Verified,
			})
		}
	}
	return v, controlv1.ModelKind(kind), nil
}

func (s *SQLStore) scanModel(row *sql.Row) (*controlv1.Model, error) {
	var (
		id, tenantID, name, family, labelsJSON string
		kind                                   int
		version                                int64
		createdAt, updatedAt                   time.Time
	)
	err := row.Scan(&id, &tenantID, &name, &kind, &family, &labelsJSON, &version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return assembleModel(id, tenantID, name, kind, family, labelsJSON, version, createdAt, updatedAt), nil
}

func (s *SQLStore) scanModelRow(rows *sql.Rows) (*controlv1.Model, error) {
	var (
		id, tenantID, name, family, labelsJSON string
		kind                                   int
		version                                int64
		createdAt, updatedAt                   time.Time
	)
	if err := rows.Scan(&id, &tenantID, &name, &kind, &family, &labelsJSON, &version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return assembleModel(id, tenantID, name, kind, family, labelsJSON, version, createdAt, updatedAt), nil
}

func assembleModel(id, tenantID, name string, kind int, family, labelsJSON string, version int64, createdAt, updatedAt time.Time) *controlv1.Model {
	var labels []string
	_ = json.Unmarshal([]byte(labelsJSON), &labels)
	return &controlv1.Model{
		Id: id, Tenant: &commonv1.TenantRef{Id: tenantID},
		Name: name, Kind: controlv1.ModelKind(kind), Family: family,
		Labels: labels,
		Audit: &commonv1.AuditFields{
			CreatedAt: timestamppb.New(createdAt),
			UpdatedAt: timestamppb.New(updatedAt),
			Version:   version,
		},
	}
}

func (s *SQLStore) ph(q string) string {
	if !s.Postgres {
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

// _ keeps proto in the import set even if a future edit drops a reference.
var _ = proto.Marshal
