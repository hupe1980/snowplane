package accountrole

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newAccountRole() *snowplanev1alpha1.AccountRole {
	return &snowplanev1alpha1.AccountRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.AccountRoleSpec{
			Name: "TEST_ROLE",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	a := &adapter{}

	t.Run("fills comment from observation", func(t *testing.T) {
		obj := newAccountRole()
		obs := &reconciler.Observation[*snowflake.AccountRoleObservation]{
			Exists: true,
			Detail: &snowflake.AccountRoleObservation{
				ShowOutput: &snowflake.AccountRoleShowOutput{
					Comment: "role comment",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "role comment", *obj.Spec.Comment)
	})

	t.Run("does not overwrite existing comment", func(t *testing.T) {
		obj := newAccountRole()
		obj.Spec.Comment = ptr("user comment")

		obs := &reconciler.Observation[*snowflake.AccountRoleObservation]{
			Exists: true,
			Detail: &snowflake.AccountRoleObservation{
				ShowOutput: &snowflake.AccountRoleShowOutput{
					Comment: "snowflake comment",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
	})

	t.Run("returns false when observation is nil", func(t *testing.T) {
		obj := newAccountRole()
		obs := &reconciler.Observation[*snowflake.AccountRoleObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("skips empty comment", func(t *testing.T) {
		obj := newAccountRole()
		obs := &reconciler.Observation[*snowflake.AccountRoleObservation]{
			Exists: true,
			Detail: &snowflake.AccountRoleObservation{
				ShowOutput: &snowflake.AccountRoleShowOutput{
					Comment: "",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Nil(t, obj.Spec.Comment)
	})
}
