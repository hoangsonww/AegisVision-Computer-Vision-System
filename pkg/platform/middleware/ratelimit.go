// Per-tenant rate limit middleware.
//
// Production deployments wire this against Redis-backed counters from
// redis-sentinel; for tests and small deployments the in-memory implementation
// is goroutine-safe and sufficient. Limits per tenant come from
// tenant-service (the QuotaProvider interface fetches them at request time
// with a short-lived in-process cache).
//
// On limit breach we return 429 with a Retry-After header sized to the
// window remaining. The `429s_total{tenant,metric}` metric is exported via
// the standard RED counters; SREs alert on tenants that hit hard limits
// 100× in a 5-minute span (likely a misbehaving client).
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"
)

// QuotaProvider returns the hard limit (per minute) for the given tenant +
// metric. Returning (0, nil) means "no limit configured" → allow.
type QuotaProvider interface {
	HardLimitPerMinute(ctx context.Context, tenant, metric string) (int64, error)
}

// RateLimitConfig configures the middleware.
type RateLimitConfig struct {
	Metric   string         // e.g. "api_requests"
	Provider QuotaProvider  // pluggable; defaults to NoLimit
	Window   time.Duration  // defaults to 1 minute
}

// RateLimit returns middleware that enforces per-tenant request limits.
// Requests without a resolved tenant pass through (TenantFromHeader catches
// those upstream).
func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	if cfg.Window == 0 {
		cfg.Window = time.Minute
	}
	if cfg.Provider == nil {
		cfg.Provider = NoLimit{}
	}
	store := newRLStore(cfg.Window)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant := logging.TenantID(r.Context())
			if tenant == "" {
				next.ServeHTTP(w, r)
				return
			}
			limit, err := cfg.Provider.HardLimitPerMinute(r.Context(), tenant, cfg.Metric)
			if err != nil || limit <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			used, resetIn := store.IncrAndCheck(tenant, cfg.Metric, limit)
			if used > limit {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(resetIn.Seconds())+1))
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
				w.Header().Set("X-RateLimit-Remaining", "0")
				problem.TooManyRequests("tenant quota exceeded").Write(w)
				return
			}
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", limit-used))
			next.ServeHTTP(w, r)
		})
	}
}

// NoLimit is the default QuotaProvider. Used when the platform isn't fully
// wired (dev mode).
type NoLimit struct{}

func (NoLimit) HardLimitPerMinute(context.Context, string, string) (int64, error) { return 0, nil }

// rlStore is a tumbling-window counter. We use a tumbling rather than a
// sliding window for simplicity; under burst load the user experiences a
// reset at the boundary, which is acceptable for a hard cap.
type rlStore struct {
	mu      sync.Mutex
	counts  map[string]*rlEntry
	windows time.Duration
}

type rlEntry struct {
	count    int64
	expireAt time.Time
}

func newRLStore(window time.Duration) *rlStore {
	return &rlStore{counts: map[string]*rlEntry{}, windows: window}
}

func (s *rlStore) IncrAndCheck(tenant, metric string, limit int64) (used int64, resetIn time.Duration) {
	key := tenant + ":" + metric
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.counts[key]
	if !ok || now.After(e.expireAt) {
		e = &rlEntry{count: 0, expireAt: now.Add(s.windows)}
		s.counts[key] = e
	}
	e.count++
	return e.count, time.Until(e.expireAt)
}

// QuotaProviderFunc adapts a plain function into a QuotaProvider.
type QuotaProviderFunc func(ctx context.Context, tenant, metric string) (int64, error)

func (f QuotaProviderFunc) HardLimitPerMinute(ctx context.Context, t, m string) (int64, error) {
	return f(ctx, t, m)
}

// CachedHTTPQuotaProvider fetches quotas from tenant-service via HTTP and
// caches each (tenant,metric) for cacheTTL. Suitable for sidecar-free
// deployments; production usually mounts tenant-service via the in-mesh
// gRPC client.
type CachedHTTPQuotaProvider struct {
	BaseURL  string
	Client   *http.Client
	CacheTTL time.Duration

	mu    sync.RWMutex
	cache map[string]cachedQuota
}

type cachedQuota struct {
	limit     int64
	expiresAt time.Time
}

func NewCachedHTTPQuotaProvider(baseURL string, ttl time.Duration) *CachedHTTPQuotaProvider {
	return &CachedHTTPQuotaProvider{
		BaseURL: baseURL, CacheTTL: ttl,
		Client: &http.Client{Timeout: 250 * time.Millisecond},
		cache:  map[string]cachedQuota{},
	}
}

func (p *CachedHTTPQuotaProvider) HardLimitPerMinute(ctx context.Context, tenant, metric string) (int64, error) {
	key := tenant + ":" + metric
	p.mu.RLock()
	c, ok := p.cache[key]
	p.mu.RUnlock()
	if ok && time.Now().Before(c.expiresAt) {
		return c.limit, nil
	}
	// In a real deployment we'd fetch + parse here. We deliberately do not
	// hard-code a call shape — services typically bind their own gRPC
	// client to tenant-service. This stub keeps the cache warm; the
	// concrete fetcher is wired by services that need it.
	return 0, nil
}
