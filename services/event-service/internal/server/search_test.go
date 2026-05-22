package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aegisvision/pkg/platform/logging"

	"github.com/aegisvision/services/event-service/internal/server"
	"github.com/aegisvision/services/event-service/internal/store"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

func TestSearch_RequiresTenant(t *testing.T) {
	r := store.New(64)
	s := &server.Search{Store: r}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/events", nil)
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSearch_ListsForTenant(t *testing.T) {
	r := store.New(64)
	r.Append(&dataplanev1.Event{Id: "e1", TenantId: "t-1", Summary: "person dwell"})
	r.Append(&dataplanev1.Event{Id: "e2", TenantId: "t-1", Summary: "vehicle parked"})
	r.Append(&dataplanev1.Event{Id: "e3", TenantId: "t-2", Summary: "person dwell"})

	s := &server.Search{Store: r}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/events", nil).
		WithContext(logging.WithTenantID(context.Background(), "t-1"))
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if len(body.Items) != 2 {
		t.Fatalf("expected 2 items for t-1, got %d", len(body.Items))
	}
	for _, it := range body.Items {
		if it["tenant_id"] != "t-1" {
			t.Fatalf("cross-tenant leak: %+v", it)
		}
	}
}

func TestSearch_QueryFiltersSubstring(t *testing.T) {
	r := store.New(64)
	r.Append(&dataplanev1.Event{Id: "e1", TenantId: "t-1", Summary: "person dwell at zone A"})
	r.Append(&dataplanev1.Event{Id: "e2", TenantId: "t-1", Summary: "vehicle parked", ClassLabel: "vehicle"})
	r.Append(&dataplanev1.Event{Id: "e3", TenantId: "t-1", RuleId: "r-personloiter"})

	s := &server.Search{Store: r}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/events?q=person", nil).
		WithContext(logging.WithTenantID(context.Background(), "t-1"))
	s.Handler().ServeHTTP(rec, req)
	var body struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if len(body.Items) != 2 {
		t.Fatalf("expected 2 person-related, got %d", len(body.Items))
	}
	// Both matches should mention "person" somewhere.
	for _, it := range body.Items {
		hay := strings.ToLower(
			toString(it["summary"]) + " " +
				toString(it["class_label"]) + " " +
				toString(it["rule_id"]))
		if !strings.Contains(hay, "person") {
			t.Fatalf("non-match leaked: %+v", it)
		}
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
