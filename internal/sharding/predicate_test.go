package sharding

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func newObj(namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}

func TestOptions_Enabled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		opts     Options
		expected bool
	}{
		{"shard count 0", Options{ShardID: 0, ShardCount: 0}, false},
		{"shard count 1", Options{ShardID: 0, ShardCount: 1}, false},
		{"shard count 2", Options{ShardID: 0, ShardCount: 2}, true},
		{"shard count 10", Options{ShardID: 3, ShardCount: 10}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, tc.opts.Enabled())
		})
	}
}

func TestNewPredicate_Disabled(t *testing.T) {
	t.Parallel()
	p := NewPredicate(Options{ShardID: 0, ShardCount: 1})
	obj := newObj("default", "test-obj")
	assert.True(t, p.Create(event.CreateEvent{Object: obj}))
	assert.True(t, p.Update(event.UpdateEvent{ObjectNew: obj}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: obj}))
	assert.True(t, p.Generic(event.GenericEvent{Object: obj}))
}

func TestNewPredicate_Deterministic(t *testing.T) {
	t.Parallel()
	opts := Options{ShardID: 0, ShardCount: 3}
	p := NewPredicate(opts)
	obj := newObj("default", "my-resource")
	result1 := p.Create(event.CreateEvent{Object: obj})
	result2 := p.Create(event.CreateEvent{Object: obj})
	assert.Equal(t, result1, result2, "predicate should be deterministic")
}

func TestNewPredicate_DistributesAcrossShards(t *testing.T) {
	t.Parallel()
	shardCount := 3
	accepted := make([]int, shardCount)
	for i := range 100 {
		obj := newObj("default", fmt.Sprintf("obj-%d", i))
		for shard := range shardCount {
			p := NewPredicate(Options{ShardID: shard, ShardCount: shardCount})
			if p.Create(event.CreateEvent{Object: obj}) {
				accepted[shard]++
			}
		}
	}
	for shard, count := range accepted {
		assert.Greater(t, count, 0, "shard %d should accept at least one object", shard)
	}
	total := 0
	for _, c := range accepted {
		total += c
	}
	assert.Equal(t, 100, total, "each object should be accepted by exactly one shard")
}

func TestNewPredicate_ExactlyOneShard(t *testing.T) {
	t.Parallel()
	shardCount := 5
	obj := newObj("my-ns", "my-resource")
	acceptedBy := 0
	for shard := range shardCount {
		p := NewPredicate(Options{ShardID: shard, ShardCount: shardCount})
		if p.Create(event.CreateEvent{Object: obj}) {
			acceptedBy++
		}
	}
	assert.Equal(t, 1, acceptedBy, "object should be accepted by exactly one shard")
}

func TestNewPredicate_AllEventTypes(t *testing.T) {
	t.Parallel()
	opts := Options{ShardID: 0, ShardCount: 2}
	p := NewPredicate(opts)
	obj := newObj("default", "test")
	createResult := p.Create(event.CreateEvent{Object: obj})
	updateResult := p.Update(event.UpdateEvent{ObjectNew: obj})
	delResult := p.Delete(event.DeleteEvent{Object: obj})
	genericResult := p.Generic(event.GenericEvent{Object: obj})
	assert.Equal(t, createResult, updateResult)
	assert.Equal(t, createResult, delResult)
	assert.Equal(t, createResult, genericResult)
}

func TestObjectShard_Deterministic(t *testing.T) {
	t.Parallel()
	obj := newObj("default", "test")
	shard1 := objectShard(obj, 3)
	shard2 := objectShard(obj, 3)
	assert.Equal(t, shard1, shard2, "objectShard should be deterministic")
	assert.GreaterOrEqual(t, shard1, 0)
	assert.Less(t, shard1, 3)
}

func TestObjectShard_ClusterScoped(t *testing.T) {
	t.Parallel()
	// Cluster-scoped objects have empty namespace — key is just the name.
	obj := newObj("", "my-cluster-resource")
	shard := objectShard(obj, 5)
	assert.GreaterOrEqual(t, shard, 0)
	assert.Less(t, shard, 5)

	// Same object always maps to the same shard.
	assert.Equal(t, shard, objectShard(obj, 5))

	// A namespaced object with the same name maps differently (different key).
	nsObj := newObj("default", "my-cluster-resource")
	nsShard := objectShard(nsObj, 5)
	// They CAN be equal by coincidence, but the keys are different.
	assert.GreaterOrEqual(t, nsShard, 0)
	assert.Less(t, nsShard, 5)
}

func TestNewPredicate_UniformDistribution(t *testing.T) {
	t.Parallel()
	shardCount := 5
	accepted := make([]int, shardCount)
	numObjects := 1000
	for i := range numObjects {
		obj := newObj("default", fmt.Sprintf("resource-%d", i))
		for shard := range shardCount {
			p := NewPredicate(Options{ShardID: shard, ShardCount: shardCount})
			if p.Create(event.CreateEvent{Object: obj}) {
				accepted[shard]++
			}
		}
	}
	// Each shard should get roughly 1/shardCount of objects (±20% tolerance).
	expected := numObjects / shardCount
	for shard, count := range accepted {
		assert.Greater(t, count, expected*80/100,
			"shard %d accepted too few objects: %d (expected ~%d)", shard, count, expected)
		assert.Less(t, count, expected*120/100,
			"shard %d accepted too many objects: %d (expected ~%d)", shard, count, expected)
	}
}

func TestOptions_EnabledEdgeCases(t *testing.T) {
	t.Parallel()
	assert.False(t, Options{ShardID: 0, ShardCount: -1}.Enabled(), "negative count")
	assert.False(t, Options{ShardID: 0, ShardCount: 0}.Enabled(), "zero count")
	assert.False(t, Options{ShardID: 0, ShardCount: 1}.Enabled(), "single instance")
	assert.True(t, Options{ShardID: 0, ShardCount: 2}.Enabled(), "two shards")
	assert.True(t, Options{ShardID: 99, ShardCount: 100}.Enabled(), "hundred shards")
}

func TestOptions_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
		errMsg  string
	}{
		{"valid single instance", Options{ShardID: 0, ShardCount: 1}, false, ""},
		{"valid shard 0 of 3", Options{ShardID: 0, ShardCount: 3}, false, ""},
		{"valid shard 2 of 3", Options{ShardID: 2, ShardCount: 3}, false, ""},
		{"valid shard 99 of 100", Options{ShardID: 99, ShardCount: 100}, false, ""},
		{"zero count", Options{ShardID: 0, ShardCount: 0}, true, "shard-count must be >= 1"},
		{"negative count", Options{ShardID: 0, ShardCount: -1}, true, "shard-count must be >= 1"},
		{"negative shard id", Options{ShardID: -1, ShardCount: 3}, true, "shard-id must be in [0, 3)"},
		{"shard id equals count", Options{ShardID: 3, ShardCount: 3}, true, "shard-id must be in [0, 3)"},
		{"shard id exceeds count", Options{ShardID: 5, ShardCount: 3}, true, "shard-id must be in [0, 3)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.opts.Validate()
			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
