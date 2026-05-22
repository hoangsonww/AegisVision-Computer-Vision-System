// Stream handlers. Same product-noun discipline as pipelines: /v1/streams,
// IDs opaque, URLs returned already redacted from the server side.
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
)

type StreamHandlers struct {
	Streams *upstream.StreamClient
	Cursors *pagination.Encoder
}

type streamJSON struct {
	ID           string  `json:"id"`
	TenantID     string  `json:"tenant_id"`
	ProjectID    string  `json:"project_id"`
	CameraID     string  `json:"camera_id,omitempty"`
	SiteID       string  `json:"site_id,omitempty"`
	Name         string  `json:"name"`
	Protocol     string  `json:"protocol"`
	URL          string  `json:"url"`
	Health       string  `json:"health"`
	ObservedFPS  float64 `json:"observed_fps"`
	ObservedKbps int64   `json:"observed_kbps"`
}

type streamListResp struct {
	Items         []streamJSON `json:"items"`
	NextPageToken string       `json:"next_page_token,omitempty"`
}

func (h *StreamHandlers) List(w http.ResponseWriter, r *http.Request) {
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
	cursor, err := h.Cursors.Decode("streams", q.Get("page_token"))
	if err != nil {
		problem.BadRequest("invalid page_token").Write(w)
		return
	}
	req := &controlv1.ListStreamsRequest{
		TenantId:  logging.TenantID(r.Context()),
		ProjectId: q.Get("project_id"),
		SiteId:    q.Get("site_id"),
		Page:      &commonv1.PageRequest{PageSize: int32(pageSize), PageToken: cursor.LastID},
	}
	resp, err := h.Streams.List(r.Context(), req)
	if err != nil {
		toProblem(err).Write(w)
		return
	}
	out := streamListResp{Items: make([]streamJSON, 0, len(resp.GetStreams()))}
	for _, s := range resp.GetStreams() {
		out.Items = append(out.Items, streamToJSON(s))
	}
	if nxt := resp.GetPage().GetNextPageToken(); nxt != "" {
		out.NextPageToken = h.Cursors.Encode("streams", nxt)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *StreamHandlers) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/streams/")
	if id == "" || strings.ContainsRune(id, '/') {
		problem.NotFound("stream not found").Write(w)
		return
	}
	s, err := h.Streams.Get(r.Context(), id)
	if err != nil {
		toProblem(err).Write(w)
		return
	}
	if s == nil || s.GetTenant().GetId() != logging.TenantID(r.Context()) {
		problem.NotFound("stream not found").Write(w)
		return
	}
	writeJSON(w, http.StatusOK, streamToJSON(s))
}

type createStreamReq struct {
	ProjectID  string `json:"project_id"`
	CameraID   string `json:"camera_id"`
	SiteID     string `json:"site_id"`
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	URL        string `json:"url"`
	PipelineID string `json:"pipeline_id"`
}

func (h *StreamHandlers) Create(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var in createStreamReq
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
	if strings.TrimSpace(in.URL) == "" {
		violations = append(violations, problem.FieldViolation{Field: "url", Message: "is required", Code: "required"})
	}
	if len(violations) > 0 {
		problem.Unprocessable("invalid stream", violations...).Write(w)
		return
	}
	tenant := logging.TenantID(r.Context())
	s, err := h.Streams.Create(r.Context(), &controlv1.CreateStreamRequest{
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Tenant:         &commonv1.TenantRef{Id: tenant},
		Project:        &commonv1.ProjectRef{Id: in.ProjectID, TenantId: tenant},
		CameraId:       in.CameraID,
		SiteId:         in.SiteID,
		Name:           in.Name,
		Protocol:       parseProtocol(in.Protocol),
		Url:            in.URL,
		PipelineId:     in.PipelineID,
	})
	if err != nil {
		toProblem(err).Write(w)
		return
	}
	writeJSON(w, http.StatusCreated, streamToJSON(s))
}

func (h *StreamHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/streams/")
	if id == "" || strings.ContainsRune(id, '/') {
		problem.NotFound("stream not found").Write(w)
		return
	}
	if err := h.Streams.Delete(r.Context(), id); err != nil {
		toProblem(err).Write(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func streamToJSON(s *controlv1.Stream) streamJSON {
	return streamJSON{
		ID:           s.GetId(),
		TenantID:     s.GetTenant().GetId(),
		ProjectID:    s.GetProject().GetId(),
		CameraID:     s.GetCameraId(),
		SiteID:       s.GetSiteId(),
		Name:         s.GetName(),
		Protocol:     streamProtocolString(s.GetProtocol()),
		URL:          s.GetUrlRedacted(),
		Health:       streamHealthString(s.GetHealth()),
		ObservedFPS:  s.GetObservedFps(),
		ObservedKbps: s.GetObservedKbps(),
	}
}

func streamProtocolString(p controlv1.StreamProtocol) string {
	switch p {
	case controlv1.StreamProtocol_STREAM_PROTOCOL_RTSP:
		return "rtsp"
	case controlv1.StreamProtocol_STREAM_PROTOCOL_RTMP:
		return "rtmp"
	case controlv1.StreamProtocol_STREAM_PROTOCOL_HLS:
		return "hls"
	case controlv1.StreamProtocol_STREAM_PROTOCOL_WEBRTC:
		return "webrtc"
	case controlv1.StreamProtocol_STREAM_PROTOCOL_FILE:
		return "file"
	default:
		return "unspecified"
	}
}

func streamHealthString(h controlv1.StreamHealth) string {
	switch h {
	case controlv1.StreamHealth_STREAM_HEALTH_GREEN:
		return "green"
	case controlv1.StreamHealth_STREAM_HEALTH_YELLOW:
		return "yellow"
	case controlv1.StreamHealth_STREAM_HEALTH_RED:
		return "red"
	case controlv1.StreamHealth_STREAM_HEALTH_DISCONNECTED:
		return "disconnected"
	default:
		return "unspecified"
	}
}

func parseProtocol(s string) controlv1.StreamProtocol {
	switch strings.ToLower(s) {
	case "rtsp":
		return controlv1.StreamProtocol_STREAM_PROTOCOL_RTSP
	case "rtmp":
		return controlv1.StreamProtocol_STREAM_PROTOCOL_RTMP
	case "hls":
		return controlv1.StreamProtocol_STREAM_PROTOCOL_HLS
	case "webrtc":
		return controlv1.StreamProtocol_STREAM_PROTOCOL_WEBRTC
	case "file":
		return controlv1.StreamProtocol_STREAM_PROTOCOL_FILE
	default:
		return controlv1.StreamProtocol_STREAM_PROTOCOL_UNSPECIFIED
	}
}
