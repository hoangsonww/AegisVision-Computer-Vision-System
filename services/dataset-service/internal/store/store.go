// Dataset + DatasetVersion store. Per the architecture doc §22, dataset
// versions are immutable: a "promotion" is a new version, not an update.
// Storage refs live in object storage (S3/MinIO) and are stored here as URIs.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aegisvision/pkg/store/migrate"
)

//go:embed *.sql
var migrationsFS embed.FS

func MigrateUp(ctx context.Context, db *sql.DB) error {
	ms, err := migrate.LoadFS(migrationsFS)
	if err != nil {
		return err
	}
	return migrate.Apply(ctx, db, ms)
}

var ErrNotFound = errors.New("dataset: not found")

type Dataset struct {
	ID, TenantID, Name, Description, Kind string
	CreatedAt, UpdatedAt                  time.Time
}

type Version struct {
	ID, DatasetID, Semver, ArtifactURI string
	ItemCount                          int64
	Immutable                          bool
	CreatedAt                          time.Time
}

type Store struct {
	DB         *sql.DB
	IsPostgres bool
}

func New(db *sql.DB, isPostgres bool) *Store { return &Store{DB: db, IsPostgres: isPostgres} }

func (s *Store) CreateDataset(ctx context.Context, d Dataset) error {
	now := time.Now().UTC()
	q := s.ph(`INSERT INTO datasets(id, tenant_id, name, description, kind, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q, d.ID, d.TenantID, d.Name, d.Description, d.Kind, now, now)
	return err
}

func (s *Store) GetDataset(ctx context.Context, id string) (*Dataset, error) {
	q := s.ph(`SELECT id, tenant_id, name, description, kind, created_at, updated_at FROM datasets WHERE id = ?`)
	row := s.DB.QueryRowContext(ctx, q, id)
	var d Dataset
	err := row.Scan(&d.ID, &d.TenantID, &d.Name, &d.Description, &d.Kind, &d.CreatedAt, &d.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &d, err
}

func (s *Store) ListDatasets(ctx context.Context, tenant string) ([]Dataset, error) {
	q := s.ph(`SELECT id, tenant_id, name, description, kind, created_at, updated_at FROM datasets WHERE tenant_id = ? ORDER BY name`)
	rows, err := s.DB.QueryContext(ctx, q, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dataset
	for rows.Next() {
		var d Dataset
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Name, &d.Description, &d.Kind, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) CreateVersion(ctx context.Context, v Version) error {
	now := time.Now().UTC()
	imm := 1
	if !v.Immutable {
		imm = 0
	}
	q := s.ph(`INSERT INTO dataset_versions(id, dataset_id, semver, item_count, artifact_uri, immutable, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q, v.ID, v.DatasetID, v.Semver, v.ItemCount, v.ArtifactURI, imm, now)
	return err
}

func (s *Store) ListVersions(ctx context.Context, dataset string) ([]Version, error) {
	q := s.ph(`SELECT id, dataset_id, semver, item_count, artifact_uri, immutable, created_at
        FROM dataset_versions WHERE dataset_id = ? ORDER BY created_at DESC`)
	rows, err := s.DB.QueryContext(ctx, q, dataset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Version
	for rows.Next() {
		var v Version
		var imm int
		if err := rows.Scan(&v.ID, &v.DatasetID, &v.Semver, &v.ItemCount, &v.ArtifactURI, &imm, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.Immutable = imm != 0
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ph(q string) string {
	if !s.IsPostgres {
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
