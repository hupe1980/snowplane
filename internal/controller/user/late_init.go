package user

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// LateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
// SecretKeyReference fields (Password, RSAPublicKey, RSAPublicKey2) are
// deliberately excluded — they reference K8s Secrets, not Snowflake values.
func (a *adapter) LateInitialize(obj *snowplanev1alpha1.User, obs *reconciler.Observation[*snowflake.UserObservation]) bool {
	detail := obs.Detail
	if detail == nil {
		return false
	}

	var modified bool

	if detail.ShowOutput != nil {
		s := detail.ShowOutput

		if reconciler.LateInitNonZero(&obj.Spec.LoginName, s.LoginName) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.DisplayName, s.DisplayName) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.Email, s.Email) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.FirstName, s.FirstName) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.LastName, s.LastName) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.MiddleName, s.MiddleName) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.Comment, s.Comment) {
			modified = true
		}

		if s.Type != "" && obj.Spec.Type == nil {
			v := snowplanev1alpha1.UserType(s.Type)
			obj.Spec.Type = &v
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.DefaultRole, s.DefaultRole) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.DefaultSecondaryRoles, s.DefaultSecondaryRoles) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.DefaultWarehouse, s.DefaultWarehouse) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.DefaultNamespace, s.DefaultNamespace) {
			modified = true
		}

		if reconciler.LateInit(&obj.Spec.MustChangePassword, s.MustChangePassword) {
			modified = true
		}

		if reconciler.LateInit(&obj.Spec.Disabled, s.Disabled) {
			modified = true
		}

		if reconciler.LateInit(&obj.Spec.DisableMFA, s.DisableMFA) {
			modified = true
		}
	}

	if detail.DescribeOutput != nil {
		if reconciler.LateInitNonZero(&obj.Spec.NetworkPolicy, detail.DescribeOutput.NetworkPolicy) {
			modified = true
		}
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.User, *snowflake.UserObservation] = (*adapter)(nil)
