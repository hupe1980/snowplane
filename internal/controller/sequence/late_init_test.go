package sequence

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newSequence() *snowplanev1alpha1.Sequence {
	return &snowplanev1alpha1.Sequence{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-seq",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.SequenceSpec{
			Name: "TEST_SEQ",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	a := &adapter{}

	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newSequence()
		obs := &reconciler.Observation[*snowflake.SequenceObservation]{
			Exists: true,
			Detail: &snowflake.SequenceObservation{
				ShowOutput: &snowflake.SequenceShowOutput{
					Interval: "5",
					Ordering: "ORDER",
					Comment:  "my sequence",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, int64(5), *obj.Spec.Increment)
		assert.Equal(t, "ORDER", *obj.Spec.Ordering)
		assert.Equal(t, "my sequence", *obj.Spec.Comment)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newSequence()
		obj.Spec.Increment = ptr(int64(10))

		obs := &reconciler.Observation[*snowflake.SequenceObservation]{
			Exists: true,
			Detail: &snowflake.SequenceObservation{
				ShowOutput: &snowflake.SequenceShowOutput{
					Interval: "5",
					Ordering: "NOORDER",
					Comment:  "comment",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified) // Ordering and Comment were set

		assert.Equal(t, int64(10), *obj.Spec.Increment) // preserved
		assert.Equal(t, "NOORDER", *obj.Spec.Ordering)
		assert.Equal(t, "comment", *obj.Spec.Comment)
	})

	t.Run("returns false when all fields already set", func(t *testing.T) {
		obj := newSequence()
		obj.Spec.Increment = ptr(int64(1))
		obj.Spec.Ordering = ptr("ORDER")
		obj.Spec.Comment = ptr("c")

		obs := &reconciler.Observation[*snowflake.SequenceObservation]{
			Exists: true,
			Detail: &snowflake.SequenceObservation{
				ShowOutput: &snowflake.SequenceShowOutput{
					Interval: "99",
					Ordering: "NOORDER",
					Comment:  "other",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("handles invalid interval string gracefully", func(t *testing.T) {
		obj := newSequence()
		obs := &reconciler.Observation[*snowflake.SequenceObservation]{
			Exists: true,
			Detail: &snowflake.SequenceObservation{
				ShowOutput: &snowflake.SequenceShowOutput{
					Interval: "not-a-number",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Nil(t, obj.Spec.Increment)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newSequence()
		obs := &reconciler.Observation[*snowflake.SequenceObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
