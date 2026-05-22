// Annotation store. Each annotation is a (dataset, item, label, bbox)
// with a review workflow. Reviewers move pending → approved/rejected.
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

var ErrNotFound = errors.New("annotation: not found")

const (
	ReviewPending  = "pending"
	ReviewApproved = "approved"
	ReviewRejected = "rejected"
)

type Annotation struct {
	ID, TenantID, DatasetID, ItemID, Annotator, Label string
	BBoxX, BBoxY, BBoxW, BBoxH                        float32
	ReviewStatus, Reviewer                            string
	CreatedAt, UpdatedAt                              time.Time
}

type Store struct {
	DB         *sql.DB
	IsPostgres bool
}

func New(db *sql.DB, isPostgres bool) *Store { return &Store{DB: db, IsPostgres: isPostgres} }

func (s *Store) Create(ctx context.Context, a Annotation) error {
	now := time.Now().UTC()
	if a.ReviewStatus == "" {
		a.ReviewStatus = ReviewPending
	}
	q := s.ph(`INSERT INTO annotations(id, tenant_id, dataset_id, item_id, annotator,
        label, bbox_x, bbox_y, bbox_w, bbox_h, review_status, reviewer, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.DB.ExecContext(ctx, q,
		a.ID, a.TenantID, a.DatasetID, a.ItemID, a.Annotator,
		a.Label, a.BBoxX, a.BBoxY, a.BBoxW, a.BBoxH,
		a.ReviewStatus, a.Reviewer, now, now)
	return err
}

func (s *Store) Review(ctx context.Context, id, status, reviewer string) error {
	if status != ReviewApproved && status != ReviewRejected {
		return errors.New("annotation: invalid review status")
	}
	now := time.Now().UTC()
	q := s.ph(`UPDATE annotations SET review_status = ?, reviewer = ?, updated_at = ? WHERE id = ?`)
	res, err := s.DB.ExecContext(ctx, q, status, reviewer, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListByItem(ctx context.Context, dataset, item string) ([]Annotation, error) {
	q := s.ph(`SELECT id, tenant_id, dataset_id, item_id, annotator, label,
        bbox_x, bbox_y, bbox_w, bbox_h, review_status, reviewer, created_at, updated_at
        FROM annotations WHERE dataset_id = ? AND item_id = ?`)
	rows, err := s.DB.QueryContext(ctx, q, dataset, item)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Annotation
	for rows.Next() {
		var a Annotation
		if err := rows.Scan(&a.ID, &a.TenantID, &a.DatasetID, &a.ItemID, &a.Annotator, &a.Label,
			&a.BBoxX, &a.BBoxY, &a.BBoxW, &a.BBoxH, &a.ReviewStatus, &a.Reviewer, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
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
