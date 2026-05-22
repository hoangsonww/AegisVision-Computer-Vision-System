// Package logging wires structured logging across every AegisVision service.
//
// JSON output is the default in production so Loki/OpenSearch can parse it;
// dev mode switches to a human-friendly text handler. Every log line is
// annotated with service / env / instance and — when present in the context —
// tenant_id, request_id, trace_id, and span_id. The tracing fields make it
// possible to pivot from a log line to a distributed trace and back without
// custom tooling.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type ctxKey string

const (
	tenantIDKey  ctxKey = "tenant_id"
	requestIDKey ctxKey = "request_id"
)

// WithTenantID returns a context that carries the given tenant id into log
// lines emitted via the contextual handler.
func WithTenantID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantIDKey, id)
}

func TenantID(ctx context.Context) string {
	v, _ := ctx.Value(tenantIDKey).(string)
	return v
}

func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, id)
}

func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// Options controls how the default logger is configured.
type Options struct {
	// Service name, e.g. "api-gateway". Required.
	Service string
	// Environment label, e.g. "prod" | "staging" | "dev". Required.
	Env string
	// Instance name (typically the pod name).
	Instance string
	// One of "debug" | "info" | "warn" | "error". Defaults to info.
	Level string
	// If true, emit human-readable text. Defaults to JSON.
	Pretty bool
	// Where to write. Defaults to os.Stdout.
	Out io.Writer
}

// New constructs a *slog.Logger configured per Options. The returned logger
// adds tenant/request/trace identifiers from the context to every record.
func New(opts Options) *slog.Logger {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Instance == "" {
		opts.Instance = hostname()
	}

	leveler := new(slog.LevelVar)
	leveler.Set(parseLevel(opts.Level))

	handlerOpts := &slog.HandlerOptions{
		Level:     leveler,
		AddSource: false,
	}

	var base slog.Handler
	if opts.Pretty {
		base = slog.NewTextHandler(opts.Out, handlerOpts)
	} else {
		base = slog.NewJSONHandler(opts.Out, handlerOpts)
	}

	h := &contextHandler{Handler: base}
	return slog.New(h).With(
		slog.String("service", opts.Service),
		slog.String("env", opts.Env),
		slog.String("instance", opts.Instance),
	)
}

// Install replaces slog.Default() with a logger built from opts. Convenient
// for `main` so package-level slog.Info calls produce structured output too.
func Install(opts Options) *slog.Logger {
	l := New(opts)
	slog.SetDefault(l)
	return l
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

// contextHandler decorates an upstream handler with context-derived attrs.
type contextHandler struct {
	slog.Handler
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if t := TenantID(ctx); t != "" {
		r.AddAttrs(slog.String("tenant_id", t))
	}
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", span.TraceID().String()),
			slog.String("span_id", span.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}
