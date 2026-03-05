package databaserole

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newDatabaseRole() *snowplanev1alpha1.DatabaseRole {
	return &snowplanev1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dbrole",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseRoleSpec{
			Name: "TEST_DB_ROLE",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills comment from observation", func(t *testing.T) {
		obj := newDatabaseRole()
		obs := &reconciler.Observation[*snowflake.DatabaseRoleObservation]{
			Exists: true,
			Detail: &snowflake.DatabaseRoleObservation{
				ShowOutput: &snowplanev1alpha1.DatabaseRoleShowOutput{
					Comment: "dbrole comment",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "dbrole comment", *obj.Spec.Comment)
	})

	t.Run("does not overwrite existing comment", func(t *testing.T) {
		obj := newDatabaseRole()
		obj.Spec.Comment = ptr("user comment")

		obs := &reconciler.Observation[*snowflake.DatabaseRoleObservation]{
			Exists: true,
			Detail: &snowflake.DatabaseRoleObservation{
				ShowOutput: &snowplanev1alpha1.DatabaseRoleShowOutput{
					Comment: "snowflake comment",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
	})

	t.Run("returns false when observation is nil", func(t *testing.T) {
		obj := newDatabaseRole()
		obs := &reconciler.Observation[*snowflake.DatabaseRoleObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("skips empty comment", func(t *testing.T) {
		obj := newDatabaseRole()
		obs := &reconciler.Observation[*snowflake.DatabaseRoleObservation]{
			Exists: true,
			Detail: &snowflake.DatabaseRoleObservation{
				ShowOutput: &snowplanev1alpha1.DatabaseRoleShowOutput{
					Comment: "",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Nil(t, obj.Spec.Comment)
	})
}
