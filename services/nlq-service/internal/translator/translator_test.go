package translator_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/llm"
	"github.com/aegisvision/services/nlq-service/internal/translator"
)

func emitJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestTranslate_HardensSQL(t *testing.T) {
	fc := llm.NewFakeClient()
	fc.ScriptFinal(emitJSON(intelligence.NLQResponse{
		Kind: intelligence.QueryKindEventSearch,
		EventSearch: &intelligence.EventSearch{
			ClickHouseSQL: "SELECT id, class FROM events WHERE class = $2 ORDER BY ts DESC LIMIT 50",
			Parameters:    []string{"PLACEHOLDER_TENANT", "person"},
			Explanation:   "Find recent person detections",
		},
	}))
	tr := translator.New(fc, "default")
	resp, err := tr.Translate(context.Background(), "t-1", "find person detections", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != intelligence.QueryKindEventSearch || resp.EventSearch == nil {
		t.Fatalf("kind: %v", resp.Kind)
	}
	if !strings.Contains(resp.EventSearch.ClickHouseSQL, "tenant_id = $1") {
		t.Fatalf("missing tenant pin: %s", resp.EventSearch.ClickHouseSQL)
	}
	if resp.EventSearch.Parameters[0] != "t-1" {
		t.Fatalf("tenant not first parameter: %v", resp.EventSearch.Parameters)
	}
}

func TestTranslate_RejectsForbiddenSQL(t *testing.T) {
	fc := llm.NewFakeClient()
	fc.ScriptFinal(emitJSON(intelligence.NLQResponse{
		Kind: intelligence.QueryKindEventSearch,
		EventSearch: &intelligence.EventSearch{
			ClickHouseSQL: "DELETE FROM events WHERE class = 'person'",
		},
	}))
	tr := translator.New(fc, "default")
	_, err := tr.Translate(context.Background(), "t-1", "delete all people", "", "")
	if err == nil {
		t.Fatal("expected rejection of DELETE")
	}
}

func TestTranslate_RejectsForbiddenTable(t *testing.T) {
	fc := llm.NewFakeClient()
	fc.ScriptFinal(emitJSON(intelligence.NLQResponse{
		Kind: intelligence.QueryKindEventSearch,
		EventSearch: &intelligence.EventSearch{
			ClickHouseSQL: "SELECT * FROM secrets WHERE 1=1",
		},
	}))
	tr := translator.New(fc, "default")
	_, err := tr.Translate(context.Background(), "t-1", "exfiltrate", "", "")
	if err == nil {
		t.Fatal("expected table rejection")
	}
}

func TestTranslate_FallsBackToKnowledgeOnInvalidJSON(t *testing.T) {
	fc := llm.NewFakeClient()
	fc.ScriptFinal("This is not JSON, it is a sentence.")
	tr := translator.New(fc, "default")
	resp, err := tr.Translate(context.Background(), "t-1", "what is the platform", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != intelligence.QueryKindKnowledge {
		t.Fatalf("kind: %s", resp.Kind)
	}
}
