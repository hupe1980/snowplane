package user

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newUser() *snowplanev1alpha1.User {
	return &snowplanev1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-user",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.UserSpec{
			Name: "TEST_USER",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newUser()
		obs := &reconciler.Observation[*snowflake.UserObservation]{
			Exists: true,
			Detail: &snowflake.UserObservation{
				ShowOutput: &snowplanev1alpha1.UserShowOutput{
					LoginName:             "TEST_USER",
					DisplayName:           "Test User",
					Email:                 "test@example.com",
					FirstName:             "Test",
					LastName:              "User",
					MiddleName:            "M",
					Comment:               "user comment",
					Type:                  "PERSON",
					DefaultRole:           "PUBLIC",
					DefaultSecondaryRoles: "ALL",
					DefaultWarehouse:      "COMPUTE_WH",
					DefaultNamespace:      "MY_DB.MY_SCHEMA",
					MustChangePassword:    false,
					Disabled:              false,
					DisableMFA:            true,
				},
				DescribeOutput: &snowflake.UserDescribeOutput{
					NetworkPolicy: "MY_POLICY",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, "TEST_USER", *obj.Spec.LoginName)
		assert.Equal(t, "Test User", *obj.Spec.DisplayName)
		assert.Equal(t, "test@example.com", *obj.Spec.Email)
		assert.Equal(t, "Test", *obj.Spec.FirstName)
		assert.Equal(t, "User", *obj.Spec.LastName)
		assert.Equal(t, "M", *obj.Spec.MiddleName)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, snowplanev1alpha1.UserType("PERSON"), *obj.Spec.Type)
		assert.Equal(t, "PUBLIC", *obj.Spec.DefaultRole)
		assert.Equal(t, "ALL", *obj.Spec.DefaultSecondaryRoles)
		assert.Equal(t, "COMPUTE_WH", *obj.Spec.DefaultWarehouse)
		assert.Equal(t, "MY_DB.MY_SCHEMA", *obj.Spec.DefaultNamespace)
		assert.Equal(t, false, *obj.Spec.MustChangePassword)
		assert.Equal(t, false, *obj.Spec.Disabled)
		assert.Equal(t, true, *obj.Spec.DisableMFA)
		assert.Equal(t, "MY_POLICY", *obj.Spec.NetworkPolicy)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newUser()
		obj.Spec.LoginName = ptr("EXISTING_LOGIN")
		obj.Spec.Comment = ptr("user comment")
		ut := snowplanev1alpha1.UserType("SERVICE")
		obj.Spec.Type = &ut

		obs := &reconciler.Observation[*snowflake.UserObservation]{
			Exists: true,
			Detail: &snowflake.UserObservation{
				ShowOutput: &snowplanev1alpha1.UserShowOutput{
					LoginName:   "DIFFERENT_LOGIN",
					Comment:     "snowflake comment",
					Type:        "PERSON",
					Email:       "test@example.com",
					DefaultRole: "ADMIN",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified) // Email and DefaultRole were set

		assert.Equal(t, "EXISTING_LOGIN", *obj.Spec.LoginName)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, snowplanev1alpha1.UserType("SERVICE"), *obj.Spec.Type)
		assert.Equal(t, "test@example.com", *obj.Spec.Email)
		assert.Equal(t, "ADMIN", *obj.Spec.DefaultRole)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newUser()
		obs := &reconciler.Observation[*snowflake.UserObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("handles nil show and describe output", func(t *testing.T) {
		obj := newUser()
		obs := &reconciler.Observation[*snowflake.UserObservation]{
			Exists: true,
			Detail: &snowflake.UserObservation{
				ShowOutput:     nil,
				DescribeOutput: nil,
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("skips empty strings from show output", func(t *testing.T) {
		obj := newUser()
		obs := &reconciler.Observation[*snowflake.UserObservation]{
			Exists: true,
			Detail: &snowflake.UserObservation{
				ShowOutput: &snowplanev1alpha1.UserShowOutput{
					LoginName: "",
					Email:     "",
					Comment:   "only-comment",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Nil(t, obj.Spec.LoginName) // empty string skipped
		assert.Nil(t, obj.Spec.Email)     // empty string skipped
		assert.Equal(t, "only-comment", *obj.Spec.Comment)
	})
}
