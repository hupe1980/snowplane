package table

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newTable() *snowplanev1alpha1.Table {
	return &snowplanev1alpha1.Table{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-table",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.TableSpec{
			Name: "TEST_TABLE",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	a := &adapter{}

	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newTable()
		obs := &reconciler.Observation[*snowflake.TableObservation]{
			Exists: true,
			Detail: &snowflake.TableObservation{
				ShowOutput: &snowflake.TableShowOutput{
					Comment:               "table comment",
					RetentionTime:         14,
					ChangeTracking:        true,
					EnableSchemaEvolution: true,
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, "table comment", *obj.Spec.Comment)
		assert.Equal(t, int32(14), *obj.Spec.DataRetentionTimeInDays)
		assert.Equal(t, true, *obj.Spec.ChangeTracking)
		assert.Equal(t, true, *obj.Spec.EnableSchemaEvolution)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newTable()
		obj.Spec.Comment = ptr("user comment")
		obj.Spec.DataRetentionTimeInDays = ptr(int32(7))

		obs := &reconciler.Observation[*snowflake.TableObservation]{
			Exists: true,
			Detail: &snowflake.TableObservation{
				ShowOutput: &snowflake.TableShowOutput{
					Comment:        "snowflake comment",
					RetentionTime:  14,
					ChangeTracking: true,
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified) // ChangeTracking was set

		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, int32(7), *obj.Spec.DataRetentionTimeInDays)
		assert.Equal(t, true, *obj.Spec.ChangeTracking)
	})

	t.Run("returns false when all fields already set", func(t *testing.T) {
		obj := newTable()
		obj.Spec.Comment = ptr("c")
		obj.Spec.DataRetentionTimeInDays = ptr(int32(1))
		obj.Spec.ChangeTracking = ptr(false)
		obj.Spec.EnableSchemaEvolution = ptr(false)

		obs := &reconciler.Observation[*snowflake.TableObservation]{
			Exists: true,
			Detail: &snowflake.TableObservation{
				ShowOutput: &snowflake.TableShowOutput{
					Comment:       "other",
					RetentionTime: 99,
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newTable()
		obs := &reconciler.Observation[*snowflake.TableObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("handles nil show output", func(t *testing.T) {
		obj := newTable()
		obs := &reconciler.Observation[*snowflake.TableObservation]{
			Exists: true,
			Detail: &snowflake.TableObservation{
				ShowOutput: nil,
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
