package database

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newDatabase() *snowplanev1alpha1.Database {
	return &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-db",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			Name: "TEST_DB",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	a := &adapter{}

	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newDatabase()
		obs := &reconciler.Observation[*snowflake.DatabaseObservation]{
			Exists: true,
			Detail: &snowflake.DatabaseObservation{
				ShowOutput: &snowflake.DatabaseShowOutput{
					Comment: "existing comment",
				},
				Parameters: &snowflake.DatabaseParameters{
					DataRetentionTimeInDays:    ptr(int32(14)),
					MaxDataExtensionTimeInDays: ptr(int32(28)),
					ReplaceInvalidCharacters:   ptr(true),
					DefaultDDLCollation:        "en-ci",
					Catalog:                    "MY_CATALOG",
					ExternalVolume:             "MY_VOLUME",
					StorageSerializationPolicy: "OPTIMIZED",
					LogLevel:                   "INFO",
					MetricLevel:                "ALL",
					TraceLevel:                 "ON_EVENT",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, "existing comment", *obj.Spec.Comment)
		assert.Equal(t, int32(14), *obj.Spec.DataRetentionTimeInDays)
		assert.Equal(t, int32(28), *obj.Spec.MaxDataExtensionTimeInDays)
		assert.Equal(t, true, *obj.Spec.ReplaceInvalidCharacters)
		assert.Equal(t, "en-ci", *obj.Spec.DefaultDDLCollation)
		assert.Equal(t, "MY_CATALOG", *obj.Spec.Catalog)
		assert.Equal(t, "MY_VOLUME", *obj.Spec.ExternalVolume)
		assert.Equal(t, snowplanev1alpha1.StorageSerializationPolicy("OPTIMIZED"), *obj.Spec.StorageSerializationPolicy)
		assert.Equal(t, snowplanev1alpha1.LogLevel("INFO"), *obj.Spec.LogLevel)
		assert.Equal(t, snowplanev1alpha1.MetricLevel("ALL"), *obj.Spec.MetricLevel)
		assert.Equal(t, snowplanev1alpha1.TraceLevel("ON_EVENT"), *obj.Spec.TraceLevel)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newDatabase()
		obj.Spec.Comment = ptr("user comment")
		obj.Spec.DataRetentionTimeInDays = ptr(int32(7))

		obs := &reconciler.Observation[*snowflake.DatabaseObservation]{
			Exists: true,
			Detail: &snowflake.DatabaseObservation{
				ShowOutput: &snowflake.DatabaseShowOutput{
					Comment: "snowflake comment",
				},
				Parameters: &snowflake.DatabaseParameters{
					DataRetentionTimeInDays:    ptr(int32(14)),
					MaxDataExtensionTimeInDays: ptr(int32(28)),
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified) // MaxDataExtensionTimeInDays was set

		// Existing fields preserved
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, int32(7), *obj.Spec.DataRetentionTimeInDays)

		// New field populated
		assert.Equal(t, int32(28), *obj.Spec.MaxDataExtensionTimeInDays)
	})

	t.Run("returns false when all fields already set", func(t *testing.T) {
		obj := newDatabase()
		obj.Spec.Comment = ptr("user comment")
		obj.Spec.DataRetentionTimeInDays = ptr(int32(7))
		obj.Spec.MaxDataExtensionTimeInDays = ptr(int32(14))
		obj.Spec.ReplaceInvalidCharacters = ptr(false)
		obj.Spec.DefaultDDLCollation = ptr("utf8")
		obj.Spec.Catalog = ptr("cat")
		obj.Spec.ExternalVolume = ptr("vol")
		ssp := snowplanev1alpha1.StorageSerializationPolicy("COMPATIBLE")
		obj.Spec.StorageSerializationPolicy = &ssp
		ll := snowplanev1alpha1.LogLevel("OFF")
		obj.Spec.LogLevel = &ll
		ml := snowplanev1alpha1.MetricLevel("NONE")
		obj.Spec.MetricLevel = &ml
		tl := snowplanev1alpha1.TraceLevel("OFF")
		obj.Spec.TraceLevel = &tl

		obs := &reconciler.Observation[*snowflake.DatabaseObservation]{
			Exists: true,
			Detail: &snowflake.DatabaseObservation{
				ShowOutput: &snowflake.DatabaseShowOutput{Comment: "other"},
				Parameters: &snowflake.DatabaseParameters{
					DataRetentionTimeInDays: ptr(int32(99)),
					LogLevel:                "INFO",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newDatabase()
		obs := &reconciler.Observation[*snowflake.DatabaseObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("handles nil show output and parameters", func(t *testing.T) {
		obj := newDatabase()
		obs := &reconciler.Observation[*snowflake.DatabaseObservation]{
			Exists: true,
			Detail: &snowflake.DatabaseObservation{
				ShowOutput: nil,
				Parameters: nil,
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
