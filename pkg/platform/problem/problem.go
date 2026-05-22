// Package problem implements RFC 9457 application/problem+json errors.
//
// Every public API in AegisVision emits errors in this format; clients can
// rely on the `code` field being a stable, machine-readable identifier even
// when the human-readable `detail` changes across releases.
package problem

import (
	"encoding/json"
	"errors"
	"net/http"
)

// ContentType is the RFC 9457 media type. Set it on every error response.
const ContentType = "application/problem+json"

// Problem mirrors the wire format. Marshal with json.Marshal; only non-zero
// fields are emitted, so callers can omit pieces they don't need.
type Problem struct {
	Type       string           `json:"type,omitempty"`
	Title      string           `json:"title"`
	Status     int              `json:"status"`
	Detail     string           `json:"detail,omitempty"`
	Instance   string           `json:"instance,omitempty"`
	Code       string           `json:"code,omitempty"`
	Violations []FieldViolation `json:"violations,omitempty"`
}

// FieldViolation reports one field-level validation failure.
type FieldViolation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// Error makes Problem usable as a regular error.
func (p *Problem) Error() string {
	if p.Detail != "" {
		return p.Title + ": " + p.Detail
	}
	return p.Title
}

// Write emits the problem as a JSON response with the right Content-Type and
// status code. Suitable as a terminal HTTP response.
func (p *Problem) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", ContentType)
	if p.Status == 0 {
		p.Status = http.StatusInternalServerError
	}
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// Canonical constructors. The stable `code` strings are part of the public
// contract — never repurpose one without a major version bump.

func BadRequest(detail string, violations ...FieldViolation) *Problem {
	return &Problem{
		Title:      "Bad Request",
		Status:     http.StatusBadRequest,
		Detail:     detail,
		Code:       "bad_request",
		Violations: violations,
	}
}

func Unauthorized(detail string) *Problem {
	return &Problem{Title: "Unauthorized", Status: http.StatusUnauthorized, Detail: detail, Code: "unauthorized"}
}

func Forbidden(detail string) *Problem {
	return &Problem{Title: "Forbidden", Status: http.StatusForbidden, Detail: detail, Code: "forbidden"}
}

func NotFound(detail string) *Problem {
	return &Problem{Title: "Not Found", Status: http.StatusNotFound, Detail: detail, Code: "not_found"}
}

func Conflict(detail string) *Problem {
	return &Problem{Title: "Conflict", Status: http.StatusConflict, Detail: detail, Code: "conflict"}
}

func PreconditionFailed(detail string) *Problem {
	return &Problem{Title: "Precondition Failed", Status: http.StatusPreconditionFailed, Detail: detail, Code: "precondition_failed"}
}

func Unprocessable(detail string, violations ...FieldViolation) *Problem {
	return &Problem{
		Title:      "Unprocessable Entity",
		Status:     http.StatusUnprocessableEntity,
		Detail:     detail,
		Code:       "unprocessable_entity",
		Violations: violations,
	}
}

func TooManyRequests(detail string) *Problem {
	return &Problem{Title: "Too Many Requests", Status: http.StatusTooManyRequests, Detail: detail, Code: "too_many_requests"}
}

func Internal(detail string) *Problem {
	return &Problem{Title: "Internal Server Error", Status: http.StatusInternalServerError, Detail: detail, Code: "internal"}
}

func ServiceUnavailable(detail string) *Problem {
	return &Problem{Title: "Service Unavailable", Status: http.StatusServiceUnavailable, Detail: detail, Code: "service_unavailable"}
}

// From maps an arbitrary error into a Problem. If err is already a *Problem,
// it is returned unchanged; otherwise an opaque 500 is produced so internal
// errors never leak detail into a public response.
func From(err error) *Problem {
	if err == nil {
		return nil
	}
	var p *Problem
	if errors.As(err, &p) {
		return p
	}
	return Internal("an internal error occurred")
}
