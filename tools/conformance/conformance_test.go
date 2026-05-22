// Conformance test for every Helm chart under deploy/helm.
//
// The architecture doc declares a small set of structural commitments that
// every service must satisfy. This test enforces them by walking the
// directory tree — failing CI is the only way to keep a service from
// shipping without (say) a mTLS STRICT PeerAuthentication or with a root
// container.
//
// The checks are intentionally syntactic — we read raw YAML rather than
// pull in a Kubernetes type set. That keeps the test fast and dep-free
// and the rules easy to audit.
package conformance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryChart_MeetsGoldenPath(t *testing.T) {
	root := chartRoot(t)
	charts, err := filepath.Glob(filepath.Join(root, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(charts) == 0 {
		t.Fatal("no charts found under deploy/helm")
	}

	// Charts that intentionally don't fit the service pattern.
	//   - "nats" is a stateful infrastructure chart governed by the
	//     platform-wide network policy + a dedicated SPIFFE identity.
	//   - "edge-profile" is a values-overlay chart that ships namespaces +
	//     priority classes + a ConfigMap, not a Deployment; the edge
	//     services use existing per-service charts.
	infrastructure := map[string]bool{"nats": true, "edge-profile": true}

	for _, dir := range charts {
		base := filepath.Base(dir)
		if base == "" || strings.HasPrefix(base, ".") {
			continue
		}
		t.Run(base, func(t *testing.T) {
			tmpl := filepath.Join(dir, "templates")
			if _, err := os.Stat(tmpl); err != nil {
				t.Fatalf("missing templates dir: %v", err)
			}
			all := readAll(t, tmpl)
			vals := readValues(dir)
			infra := infrastructure[base]

			// 1. Mesh posture — mTLS STRICT + AuthorizationPolicy +
			//    namespace-scoped NetworkPolicy. Infra charts use the
			//    platform-wide policy instead.
			if !infra {
				if !contains(all, "kind: PeerAuthentication") {
					t.Errorf("missing PeerAuthentication")
				}
				if !contains(all, "mode: STRICT") {
					t.Errorf("PeerAuthentication is not STRICT (Istio Ambient mTLS commitment)")
				}
				if !contains(all, "kind: AuthorizationPolicy") {
					t.Errorf("missing AuthorizationPolicy")
				}
				if !contains(all, "kind: NetworkPolicy") {
					t.Errorf("missing NetworkPolicy (Cilium default-deny override)")
				}
			}

			// 2. Observability.
			if !contains(all, "kind: ServiceMonitor") && !infra {
				t.Errorf("missing ServiceMonitor (observability commitment)")
			}

			// 3. Scaling shape — HPA + PDB. StatefulSet infra charts exempt.
			if !infra {
				if !contains(all, "kind: HorizontalPodAutoscaler") {
					t.Errorf("missing HorizontalPodAutoscaler")
				}
				if !contains(all, "kind: PodDisruptionBudget") {
					t.Errorf("missing PodDisruptionBudget")
				}
			}

			// 4. Pod security posture. Either the rendered template carries
			//    a hard-coded `true`, OR the template references a values
			//    knob whose values.yaml default is `true`.
			if !infra {
				if !securityTrue(all, vals, "runAsNonRoot") {
					t.Errorf("Deployment must declare runAsNonRoot: true")
				}
				if !contains(all, "seccompProfile") {
					t.Errorf("Deployment must declare seccompProfile")
				}
				if !securityTrue(all, vals, "readOnlyRootFilesystem") {
					t.Errorf("container must declare readOnlyRootFilesystem: true")
				}
				if !dropsAllCaps(all, vals) {
					t.Errorf("container must drop ALL capabilities")
				}
			}

			// 5. Service + priority class — required for service charts.
			if !infra {
				if !contains(all, "kind: Service") {
					t.Errorf("missing Service")
				}
				if !contains(all, "priorityClassName:") {
					t.Errorf("missing priorityClassName")
				}
			}

			// 6. Forbidden escapes.
			if contains(all, "privileged: true") {
				t.Errorf("forbidden: privileged: true")
			}
			if contains(all, "hostNetwork: true") {
				t.Errorf("forbidden: hostNetwork: true")
			}
		})
	}
}

func chartRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		p := filepath.Join(wd, "deploy", "helm")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		wd = filepath.Dir(wd)
	}
	t.Fatal("could not find deploy/helm")
	return ""
}

func readAll(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(string(body))
		b.WriteByte('\n')
	}
	return b.String()
}

func readValues(chart string) string {
	body, err := os.ReadFile(filepath.Join(chart, "values.yaml"))
	if err != nil {
		return ""
	}
	return string(body)
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

// securityTrue accepts either a literal `name: true` in templates or in
// values.yaml. Allows charts to template the value through values without
// failing the literal-text check.
func securityTrue(rendered, values, name string) bool {
	if strings.Contains(rendered, name+": true") {
		return true
	}
	if strings.Contains(rendered, name+":") && strings.Contains(values, name+": true") {
		return true
	}
	return false
}

// dropsAllCaps accepts the three rendering styles for "drop ALL".
func dropsAllCaps(rendered, values string) bool {
	if strings.Contains(rendered, "drop: [ALL]") {
		return true
	}
	if strings.Contains(rendered, "- ALL") && strings.Contains(rendered, "drop:") {
		return true
	}
	if strings.Contains(rendered, "drop:") && strings.Contains(values, "drop: [ALL]") {
		return true
	}
	return false
}
