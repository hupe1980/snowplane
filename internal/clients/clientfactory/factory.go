// Package clientfactory manages cached Snowflake client instances keyed by provider name.
package clientfactory

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

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

// ClientFactory creates and caches Snowflake clients keyed by a provider name
// and a config hash. When the hash changes, the old client is closed and a new
// one is created. An optional MaxSize limits the number of cached clients; when
// exceeded the least-recently-used client is evicted.
type ClientFactory struct {
	mu      sync.RWMutex
	clients map[string]cachedClient
	order   []string // insertion/access order for LRU eviction
	newFn   func(cfg snowflake.Config) (SnowflakeClient, error)
	maxSize int // 0 = unlimited
}

type cachedClient struct {
	client SnowflakeClient
	hash   string
}

// NewClientFactory creates a new ClientFactory with the default constructor.
// Use WithMaxSize to set an upper bound on cached clients.
func NewClientFactory() *ClientFactory {
	return &ClientFactory{
		clients: make(map[string]cachedClient),
		newFn: func(cfg snowflake.Config) (SnowflakeClient, error) {
			return snowflake.NewClient(cfg)
		},
	}
}

// WithMaxSize sets the maximum number of cached clients. When exceeded,
// the least-recently-used client is evicted. 0 = unlimited (default).
func (f *ClientFactory) WithMaxSize(n int) *ClientFactory {
	f.maxSize = n
	return f
}

// NewTestClientFactoryWithFn creates a ClientFactory with a custom constructor for testing.
func NewTestClientFactoryWithFn(fn func(cfg snowflake.Config) (SnowflakeClient, error)) *ClientFactory {
	return &ClientFactory{
		clients: make(map[string]cachedClient),
		newFn:   fn,
	}
}

// GetOrCreate returns a cached client for the provider. If the config hash has
// changed, the old client is closed and a new one is created.
func (f *ClientFactory) GetOrCreate(provider string, hash string, cfg snowflake.Config) (SnowflakeClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Return cached client if hash matches.
	if cc, ok := f.clients[provider]; ok && cc.hash == hash {
		f.touchOrder(provider)
		return cc.client, nil
	}

	// Close stale client if it exists.
	if cc, ok := f.clients[provider]; ok {
		_ = cc.client.Close()
		delete(f.clients, provider)
		f.removeFromOrder(provider)
	}

	// Evict LRU client if at capacity.
	if f.maxSize > 0 && len(f.clients) >= f.maxSize {
		f.evictLRU()
	}

	client, err := f.newFn(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating snowflake client for provider %q: %w", provider, err)
	}

	f.clients[provider] = cachedClient{client: client, hash: hash}
	f.order = append(f.order, provider)

	metrics.ClientPoolSize.Set(float64(len(f.clients)))

	return client, nil
}

// touchOrder moves the provider to the end of the LRU order (most-recently-used).
// Must be called under write lock.
func (f *ClientFactory) touchOrder(provider string) {
	for i, p := range f.order {
		if p == provider {
			f.order = append(f.order[:i], f.order[i+1:]...)
			f.order = append(f.order, provider)

			return
		}
	}
	// Not found — append.
	f.order = append(f.order, provider)
}

// removeFromOrder removes the provider from the LRU order.
// Must be called under write lock.
func (f *ClientFactory) removeFromOrder(provider string) {
	for i, p := range f.order {
		if p == provider {
			f.order = append(f.order[:i], f.order[i+1:]...)
			return
		}
	}
}

// evictLRU closes and removes the least-recently-used client.
// Must be called under write lock.
func (f *ClientFactory) evictLRU() {
	if len(f.order) == 0 {
		return
	}

	victim := f.order[0]
	f.order = f.order[1:]

	if cc, ok := f.clients[victim]; ok {
		_ = cc.client.Close()
		delete(f.clients, victim)
	}
}

// Evict closes and removes the client for the given provider.
func (f *ClientFactory) Evict(provider string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if cc, ok := f.clients[provider]; ok {
		_ = cc.client.Close()
		delete(f.clients, provider)
		f.removeFromOrder(provider)

		metrics.ClientPoolSize.Set(float64(len(f.clients)))
	}
}

// HasStaleHash returns true if the factory has a cached client for the provider
// whose hash differs from the given hash. This indicates credentials or config
// have changed and the client will be replaced on the next GetOrCreate call.
func (f *ClientFactory) HasStaleHash(provider, hash string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	cc, ok := f.clients[provider]
	if !ok {
		return false
	}

	return cc.hash != hash
}

// Close closes all cached clients.
func (f *ClientFactory) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for name, cc := range f.clients {
		_ = cc.client.Close()
		delete(f.clients, name)
	}

	f.order = nil
}

// CheckHealth pings all cached Snowflake clients and returns an error if any
// client is unreachable. It satisfies the healthz.Checker interface
// (func(*http.Request) error) and is used as a readiness probe to verify
// Snowflake connectivity. When no clients are cached (e.g. at startup before
// any ProviderConfig is reconciled), the check passes.
func (f *ClientFactory) CheckHealth(r *http.Request) error {
	f.mu.RLock()
	// Snapshot under read lock to avoid holding the lock during I/O.
	providers := make([]string, 0, len(f.clients))
	clients := make([]SnowflakeClient, 0, len(f.clients))

	for p, cc := range f.clients {
		providers = append(providers, p)
		clients = append(clients, cc.client)
	}

	f.mu.RUnlock()

	baseCtx := context.Background()
	if r != nil {
		baseCtx = r.Context()
	}

	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()

	for i, c := range clients {
		if err := c.Ping(ctx); err != nil {
			return fmt.Errorf("snowflake connectivity check failed for provider %q: %w", providers[i], err)
		}
	}

	return nil
}
