package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// ---- errors.Join: multi-error scenarios ----

func TestDatabaseValidator_MultipleViolations(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("mydb", false, 1)
	newObj := newDB("renamed", true, 1)
	empty := ""
	newObj.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, newObj))

	require.False(t, resp.Allowed)
	msg := resp.Result.Message
	assert.Contains(t, msg, "spec.useRole must not be an empty string")
	assert.Contains(t, msg, "spec.name is immutable")
	assert.Contains(t, msg, "spec.transient is immutable")
}

func TestSchemaValidator_MultipleViolations(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	oldS := newSchema("myschema", "mydb", false, 1)
	newS := newSchema("renamed", "otherdb", true, 1)
	empty := ""
	newS.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, newS))

	require.False(t, resp.Allowed)
	msg := resp.Result.Message
	assert.Contains(t, msg, "spec.useRole must not be an empty string")
	assert.Contains(t, msg, "spec.name is immutable")
	assert.Contains(t, msg, "spec.databaseRef.name is immutable")
	assert.Contains(t, msg, "spec.transient is immutable")
}

func TestWarehouseValidator_MultipleViolations(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	oldWH := newWarehouse("MYWH", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeStandard), 1)
	newWH := newWarehouse("RENAMED", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeSnowparkOptimized), 1)
	empty := ""
	newWH.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeUpdateRequest(oldWH, newWH))

	require.False(t, resp.Allowed)
	msg := resp.Result.Message
	assert.Contains(t, msg, "spec.useRole must not be an empty string")
	assert.Contains(t, msg, "spec.name is immutable")
	// warehouseType is now mutable — should NOT appear as a violation
	assert.NotContains(t, msg, "spec.warehouseType is immutable")
}

func TestUserValidator_MultipleViolations(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	oldUser := newUser("MY_USER", ptrUserType(snowplanev1alpha1.UserTypePerson), 1)
	badType := snowplanev1alpha1.UserType("BAD")
	newU := newUser("CHANGED", &badType, 1)
	empty := ""
	newU.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeUpdateRequest(oldUser, newU))

	require.False(t, resp.Allowed)
	msg := resp.Result.Message
	assert.Contains(t, msg, "spec.type must be one of")
	assert.Contains(t, msg, "spec.useRole must not be an empty string")
	assert.Contains(t, msg, "spec.name is immutable")
	assert.Contains(t, msg, "spec.type is immutable")
}

// ---- ForceNew: immutability bypass ----

func setForceNew(annotations map[string]string) map[string]string {
	if annotations == nil {
		annotations = map[string]string{}
	}

	annotations[snowplanev1alpha1.AnnotationForceNew] = "true"

	return annotations
}

func TestDatabaseValidator_ForceNewBypassesImmutability(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("mydb", false, 1)
	newObj := newDB("renamed", true, 1)
	newObj.Annotations = setForceNew(newObj.Annotations)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, newObj))

	assert.True(t, resp.Allowed, "force-new should bypass immutable-field checks")
}

func TestDatabaseValidator_ForceNewStillValidatesSpec(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("mydb", false, 1)
	newObj := newDB("renamed", true, 1)
	newObj.Annotations = setForceNew(newObj.Annotations)
	empty := ""
	newObj.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, newObj))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole must not be an empty string")
	assert.NotContains(t, resp.Result.Message, "spec.name is immutable")
}

func TestSchemaValidator_ForceNewBypassesImmutability(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	oldS := newSchema("myschema", "mydb", false, 1)
	newS := newSchema("renamed", "otherdb", true, 1)
	newS.Annotations = setForceNew(newS.Annotations)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, newS))

	assert.True(t, resp.Allowed)
}

func TestWarehouseValidator_ForceNewBypassesImmutability(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	// Only name and useRole are immutable now; warehouseType is mutable.
	oldWH := newWarehouse("MYWH", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeStandard), 1)
	newWH := newWarehouse("RENAMED", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeSnowparkOptimized), 1)
	newWH.Annotations = setForceNew(newWH.Annotations)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldWH, newWH))

	assert.True(t, resp.Allowed)
}

func TestAccountRoleValidator_ForceNewBypassesImmutability(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleValidator(testScheme())
	oldRole := newAccountRole("MY_ROLE", 1)
	newRole := newAccountRole("CHANGED_ROLE", 1)
	newRole.Annotations = setForceNew(newRole.Annotations)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldRole, newRole))

	assert.True(t, resp.Allowed)
}

func TestUserValidator_ForceNewBypassesImmutability(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	oldUser := newUser("MY_USER", ptrUserType(snowplanev1alpha1.UserTypePerson), 1)
	newU := newUser("CHANGED", ptrUserType(snowplanev1alpha1.UserTypeService), 1)
	newU.Annotations = setForceNew(newU.Annotations)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldUser, newU))

	assert.True(t, resp.Allowed)
}

// ---- Dangerous Grant (AccountRoleGrant) ----

func newAccountRoleGrant(privilege string, on snowplanev1alpha1.GrantOn, accountRole string) *snowplanev1alpha1.AccountRoleGrant {
	return &snowplanev1alpha1.AccountRoleGrant{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "AccountRoleGrant",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grant",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.AccountRoleGrantSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Privilege:   privilege,
			On:          on,
			AccountRole: accountRole,
		},
	}
}

func TestAccountRoleGrantValidator_BlocksDangerousSystemRole(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleGrantValidator(testScheme())
	g := newAccountRoleGrant("USAGE", snowplanev1alpha1.GrantOn{Account: true}, "ACCOUNTADMIN")

	resp := v.Handle(context.Background(), makeCreateRequest(g))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "system role ACCOUNTADMIN")
}

func TestAccountRoleGrantValidator_BlocksManageGrantsOnAccount(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleGrantValidator(testScheme())
	g := newAccountRoleGrant("MANAGE GRANTS", snowplanev1alpha1.GrantOn{Account: true}, "CUSTOM_ROLE")

	resp := v.Handle(context.Background(), makeCreateRequest(g))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "MANAGE GRANTS on ACCOUNT")
}

func TestAccountRoleGrantValidator_AllowsDangerousWithAnnotation(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleGrantValidator(testScheme())
	g := newAccountRoleGrant("MANAGE GRANTS", snowplanev1alpha1.GrantOn{Account: true}, "ACCOUNTADMIN")
	g.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationAllowDangerousGrant: "true",
	}

	resp := v.Handle(context.Background(), makeCreateRequest(g))

	assert.True(t, resp.Allowed, "annotation should bypass dangerous-grant checks")
}

func TestAccountRoleGrantValidator_SafeGrantAllowed(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleGrantValidator(testScheme())
	g := newAccountRoleGrant("CREATE DATABASE", snowplanev1alpha1.GrantOn{Account: true}, "DATA_READER")

	resp := v.Handle(context.Background(), makeCreateRequest(g))

	assert.True(t, resp.Allowed)
}

func TestAccountRoleGrantValidator_AnnotationFalseStillBlocks(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleGrantValidator(testScheme())
	g := newAccountRoleGrant("USAGE", snowplanev1alpha1.GrantOn{Account: true}, "SECURITYADMIN")
	g.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationAllowDangerousGrant: "false",
	}

	resp := v.Handle(context.Background(), makeCreateRequest(g))

	require.False(t, resp.Allowed, "annotation 'false' should NOT bypass")
	assert.Contains(t, resp.Result.Message, "system role SECURITYADMIN")
}
