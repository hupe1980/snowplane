package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newSchema() *snowplanev1alpha1.Schema {
	return &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-schema",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			Name: "TEST_SCHEMA",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newSchema()
		obs := &reconciler.Observation[*snowflake.SchemaObservation]{
			Exists: true,
			Detail: &snowflake.SchemaObservation{
				ShowOutput: &snowplanev1alpha1.SchemaShowOutput{
					Comment: "schema comment",
				},
				Parameters: &snowflake.SchemaParameters{
					DataRetentionTimeInDays:    ptr(int32(14)),
					MaxDataExtensionTimeInDays: ptr(int32(28)),
					ReplaceInvalidCharacters:   ptr(true),
					DefaultDDLCollation:        "en-ci",
					StorageSerializationPolicy: "OPTIMIZED",
					LogLevel:                   "INFO",
					MetricLevel:                "ALL",
					TraceLevel:                 "ON_EVENT",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, "schema comment", *obj.Spec.Comment)
		assert.Equal(t, int32(14), *obj.Spec.DataRetentionTimeInDays)
		assert.Equal(t, int32(28), *obj.Spec.MaxDataExtensionTimeInDays)
		assert.Equal(t, true, *obj.Spec.ReplaceInvalidCharacters)
		assert.Equal(t, "en-ci", *obj.Spec.DefaultDDLCollation)
		assert.Equal(t, snowplanev1alpha1.StorageSerializationPolicy("OPTIMIZED"), *obj.Spec.StorageSerializationPolicy)
		assert.Equal(t, snowplanev1alpha1.LogLevel("INFO"), *obj.Spec.LogLevel)
		assert.Equal(t, snowplanev1alpha1.MetricLevel("ALL"), *obj.Spec.MetricLevel)
		assert.Equal(t, snowplanev1alpha1.TraceLevel("ON_EVENT"), *obj.Spec.TraceLevel)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newSchema()
		obj.Spec.Comment = ptr("user comment")
		obj.Spec.DataRetentionTimeInDays = ptr(int32(7))

		obs := &reconciler.Observation[*snowflake.SchemaObservation]{
			Exists: true,
			Detail: &snowflake.SchemaObservation{
				ShowOutput: &snowplanev1alpha1.SchemaShowOutput{
					Comment: "snowflake comment",
				},
				Parameters: &snowflake.SchemaParameters{
					DataRetentionTimeInDays:    ptr(int32(14)),
					MaxDataExtensionTimeInDays: ptr(int32(28)),
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, int32(7), *obj.Spec.DataRetentionTimeInDays)
		assert.Equal(t, int32(28), *obj.Spec.MaxDataExtensionTimeInDays)
	})

	t.Run("returns false when all fields already set", func(t *testing.T) {
		obj := newSchema()
		obj.Spec.Comment = ptr("c")
		obj.Spec.DataRetentionTimeInDays = ptr(int32(7))
		obj.Spec.MaxDataExtensionTimeInDays = ptr(int32(14))
		obj.Spec.ReplaceInvalidCharacters = ptr(false)
		obj.Spec.DefaultDDLCollation = ptr("utf8")
		ssp := snowplanev1alpha1.StorageSerializationPolicy("COMPATIBLE")
		obj.Spec.StorageSerializationPolicy = &ssp
		ll := snowplanev1alpha1.LogLevel("OFF")
		obj.Spec.LogLevel = &ll
		ml := snowplanev1alpha1.MetricLevel("NONE")
		obj.Spec.MetricLevel = &ml
		tl := snowplanev1alpha1.TraceLevel("OFF")
		obj.Spec.TraceLevel = &tl

		obs := &reconciler.Observation[*snowflake.SchemaObservation]{
			Exists: true,
			Detail: &snowflake.SchemaObservation{
				ShowOutput: &snowplanev1alpha1.SchemaShowOutput{Comment: "other"},
				Parameters: &snowflake.SchemaParameters{
					DataRetentionTimeInDays: ptr(int32(99)),
					LogLevel:                "INFO",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newSchema()
		obs := &reconciler.Observation[*snowflake.SchemaObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("handles nil show output and parameters", func(t *testing.T) {
		obj := newSchema()
		obs := &reconciler.Observation[*snowflake.SchemaObservation]{
			Exists: true,
			Detail: &snowflake.SchemaObservation{
				ShowOutput: nil,
				Parameters: nil,
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
