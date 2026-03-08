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
