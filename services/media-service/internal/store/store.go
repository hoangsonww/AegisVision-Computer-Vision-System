// Media reference store. Per the architecture doc's Matroid lessons, media
// retention is per-stream + shortest-wins, and any media promoted to a
// training dataset must be copy-on-promote so retention drops don't take
// the training data with it. The Promote method enforces that.
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

var ErrNotFound = errors.New("media: not found")

type Media struct {
	ID, TenantID, StreamID, Kind, ArtifactURI, MIMEType string
	Bytes                                               int64
	Width, Height                                       int
	DurationMS                                          int64
	RetentionAt                                         sql.NullTime
	Promoted                                            bool
	CreatedAt                                           time.Time
}

type Store struct {
	DB         *sql.DB
	IsPostgres bool
}

func New(db *sql.DB, isPostgres bool) *Store { return &Store{DB: db, IsPostgres: isPostgres} }

func (s *Store) Create(ctx context.Context, m Media) error {
	now := time.Now().UTC()
	promoted := 0
	if m.Promoted {
		promoted = 1
	}
	q := s.ph(`INSERT INTO media(id, tenant_id, stream_id, kind, artifact_uri, mime_type,
        bytes, width, height, duration_ms, retention_at, promoted, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q,
		m.ID, m.TenantID, m.StreamID, m.Kind, m.ArtifactURI, m.MIMEType,
		m.Bytes, m.Width, m.Height, m.DurationMS, m.RetentionAt, promoted, now)
	return err
}

func (s *Store) Get(ctx context.Context, id string) (*Media, error) {
	q := s.ph(`SELECT id, tenant_id, stream_id, kind, artifact_uri, mime_type, bytes, width, height,
        duration_ms, retention_at, promoted, created_at FROM media WHERE id = ?`)
	row := s.DB.QueryRowContext(ctx, q, id)
	var m Media
	var promoted int
	err := row.Scan(&m.ID, &m.TenantID, &m.StreamID, &m.Kind, &m.ArtifactURI, &m.MIMEType,
		&m.Bytes, &m.Width, &m.Height, &m.DurationMS, &m.RetentionAt, &promoted, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.Promoted = promoted != 0
	return &m, nil
}

// Promote marks a media item as copy-on-promote. The application layer is
// responsible for actually copying the underlying object — Promote just
// records the intent so retention deletes skip this row.
func (s *Store) Promote(ctx context.Context, id, newURI string) error {
	q := s.ph(`UPDATE media SET promoted = 1, artifact_uri = ?, retention_at = NULL WHERE id = ?`)
	res, err := s.DB.ExecContext(ctx, q, newURI, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DueForRetention returns media past their retention_at AND not promoted.
func (s *Store) DueForRetention(ctx context.Context, before time.Time, limit int) ([]Media, error) {
	q := s.ph(`SELECT id, tenant_id, stream_id, kind, artifact_uri, mime_type, bytes, width, height,
        duration_ms, retention_at, promoted, created_at FROM media
        WHERE promoted = 0 AND retention_at IS NOT NULL AND retention_at <= ? LIMIT ?`)
	rows, err := s.DB.QueryContext(ctx, q, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Media
	for rows.Next() {
		var m Media
		var promoted int
		if err := rows.Scan(&m.ID, &m.TenantID, &m.StreamID, &m.Kind, &m.ArtifactURI, &m.MIMEType,
			&m.Bytes, &m.Width, &m.Height, &m.DurationMS, &m.RetentionAt, &promoted, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Promoted = promoted != 0
		out = append(out, m)
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
