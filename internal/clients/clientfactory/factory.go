// Package clientfactory manages cached Snowflake client instances keyed by provider name.
package clientfactory

import (
	"container/list"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/metrics"
)

// SnowflakeClient is the interface used by reconcilers for Snowflake operations.
// It combines health-check, lifecycle, SQL execution, and role-switching methods.
type SnowflakeClient interface {
	Ping(ctx context.Context) error
	Close() error

	// Embed SQL execution methods so resource clients can use the same
	// interface without needing a concrete *snowflake.Client.
	snowflake.SQLExecutor

	// WithRole pins a connection, switches to the given role, and returns a
	// scoped *Client plus a cleanup function that restores the original role.
	WithRole(ctx context.Context, role string) (*snowflake.Client, func(context.Context), error)
}

// StatsProvider is an optional interface implemented by Snowflake clients
// that expose sql.DB connection pool statistics.
type StatsProvider interface {
	Stats() sql.DBStats
}

// healthCheckTimeout is the maximum time allowed for a health check ping.
const healthCheckTimeout = 5 * time.Second

// ClientFactory creates and caches Snowflake clients keyed by a provider name
// and a config hash. When the hash changes, the old client is closed and a new
// one is created. An optional MaxSize limits the number of cached clients; when
// exceeded the least-recently-used client is evicted.
//
// An optional IdleTTL causes clients unused for longer than the TTL to be
// closed and recreated on the next GetOrCreate call. This prevents holding
// stale TCP connections or expired tokens indefinitely.
//
// LRU tracking uses container/list (doubly-linked list) with a map of
// provider→*list.Element for O(1) touch, remove, and eviction operations.
//
// Connection creation is deduplicated using singleflight: when multiple
// goroutines request the same provider concurrently, only one connection
// attempt is made and the result is shared. This prevents thundering-herd
// connection storms during controller restarts or bulk reconciliation.
type ClientFactory struct {
	mu       sync.RWMutex
	clients  map[string]*list.Element // provider → *list.Element (value is lruEntry)
	lruOrder *list.List               // front = LRU (oldest), back = MRU (newest)
	newFn    func(cfg snowflake.Config) (SnowflakeClient, error)
	maxSize  int                // 0 = unlimited
	idleTTL  time.Duration      // 0 = disabled (default)
	sfGroup  singleflight.Group // deduplicates concurrent connection attempts per provider

	// startupGrace is the duration after factory creation during which the
	// readiness probe passes even when no clients are cached. This prevents
	// the probe from failing during the initial startup window before any
	// ProviderConfig has been reconciled and a Snowflake client cached.
	startupGrace time.Duration
	startTime    time.Time

	// logger for non-critical warnings (e.g. Close errors during eviction).
	// Defaults to slog.Default() if not set.
	logger *slog.Logger
}

// lruEntry is the value stored in each list.Element.
type lruEntry struct {
	provider string
	cached   cachedClient
}

type cachedClient struct {
	client     SnowflakeClient
	hash       string
	lastAccess time.Time
}

// NewClientFactory creates a new ClientFactory with the default constructor.
// Use WithMaxSize to set an upper bound on cached clients.
func NewClientFactory() *ClientFactory {
	return &ClientFactory{
		clients:  make(map[string]*list.Element),
		lruOrder: list.New(),
		newFn: func(cfg snowflake.Config) (SnowflakeClient, error) {
			return snowflake.NewClient(cfg)
		},
		startTime: time.Now(),
		logger:    slog.Default(),
	}
}

// WithMaxSize sets the maximum number of cached clients. When exceeded,
// the least-recently-used client is evicted. 0 = unlimited (default).
func (f *ClientFactory) WithMaxSize(n int) *ClientFactory {
	f.maxSize = n
	return f
}

// WithIdleTTL sets the idle time-to-live for cached clients. Clients not
// accessed within this duration are closed and recreated on the next request.
// 0 = disabled (default, clients live until config changes or LRU eviction).
func (f *ClientFactory) WithIdleTTL(d time.Duration) *ClientFactory {
	f.idleTTL = d
	return f
}

// WithStartupGrace sets the duration after factory creation during which the
// readiness probe passes even when no clients are cached. This prevents the
// probe from failing during the initial startup window before any ProviderConfig
// has been reconciled. Default is 0 (no grace period).
func (f *ClientFactory) WithStartupGrace(d time.Duration) *ClientFactory {
	f.startupGrace = d
	return f
}

// WithLogger sets a custom logger for non-critical warnings (e.g. Close errors
// during eviction). Defaults to slog.Default().
func (f *ClientFactory) WithLogger(l *slog.Logger) *ClientFactory {
	f.logger = l
	return f
}

// NewTestClientFactoryWithFn creates a ClientFactory with a custom constructor for testing.
func NewTestClientFactoryWithFn(fn func(cfg snowflake.Config) (SnowflakeClient, error)) *ClientFactory {
	return &ClientFactory{
		clients:   make(map[string]*list.Element),
		lruOrder:  list.New(),
		newFn:     fn,
		startTime: time.Now(),
		logger:    slog.Default(),
	}
}

// GetOrCreate returns a cached client for the provider. If the config hash has
// changed or the client has exceeded the idle TTL, the old client is closed and
// a new one is created.
//
// Concurrent requests for the same provider are deduplicated via singleflight:
// only one connection attempt proceeds while others wait and receive the same
// result. This prevents thundering-herd connection storms during controller
// restarts or mass reconciliation.
func (f *ClientFactory) GetOrCreate(provider string, hash string, cfg snowflake.Config) (SnowflakeClient, error) {
	// Fast path: check cache under read lock. This avoids the write lock and
	// singleflight overhead for the common case (cache hit, same hash, TTL ok).
	if c, ok := f.tryGetCached(provider, hash); ok {
		return c, nil
	}

	// Slow path: use singleflight to deduplicate concurrent connection
	// attempts for the same provider. The singleflight key includes the hash
	// so that a config change doesn't share results with stale requests.
	sfKey := provider + "\x00" + hash

	v, err, _ := f.sfGroup.Do(sfKey, func() (any, error) {
		return f.getOrCreateLocked(provider, hash, cfg)
	})
	if err != nil {
		return nil, err
	}

	return v.(SnowflakeClient), nil //nolint:forcetypeassert // singleflight returns our type
}

// tryGetCached returns a cached client if the hash matches and TTL is valid.
// Returns (client, true) on cache hit, (nil, false) on miss or stale entry.
// On hit, promotes the entry to MRU position and updates lastAccess.
func (f *ClientFactory) tryGetCached(provider, hash string) (SnowflakeClient, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	elem, ok := f.clients[provider]
	if !ok {
		return nil, false
	}

	entry := elem.Value.(*lruEntry)
	if entry.cached.hash != hash {
		return nil, false // hash changed — need creation path
	}

	now := time.Now()
	if f.idleTTL > 0 && now.Sub(entry.cached.lastAccess) > f.idleTTL {
		return nil, false // expired — need creation path
	}

	entry.cached.lastAccess = now
	f.lruOrder.MoveToBack(elem) // O(1) LRU touch

	return entry.cached.client, true
}

// getOrCreateLocked performs the cache-miss path under a write lock.
// Called from within singleflight.Do, so at most one goroutine per provider
// executes this at a time; however, the write lock is still needed to protect
// the shared map/list from concurrent access by other providers.
func (f *ClientFactory) getOrCreateLocked(provider, hash string, cfg snowflake.Config) (SnowflakeClient, error) {
	f.mu.Lock()

	now := time.Now()

	// Double-check: another goroutine may have populated the cache between
	// the read-lock miss and acquiring the write lock.
	if elem, ok := f.clients[provider]; ok {
		entry := elem.Value.(*lruEntry)
		if entry.cached.hash == hash {
			if f.idleTTL == 0 || now.Sub(entry.cached.lastAccess) <= f.idleTTL {
				entry.cached.lastAccess = now
				f.lruOrder.MoveToBack(elem) // O(1) touch
				f.mu.Unlock()

				return entry.cached.client, nil
			}

			// Idle too long — evict and recreate below.
			f.closeClient(entry.cached.client, provider, "idle TTL expired")
			f.lruOrder.Remove(elem)
			delete(f.clients, provider)
		} else {
			// Hash changed — close stale client.
			f.closeClient(entry.cached.client, provider, "config hash changed")
			f.lruOrder.Remove(elem)
			delete(f.clients, provider)
		}
	}

	// Evict LRU client if at capacity.
	if f.maxSize > 0 && len(f.clients) >= f.maxSize {
		f.evictLRU()
	}

	// Release the lock during the potentially slow newFn call (network I/O).
	// This is the key optimization: other providers can be served from the
	// cache while this connection is being established.
	f.mu.Unlock()

	client, err := f.newFn(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating snowflake client for provider %q: %w", provider, err)
	}

	// Re-acquire the lock to insert the new client.
	f.mu.Lock()
	defer f.mu.Unlock()

	// Check again: a concurrent eviction or close could have happened while
	// we were creating the client without the lock.
	if elem, ok := f.clients[provider]; ok {
		// Another goroutine snuck in — close the duplicate we just created
		// and use the existing one.
		f.closeClient(client, provider, "duplicate creation race")

		entry := elem.Value.(*lruEntry)
		entry.cached.lastAccess = time.Now()
		f.lruOrder.MoveToBack(elem)

		return entry.cached.client, nil
	}

	entry := &lruEntry{
		provider: provider,
		cached:   cachedClient{client: client, hash: hash, lastAccess: time.Now()},
	}
	elem := f.lruOrder.PushBack(entry) // O(1) insert at MRU end
	f.clients[provider] = elem

	metrics.ClientPoolSize.Set(float64(len(f.clients)))

	return client, nil
}

// evictLRU closes and removes the least-recently-used client (front of list).
// Must be called under write lock. O(1).
func (f *ClientFactory) evictLRU() {
	front := f.lruOrder.Front()
	if front == nil {
		return
	}

	entry := front.Value.(*lruEntry)
	f.closeClient(entry.cached.client, entry.provider, "LRU eviction")
	f.lruOrder.Remove(front)
	delete(f.clients, entry.provider)
}

// closeClient closes a Snowflake client and logs any Close error at debug level.
// Close errors on database connections are rarely actionable, but logging aids
// post-mortem diagnostics.
func (f *ClientFactory) closeClient(c SnowflakeClient, provider, reason string) {
	if err := c.Close(); err != nil {
		f.logger.Debug("error closing Snowflake client",
			"provider", provider,
			"reason", reason,
			"error", err,
		)
	}
}

// Evict closes and removes the client for the given provider.
func (f *ClientFactory) Evict(provider string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if elem, ok := f.clients[provider]; ok {
		entry := elem.Value.(*lruEntry)
		f.closeClient(entry.cached.client, provider, "explicit eviction")
		f.lruOrder.Remove(elem)
		delete(f.clients, provider)

		metrics.ClientPoolSize.Set(float64(len(f.clients)))
	}
}

// HasStaleHash returns true if the factory has a cached client for the provider
// whose hash differs from the given hash. This indicates credentials or config
// have changed and the client will be replaced on the next GetOrCreate call.
func (f *ClientFactory) HasStaleHash(provider, hash string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	elem, ok := f.clients[provider]
	if !ok {
		return false
	}

	return elem.Value.(*lruEntry).cached.hash != hash
}

// Close closes all cached clients.
func (f *ClientFactory) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for name, elem := range f.clients {
		entry := elem.Value.(*lruEntry)
		f.closeClient(entry.cached.client, name, "factory shutdown")
		delete(f.clients, name)
	}

	f.lruOrder.Init() // reset list
}

// CheckHealth pings all cached Snowflake clients and returns an error if any
// client is unreachable. It satisfies the healthz.Checker interface
// (func(*http.Request) error) and is used as a readiness probe to verify
// Snowflake connectivity.
//
// All providers are checked — a combined error is returned listing every
// unhealthy provider, giving operators full outage visibility.
//
// During the startup grace period, the check passes even when no clients are
// cached (the operator hasn't reconciled any ProviderConfig yet). After the
// grace period, when no clients are cached, the check still passes — having
// zero providers is a valid steady-state configuration.
//
// When r is nil (e.g. in tests), context.Background() is used as the base
// context. The health check is bounded by a hardcoded 5-second timeout
// regardless of the request context.
func (f *ClientFactory) CheckHealth(r *http.Request) error {
	f.mu.RLock()
	// Snapshot under read lock to avoid holding the lock during I/O.
	providers := make([]string, 0, len(f.clients))
	clients := make([]SnowflakeClient, 0, len(f.clients))

	for _, elem := range f.clients {
		entry := elem.Value.(*lruEntry)
		providers = append(providers, entry.provider)
		clients = append(clients, entry.cached.client)
	}

	f.mu.RUnlock()

	// During startup grace period, skip connectivity checks — Snowflake
	// clients may not be cached yet.
	if f.startupGrace > 0 && len(clients) == 0 && time.Since(f.startTime) < f.startupGrace {
		return nil
	}

	baseCtx := context.Background()
	if r != nil {
		baseCtx = r.Context()
	}

	ctx, cancel := context.WithTimeout(baseCtx, healthCheckTimeout)
	defer cancel()

	// Ping all providers in parallel using errgroup. Errors are collected
	// concurrently and joined into a single combined error.
	var (
		mu   sync.Mutex
		errs []error
	)

	g, gCtx := errgroup.WithContext(ctx)

	for i, c := range clients {
		provider := providers[i]
		client := c

		g.Go(func() error {
			if err := client.Ping(gCtx); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("provider %q: %w", provider, err))
				mu.Unlock()
			}

			return nil // don't cancel other pings on individual failure
		})
	}

	_ = g.Wait() // errors are collected in errs, not returned by goroutines

	if len(errs) > 0 {
		return fmt.Errorf("snowflake connectivity check failed: %w", errors.Join(errs...))
	}

	return nil
}

// CollectDBStats publishes sql.DBStats metrics for all cached clients that
// support the StatsProvider interface. This should be called periodically
// (e.g., from a background ticker or alongside health checks).
//
// Like CheckHealth, this method snapshots clients under the read lock and
// then performs I/O (gauge updates) outside the lock to avoid holding the
// lock during Prometheus operations.
func (f *ClientFactory) CollectDBStats() {
	f.mu.RLock()

	type snapshot struct {
		provider string
		sp       StatsProvider
	}

	snaps := make([]snapshot, 0, len(f.clients))

	for _, elem := range f.clients {
		entry := elem.Value.(*lruEntry)
		if sp, ok := entry.cached.client.(StatsProvider); ok {
			snaps = append(snaps, snapshot{provider: entry.provider, sp: sp})
		}
	}

	f.mu.RUnlock()

	for _, s := range snaps {
		stats := s.sp.Stats()
		metrics.RecordDBStats(
			s.provider,
			stats.MaxOpenConnections,
			stats.OpenConnections,
			stats.InUse,
			stats.Idle,
			stats.WaitCount,
			stats.WaitDuration,
		)
	}
}
