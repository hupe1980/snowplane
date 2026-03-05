package saml2integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newSAML2Integration() *snowplanev1alpha1.SAML2Integration {
	return &snowplanev1alpha1.SAML2Integration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-si",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.SAML2IntegrationSpec{
			Name:     "TEST_SAML2",
			Issuer:   "https://idp.example.com",
			SSOURL:   "https://idp.example.com/sso",
			Provider: "CUSTOM",
			X509Cert: "cert",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newSAML2Integration()
		obs := &reconciler.Observation[*snowflake.SAML2IntegrationObservation]{
			Exists: true,
			Detail: &snowflake.SAML2IntegrationObservation{
				ShowOutput: &snowplanev1alpha1.SAML2IntegrationShowOutput{
					Comment: "saml comment",
					Enabled: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "saml comment", *obj.Spec.Comment)
		assert.Equal(t, true, *obj.Spec.Enabled)
	})

	t.Run("does not overwrite existing fields", func(t *testing.T) {
		obj := newSAML2Integration()
		obj.Spec.Comment = ptr("user comment")
		obj.Spec.Enabled = ptr(false)

		obs := &reconciler.Observation[*snowflake.SAML2IntegrationObservation]{
			Exists: true,
			Detail: &snowflake.SAML2IntegrationObservation{
				ShowOutput: &snowplanev1alpha1.SAML2IntegrationShowOutput{
					Comment: "sf comment",
					Enabled: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, false, *obj.Spec.Enabled)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newSAML2Integration()

		modified := lateInitialize(obj, &reconciler.Observation[*snowflake.SAML2IntegrationObservation]{
			Exists: true,
			Detail: nil,
		})
		assert.False(t, modified)
	})

	t.Run("skips empty comment but sets enabled", func(t *testing.T) {
		obj := newSAML2Integration()

		obs := &reconciler.Observation[*snowflake.SAML2IntegrationObservation]{
			Exists: true,
			Detail: &snowflake.SAML2IntegrationObservation{
				ShowOutput: &snowplanev1alpha1.SAML2IntegrationShowOutput{
					Comment: "",
					Enabled: false,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified) // Enabled was set
		assert.Nil(t, obj.Spec.Comment)
		assert.Equal(t, false, *obj.Spec.Enabled)
	})
}
