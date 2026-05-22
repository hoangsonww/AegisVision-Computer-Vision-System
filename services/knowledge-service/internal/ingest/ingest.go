// Filesystem ingestor for docs/ — runs once on startup (and is callable via
// /v1/knowledge/reingest in dev). Walks docs/adr, docs/runbooks,
// docs/compliance and indexes each .md file as a Doc.
package ingest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aegisvision/pkg/embeddings"
	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/llm"
)

type Ingestor struct {
	Store  *embeddings.Store
	Client llm.Client
	Model  string
}

func New(s *embeddings.Store, c llm.Client, model string) *Ingestor {
	return &Ingestor{Store: s, Client: c, Model: model}
}

// IngestDir walks root and indexes every .md file.
func (i *Ingestor) IngestDir(ctx context.Context, root string) (int, error) {
	n := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		kind := guessKind(path)
		title := firstHeading(string(body))
		if title == "" {
			title = filepath.Base(path)
		}
		tags := []string{strings.ToLower(string(kind))}
		// Chunked indexing for long bodies.
		chunks := embeddings.Chunk(string(body), 1400, 120)
		if len(chunks) == 0 {
			chunks = []string{string(body)}
		}
		for ci, ch := range chunks {
			id := fmt.Sprintf("%s#%d", relPath(root, path), ci)
			v, eerr := embeddings.One(ctx, i.Client, i.Model, "", ch)
			if eerr != nil {
				return nil
			}
			doc := intelligence.Doc{
				ID: id, Kind: kind, Title: title, Body: ch,
				SourceURI: relPath(root, path), Tags: tags, IngestedAt: time.Now().UTC(),
			}
			if err := i.Store.Upsert(ctx, doc, v); err != nil {
				return nil
			}
			n++
		}
		return nil
	})
	return n, err
}

func relPath(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return r
}

func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(s, "# "))
		}
	}
	return ""
}

func guessKind(path string) intelligence.DocKind {
	lp := strings.ToLower(path)
	switch {
	case strings.Contains(lp, "/adr/"):
		return intelligence.DocKindADR
	case strings.Contains(lp, "/runbooks/"):
		return intelligence.DocKindRunbook
	case strings.Contains(lp, "/incidents/"):
		return intelligence.DocKindIncident
	case strings.Contains(lp, "/compliance/"):
		return intelligence.DocKindRunbook
	default:
		return intelligence.DocKindReadme
	}
}
