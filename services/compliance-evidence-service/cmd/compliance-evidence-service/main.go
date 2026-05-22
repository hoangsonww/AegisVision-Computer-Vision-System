// Command compliance-evidence-service is the read-only window auditors
// (SOC 2, EU AI Act, ISO 27001, internal compliance) consult during a
// review. Per ADR-0029 it owns no data — it composes queries against
// audit-service, policy-gate-service, slo-watchdog, drift-detection-service
// and emits a unified per-control evidence report.
//
// HTTP routes live in internal/server; collectors in internal/evidence.
package main

import (
	"context"
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

	"github.com/aegisvision/services/compliance-evidence-service/internal/evidence"
	"github.com/aegisvision/services/compliance-evidence-service/internal/server"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "compliance-evidence-service"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8520")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8521")
	auditURL := config.String("AEGIS_AUDIT_SERVICE_URL", "")
	gateURL := config.String("AEGIS_POLICY_GATE_URL", "")
	sloURL := config.String("AEGIS_SLO_WATCHDOG_URL", "")
	driftURL := config.String("AEGIS_DRIFT_DETECTION_URL", "")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	ctx := context.Background()
	otelShutdown, err := otelinit.Setup(ctx, otelinit.Options{
		ServiceName: serviceName, ServiceVersion: version, Env: env,
		OTLPEndpoint: otlpEndpoint, Insecure: env == "dev", SamplingRatio: 1.0,
	})
	if err != nil {
		return err
	}

	svc := evidence.New(
		evidence.NewAuditCollector(auditURL),
		evidence.NewGateCollector(gateURL),
		evidence.NewSLOCollector(sloURL),
		evidence.NewDriftCollector(driftURL),
	)

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	queries := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aegis_evidence_queries_total", Help: "Evidence queries by control.",
	}, []string{"control"})
	reg.MustRegister(queries)

	apiHandler := middleware.Recovery(logger)(
		middleware.RequestID()(
			middleware.Logging(logger)(
				red.HTTPMiddleware(metrics.RouteFromPattern)(
					middleware.TenantFromHeader()(server.Mux(svc))))))

	httpSrv := &http.Server{Addr: httpAddr, Handler: apiHandler, ReadHeaderTimeout: 5 * time.Second}

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
	logger.Info("started", slog.String("addr", httpAddr),
		slog.Bool("audit_wired", auditURL != ""),
		slog.Bool("gate_wired", gateURL != ""),
		slog.Bool("slo_wired", sloURL != ""),
		slog.Bool("drift_wired", driftURL != ""))
	return runner.Wait(ctx)
}
