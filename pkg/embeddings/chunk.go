// Package embeddings provides:
//
//   - Token-bounded chunking of text bodies suitable for embedding.
//   - A pgvector-friendly store backed by Postgres (production) and pure-SQL
//     brute-force cosine over SQLite (tests / single-tenant dev).
//   - Search by cosine similarity.
//
// The model name is intentionally not pinned here — the caller passes it in
// through pkg/llm.EmbeddingRequest. Picking the model + embedding dim is a
// deployment concern (text-embedding-3-small at 1536, bge-small at 384,
// etc.), not a library one.
package embeddings

import "strings"

// Chunk splits long bodies into overlapping windows suitable for embedding.
// We use a simple paragraph + sentence-boundary heuristic:
//
//  1. Split on double-newline (paragraphs).
//  2. Pack paragraphs into windows up to maxRunes runes.
//  3. If a single paragraph exceeds the window, split it by sentence-ish.
//  4. Successive windows share `overlap` runes for context.
//
// This is good enough for ADR + runbook + incident bodies; for hostile
// inputs (e.g. minified JSON) callers should pre-treat the data.
func Chunk(text string, maxRunes, overlap int) []string {
	if maxRunes <= 0 {
		maxRunes = 800
	}
	if overlap < 0 || overlap >= maxRunes {
		overlap = 0
	}
	paragraphs := strings.Split(text, "\n\n")
	var out []string
	var window strings.Builder
	flush := func() {
		if window.Len() == 0 {
			return
		}
		out = append(out, strings.TrimSpace(window.String()))
		// Apply overlap by retaining the tail of the just-emitted window.
		if overlap > 0 {
			tail := tailRunes(window.String(), overlap)
			window.Reset()
			window.WriteString(tail)
		} else {
			window.Reset()
		}
	}
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Paragraph larger than the window — split on sentence-ish.
		if runeLen(p) > maxRunes {
			for _, s := range splitSentencesApprox(p) {
				appendChunked(&window, s, maxRunes, flush)
			}
			continue
		}
		// Will it fit?
		if runeLen(window.String())+runeLen(p)+2 > maxRunes {
			flush()
		}
		if window.Len() > 0 {
			window.WriteString("\n\n")
		}
		window.WriteString(p)
	}
	flush()
	return out
}

func appendChunked(window *strings.Builder, s string, max int, flush func()) {
	if runeLen(window.String())+runeLen(s)+1 > max {
		flush()
	}
	if window.Len() > 0 {
		window.WriteString(" ")
	}
	window.WriteString(s)
}

func splitSentencesApprox(p string) []string {
	// Sentence-ish: split on ". " / "? " / "! " preserving boundary.
	var out []string
	var cur strings.Builder
	for i, r := range p {
		cur.WriteRune(r)
		if (r == '.' || r == '!' || r == '?') && i+1 < len(p) && p[i+1] == ' ' {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}

func runeLen(s string) int { return len([]rune(s)) }

func tailRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}
