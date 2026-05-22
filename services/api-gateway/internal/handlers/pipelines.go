// Package handlers contains the public HTTP handlers for api-gateway.
//
// The shape of the JSON response is deliberately minimal — only fields with
// stable semantics across releases. Internal storage details (Postgres keys,
// internal protobuf field numbers) never leak. Per the doc's naming
// discipline, path segments are plural product nouns.
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	controlv1 "github.com/aegisvision/proto/gen/go/aegisvision/control/v1"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/pagination"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/api-gateway/internal/upstream"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PipelineHandlers carries the dependencies the handlers need.
type PipelineHandlers struct {
	Pipelines *upstream.PipelineClient
	Cursors   *pagination.Encoder
}

// JSON wire form. Snake_case keys to match the conventions in the docs.
type pipelineJSON struct {
	ID              string   `json:"id"`
	TenantID        string   `json:"tenant_id"`
	ProjectID       string   `json:"project_id"`
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	Labels          []string `json:"labels,omitempty"`
	Status          string   `json:"status"`
	CurrentRevision int64    `json:"current_revision"`
	DagJSON         string   `json:"dag_json,omitempty"`
	CreatedAt       string   `json:"created_at,omitempty"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
	Version         int64    `json:"version,omitempty"`
}

type listResp struct {
	Items         []pipelineJSON `json:"items"`
	NextPageToken string         `json:"next_page_token,omitempty"`
}

// Get handles GET /v1/pipelines/{id}.
func (h *PipelineHandlers) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/pipelines/")
	if id == "" || strings.ContainsRune(id, '/') {
		problem.NotFound("pipeline not found").Write(w)
		return
	}
	p, err := h.Pipelines.Get(r.Context(), id)
	if err != nil {
		toProblem(err).Write(w)
		return
	}
	if p == nil {
		problem.NotFound("pipeline not found").Write(w)
		return
	}
	if p.GetTenant().GetId() != logging.TenantID(r.Context()) {
		// Cross-tenant read is indistinguishable from "not found" — leaking
		// the existence of another tenant's id would itself be an info leak.
		problem.NotFound("pipeline not found").Write(w)
		return
	}
	writeJSON(w, http.StatusOK, toJSON(p))
}

// List handles GET /v1/pipelines.
//
// Query parameters:
//
//	project_id      string  optional
//	name_contains   string  optional case-sensitive substring filter
//	status          string  optional, repeatable: draft|running|paused|...
//	page_size       int     optional, defaults to 50, clamped to [1, 200]
//	page_token      string  optional cursor returned from a prior call
func (h *PipelineHandlers) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	pageSize := pagination.DefaultPageSize
	if s := q.Get("page_size"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			problem.BadRequest("page_size must be an integer").Write(w)
			return
		}
		pageSize = pagination.ClampPageSize(int32(n))
	}

	cursor, err := h.Cursors.Decode("pipelines", q.Get("page_token"))
	if err != nil {
		problem.BadRequest("invalid page_token").Write(w)
		return
	}

	req := &controlv1.ListPipelinesRequest{
		TenantId:     logging.TenantID(r.Context()),
		ProjectId:    q.Get("project_id"),
		NameContains: q.Get("name_contains"),
		StatusIn:     parseStatuses(q["status"]),
		Page: &commonv1.PageRequest{
			PageSize:  int32(pageSize),
			PageToken: cursor.LastID,
		},
	}

	resp, err := h.Pipelines.List(r.Context(), req)
	if err != nil {
		toProblem(err).Write(w)
		return
	}

	out := listResp{Items: make([]pipelineJSON, 0, len(resp.GetPipelines()))}
	for _, p := range resp.GetPipelines() {
		out.Items = append(out.Items, toJSON(p))
	}
	if nxt := resp.GetPage().GetNextPageToken(); nxt != "" {
		out.NextPageToken = h.Cursors.Encode("pipelines", nxt)
	}
	writeJSON(w, http.StatusOK, out)
}

type createReq struct {
	ProjectID   string   `json:"project_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	DagJSON     string   `json:"dag_json"`
}

// Create handles POST /v1/pipelines.
// The Idempotency-Key middleware deduplicates replays; this handler just
// receives a clean request and proxies it to pipeline-service.
func (h *PipelineHandlers) Create(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var in createReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		problem.BadRequest("invalid JSON body").Write(w)
		return
	}
	violations := []problem.FieldViolation{}
	if strings.TrimSpace(in.Name) == "" {
		violations = append(violations, problem.FieldViolation{Field: "name", Message: "is required", Code: "required"})
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		violations = append(violations, problem.FieldViolation{Field: "project_id", Message: "is required", Code: "required"})
	}
	if len(violations) > 0 {
		problem.Unprocessable("invalid pipeline", violations...).Write(w)
		return
	}

	tenant := logging.TenantID(r.Context())
	created, err := h.Pipelines.Create(r.Context(), &controlv1.CreatePipelineRequest{
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Tenant:         &commonv1.TenantRef{Id: tenant},
		Project:        &commonv1.ProjectRef{Id: in.ProjectID, TenantId: tenant},
		Name:           in.Name,
		Description:    in.Description,
		Labels:         in.Labels,
		DagJson:        in.DagJSON,
	})
	if err != nil {
		toProblem(err).Write(w)
		return
	}
	writeJSON(w, http.StatusCreated, toJSON(created))
}

// Deploy handles POST /v1/pipelines/{id}:deploy.
//
// Action verbs follow Google's API style guide so the verb is unambiguously
// a verb on the resource and cannot collide with sub-resources.
func (h *PipelineHandlers) Deploy(w http.ResponseWriter, r *http.Request) {
	// Path is /v1/pipelines/{id}:deploy → strip the prefix and the action.
	path := strings.TrimPrefix(r.URL.Path, "/v1/pipelines/")
	id := strings.TrimSuffix(path, ":deploy")
	if id == "" || strings.ContainsRune(id, '/') {
		problem.NotFound("pipeline not found").Write(w)
		return
	}
	resp, err := h.Pipelines.Deploy(r.Context(), id)
	if err != nil {
		toProblem(err).Write(w)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"workflow_id": resp.GetWorkflowId(),
		"status":      pipelineStatusString(resp.GetStatus()),
	})
}

func parseStatuses(in []string) []controlv1.PipelineStatus {
	out := make([]controlv1.PipelineStatus, 0, len(in))
	for _, s := range in {
		switch strings.ToLower(s) {
		case "draft":
			out = append(out, controlv1.PipelineStatus_PIPELINE_STATUS_DRAFT)
		case "validated":
			out = append(out, controlv1.PipelineStatus_PIPELINE_STATUS_VALIDATED)
		case "deploying":
			out = append(out, controlv1.PipelineStatus_PIPELINE_STATUS_DEPLOYING)
		case "running":
			out = append(out, controlv1.PipelineStatus_PIPELINE_STATUS_RUNNING)
		case "degraded":
			out = append(out, controlv1.PipelineStatus_PIPELINE_STATUS_DEGRADED)
		case "paused":
			out = append(out, controlv1.PipelineStatus_PIPELINE_STATUS_PAUSED)
		case "failed":
			out = append(out, controlv1.PipelineStatus_PIPELINE_STATUS_FAILED)
		}
	}
	return out
}

func pipelineStatusString(s controlv1.PipelineStatus) string {
	switch s {
	case controlv1.PipelineStatus_PIPELINE_STATUS_DRAFT:
		return "draft"
	case controlv1.PipelineStatus_PIPELINE_STATUS_VALIDATED:
		return "validated"
	case controlv1.PipelineStatus_PIPELINE_STATUS_DEPLOYING:
		return "deploying"
	case controlv1.PipelineStatus_PIPELINE_STATUS_RUNNING:
		return "running"
	case controlv1.PipelineStatus_PIPELINE_STATUS_DEGRADED:
		return "degraded"
	case controlv1.PipelineStatus_PIPELINE_STATUS_PAUSED:
		return "paused"
	case controlv1.PipelineStatus_PIPELINE_STATUS_FAILED:
		return "failed"
	case controlv1.PipelineStatus_PIPELINE_STATUS_DELETED:
		return "deleted"
	default:
		return "unspecified"
	}
}

func toJSON(p *controlv1.Pipeline) pipelineJSON {
	out := pipelineJSON{
		ID:              p.GetId(),
		TenantID:        p.GetTenant().GetId(),
		ProjectID:       p.GetProject().GetId(),
		Name:            p.GetName(),
		Description:     p.GetDescription(),
		Labels:          p.GetLabels(),
		Status:          pipelineStatusString(p.GetStatus()),
		CurrentRevision: p.GetCurrentRevision(),
		DagJSON:         p.GetDagJson(),
	}
	if a := p.GetAudit(); a != nil {
		if a.GetCreatedAt() != nil {
			out.CreatedAt = a.GetCreatedAt().AsTime().UTC().Format("2006-01-02T15:04:05Z")
		}
		if a.GetUpdatedAt() != nil {
			out.UpdatedAt = a.GetUpdatedAt().AsTime().UTC().Format("2006-01-02T15:04:05Z")
		}
		out.Version = a.GetVersion()
	}
	return out
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func toProblem(err error) *problem.Problem {
	st, ok := status.FromError(err)
	if !ok {
		return problem.Internal("upstream error")
	}
	switch st.Code() {
	case codes.NotFound:
		return problem.NotFound(st.Message())
	case codes.InvalidArgument:
		return problem.BadRequest(st.Message())
	case codes.AlreadyExists:
		return problem.Conflict(st.Message())
	case codes.PermissionDenied:
		return problem.Forbidden(st.Message())
	case codes.Unauthenticated:
		return problem.Unauthorized(st.Message())
	case codes.Unavailable, codes.DeadlineExceeded:
		return problem.ServiceUnavailable("upstream unavailable")
	default:
		return problem.Internal("internal upstream error")
	}
}
