package shareddatabase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func TestLateInitialize(t *testing.T) {
	t.Parallel()

	t.Run("NilDetail", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("NilShowOutput", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("CommentAdopted", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{Comment: "remote comment"},
			},
		}
		modified := lateInitialize(obj, obs)
		require.True(t, modified)
		require.NotNil(t, obj.Spec.Comment)
		assert.Equal(t, "remote comment", *obj.Spec.Comment)
	})

	t.Run("CommentNotOverwritten", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{
			Spec: snowplanev1alpha1.SharedDatabaseSpec{Comment: testutil.Ptr("local")},
		}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{Comment: "remote"},
			},
		}
		assert.False(t, lateInitialize(obj, obs))
		assert.Equal(t, "local", *obj.Spec.Comment)
	})

	t.Run("EmptyCommentNotAdopted", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{Comment: ""},
			},
		}
		assert.False(t, lateInitialize(obj, obs))
		assert.Nil(t, obj.Spec.Comment)
	})

	t.Run("ExternalVolumeAdopted", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{},
				Parameters: &snowflake.DatabaseParameters{ExternalVolume: "my_vol"},
			},
		}
		modified := lateInitialize(obj, obs)
		require.True(t, modified)
		require.NotNil(t, obj.Spec.ExternalVolume)
		assert.Equal(t, "my_vol", *obj.Spec.ExternalVolume)
	})

	t.Run("ExternalVolumeNotOverwritten", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{
			Spec: snowplanev1alpha1.SharedDatabaseSpec{ExternalVolume: testutil.Ptr("local_vol")},
		}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{},
				Parameters: &snowflake.DatabaseParameters{ExternalVolume: "remote_vol"},
			},
		}
		assert.False(t, lateInitialize(obj, obs))
		assert.Equal(t, "local_vol", *obj.Spec.ExternalVolume)
	})

	t.Run("ReplaceInvalidCharsAdopted", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{}
		replaceChars := true
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{},
				Parameters: &snowflake.DatabaseParameters{ReplaceInvalidCharacters: &replaceChars},
			},
		}
		modified := lateInitialize(obj, obs)
		require.True(t, modified)
		require.NotNil(t, obj.Spec.ReplaceInvalidCharacters)
		assert.True(t, *obj.Spec.ReplaceInvalidCharacters)
	})

	t.Run("NilParametersNoChange", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{Comment: ""},
				Parameters: nil,
			},
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("StorageSerializationPolicyAdopted", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{},
				Parameters: &snowflake.DatabaseParameters{StorageSerializationPolicy: "OPTIMIZED"},
			},
		}
		modified := lateInitialize(obj, obs)
		require.True(t, modified)
		require.NotNil(t, obj.Spec.StorageSerializationPolicy)
		assert.Equal(t, snowplanev1alpha1.StorageSerializationPolicy("OPTIMIZED"), *obj.Spec.StorageSerializationPolicy)
	})

	t.Run("StorageSerializationPolicyNotOverwritten", func(t *testing.T) {
		t.Parallel()
		existing := snowplanev1alpha1.StorageSerializationPolicy("COMPATIBLE")
		obj := &snowplanev1alpha1.SharedDatabase{
			Spec: snowplanev1alpha1.SharedDatabaseSpec{StorageSerializationPolicy: &existing},
		}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{},
				Parameters: &snowflake.DatabaseParameters{StorageSerializationPolicy: "OPTIMIZED"},
			},
		}
		assert.False(t, lateInitialize(obj, obs))
		assert.Equal(t, snowplanev1alpha1.StorageSerializationPolicy("COMPATIBLE"), *obj.Spec.StorageSerializationPolicy)
	})

	t.Run("LogLevelAdopted", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{},
				Parameters: &snowflake.DatabaseParameters{LogLevel: "WARN"},
			},
		}
		modified := lateInitialize(obj, obs)
		require.True(t, modified)
		require.NotNil(t, obj.Spec.LogLevel)
		assert.Equal(t, snowplanev1alpha1.LogLevel("WARN"), *obj.Spec.LogLevel)
	})

	t.Run("LogLevelNotOverwritten", func(t *testing.T) {
		t.Parallel()
		existing := snowplanev1alpha1.LogLevel("INFO")
		obj := &snowplanev1alpha1.SharedDatabase{
			Spec: snowplanev1alpha1.SharedDatabaseSpec{LogLevel: &existing},
		}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{},
				Parameters: &snowflake.DatabaseParameters{LogLevel: "WARN"},
			},
		}
		assert.False(t, lateInitialize(obj, obs))
		assert.Equal(t, snowplanev1alpha1.LogLevel("INFO"), *obj.Spec.LogLevel)
	})

	t.Run("TraceLevelAdopted", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{},
				Parameters: &snowflake.DatabaseParameters{TraceLevel: "ON_EVENT"},
			},
		}
		modified := lateInitialize(obj, obs)
		require.True(t, modified)
		require.NotNil(t, obj.Spec.TraceLevel)
		assert.Equal(t, snowplanev1alpha1.TraceLevel("ON_EVENT"), *obj.Spec.TraceLevel)
	})

	t.Run("TraceLevelNotOverwritten", func(t *testing.T) {
		t.Parallel()
		existing := snowplanev1alpha1.TraceLevel("OFF")
		obj := &snowplanev1alpha1.SharedDatabase{
			Spec: snowplanev1alpha1.SharedDatabaseSpec{TraceLevel: &existing},
		}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{},
				Parameters: &snowflake.DatabaseParameters{TraceLevel: "ON_EVENT"},
			},
		}
		assert.False(t, lateInitialize(obj, obs))
		assert.Equal(t, snowplanev1alpha1.TraceLevel("OFF"), *obj.Spec.TraceLevel)
	})

	t.Run("EmptyEnumNotAdopted", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.SharedDatabase{}
		obs := &reconciler.Observation[*snowflake.SharedDatabaseObservation]{
			Detail: &snowflake.SharedDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{},
				Parameters: &snowflake.DatabaseParameters{
					StorageSerializationPolicy: "",
					LogLevel:                   "",
					TraceLevel:                 "",
				},
			},
		}
		assert.False(t, lateInitialize(obj, obs))
		assert.Nil(t, obj.Spec.StorageSerializationPolicy)
		assert.Nil(t, obj.Spec.LogLevel)
		assert.Nil(t, obj.Spec.TraceLevel)
	})
}
