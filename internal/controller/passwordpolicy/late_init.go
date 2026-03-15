package passwordpolicy

import (
	"strconv"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitInt32FromMap parses a string value from the DescribeOutput map
// and sets the target if it is nil. Returns true if a value was set.
func lateInitInt32FromMap(target **int32, m map[string]string, key string) bool {
	if *target != nil {
		return false
	}

	s, ok := m[key]
	if !ok || s == "" {
		return false
	}

	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return false
	}

	i := int32(v)
	*target = &i

	return true
}

// lateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
// Ref fields (DatabaseRef, SchemaRef) are excluded.
func lateInitialize(obj *snowplanev1alpha1.PasswordPolicy, obs *reconciler.Observation[*snowflake.PasswordPolicyObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	var modified bool

	if reconciler.LateInitNonZero(&obj.Spec.Comment, detail.ShowOutput.Comment) {
		modified = true
	}

	if detail.DescribeOutput != nil {
		m := detail.DescribeOutput

		if lateInitInt32FromMap(&obj.Spec.PasswordMinLength, m, "PASSWORD_MIN_LENGTH") {
			modified = true
		}

		if lateInitInt32FromMap(&obj.Spec.PasswordMaxLength, m, "PASSWORD_MAX_LENGTH") {
			modified = true
		}

		if lateInitInt32FromMap(&obj.Spec.PasswordMinUpperCaseChars, m, "PASSWORD_MIN_UPPER_CASE_CHARS") {
			modified = true
		}

		if lateInitInt32FromMap(&obj.Spec.PasswordMinLowerCaseChars, m, "PASSWORD_MIN_LOWER_CASE_CHARS") {
			modified = true
		}

		if lateInitInt32FromMap(&obj.Spec.PasswordMinNumericChars, m, "PASSWORD_MIN_NUMERIC_CHARS") {
			modified = true
		}

		if lateInitInt32FromMap(&obj.Spec.PasswordMinSpecialChars, m, "PASSWORD_MIN_SPECIAL_CHARS") {
			modified = true
		}

		if lateInitInt32FromMap(&obj.Spec.PasswordMinAgeDays, m, "PASSWORD_MIN_AGE_DAYS") {
			modified = true
		}

		if lateInitInt32FromMap(&obj.Spec.PasswordMaxAgeDays, m, "PASSWORD_MAX_AGE_DAYS") {
			modified = true
		}

		if lateInitInt32FromMap(&obj.Spec.PasswordMaxRetries, m, "PASSWORD_MAX_RETRIES") {
			modified = true
		}

		if lateInitInt32FromMap(&obj.Spec.PasswordLockoutTimeMins, m, "PASSWORD_LOCKOUT_TIME_MINS") {
			modified = true
		}

		if lateInitInt32FromMap(&obj.Spec.PasswordHistory, m, "PASSWORD_HISTORY") {
			modified = true
		}
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.PasswordPolicy, *snowflake.PasswordPolicyObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation])(nil)
