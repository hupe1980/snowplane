// Package sharding provides a deterministic, hash-based event predicate for
// horizontally partitioning CRD reconciliation across multiple controller
// manager replicas.
//
// Each manager instance is assigned a (shardID, shardCount) tuple via CLI
// flags. The predicate hashes the object's namespace + name to a shard index
// and accepts the event only when the hash maps to this manager's shard.
//
// Key properties:
//   - Deterministic: the same object always maps to the same shard.
//   - Uniform: FNV-1a distributes objects evenly across shards.
//   - Zero coordination: no leader election or external state is required.
//   - Supports dynamic rescaling: changing shardCount re-balances objects on
//     the next reconcile cycle (brief duplicate processing during rollout is
//     harmless because reconciliation is idempotent).
package sharding

import (
	"fmt"
	"hash/fnv"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Options configures the sharding predicate.
type Options struct {
	// ShardID is the zero-based index of this manager instance.
	ShardID int

	// ShardCount is the total number of manager shards.
	// 1 means sharding is disabled (single instance).
	ShardCount int
}

// Enabled returns true when horizontal sharding is active.
func (o Options) Enabled() bool {
	return o.ShardCount > 1
}

// Validate returns an error if the options are invalid.
func (o Options) Validate() error {
	if o.ShardCount < 1 {
		return fmt.Errorf("shard-count must be >= 1, got %d", o.ShardCount)
	}

	if o.ShardID < 0 || o.ShardID >= o.ShardCount {
		return fmt.Errorf("shard-id must be in [0, %d), got %d", o.ShardCount, o.ShardID)
	}

	return nil
}

// NewPredicate returns a predicate that accepts events only for objects whose
// deterministic hash maps to the given shard.
//
// When shardCount <= 1, the predicate accepts all events (no-op pass-through).
func NewPredicate(opts Options) predicate.Predicate {
	if !opts.Enabled() {
		return predicate.Funcs{} // accept everything
	}

	owns := func(obj client.Object) bool {
		return objectShard(obj, opts.ShardCount) == opts.ShardID
	}

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return owns(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return owns(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return owns(e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return owns(e.Object)
		},
	}
}

// objectShard deterministically maps an object to a shard index using FNV-1a
// on "namespace/name". For cluster-scoped resources (empty namespace), the key
// is just the name.
func objectShard(obj client.Object, shardCount int) int {
	key := obj.GetName()
	if ns := obj.GetNamespace(); ns != "" {
		key = ns + "/" + key
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(key)) // Write never errors for fnv

	return int(h.Sum32() % uint32(shardCount)) //nolint:gosec // intentional modulo
}
