// Command auth-proxy is the platform's request-authentication front-door.
// It runs as an Envoy ext_authz target (or as a sidecar fronting any
// service): receives a request, verifies the bearer JWT against JWKS,
// resolves tenant + roles + scopes, signs the resolved Identity into a
// downstream header, and returns the verdict.
//
// Two integration modes:
//
//   1. Envoy / Istio AuthorizationPolicy custom action — POST to /v1/check
//      with the proxied request envelope. Returns 200 + headers when allow,
//      401/403 otherwise. (Envoy injects the headers downstream.)
//
//   2. Forward-auth (NGINX / Traefik) — GET /v1/check; downstream service
//      reads the X-Aegis-Identity header populated on success.
//
// The signed identity header is HMAC-signed with a key shared between this
// service and downstream services. Downstream code refuses requests whose
// header is missing or whose HMAC doesn't verify — defense against direct
// access bypassing the proxy.
//
// JWKS fetching + caching lives in internal/jwks; the /v1/check handler
// + identity HMAC live in internal/server.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aegisvision/pkg/platform/config"
	"github.com/aegisvision/pkg/platform/health"
	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/metrics"
	"github.com/aegisvision/pkg/platform/middleware"
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/shutdown"

	"github.com/aegisvision/services/auth-proxy/internal/server"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "auth-proxy"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8310")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8311")
	jwksURL := config.String("AEGIS_JWKS_URL", "")
	issuer := config.String("AEGIS_JWT_ISSUER", "")
	audience := config.String("AEGIS_JWT_AUDIENCE", "")
	identityKey := config.String("AEGIS_IDENTITY_HMAC_KEY", "")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	if env != "dev" && env != "test" {
		if jwksURL == "" || issuer == "" {
			return errors.New("AEGIS_JWKS_URL and AEGIS_JWT_ISSUER are required outside dev")
		}
		if len(identityKey) < 32 {
			return errors.New("AEGIS_IDENTITY_HMAC_KEY must be >= 32 bytes outside dev")
		}
	}

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	ctx := context.Background()
	otelShutdown, err := otelinit.Setup(ctx, otelinit.Options{
		ServiceName: serviceName, ServiceVersion: version, Env: env,
		OTLPEndpoint: otlpEndpoint, Insecure: env == "dev", SamplingRatio: 1.0,
	})
	if err != nil {
		return err
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	authn := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aegis_authn_decisions_total", Help: "Decisions by outcome.",
	}, []string{"outcome"})
	reg.MustRegister(authn)

	// JWT verifier — when configured. In dev with no JWKS, the proxy emits
	// a synthetic identity from the X-Tenant-Id header so the rest of the
	// platform still functions.
	var verify func(http.Handler) http.Handler
	if jwksURL != "" {
		v, err := middleware.Authn(middleware.AuthnConfig{
			JWKSURL: jwksURL, Issuer: issuer, Audience: audience,
		})
		if err != nil {
			return err
		}
		verify = v
	} else {
		verify = middleware.TenantFromHeader()
		logger.Warn("auth-proxy in synthetic mode (X-Tenant-Id only) — set AEGIS_JWKS_URL for production")
	}

	// /v1/check is the ext_authz endpoint. server.CheckHandler emits the
	// signed identity + tenant headers on success, 401 on failure.
	check := server.Chain(
		middleware.Recovery(logger),
		middleware.RequestID(),
		middleware.Logging(logger),
		red.HTTPMiddleware(metrics.RouteFromPattern),
		verify,
	)(server.CheckHandler(identityKey, authn))

	mux := http.NewServeMux()
	mux.Handle("/v1/check", check)

	httpSrv := &http.Server{Addr: httpAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	hr := health.NewRegistry()
	hr.Register("self", func(context.Context) error { return nil })
	hmux := http.NewServeMux()
	hmux.Handle("/healthz", health.LivenessHandler())
	hmux.Handle("/readyz", hr.ReadinessHandler())
	hmux.Handle("/metrics", metrics.Handler(reg))
	hsrv := &http.Server{Addr: healthAddr, Handler: hmux, ReadHeaderTimeout: 5 * time.Second}

	runner := shutdown.New(logger)
	runner.Register("http", httpSrv.Shutdown)
	runner.Register("health-http", hsrv.Shutdown)
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })

	go func() { _ = httpSrv.ListenAndServe() }()
	go func() { _ = hsrv.ListenAndServe() }()
	logger.Info("started", slog.String("addr", httpAddr), slog.Bool("jwt_enabled", jwksURL != ""))
	return runner.Wait(ctx)
}
