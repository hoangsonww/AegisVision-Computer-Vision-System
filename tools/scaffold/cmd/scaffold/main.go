// Command scaffold generates a new AegisVision service from the golden-path
// template. Stamps out the standard layout (cmd/, internal/server, internal/
// store, Dockerfile) plus a fully-wired Helm chart that satisfies the
// conformance test.
//
//	scaffold new --name billing-service --port 8300
//
// produces services/billing-service/... and deploy/helm/billing-service/...
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: scaffold new --name <name> --port <http_port>")
		os.Exit(2)
	}
	if os.Args[1] != "new" {
		fmt.Fprintln(os.Stderr, "only 'new' command is supported")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	name := fs.String("name", "", "service name (kebab-case)")
	port := fs.Int("port", 8300, "HTTP port")
	healthPort := fs.Int("health-port", 8301, "health/metrics port")
	plane := fs.String("plane", "control", "control | data | intelligence")
	tier := fs.String("tier", "T1", "service tier (T0 | T1 | T2)")
	rootDir := fs.String("root", ".", "monorepo root")
	_ = fs.Parse(os.Args[2:])

	if *name == "" {
		fmt.Fprintln(os.Stderr, "--name is required")
		os.Exit(2)
	}
	if err := scaffold(*rootDir, *name, *port, *healthPort, *plane, *tier); err != nil {
		fmt.Fprintln(os.Stderr, "scaffold:", err)
		os.Exit(1)
	}
	fmt.Println("scaffolded", *name)
	fmt.Println("next steps:")
	fmt.Println("  1. add services/" + *name + " to go.work")
	fmt.Println("  2. cd services/" + *name + " && go mod tidy")
	fmt.Println("  3. add deploy/helm/" + *name + " to the ArgoCD ApplicationSet")
	fmt.Println("  4. write your domain logic in internal/service/")
}

type templateData struct {
	Name, Plane, Tier             string
	Port, HealthPort              int
	NamePascal, NameCamel         string
	PriorityClass                 string
}

func scaffold(root, name string, port, healthPort int, plane, tier string) error {
	d := templateData{
		Name: name, Plane: plane, Tier: tier,
		Port: port, HealthPort: healthPort,
		NamePascal: pascal(name), NameCamel: camel(name),
	}
	switch plane {
	case "data":
		d.PriorityClass = "aegis-data-plane"
	default:
		d.PriorityClass = "aegis-control-plane"
	}

	files := map[string]string{
		filepath.Join("services", name, "go.mod"):                            tmplGoMod,
		filepath.Join("services", name, "Dockerfile"):                        tmplDockerfile,
		filepath.Join("services", name, "cmd", name, "main.go"):              tmplMain,
		filepath.Join("services", name, "internal", "service", "service.go"): tmplService,
		filepath.Join("deploy", "helm", name, "Chart.yaml"):                  tmplChart,
		filepath.Join("deploy", "helm", name, "values.yaml"):                 tmplValues,
		filepath.Join("deploy", "helm", name, "templates", "serviceaccount.yaml"):   tmplSA,
		filepath.Join("deploy", "helm", name, "templates", "deployment.yaml"):       tmplDeployment,
		filepath.Join("deploy", "helm", name, "templates", "service.yaml"):          tmplService_,
		filepath.Join("deploy", "helm", name, "templates", "peerauthentication.yaml"): tmplPeerAuth,
		filepath.Join("deploy", "helm", name, "templates", "authorizationpolicy.yaml"): tmplAuthZ,
		filepath.Join("deploy", "helm", name, "templates", "networkpolicy.yaml"):    tmplNetPol,
		filepath.Join("deploy", "helm", name, "templates", "hpa.yaml"):              tmplHPA,
		filepath.Join("deploy", "helm", name, "templates", "servicemonitor.yaml"):   tmplSM,
		filepath.Join("deploy", "helm", name, "templates", "poddisruptionbudget.yaml"): tmplPDB,
	}
	for rel, body := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		rendered := render(body, d)
		if err := os.WriteFile(abs, []byte(rendered), 0o644); err != nil {
			return err
		}
	}
	// Final sanity: ensure produced files exist.
	return filepath.WalkDir(filepath.Join(root, "services", name), func(p string, _ fs.DirEntry, err error) error {
		return err
	})
}

func render(s string, d templateData) string {
	r := strings.NewReplacer(
		"{{.Name}}", d.Name,
		"{{.NamePascal}}", d.NamePascal,
		"{{.NameCamel}}", d.NameCamel,
		"{{.Plane}}", d.Plane,
		"{{.Tier}}", d.Tier,
		"{{.Port}}", fmt.Sprintf("%d", d.Port),
		"{{.HealthPort}}", fmt.Sprintf("%d", d.HealthPort),
		"{{.PriorityClass}}", d.PriorityClass,
	)
	return r.Replace(s)
}

func pascal(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}
func camel(s string) string {
	p := pascal(s)
	if p == "" {
		return p
	}
	return strings.ToLower(p[:1]) + p[1:]
}

// --- templates ---

const tmplGoMod = `module github.com/aegisvision/services/{{.Name}}

go 1.26.0

require (
	github.com/aegisvision/pkg/platform v0.0.0
	github.com/prometheus/client_golang v1.20.5
)

replace github.com/aegisvision/pkg/platform => ../../pkg/platform
`

const tmplDockerfile = `FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/{{.Name}}/ ./services/{{.Name}}/
WORKDIR /src/services/{{.Name}}
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/{{.Name}} ./cmd/{{.Name}}
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/{{.Name}} /usr/local/bin/{{.Name}}
USER 65532:65532
EXPOSE {{.Port}} {{.HealthPort}}
ENTRYPOINT ["/usr/local/bin/{{.Name}}"]
`

const tmplMain = `// Command {{.Name}} is a generated golden-path service. Replace this main
// with your domain wiring; the platform packages plug in unchanged.
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
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/shutdown"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "{{.Name}}"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":{{.Port}}")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":{{.HealthPort}}")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	ctx := context.Background()
	otelShutdown, err := otelinit.Setup(ctx, otelinit.Options{
		ServiceName: serviceName, ServiceVersion: version, Env: env, Insecure: env == "dev", SamplingRatio: 1,
	})
	if err != nil {
		return err
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	_ = metrics.New(serviceName, reg)

	mux := http.NewServeMux()
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
	logger.Info("started", slog.String("addr", httpAddr))
	return runner.Wait(ctx)
}
`

const tmplService = `// Package service is where your domain logic lives. Edit this file.
package service

type Server struct{}

func New() *Server { return &Server{} }
`

const tmplChart = `apiVersion: v2
name: {{.Name}}
description: AegisVision {{.Plane}}-plane {{.Tier}} service.
type: application
version: 0.1.0
appVersion: "0.1.0"
`

const tmplValues = `image: { repository: aegisvision/{{.Name}}, tag: "0.1.0", pullPolicy: IfNotPresent }
replicaCount: 2
env: prod
logLevel: info
resources:
  requests: { cpu: 100m, memory: 128Mi }
  limits:   { cpu: "1",  memory: 512Mi }
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 8
  targetCPUUtilizationPercentage: 70
priorityClassName: {{.PriorityClass}}
serviceMonitor: { enabled: true, interval: 30s }
`

const tmplSA = `apiVersion: v1
kind: ServiceAccount
metadata: { name: {{.Name}}, namespace: aegis-system }
`

const tmplDeployment = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{.Name}}
  namespace: aegis-system
  labels: { app.kubernetes.io/name: {{.Name}} }
spec:
  replicas: {{"{{ .Values.replicaCount }}"}}
  selector: { matchLabels: { app.kubernetes.io/name: {{.Name}} } }
  template:
    metadata:
      labels: { app.kubernetes.io/name: {{.Name}} }
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "{{.HealthPort}}"
        prometheus.io/path: /metrics
    spec:
      serviceAccountName: {{.Name}}
      priorityClassName: {{"{{ .Values.priorityClassName | quote }}"}}
      securityContext: { runAsNonRoot: true, runAsUser: 65532, fsGroup: 65532, seccompProfile: { type: RuntimeDefault } }
      containers:
        - name: {{.Name}}
          image: "{{"{{ .Values.image.repository }}:{{ .Values.image.tag }}"}}"
          imagePullPolicy: {{"{{ .Values.image.pullPolicy }}"}}
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities: { drop: [ALL] }
          ports:
            - { name: http, containerPort: {{.Port}} }
            - { name: metrics, containerPort: {{.HealthPort}} }
          env:
            - { name: AEGIS_ENV, value: {{"{{ .Values.env | quote }}"}} }
            - { name: AEGIS_LOG_LEVEL, value: {{"{{ .Values.logLevel | quote }}"}} }
            - { name: AEGIS_HTTP_ADDR, value: ":{{.Port}}" }
            - { name: AEGIS_HEALTH_ADDR, value: ":{{.HealthPort}}" }
          readinessProbe: { httpGet: { path: /readyz, port: metrics }, initialDelaySeconds: 2, periodSeconds: 5 }
          livenessProbe:  { httpGet: { path: /healthz, port: metrics }, initialDelaySeconds: 10, periodSeconds: 10 }
          resources: {{"{{ toYaml .Values.resources | nindent 12 }}"}}
`

const tmplService_ = `apiVersion: v1
kind: Service
metadata: { name: {{.Name}}, namespace: aegis-system, labels: { app.kubernetes.io/name: {{.Name}} } }
spec:
  selector: { app.kubernetes.io/name: {{.Name}} }
  ports:
    - { name: http, port: {{.Port}}, targetPort: {{.Port}}, appProtocol: http }
    - { name: metrics, port: {{.HealthPort}}, targetPort: {{.HealthPort}} }
`

const tmplPeerAuth = `apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata: { name: {{.Name}}-mtls-strict, namespace: aegis-system }
spec:
  selector: { matchLabels: { app.kubernetes.io/name: {{.Name}} } }
  mtls: { mode: STRICT }
`

const tmplAuthZ = `apiVersion: security.istio.io/v1
kind: AuthorizationPolicy
metadata: { name: {{.Name}}, namespace: aegis-system }
spec:
  selector: { matchLabels: { app.kubernetes.io/name: {{.Name}} } }
  action: ALLOW
  rules:
    - from:
        - source: { principals: [cluster.local/ns/aegis-system/sa/api-gateway] }
      to: [{ operation: { ports: ["{{.Port}}"] } }]
    - from:
        - source: { principals: [cluster.local/ns/aegis-observability/sa/prometheus] }
      to: [{ operation: { ports: ["{{.HealthPort}}"] } }]
`

const tmplNetPol = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: { name: {{.Name}}, namespace: aegis-system }
spec:
  podSelector: { matchLabels: { app.kubernetes.io/name: {{.Name}} } }
  policyTypes: [Ingress, Egress]
  ingress:
    - from:
        - podSelector: { matchLabels: { app.kubernetes.io/name: api-gateway } }
      ports: [{ port: {{.Port}}, protocol: TCP }]
    - from:
        - namespaceSelector: { matchLabels: { kubernetes.io/metadata.name: aegis-observability } }
          podSelector: { matchLabels: { app.kubernetes.io/name: prometheus } }
      ports: [{ port: {{.HealthPort}}, protocol: TCP }]
`

const tmplHPA = `{{"{{- if .Values.autoscaling.enabled }}"}}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata: { name: {{.Name}}, namespace: aegis-system }
spec:
  scaleTargetRef: { apiVersion: apps/v1, kind: Deployment, name: {{.Name}} }
  minReplicas: {{"{{ .Values.autoscaling.minReplicas }}"}}
  maxReplicas: {{"{{ .Values.autoscaling.maxReplicas }}"}}
  metrics:
    - type: Resource
      resource: { name: cpu, target: { type: Utilization, averageUtilization: {{"{{ .Values.autoscaling.targetCPUUtilizationPercentage }}"}} } }
{{"{{- end }}"}}
`

const tmplSM = `{{"{{- if .Values.serviceMonitor.enabled }}"}}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata: { name: {{.Name}}, namespace: aegis-system, labels: { release: prometheus } }
spec:
  selector: { matchLabels: { app.kubernetes.io/name: {{.Name}} } }
  endpoints: [{ port: metrics, interval: {{"{{ .Values.serviceMonitor.interval }}"}}, path: /metrics }]
{{"{{- end }}"}}
`

const tmplPDB = `apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: { name: {{.Name}}, namespace: aegis-system }
spec:
  minAvailable: 50%
  selector: { matchLabels: { app.kubernetes.io/name: {{.Name}} } }
`
