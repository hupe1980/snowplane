//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// updateWithCELCheck performs a Get → mutate → Update loop with conflict retry.
// When the reconciler modifies the resource (finalizer, status) between Get and
// Update, etcd returns a 409 Conflict. This helper retries on conflicts until
// the "real" CEL rejection (or an unexpected success) is surfaced.
func updateWithCELCheck(t *testing.T, key types.NamespacedName, obj client.Object, mutate func()) error {
	t.Helper()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if getErr := k8sClient.Get(ctx, key, obj); getErr != nil {
			return getErr
		}

		mutate()

		return k8sClient.Update(ctx, obj)
	})
}

// --------------------------------------------------------------------------
// CEL Validation Integration Tests
//
// These tests verify that CRD-level CEL validation rules (x-kubernetes-validations)
// correctly enforce immutable fields, defaults, policy body blocklist, and
// other business rules without requiring admission webhooks.
//
// Key design:
// - Immutability is enforced at the spec level via CEL transition rules
//   (oldSelf). Transition rules are only evaluated on UPDATE, not CREATE.
// - No observedGeneration guard: fields are immutable from the first UPDATE.
// - No ForceNew annotation bypass: users must delete+recreate to change
//   immutable fields.
// - Dangerous grant checks remain in the reconciler (not CEL) because they
//   need annotation-based bypass which CEL cannot access.
// --------------------------------------------------------------------------

// TestCEL_Database_ImmutableName verifies that spec.name is immutable on UPDATE.
func TestCEL_Database_ImmutableName(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-db-immutable-name",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			Name: "MY_DB",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, db))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, db) })

	// Attempt to change spec.name — should be rejected by CEL transition rule.
	err := updateWithCELCheck(t, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, db, func() {
		db.Spec.Name = "ANOTHER_DB"
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

// TestCEL_Database_ImmutableTransient verifies that spec.transient is immutable.
func TestCEL_Database_ImmutableTransient(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-db-immutable-transient",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			Name:      "MY_DB_T",
			Transient: true,
		},
	}

	require.NoError(t, k8sClient.Create(ctx, db))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, db) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, db, func() {
		db.Spec.Transient = false
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.transient is immutable")
}

// TestCEL_Database_ImmutableUseRole verifies that spec.useRole (optional pointer) is immutable.
func TestCEL_Database_ImmutableUseRole(t *testing.T) {
	t.Parallel()

	role := "SYSADMIN"
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-db-immutable-userole",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				UseRole: &role,
			},
			Name: "MY_DB_UR",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, db))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, db) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, db, func() {
		newRole := "SECURITYADMIN"
		db.Spec.UseRole = &newRole
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.useRole is immutable")
}

// TestCEL_Database_MutableFieldsAllowed verifies that non-immutable fields
// (e.g., comment) CAN be changed on UPDATE.
func TestCEL_Database_MutableFieldsAllowed(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-db-mutable-ok",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			Name: "MY_DB_M",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, db))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, db) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, db, func() {
		comment := "updated comment"
		db.Spec.Comment = &comment
	})
	require.NoError(t, err, "mutable fields should be changeable")
}

// TestCEL_Database_Defaults verifies that CRD defaults are applied.
func TestCEL_Database_Defaults(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-db-defaults",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			Name: "MY_DB_D",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, db))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, db) })

	// Re-fetch from API server to see defaulted values.
	var fresh snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, &fresh))

	assert.Equal(t, snowplanev1alpha1.DeletionPolicyDelete, fresh.Spec.DeletionPolicy, "deletionPolicy should default to Delete")
	assert.Equal(t, "default", fresh.Spec.ProviderRef.Name, "providerRef.name should default to 'default'")
}

// TestCEL_Schema_ImmutableDatabaseRef verifies that optional pointer ref fields are immutable.
func TestCEL_Schema_ImmutableDatabaseRef(t *testing.T) {
	t.Parallel()

	sch := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-schema-immutable-ref",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			Name:        "MY_SCHEMA",
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: "db1"},
		},
	}

	require.NoError(t, k8sClient.Create(ctx, sch))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, sch) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: sch.Name, Namespace: sch.Namespace}, sch, func() {
		sch.Spec.DatabaseRef = &snowplanev1alpha1.ObjectReference{Name: "db2"}
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.databaseRef is immutable")
}

// TestCEL_FieldExport_Immutable verifies that FieldExport fields are immutable on UPDATE.
func TestCEL_FieldExport_Immutable(t *testing.T) {
	t.Parallel()

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-fe-immutable",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{
					Kind: "Database",
					Name: "my-db",
				},
				Path: ".status.fullyQualifiedName",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
				Name: "my-configmap",
				Key:  "dbName",
			},
		},
	}

	require.NoError(t, k8sClient.Create(ctx, fe))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, fe) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: fe.Name, Namespace: fe.Namespace}, fe, func() {
		fe.Spec.To.Name = "other-configmap"
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.to.name is immutable")
}

// TestCEL_FieldExport_PathValidation verifies CEL rules for FieldExport path.
func TestCEL_FieldExport_PathValidation(t *testing.T) {
	t.Parallel()

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-fe-bad-path",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{
					Kind: "Database",
					Name: "my-db",
				},
				Path: ".spec.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
				Name: "my-cm",
				Key:  "val",
			},
		},
	}

	err := k8sClient.Create(ctx, fe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.from.path must start with")
}

// TestCEL_FieldExport_InvalidSourceKind verifies that invalid source kinds are rejected.
func TestCEL_FieldExport_InvalidSourceKind(t *testing.T) {
	t.Parallel()

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-fe-bad-kind",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{
					Kind: "InvalidResource",
					Name: "my-thing",
				},
				Path: ".status.fullyQualifiedName",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetSecret,
				Name: "my-secret",
				Key:  "val",
			},
		},
	}

	err := k8sClient.Create(ctx, fe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.from.resource.kind must be a supported")
}

// TestCEL_ProviderConfig_ImmutableAccount verifies that spec.account is immutable.
func TestCEL_ProviderConfig_ImmutableAccount(t *testing.T) {
	t.Parallel()

	pc := &snowplanev1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-pc-immutable-account",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.ProviderConfigSpec{
			Account:            "myaccount",
			User:               "myuser",
			AuthenticationType: snowplanev1alpha1.AuthenticationTypeKeyPair,
			Credentials: snowplanev1alpha1.ProviderCredentials{
				SecretRef: &snowplanev1alpha1.SecretKeyReference{
					Name: "my-secret",
					Key:  "private-key",
				},
			},
		},
	}

	require.NoError(t, k8sClient.Create(ctx, pc))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pc) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: pc.Name, Namespace: pc.Namespace}, pc, func() {
		pc.Spec.Account = "otheraccount"
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.account is immutable")
}

// TestCEL_GrantPrivilegesToAccountRole_ImmutablePrivilege verifies that grant fields are immutable.
func TestCEL_GrantPrivilegesToAccountRole_ImmutablePrivilege(t *testing.T) {
	t.Parallel()

	grant := &snowplanev1alpha1.GrantPrivilegesToAccountRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-grant-immutable",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.GrantPrivilegesToAccountRoleSpec{
			Privilege:   "SELECT",
			AccountRole: ptr("MY_ROLE"),
			On: snowplanev1alpha1.GrantOn{
				AccountObject: &snowplanev1alpha1.GrantOnAccountObject{
					ObjectType: "DATABASE",
					ObjectName: "MY_DB",
				},
			},
		},
	}

	require.NoError(t, k8sClient.Create(ctx, grant))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, grant) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: grant.Name, Namespace: grant.Namespace}, grant, func() {
		grant.Spec.Privilege = "USAGE"
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.privilege is immutable")
}

// TestCEL_MaskingPolicy_BodyBlocklist verifies that the SQL body blocklist
// is enforced via CEL.
func TestCEL_MaskingPolicy_BodyBlocklist(t *testing.T) {
	t.Parallel()

	mp := &snowplanev1alpha1.MaskingPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-mp-body-blocked",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.MaskingPolicySpec{
			Name:         "MY_POLICY",
			DatabaseName: ptr("MY_DB"),
			SchemaName:   ptr("MY_SCHEMA"),
			Signature: []snowplanev1alpha1.MaskingPolicyArgument{
				{Name: "val", Type: "VARCHAR"},
			},
			Body: "CASE WHEN true THEN val; DROP TABLE x END",
		},
	}

	err := k8sClient.Create(ctx, mp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked SQL pattern")
}

// TestCEL_MaskingPolicy_BodyAllowed verifies that a clean SQL body is accepted.
func TestCEL_MaskingPolicy_BodyAllowed(t *testing.T) {
	t.Parallel()

	mp := &snowplanev1alpha1.MaskingPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-mp-body-ok",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.MaskingPolicySpec{
			Name:         "CLEAN_POLICY",
			DatabaseName: ptr("MY_DB"),
			SchemaName:   ptr("MY_SCHEMA"),
			Signature: []snowplanev1alpha1.MaskingPolicyArgument{
				{Name: "val", Type: "VARCHAR"},
			},
			Body: "CASE WHEN current_role() IN ('ADMIN') THEN val ELSE '***' END",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, mp))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, mp) })
}

// TestCEL_Stage_ImmutableType verifies that the stage type (internal/external)
// cannot be changed.
func TestCEL_Stage_ImmutableType(t *testing.T) {
	t.Parallel()

	extURL := "s3://my-bucket/data/"
	stage := &snowplanev1alpha1.Stage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-stage-immutable-type",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.StageSpec{
			Name:         "MY_STAGE",
			DatabaseName: ptr("MY_DB"),
			SchemaName:   ptr("MY_SCHEMA"),
			URL:          &extURL,
		},
	}

	require.NoError(t, k8sClient.Create(ctx, stage))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, stage) })

	// Try to remove URL (convert external -> internal) — should fail.
	err := updateWithCELCheck(t, types.NamespacedName{Name: stage.Name, Namespace: stage.Namespace}, stage, func() {
		stage.Spec.URL = nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stage type")
}

// TestCEL_User_DefaultType verifies that the User type defaults to PERSON.
func TestCEL_User_DefaultType(t *testing.T) {
	t.Parallel()

	user := &snowplanev1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-user-default-type",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.UserSpec{
			Name: "MY_USER",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, user))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, user) })

	var fresh snowplanev1alpha1.User
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: user.Name, Namespace: user.Namespace}, &fresh))

	require.NotNil(t, fresh.Spec.Type, "User.Spec.Type should be defaulted")
	assert.Equal(t, snowplanev1alpha1.UserTypePerson, *fresh.Spec.Type, "User type should default to PERSON")
}

// TestCEL_GrantOwnership_ImmutableFields verifies immutable fields on GrantOwnership.
func TestCEL_GrantOwnership_ImmutableFields(t *testing.T) {
	t.Parallel()

	gow := &snowplanev1alpha1.GrantOwnership{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-gow-immutable",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.GrantOwnershipSpec{
			ObjectType:  "TABLE",
			ObjectName:  "MY_DB.PUBLIC.MY_TABLE",
			AccountRole: ptr("MY_ROLE"),
		},
	}

	require.NoError(t, k8sClient.Create(ctx, gow))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, gow) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: gow.Name, Namespace: gow.Namespace}, gow, func() {
		gow.Spec.ObjectType = "VIEW"
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.objectType is immutable")
}

// TestCEL_MutualExclusion_DatabaseRefOrName verifies that exactly one of
// databaseRef or databaseName must be set.
func TestCEL_MutualExclusion_DatabaseRefOrName(t *testing.T) {
	t.Parallel()

	// Neither set — should fail.
	sch := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-schema-no-db",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			Name: "MY_SCHEMA",
		},
	}

	err := k8sClient.Create(ctx, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of spec.databaseRef or spec.databaseName must be set")
}

// TestCEL_ProviderConfig_AuthValidation verifies auth-type credential requirements.
func TestCEL_ProviderConfig_AuthValidation(t *testing.T) {
	t.Parallel()

	// KeyPair without secretRef — should fail.
	pc := &snowplanev1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-pc-auth-missing",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.ProviderConfigSpec{
			Account:            "myaccount",
			User:               "myuser",
			AuthenticationType: snowplanev1alpha1.AuthenticationTypeKeyPair,
		},
	}

	err := k8sClient.Create(ctx, pc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required for KeyPair authentication")
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------
// Additional CEL Integration Tests — newly added immutability rules
// --------------------------------------------------------------------------

// TestCEL_MaskingPolicy_ImmutableSignature verifies that spec.signature is immutable.
func TestCEL_MaskingPolicy_ImmutableSignature(t *testing.T) {
	t.Parallel()

	mp := &snowplanev1alpha1.MaskingPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-mp-immutable-sig",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.MaskingPolicySpec{
			Name:         "SIG_POLICY",
			DatabaseName: ptr("MY_DB"),
			SchemaName:   ptr("MY_SCHEMA"),
			Signature: []snowplanev1alpha1.MaskingPolicyArgument{
				{Name: "val", Type: "VARCHAR"},
			},
			Body: "CASE WHEN true THEN val ELSE '***' END",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, mp))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, mp) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: mp.Name, Namespace: mp.Namespace}, mp, func() {
		mp.Spec.Signature = []snowplanev1alpha1.MaskingPolicyArgument{
			{Name: "val", Type: "NUMBER"},
		}
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.signature is immutable")
}

// TestCEL_RowAccessPolicy_ImmutableSignature verifies that spec.signature is immutable.
func TestCEL_RowAccessPolicy_ImmutableSignature(t *testing.T) {
	t.Parallel()

	rap := &snowplanev1alpha1.RowAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-rap-immutable-sig",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.RowAccessPolicySpec{
			Name:         "SIG_RAP",
			DatabaseName: ptr("MY_DB"),
			SchemaName:   ptr("MY_SCHEMA"),
			Signature: []snowplanev1alpha1.RowAccessPolicyArgument{
				{Name: "col", Type: "VARCHAR"},
			},
			Body: "true",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, rap))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, rap) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: rap.Name, Namespace: rap.Namespace}, rap, func() {
		rap.Spec.Signature = []snowplanev1alpha1.RowAccessPolicyArgument{
			{Name: "col", Type: "NUMBER"},
		}
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.signature is immutable")
}

// TestCEL_RowAccessPolicy_BodyBlocklist verifies that the SQL body blocklist is enforced.
func TestCEL_RowAccessPolicy_BodyBlocklist(t *testing.T) {
	t.Parallel()

	rap := &snowplanev1alpha1.RowAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-rap-body-blocked",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.RowAccessPolicySpec{
			Name:         "BAD_RAP",
			DatabaseName: ptr("MY_DB"),
			SchemaName:   ptr("MY_SCHEMA"),
			Signature: []snowplanev1alpha1.RowAccessPolicyArgument{
				{Name: "col", Type: "VARCHAR"},
			},
			Body: "CASE WHEN true THEN true; DROP TABLE x END",
		},
	}

	err := k8sClient.Create(ctx, rap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked SQL pattern")
}

// TestCEL_GrantPrivilegesToDatabaseRole_ImmutableDatabaseRoleRef verifies that spec.databaseRoleRef is immutable.
func TestCEL_GrantPrivilegesToDatabaseRole_ImmutableDatabaseRoleRef(t *testing.T) {
	t.Parallel()

	grant := &snowplanev1alpha1.GrantPrivilegesToDatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-drg-immutable-ref",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.GrantPrivilegesToDatabaseRoleSpec{
			Privilege:       "USAGE",
			DatabaseRoleRef: &snowplanev1alpha1.ObjectReference{Name: "role1"},
			On: snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
				Database: ptr("MY_DB"),
			},
		},
	}

	require.NoError(t, k8sClient.Create(ctx, grant))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, grant) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: grant.Name, Namespace: grant.Namespace}, grant, func() {
		grant.Spec.DatabaseRoleRef = &snowplanev1alpha1.ObjectReference{Name: "role2"}
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.databaseRoleRef is immutable")
}

// TestCEL_GrantPrivilegesToAccountRole_ImmutableAccountRoleRef verifies that spec.accountRoleRef is immutable.
func TestCEL_GrantPrivilegesToAccountRole_ImmutableAccountRoleRef(t *testing.T) {
	t.Parallel()

	grant := &snowplanev1alpha1.GrantPrivilegesToAccountRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-arg-immutable-ref",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.GrantPrivilegesToAccountRoleSpec{
			Privilege:      "SELECT",
			AccountRoleRef: &snowplanev1alpha1.ObjectReference{Name: "role1"},
			On: snowplanev1alpha1.GrantOn{
				AccountObject: &snowplanev1alpha1.GrantOnAccountObject{
					ObjectType: "DATABASE",
					ObjectName: "MY_DB",
				},
			},
		},
	}

	require.NoError(t, k8sClient.Create(ctx, grant))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, grant) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: grant.Name, Namespace: grant.Namespace}, grant, func() {
		grant.Spec.AccountRoleRef = &snowplanev1alpha1.ObjectReference{Name: "role2"}
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.accountRoleRef is immutable")
}

// TestCEL_Warehouse_ImmutableName verifies that warehouse spec.name is immutable.
func TestCEL_Warehouse_ImmutableName(t *testing.T) {
	t.Parallel()

	wh := &snowplanev1alpha1.Warehouse{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-wh-immutable-name",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.WarehouseSpec{
			Name: "MY_WH",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, wh))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, wh) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: wh.Name, Namespace: wh.Namespace}, wh, func() {
		wh.Spec.Name = "OTHER_WH"
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

// TestCEL_Table_ImmutableName verifies that table spec.name is immutable.
func TestCEL_Table_ImmutableName(t *testing.T) {
	t.Parallel()

	tbl := &snowplanev1alpha1.Table{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-tbl-immutable-name",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.TableSpec{
			Name:         "MY_TABLE",
			DatabaseName: ptr("MY_DB"),
			SchemaName:   ptr("MY_SCHEMA"),
			Columns: []snowplanev1alpha1.ColumnDefinition{
				{Name: "ID", Type: "NUMBER(38,0)"},
			},
		},
	}

	require.NoError(t, k8sClient.Create(ctx, tbl))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, tbl) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: tbl.Name, Namespace: tbl.Namespace}, tbl, func() {
		tbl.Spec.Name = "OTHER_TABLE"
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

// TestCEL_NetworkPolicy_ImmutableName verifies that networkpolicy spec.name is immutable.
func TestCEL_NetworkPolicy_ImmutableName(t *testing.T) {
	t.Parallel()

	np := &snowplanev1alpha1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-np-immutable-name",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.NetworkPolicySpec{
			Name: "MY_NP",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, np))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, np) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: np.Name, Namespace: np.Namespace}, np, func() {
		np.Spec.Name = "OTHER_NP"
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

// TestCEL_Task_ImmutableName verifies that task spec.name is immutable.
func TestCEL_Task_ImmutableName(t *testing.T) {
	t.Parallel()

	task := &snowplanev1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-task-immutable-name",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.TaskSpec{
			Name:         "MY_TASK",
			DatabaseName: ptr("MY_DB"),
			SchemaName:   ptr("MY_SCHEMA"),
			SQLStatement: "SELECT 1",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, task))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, task) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: task.Name, Namespace: task.Namespace}, task, func() {
		task.Spec.Name = "OTHER_TASK"
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

// TestCEL_Task_WarehouseMutualExclusion verifies that warehouse and
// userTaskManagedInitialWarehouseSize are mutually exclusive via CEL.
func TestCEL_Task_WarehouseMutualExclusion(t *testing.T) {
	t.Parallel()

	wh := "MY_WH"
	size := "SMALL"
	task := &snowplanev1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-task-wh-mutex",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.TaskSpec{
			Name:                                "MUTEX_TASK",
			DatabaseName:                        ptr("MY_DB"),
			SchemaName:                          ptr("MY_SCHEMA"),
			SQLStatement:                        "SELECT 1",
			WarehouseName:                       &wh,
			UserTaskManagedInitialWarehouseSize: &size,
		},
	}

	err := k8sClient.Create(ctx, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

// TestCEL_ResourceMonitor_ImmutableName verifies that resource monitor spec.name is immutable.
func TestCEL_ResourceMonitor_ImmutableName(t *testing.T) {
	t.Parallel()

	rm := &snowplanev1alpha1.ResourceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-rm-immutable-name",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.ResourceMonitorSpec{
			Name: "MY_MONITOR",
		},
	}

	require.NoError(t, k8sClient.Create(ctx, rm))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, rm) })

	err := updateWithCELCheck(t, types.NamespacedName{Name: rm.Name, Namespace: rm.Namespace}, rm, func() {
		rm.Spec.Name = "OTHER_MONITOR"
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

// --------------------------------------------------------------------------
// CEL Dot-Validation Tests — databaseName / schemaName must not contain dots
//
// These tests verify the L-13 dot-validation CEL rules that reject
// fully-qualified names in databaseName / schemaName fields. Users must use
// simple identifiers (e.g. "MY_DB") and set databaseName / schemaName
// separately rather than using "MY_DB.MY_SCHEMA" in a single field.
// --------------------------------------------------------------------------

// TestCEL_Schema_DatabaseNameNoDots verifies that Schema's databaseName rejects dots.
func TestCEL_Schema_DatabaseNameNoDots(t *testing.T) {
	t.Parallel()

	sch := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-schema-dbname-dots",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			Name:         "MY_SCHEMA",
			DatabaseName: ptr("MY_DB.EXTRA"),
		},
	}

	err := k8sClient.Create(ctx, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simple identifier")
}

// TestCEL_Table_DatabaseNameNoDots verifies that Table's databaseName rejects dots.
func TestCEL_Table_DatabaseNameNoDots(t *testing.T) {
	t.Parallel()

	tbl := &snowplanev1alpha1.Table{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-tbl-dbname-dots",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.TableSpec{
			Name:         "MY_TABLE",
			DatabaseName: ptr("MY_DB.EXTRA"),
			SchemaName:   ptr("MY_SCHEMA"),
			Columns: []snowplanev1alpha1.ColumnDefinition{
				{Name: "ID", Type: "NUMBER(38,0)"},
			},
		},
	}

	err := k8sClient.Create(ctx, tbl)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simple identifier")
}

// TestCEL_Table_SchemaNameNoDots verifies that Table's schemaName rejects dots.
func TestCEL_Table_SchemaNameNoDots(t *testing.T) {
	t.Parallel()

	tbl := &snowplanev1alpha1.Table{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-tbl-schemaname-dots",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.TableSpec{
			Name:         "MY_TABLE",
			DatabaseName: ptr("MY_DB"),
			SchemaName:   ptr("MY_DB.MY_SCHEMA"),
			Columns: []snowplanev1alpha1.ColumnDefinition{
				{Name: "ID", Type: "NUMBER(38,0)"},
			},
		},
	}

	err := k8sClient.Create(ctx, tbl)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simple identifier")
}

// TestCEL_View_DatabaseNameNoDots verifies that View's databaseName rejects dots.
func TestCEL_View_DatabaseNameNoDots(t *testing.T) {
	t.Parallel()

	vw := &snowplanev1alpha1.View{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-view-dbname-dots",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.ViewSpec{
			Name:         "MY_VIEW",
			DatabaseName: ptr("DB.EXTRA"),
			SchemaName:   ptr("MY_SCHEMA"),
			Statement:    "SELECT 1",
		},
	}

	err := k8sClient.Create(ctx, vw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simple identifier")
}

// TestCEL_Stage_SchemaNameNoDots verifies that Stage's schemaName rejects dots.
func TestCEL_Stage_SchemaNameNoDots(t *testing.T) {
	t.Parallel()

	stg := &snowplanev1alpha1.Stage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-stage-schema-dots",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.StageSpec{
			Name:         "MY_STAGE",
			DatabaseName: ptr("MY_DB"),
			SchemaName:   ptr("MY_DB.MY_SCHEMA"),
		},
	}

	err := k8sClient.Create(ctx, stg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simple identifier")
}

// TestCEL_ValidSimpleIdentifiers verifies that simple identifiers (no dots) are accepted.
func TestCEL_ValidSimpleIdentifiers(t *testing.T) {
	t.Parallel()

	tbl := &snowplanev1alpha1.Table{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-tbl-simple-ids",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.TableSpec{
			Name:         "MY_TABLE",
			DatabaseName: ptr("MY_DB"),
			SchemaName:   ptr("MY_SCHEMA"),
			Columns: []snowplanev1alpha1.ColumnDefinition{
				{Name: "ID", Type: "NUMBER(38,0)"},
			},
		},
	}

	require.NoError(t, k8sClient.Create(ctx, tbl))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, tbl) })
}
