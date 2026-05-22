// Store: pgvector-backed knowledge corpus.
//
// We deliberately keep the abstraction thin:
//
//   - Postgres path: column `embedding` is `vector(N)` (the pgvector ext);
//     index is ivfflat on the cosine distance op (`<=>`). Migrations create
//     the extension + index lazily; if pgvector isn't installed, store
//     falls back to a plain `text[]` column and brute-force cosine in Go.
//   - SQLite path: brute-force cosine over a BLOB-encoded float32 vector.
//     Fine for tests + tiny dev corpora; obviously not for prod.
//
// The store is per-tenant by row; the column tenant_id can be empty for
// platform-global documents (ADRs, runbooks). Search filters by either
// "tenant_id == ''" (global only) or "tenant_id in {'', '<tenant>'}".
package embeddings

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/aegisvision/pkg/intelligence"
)

// Store is the platform-internal embeddings + docs corpus.
type Store struct {
	DB       *sql.DB
	Backend  string // "postgres" | "sqlite"
	Dim      int    // embedding dim; 0 = auto-detect from first insert
	HavePgVector bool // postgres only; auto-set during Migrate
}

func New(db *sql.DB, backend string) *Store {
	return &Store{DB: db, Backend: backend}
}

// Migrate creates tables + indexes. On Postgres it tries CREATE EXTENSION
// vector; if it fails (permission, not installed), falls back to text[].
func (s *Store) Migrate(ctx context.Context) error {
	if s.Backend == "postgres" {
		return s.migratePG(ctx)
	}
	return s.migrateSQLite(ctx)
}

func (s *Store) migratePG(ctx context.Context) error {
	// Try pgvector. Best-effort.
	if _, err := s.DB.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err == nil {
		s.HavePgVector = true
	}
	if s.HavePgVector {
		dim := s.Dim
		if dim == 0 {
			dim = 1536 // common default; can be overridden by setting s.Dim before Migrate.
		}
		stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS knowledge_docs (
            id TEXT PRIMARY KEY,
            tenant_id TEXT NOT NULL DEFAULT '',
            kind TEXT NOT NULL,
            title TEXT NOT NULL,
            body TEXT NOT NULL,
            source_uri TEXT NOT NULL DEFAULT '',
            tags TEXT[] NOT NULL DEFAULT '{}',
            embedding vector(%d) NOT NULL,
            ingested_at TIMESTAMPTZ NOT NULL
        )`, dim)
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			return err
		}
		// ivfflat needs ANALYZE to be effective; the lists count is tunable.
		_, _ = s.DB.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_kd_tenant ON knowledge_docs(tenant_id)`)
		_, _ = s.DB.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_kd_kind ON knowledge_docs(kind)`)
		_, _ = s.DB.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_kd_embedding ON knowledge_docs USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)`))
		return nil
	}
	// Fallback path: store as float[] (text encoded) — brute force.
	_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS knowledge_docs (
        id TEXT PRIMARY KEY,
        tenant_id TEXT NOT NULL DEFAULT '',
        kind TEXT NOT NULL,
        title TEXT NOT NULL,
        body TEXT NOT NULL,
        source_uri TEXT NOT NULL DEFAULT '',
        tags JSONB NOT NULL DEFAULT '[]'::jsonb,
        embedding BYTEA NOT NULL,
        ingested_at TIMESTAMPTZ NOT NULL
    )`)
	return err
}

func (s *Store) migrateSQLite(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS knowledge_docs (
        id TEXT PRIMARY KEY,
        tenant_id TEXT NOT NULL DEFAULT '',
        kind TEXT NOT NULL,
        title TEXT NOT NULL,
        body TEXT NOT NULL,
        source_uri TEXT NOT NULL DEFAULT '',
        tags TEXT NOT NULL DEFAULT '[]',
        embedding BLOB NOT NULL,
        ingested_at TIMESTAMP NOT NULL
    )`)
	return err
}

// Upsert stores (or replaces) a document and its embedding.
func (s *Store) Upsert(ctx context.Context, doc intelligence.Doc, vector []float32) error {
	if len(vector) == 0 {
		return errors.New("empty embedding")
	}
	if s.Dim == 0 {
		s.Dim = len(vector)
	} else if s.Dim != len(vector) {
		return fmt.Errorf("dim mismatch: store=%d got=%d", s.Dim, len(vector))
	}
	if doc.IngestedAt.IsZero() {
		doc.IngestedAt = time.Now().UTC()
	}
	tagsJSON, _ := json.Marshal(doc.Tags)
	if s.Backend == "postgres" {
		if s.HavePgVector {
			_, err := s.DB.ExecContext(ctx,
				`INSERT INTO knowledge_docs(id, tenant_id, kind, title, body, source_uri, tags, embedding, ingested_at)
                 VALUES ($1,$2,$3,$4,$5,$6,$7,$8::vector,$9)
                 ON CONFLICT (id) DO UPDATE SET
                   tenant_id=EXCLUDED.tenant_id, kind=EXCLUDED.kind, title=EXCLUDED.title,
                   body=EXCLUDED.body, source_uri=EXCLUDED.source_uri, tags=EXCLUDED.tags,
                   embedding=EXCLUDED.embedding, ingested_at=EXCLUDED.ingested_at`,
				doc.ID, doc.TenantID, string(doc.Kind), doc.Title, doc.Body, doc.SourceURI, pqArray(doc.Tags), pgVector(vector), doc.IngestedAt)
			return err
		}
		_, err := s.DB.ExecContext(ctx,
			`INSERT INTO knowledge_docs(id, tenant_id, kind, title, body, source_uri, tags, embedding, ingested_at)
             VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9)
             ON CONFLICT (id) DO UPDATE SET embedding=EXCLUDED.embedding, body=EXCLUDED.body, tags=EXCLUDED.tags`,
			doc.ID, doc.TenantID, string(doc.Kind), doc.Title, doc.Body, doc.SourceURI, string(tagsJSON), encodeBlob(vector), doc.IngestedAt)
		return err
	}
	// SQLite
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO knowledge_docs(id, tenant_id, kind, title, body, source_uri, tags, embedding, ingested_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(id) DO UPDATE SET
           tenant_id=excluded.tenant_id, kind=excluded.kind, title=excluded.title,
           body=excluded.body, source_uri=excluded.source_uri, tags=excluded.tags,
           embedding=excluded.embedding, ingested_at=excluded.ingested_at`,
		doc.ID, doc.TenantID, string(doc.Kind), doc.Title, doc.Body, doc.SourceURI, string(tagsJSON), encodeBlob(vector), doc.IngestedAt)
	return err
}

// Search returns the top-K docs by cosine similarity. tenant filters: if
// non-empty, also include platform-global docs (tenant_id = '').
func (s *Store) Search(ctx context.Context, tenant string, query []float32, k int, kinds []intelligence.DocKind, requireTags []string) ([]intelligence.SearchHit, error) {
	if k <= 0 {
		k = 5
	}
	if s.Backend == "postgres" && s.HavePgVector {
		return s.searchPGVector(ctx, tenant, query, k, kinds, requireTags)
	}
	return s.searchBrute(ctx, tenant, query, k, kinds, requireTags)
}

func (s *Store) searchPGVector(ctx context.Context, tenant string, query []float32, k int, kinds []intelligence.DocKind, requireTags []string) ([]intelligence.SearchHit, error) {
	var where []string
	var args []any
	args = append(args, pgVector(query))
	if tenant != "" {
		where = append(where, "(tenant_id = '' OR tenant_id = $2)")
		args = append(args, tenant)
	} else {
		where = append(where, "tenant_id = ''")
	}
	if len(kinds) > 0 {
		placeholders := make([]string, len(kinds))
		for i, kd := range kinds {
			args = append(args, string(kd))
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		where = append(where, "kind IN ("+strings.Join(placeholders, ",")+")")
	}
	if len(requireTags) > 0 {
		args = append(args, pqArray(requireTags))
		where = append(where, fmt.Sprintf("tags @> $%d", len(args)))
	}
	q := fmt.Sprintf(`SELECT id, tenant_id, kind, title, body, source_uri, tags, ingested_at,
        1 - (embedding <=> $1) AS score
        FROM knowledge_docs WHERE %s
        ORDER BY embedding <=> $1 LIMIT %d`, strings.Join(where, " AND "), k)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligence.SearchHit
	for rows.Next() {
		var h intelligence.SearchHit
		var tagsArr pqArrayRecv
		if err := rows.Scan(&h.Doc.ID, &h.Doc.TenantID, &h.Doc.Kind, &h.Doc.Title, &h.Doc.Body, &h.Doc.SourceURI, &tagsArr, &h.Doc.IngestedAt, &h.Score); err != nil {
			return nil, err
		}
		h.Doc.Tags = tagsArr
		h.Snippet = snippet(h.Doc.Body, 240)
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) searchBrute(ctx context.Context, tenant string, query []float32, k int, kinds []intelligence.DocKind, requireTags []string) ([]intelligence.SearchHit, error) {
	var where []string
	var args []any
	if tenant != "" {
		if s.Backend == "postgres" {
			where = append(where, "(tenant_id = '' OR tenant_id = $1)")
		} else {
			where = append(where, "(tenant_id = '' OR tenant_id = ?)")
		}
		args = append(args, tenant)
	} else {
		where = append(where, "tenant_id = ''")
	}
	if len(kinds) > 0 {
		placeholders := make([]string, len(kinds))
		for i, kd := range kinds {
			args = append(args, string(kd))
			if s.Backend == "postgres" {
				placeholders[i] = fmt.Sprintf("$%d", len(args))
			} else {
				placeholders[i] = "?"
			}
		}
		where = append(where, "kind IN ("+strings.Join(placeholders, ",")+")")
	}
	q := fmt.Sprintf(`SELECT id, tenant_id, kind, title, body, source_uri, tags, embedding, ingested_at FROM knowledge_docs WHERE %s`, strings.Join(where, " AND "))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type candidate struct {
		Hit   intelligence.SearchHit
		Score float64
	}
	var cands []candidate
	for rows.Next() {
		var doc intelligence.Doc
		var blob []byte
		var tagsRaw string
		if err := rows.Scan(&doc.ID, &doc.TenantID, &doc.Kind, &doc.Title, &doc.Body, &doc.SourceURI, &tagsRaw, &blob, &doc.IngestedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsRaw), &doc.Tags)
		if len(requireTags) > 0 && !containsAll(doc.Tags, requireTags) {
			continue
		}
		v := decodeBlob(blob)
		score := cosine(query, v)
		cands = append(cands, candidate{Hit: intelligence.SearchHit{Doc: doc, Score: score, Snippet: snippet(doc.Body, 240)}, Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Sort by score desc; small k so insertion-sort is fine.
	for i := 1; i < len(cands); i++ {
		for j := i; j > 0 && cands[j].Score > cands[j-1].Score; j-- {
			cands[j], cands[j-1] = cands[j-1], cands[j]
		}
	}
	if k > len(cands) {
		k = len(cands)
	}
	out := make([]intelligence.SearchHit, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, cands[i].Hit)
	}
	return out, nil
}

// Cosine similarity, treating both vectors as floats. NaN/0 inputs return 0.
func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		av, bv := float64(a[i]), float64(b[i])
		dot += av * bv
		na += av * av
		nb += bv * bv
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func containsAll(have, need []string) bool {
	idx := map[string]struct{}{}
	for _, t := range have {
		idx[t] = struct{}{}
	}
	for _, t := range need {
		if _, ok := idx[t]; !ok {
			return false
		}
	}
	return true
}

func snippet(body string, max int) string {
	body = strings.TrimSpace(body)
	r := []rune(body)
	if len(r) <= max {
		return body
	}
	return string(r[:max]) + "…"
}

// encodeBlob packs a float32 slice as little-endian bytes for storage in
// BLOB / BYTEA columns. decodeBlob is the inverse.
func encodeBlob(v []float32) []byte {
	out := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(f))
	}
	return out
}

func decodeBlob(b []byte) []float32 {
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

// pgVector is the lit-form expected by pgvector ("$1::vector(N)"): "[v1,v2,...]"
type pgVector []float32

func (v pgVector) String() string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", f)
	}
	sb.WriteByte(']')
	return sb.String()
}

// pqArray formats a []string as a Postgres text[] literal: '{a,b,c}'.
func pqArray(s []string) string {
	if len(s) == 0 {
		return "{}"
	}
	var sb strings.Builder
	sb.WriteByte('{')
	for i, t := range s {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte('"')
		sb.WriteString(strings.ReplaceAll(t, `"`, `\"`))
		sb.WriteByte('"')
	}
	sb.WriteByte('}')
	return sb.String()
}

// pqArrayRecv parses Postgres '{a,b,c}' into a []string. Minimal — quotes
// the way pqArray emits them.
type pqArrayRecv []string

func (p *pqArrayRecv) Scan(src any) error {
	if src == nil {
		*p = nil
		return nil
	}
	s, ok := src.(string)
	if !ok {
		b, ok := src.([]byte)
		if !ok {
			return fmt.Errorf("pqArrayRecv: unsupported type %T", src)
		}
		s = string(b)
	}
	s = strings.TrimPrefix(strings.TrimSuffix(s, "}"), "{")
	if s == "" {
		*p = nil
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.Trim(p, `"`))
	}
	*p = out
	return nil
}
