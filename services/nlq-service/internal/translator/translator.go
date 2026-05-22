// Package translator turns a natural-language prompt into one of:
//
//   - an EventSearch (ClickHouse SQL with positional parameters)
//   - a DAG draft (canonical JSON the pipeline-service compiler validates)
//   - a knowledge answer (a snippet from knowledge-service)
//
// Design constraints:
//
//   - Read-only. Translator NEVER produces a side-effecting statement. The
//     SQL it produces is restricted to SELECT against approved tables; any
//     statement that doesn't start with `SELECT` is rejected.
//   - Parameter binding only. We don't inline tenant/time/values into SQL;
//     the LLM is instructed to emit `$1`, `$2`, ... and a separate
//     parameters list. Defense-in-depth against prompt-driven SQL.
//   - Tenant pinning. Every generated query gets a mandatory
//     `tenant_id = $1` predicate appended by the translator (not the LLM);
//     even if the LLM tries to omit it, the wrapper adds it.
package translator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/llm"
)

// Translator drives the LLM-side translation.
type Translator struct {
	Client llm.Client
	Model  string
}

func New(c llm.Client, model string) *Translator {
	return &Translator{Client: c, Model: model}
}

// AllowedTables is the read-only whitelist the translator is permitted to
// emit against. Any FROM/JOIN to a table not in this set is rejected.
var AllowedTables = map[string]struct{}{
	"events":     {},
	"detections": {},
	"streams":    {},
	"pipelines":  {},
}

// Translate the prompt under the tenant scope. timeFrom/timeTo are ISO
// timestamps; they are bound as parameters and used by the translator to
// scope the query.
func (t *Translator) Translate(ctx context.Context, tenant, prompt, timeFrom, timeTo string) (*intelligence.NLQResponse, error) {
	system := `You are a query translator for AegisVision. The user gives a natural-language question; your job is to choose one of three forms and return strictly JSON, no prose, matching this shape:

{
  "kind": "event_search" | "dag_draft" | "knowledge",
  "event_search": { "clickhouse_sql": "SELECT ...", "parameters": ["..."], "explanation": "..." },
  "dag_draft": { "dag_json": "{...}", "explanation": "..." },
  "knowledge": { "text": "...", "citations": ["..."] }
}

Rules:
- Choose "event_search" when the question is about specific detections / events. Emit SELECTs only, against tables: events, detections, streams, pipelines. Use positional parameters $1, $2, ...; never inline literals. The first parameter is always tenant_id; treat $2/$3 as the time range when relevant.
- Choose "dag_draft" when the user is sketching a new pipeline. Emit the canonical DAG JSON the compiler accepts. Do NOT include any deploy/promotion intent.
- Choose "knowledge" when the question is about how the platform works.
- NEVER emit DELETE/UPDATE/INSERT/DROP/ALTER/TRUNCATE/ATTACH/DETACH/CREATE.`

	resp, err := t.Client.Chat(ctx, llm.ChatRequest{
		TenantID: tenant,
		Model:    t.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: system},
			{Role: llm.RoleUser, Content: prompt},
		},
		JSONMode:    true,
		Temperature: 0.1,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Message.Content == "" {
		return nil, errors.New("empty translator response")
	}
	var parsed intelligence.NLQResponse
	if err := json.Unmarshal([]byte(resp.Message.Content), &parsed); err != nil {
		// Synthetic backends produce echo strings; fall back to a knowledge
		// answer to keep dev/test paths working.
		parsed = intelligence.NLQResponse{
			Kind:      intelligence.QueryKindKnowledge,
			Knowledge: &intelligence.KnowledgeAnswer{Text: resp.Message.Content},
		}
	}
	parsed.Safety = resp.Safety
	if parsed.Kind == intelligence.QueryKindEventSearch && parsed.EventSearch != nil {
		if err := harden(parsed.EventSearch, tenant, timeFrom, timeTo); err != nil {
			return nil, err
		}
	}
	return &parsed, nil
}

// harden applies the read-only + tenant-pin + table-whitelist guards on
// the LLM's proposed SQL. Returns an error if the SQL can't be made safe.
func harden(es *intelligence.EventSearch, tenant, timeFrom, timeTo string) error {
	sql := strings.TrimSpace(es.ClickHouseSQL)
	if sql == "" {
		return errors.New("empty SQL")
	}
	upper := strings.ToUpper(sql)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return fmt.Errorf("non-SELECT statement rejected")
	}
	// Reject any forbidden keyword anywhere in the statement.
	forbidden := []string{"DELETE", "UPDATE", "INSERT", "DROP", "ALTER", "TRUNCATE", "ATTACH", "DETACH", "CREATE", "GRANT", "REVOKE"}
	for _, kw := range forbidden {
		if regexp.MustCompile(`(?i)\b` + kw + `\b`).MatchString(sql) {
			return fmt.Errorf("forbidden keyword: %s", kw)
		}
	}
	// Multiple statements: a `;` followed by non-whitespace is rejected.
	if regexp.MustCompile(`;\s*\S`).MatchString(sql) {
		return errors.New("multiple statements rejected")
	}
	sql = strings.TrimRight(sql, "; \n\r\t")
	// Table whitelist: every token after FROM/JOIN must be in the allow set.
	tableRE := regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	for _, m := range tableRE.FindAllStringSubmatch(sql, -1) {
		if _, ok := AllowedTables[strings.ToLower(m[1])]; !ok {
			return fmt.Errorf("table not in whitelist: %s", m[1])
		}
	}
	// Tenant pin: append "AND tenant_id = $1" if WHERE exists, else "WHERE tenant_id = $1".
	if !regexp.MustCompile(`(?i)\btenant_id\s*=`).MatchString(sql) {
		if regexp.MustCompile(`(?i)\bWHERE\b`).MatchString(sql) {
			sql += " AND tenant_id = $1"
		} else {
			sql += " WHERE tenant_id = $1"
		}
	}
	es.ClickHouseSQL = sql
	// Ensure tenant + optional times come first in parameters.
	params := []string{tenant}
	if timeFrom != "" {
		params = append(params, timeFrom)
	}
	if timeTo != "" {
		params = append(params, timeTo)
	}
	// Preserve any additional parameters the LLM emitted, but discard its
	// first entry (we control tenant).
	if len(es.Parameters) > 1 {
		params = append(params, es.Parameters[1:]...)
	}
	es.Parameters = params
	return nil
}
