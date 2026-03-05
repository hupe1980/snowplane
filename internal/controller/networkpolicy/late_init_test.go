package networkpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newNetworkPolicy() *snowplanev1alpha1.NetworkPolicy {
	return &snowplanev1alpha1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-np",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.NetworkPolicySpec{
			Name: "TEST_NP",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills comment from observation", func(t *testing.T) {
		obj := newNetworkPolicy()
		obs := &reconciler.Observation[*snowflake.NetworkPolicyObservation]{
			Exists: true,
			Detail: &snowflake.NetworkPolicyObservation{
				ShowOutput: &snowplanev1alpha1.NetworkPolicyShowOutput{
					Comment: "np comment",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "np comment", *obj.Spec.Comment)
	})

	t.Run("does not overwrite existing comment", func(t *testing.T) {
		obj := newNetworkPolicy()
		obj.Spec.Comment = ptr("user comment")

		obs := &reconciler.Observation[*snowflake.NetworkPolicyObservation]{
			Exists: true,
			Detail: &snowflake.NetworkPolicyObservation{
				ShowOutput: &snowplanev1alpha1.NetworkPolicyShowOutput{
					Comment: "snowflake comment",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newNetworkPolicy()

		modified := lateInitialize(obj, &reconciler.Observation[*snowflake.NetworkPolicyObservation]{
			Exists: true,
			Detail: nil,
		})
		assert.False(t, modified)
	})

	t.Run("skips empty comment", func(t *testing.T) {
		obj := newNetworkPolicy()

		obs := &reconciler.Observation[*snowflake.NetworkPolicyObservation]{
			Exists: true,
			Detail: &snowflake.NetworkPolicyObservation{
				ShowOutput: &snowplanev1alpha1.NetworkPolicyShowOutput{
					Comment: "",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Nil(t, obj.Spec.Comment)
	})
}
