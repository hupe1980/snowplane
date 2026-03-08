//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	sqlstmtclient "github.com/hupe1980/snowplane/internal/clients/snowflake/sqlstatement"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// ---------------------------------------------------------------------------
// API Authentication Integrations
// ---------------------------------------------------------------------------

// ensureOAuthClientSecret creates a K8s Secret named "test-secret" in the test
// namespace with an "password" key. The API Auth controllers read this Secret
// via spec.oauthClientSecretRef during Create.
func ensureOAuthClientSecret(t *testing.T) {
	t.Helper()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: testNamespace},
		Data:       map[string][]byte{"password": []byte("s3cr3t")},
	}

	// Ignore AlreadyExists — the secret may outlive a single sub-test.
	if err := k8sClient.Create(ctx, secret); err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err, "unexpected error creating test-secret")
	}
}

func TestAPIAuthIntegrationWithAuthorizationCodeGrant_CreateLifecycle(t *testing.T) {
	resetMocks()
	ensureOAuthClientSecret(t)

	var created atomic.Bool

	apiAuthCodeGrantMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
		if id.Name() == "MY_API_AUTH_ACG" && created.Load() {
			return apiAuthIntegrationObservation("MY_API_AUTH_ACG"), nil
		}
		return &snowflake.APIAuthenticationIntegrationObservation{Exists: false}, nil
	})
	apiAuthCodeGrantMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateAPIAuthenticationIntegrationOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestAPIAuthCodeGrant("test-api-auth-acg", "MY_API_AUTH_ACG")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		apiAuthCodeGrantMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "APIAuthenticationIntegrationWithAuthorizationCodeGrant should become Ready")
}

func TestAPIAuthIntegrationWithClientCredentials_CreateLifecycle(t *testing.T) {
	resetMocks()
	ensureOAuthClientSecret(t)

	var created atomic.Bool

	apiAuthClientCredsMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
		if id.Name() == "MY_API_AUTH_CC" && created.Load() {
			return apiAuthIntegrationObservation("MY_API_AUTH_CC"), nil
		}
		return &snowflake.APIAuthenticationIntegrationObservation{Exists: false}, nil
	})
	apiAuthClientCredsMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateAPIAuthenticationIntegrationOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestAPIAuthClientCreds("test-api-auth-cc", "MY_API_AUTH_CC")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		apiAuthClientCredsMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.APIAuthenticationIntegrationWithClientCredentials
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.APIAuthenticationIntegrationWithClientCredentials{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.APIAuthenticationIntegrationWithClientCredentials
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "APIAuthenticationIntegrationWithClientCredentials should become Ready")
}

func TestAPIAuthIntegrationWithJWTBearer_CreateLifecycle(t *testing.T) {
	resetMocks()
	ensureOAuthClientSecret(t)

	var created atomic.Bool

	apiAuthJWTBearerMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
		if id.Name() == "MY_API_AUTH_JWT" && created.Load() {
			return apiAuthIntegrationObservation("MY_API_AUTH_JWT"), nil
		}
		return &snowflake.APIAuthenticationIntegrationObservation{Exists: false}, nil
	})
	apiAuthJWTBearerMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateAPIAuthenticationIntegrationOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestAPIAuthJWTBearer("test-api-auth-jwt", "MY_API_AUTH_JWT")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		apiAuthJWTBearerMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "APIAuthenticationIntegrationWithJWTBearer should become Ready")
}

// ---------------------------------------------------------------------------
// Security Integrations
// ---------------------------------------------------------------------------

func TestExternalOAuthIntegration_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	externalOAuthMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error) {
		if id.Name() == "MY_EXT_OAUTH" && created.Load() {
			return externalOAuthObservation("MY_EXT_OAUTH"), nil
		}
		return &snowflake.ExternalOAuthIntegrationObservation{Exists: false}, nil
	})
	externalOAuthMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateExternalOAuthIntegrationOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestExternalOAuthIntegration("test-ext-oauth", "MY_EXT_OAUTH")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		externalOAuthMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.ExternalOAuthIntegration
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.ExternalOAuthIntegration{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ExternalOAuthIntegration
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "ExternalOAuthIntegration should become Ready")
}

func TestSAML2Integration_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	saml2MockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.SAML2IntegrationObservation, error) {
		if id.Name() == "MY_SAML2" && created.Load() {
			return saml2Observation("MY_SAML2"), nil
		}
		return &snowflake.SAML2IntegrationObservation{Exists: false}, nil
	})
	saml2MockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSAML2IntegrationOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestSAML2Integration("test-saml2", "MY_SAML2")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		saml2MockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.SAML2Integration
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.SAML2Integration{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.SAML2Integration
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "SAML2Integration should become Ready")
}

// ---------------------------------------------------------------------------
// Failover Group
// ---------------------------------------------------------------------------

func TestFailoverGroup_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	failoverGroupMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error) {
		if id.Name() == "MY_FG" && created.Load() {
			return failoverGroupObservation("MY_FG"), nil
		}
		return &snowflake.FailoverGroupObservation{Exists: false}, nil
	})
	failoverGroupMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateFailoverGroupOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestFailoverGroup("test-fg", "MY_FG")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		failoverGroupMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.FailoverGroup
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.FailoverGroup{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.FailoverGroup
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "FailoverGroup should become Ready")
}

// ---------------------------------------------------------------------------
// Policy Attachment tests (Set/Unset pattern)
// ---------------------------------------------------------------------------

func TestMaskingPolicyApplication_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	maskingPolicyAppMockSvc.observeFn = func(_ context.Context, _ snowflake.MaskingPolicyApplicationIdentifier) (*snowflake.MaskingPolicyApplicationObservation, error) {
		if created.Load() {
			return maskingPolicyApplicationObservation("MY_DB.MY_SCHEMA.MY_MASK_POLICY"), nil
		}
		return &snowflake.MaskingPolicyApplicationObservation{Exists: false}, nil
	}
	maskingPolicyAppMockSvc.setFn = func(_ context.Context, _ snowflake.SetMaskingPolicyOptions) error {
		created.Store(true)
		return nil
	}

	cr := newTestMaskingPolicyApplication("test-mask-app", "MY_DB.MY_SCHEMA.MY_MASK_POLICY", "MY_DB.MY_SCHEMA.MY_TABLE", "COL1")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		maskingPolicyAppMockSvc.unsetFn = func(_ context.Context, _ snowflake.UnsetMaskingPolicyOptions) error { return nil }
		var obj snowplanev1alpha1.MaskingPolicyApplication
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.MaskingPolicyApplication{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.MaskingPolicyApplication
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "MaskingPolicyApplication should become Ready")
}

func TestNetworkPolicyAttachment_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	networkPolicyAttachMockSvc.observeFn = func(_ context.Context, _ snowflake.NetworkPolicyAttachmentIdentifier) (*snowflake.NetworkPolicyAttachmentObservation, error) {
		if created.Load() {
			return networkPolicyAttachmentObservation("MY_NET_POLICY"), nil
		}
		return &snowflake.NetworkPolicyAttachmentObservation{Exists: false}, nil
	}
	networkPolicyAttachMockSvc.setFn = func(_ context.Context, _ snowflake.SetNetworkPolicyOptions) error {
		created.Store(true)
		return nil
	}

	cr := newTestNetworkPolicyAttachment("test-net-pol-attach", "MY_NET_POLICY")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		networkPolicyAttachMockSvc.unsetFn = func(_ context.Context, _ snowflake.UnsetNetworkPolicyOptions) error { return nil }
		var obj snowplanev1alpha1.NetworkPolicyAttachment
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.NetworkPolicyAttachment{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.NetworkPolicyAttachment
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "NetworkPolicyAttachment should become Ready")
}

func TestPasswordPolicyAttachment_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	passwordPolicyAttachMockSvc.observeFn = func(_ context.Context, _ snowflake.PasswordPolicyAttachmentIdentifier) (*snowflake.PasswordPolicyAttachmentObservation, error) {
		if created.Load() {
			return passwordPolicyAttachmentObservation("MY_PWD_POLICY"), nil
		}
		return &snowflake.PasswordPolicyAttachmentObservation{Exists: false}, nil
	}
	passwordPolicyAttachMockSvc.setFn = func(_ context.Context, _ snowflake.SetPasswordPolicyOptions) error {
		created.Store(true)
		return nil
	}

	cr := newTestPasswordPolicyAttachment("test-pwd-pol-attach", "MY_PWD_POLICY")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		passwordPolicyAttachMockSvc.unsetFn = func(_ context.Context, _ snowflake.UnsetPasswordPolicyOptions) error { return nil }
		var obj snowplanev1alpha1.PasswordPolicyAttachment
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.PasswordPolicyAttachment{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.PasswordPolicyAttachment
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "PasswordPolicyAttachment should become Ready")
}

// ---------------------------------------------------------------------------
// Tag Association (SetTag/UnsetTag)
// ---------------------------------------------------------------------------

func TestTagAssociation_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	tagAssociationMockSvc.observeFn = func(_ context.Context, _ snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
		if created.Load() {
			return tagAssociationObservation("my-value"), nil
		}
		return &snowflake.TagAssociationObservation{Exists: false}, nil
	}
	tagAssociationMockSvc.setFn = func(_ context.Context, _ snowflake.SetTagOptions) error {
		created.Store(true)
		return nil
	}

	cr := newTestTagAssociation("test-tag-assoc", "MY_DB.MY_SCHEMA.MY_TAG", "TABLE", "MY_DB.MY_SCHEMA.MY_TABLE", "my-value")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-tag-assoc", Namespace: testNamespace}
	t.Cleanup(func() {
		tagAssociationMockSvc.unsetFn = func(_ context.Context, _ snowflake.UnsetTagOptions) error { return nil }
		var obj snowplanev1alpha1.TagAssociation
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.TagAssociation{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.TagAssociation
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "TagAssociation should become Ready")
}

// ---------------------------------------------------------------------------
// Table Constraint (AddConstraint/DropConstraint)
// ---------------------------------------------------------------------------

func TestTableConstraint_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	tableConstraintMockSvc.observeFn = func(_ context.Context, _ snowflake.TableConstraintIdentifier, _ string) (*snowflake.TableConstraintObservation, error) {
		if created.Load() {
			return tableConstraintObservation("MY_PK", "PRIMARY KEY", []string{"ID"}), nil
		}
		return &snowflake.TableConstraintObservation{Exists: false}, nil
	}
	tableConstraintMockSvc.addFn = func(_ context.Context, _ snowflake.AddConstraintOptions) error {
		created.Store(true)
		return nil
	}

	cr := newTestTableConstraint("test-tc", "MY_PK", "PRIMARY KEY", "MY_DB.MY_SCHEMA.MY_TABLE", []string{"ID"})
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-tc", Namespace: testNamespace}
	t.Cleanup(func() {
		tableConstraintMockSvc.dropFn = func(_ context.Context, _ snowflake.TableConstraintIdentifier) error { return nil }
		var obj snowplanev1alpha1.TableConstraint
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.TableConstraint{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.TableConstraint
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "TableConstraint should become Ready")
}

// ---------------------------------------------------------------------------
// SQL Statement (Execute/Revert/Observe)
// ---------------------------------------------------------------------------

func TestSQLStatement_CreateLifecycle(t *testing.T) {
	resetMocks()

	var executed atomic.Bool

	sqlStatementMockSvc.observeFn = func(_ context.Context, _ string, _ []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
		if executed.Load() {
			return sqlStatementObservation(), nil
		}
		return &sqlstmtclient.Observation{Exists: false, Matched: false}, nil
	}
	sqlStatementMockSvc.executeFn = func(_ context.Context, _ string) error {
		executed.Store(true)
		return nil
	}

	cr := newTestSQLStatement("test-sql-stmt", "CREATE TABLE IF NOT EXISTS MY_DB.MY_SCHEMA.MY_TABLE (id INT)")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-sql-stmt", Namespace: testNamespace}
	t.Cleanup(func() {
		sqlStatementMockSvc.revertFn = func(_ context.Context, _ string) error { return nil }
		var obj snowplanev1alpha1.SQLStatement
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.SQLStatement{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.SQLStatement
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "SQLStatement should become Ready")
}
