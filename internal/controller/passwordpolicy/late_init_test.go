package passwordpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newPasswordPolicy() *snowplanev1alpha1.PasswordPolicy {
	return &snowplanev1alpha1.PasswordPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pp",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.PasswordPolicySpec{
			Name: "TEST_PP",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	a := &adapter{}

	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newPasswordPolicy()
		obs := &reconciler.Observation[*snowflake.PasswordPolicyObservation]{
			Exists: true,
			Detail: &snowflake.PasswordPolicyObservation{
				ShowOutput: &snowflake.PasswordPolicyShowOutput{
					Comment: "policy comment",
				},
				DescribeOutput: map[string]string{
					"PASSWORD_MIN_LENGTH":           "8",
					"PASSWORD_MAX_LENGTH":           "256",
					"PASSWORD_MIN_UPPER_CASE_CHARS": "1",
					"PASSWORD_MIN_LOWER_CASE_CHARS": "1",
					"PASSWORD_MIN_NUMERIC_CHARS":    "1",
					"PASSWORD_MIN_SPECIAL_CHARS":    "0",
					"PASSWORD_MIN_AGE_DAYS":         "0",
					"PASSWORD_MAX_AGE_DAYS":         "90",
					"PASSWORD_MAX_RETRIES":          "5",
					"PASSWORD_LOCKOUT_TIME_MINS":    "15",
					"PASSWORD_HISTORY":              "0",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, "policy comment", *obj.Spec.Comment)
		assert.Equal(t, int32(8), *obj.Spec.PasswordMinLength)
		assert.Equal(t, int32(256), *obj.Spec.PasswordMaxLength)
		assert.Equal(t, int32(1), *obj.Spec.PasswordMinUpperCaseChars)
		assert.Equal(t, int32(1), *obj.Spec.PasswordMinLowerCaseChars)
		assert.Equal(t, int32(1), *obj.Spec.PasswordMinNumericChars)
		assert.Equal(t, int32(0), *obj.Spec.PasswordMinSpecialChars)
		assert.Equal(t, int32(0), *obj.Spec.PasswordMinAgeDays)
		assert.Equal(t, int32(90), *obj.Spec.PasswordMaxAgeDays)
		assert.Equal(t, int32(5), *obj.Spec.PasswordMaxRetries)
		assert.Equal(t, int32(15), *obj.Spec.PasswordLockoutTimeMins)
		assert.Equal(t, int32(0), *obj.Spec.PasswordHistory)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newPasswordPolicy()
		obj.Spec.PasswordMinLength = ptr(int32(10))
		obj.Spec.Comment = ptr("user comment")

		obs := &reconciler.Observation[*snowflake.PasswordPolicyObservation]{
			Exists: true,
			Detail: &snowflake.PasswordPolicyObservation{
				ShowOutput: &snowflake.PasswordPolicyShowOutput{
					Comment: "snowflake comment",
				},
				DescribeOutput: map[string]string{
					"PASSWORD_MIN_LENGTH":   "8",
					"PASSWORD_MAX_LENGTH":   "256",
					"PASSWORD_MAX_AGE_DAYS": "90",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified) // MaxLength and MaxAgeDays set

		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, int32(10), *obj.Spec.PasswordMinLength)
		assert.Equal(t, int32(256), *obj.Spec.PasswordMaxLength)
		assert.Equal(t, int32(90), *obj.Spec.PasswordMaxAgeDays)
	})

	t.Run("handles invalid numeric string gracefully", func(t *testing.T) {
		obj := newPasswordPolicy()
		obs := &reconciler.Observation[*snowflake.PasswordPolicyObservation]{
			Exists: true,
			Detail: &snowflake.PasswordPolicyObservation{
				DescribeOutput: map[string]string{
					"PASSWORD_MIN_LENGTH": "not-a-number",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Nil(t, obj.Spec.PasswordMinLength)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newPasswordPolicy()
		obs := &reconciler.Observation[*snowflake.PasswordPolicyObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("handles nil show output and describe output", func(t *testing.T) {
		obj := newPasswordPolicy()
		obs := &reconciler.Observation[*snowflake.PasswordPolicyObservation]{
			Exists: true,
			Detail: &snowflake.PasswordPolicyObservation{
				ShowOutput:     nil,
				DescribeOutput: nil,
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})
}

func TestLateInitInt32FromMap(t *testing.T) {
	t.Run("sets nil target from valid map entry", func(t *testing.T) {
		var target *int32
		m := map[string]string{"KEY": "42"}
		assert.True(t, lateInitInt32FromMap(&target, m, "KEY"))
		assert.Equal(t, int32(42), *target)
	})

	t.Run("does not overwrite existing target", func(t *testing.T) {
		existing := int32(10)
		target := &existing
		m := map[string]string{"KEY": "42"}
		assert.False(t, lateInitInt32FromMap(&target, m, "KEY"))
		assert.Equal(t, int32(10), *target)
	})

	t.Run("returns false for missing key", func(t *testing.T) {
		var target *int32
		m := map[string]string{}
		assert.False(t, lateInitInt32FromMap(&target, m, "KEY"))
		assert.Nil(t, target)
	})

	t.Run("returns false for empty value", func(t *testing.T) {
		var target *int32
		m := map[string]string{"KEY": ""}
		assert.False(t, lateInitInt32FromMap(&target, m, "KEY"))
		assert.Nil(t, target)
	})

	t.Run("returns false for invalid value", func(t *testing.T) {
		var target *int32
		m := map[string]string{"KEY": "abc"}
		assert.False(t, lateInitInt32FromMap(&target, m, "KEY"))
		assert.Nil(t, target)
	})

	t.Run("handles zero value correctly", func(t *testing.T) {
		var target *int32
		m := map[string]string{"KEY": "0"}
		assert.True(t, lateInitInt32FromMap(&target, m, "KEY"))
		assert.Equal(t, int32(0), *target)
	})
}
