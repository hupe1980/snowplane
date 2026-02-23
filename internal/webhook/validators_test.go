package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()

	if err := snowplanev1alpha1.AddToScheme(s); err != nil {
		panic(fmt.Sprintf("failed to register scheme: %v", err))
	}

	return s
}

func newDB(name string, transient bool, observedGen int64) *snowplanev1alpha1.Database {
	return &snowplanev1alpha1.Database{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "Database",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-db",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:       name,
			Transient:  transient,
		},
		Status: snowplanev1alpha1.DatabaseStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

func newSchema(name, dbRef string, transient bool, observedGen int64) *snowplanev1alpha1.Schema {
	return &snowplanev1alpha1.Schema{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "Schema",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-schema",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:        name,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRef},
			Transient:   transient,
		},
		Status: snowplanev1alpha1.SchemaStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

func makeUpdateRequest(oldObj, newObj runtime.Object) admission.Request {
	oldRaw, err := json.Marshal(oldObj)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal oldObj: %v", err))
	}

	newRaw, err := json.Marshal(newObj)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal newObj: %v", err))
	}

	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			OldObject: runtime.RawExtension{Raw: oldRaw},
			Object:    runtime.RawExtension{Raw: newRaw},
		},
	}
}

func makeCreateRequest(obj runtime.Object) admission.Request {
	raw, err := json.Marshal(obj)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal obj: %v", err))
	}

	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: raw},
		},
	}
}

// --- Database Validator Tests ---

func TestDatabaseValidator_AllowsCreate(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(newDB("mydb", false, 0)))

	assert.True(t, resp.Allowed)
}

func TestDatabaseValidator_AllowsUpdateBeforeFirstReconcile(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("mydb", false, 0)
	updatedDB := newDB("renamed", true, 0)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, updatedDB))

	assert.True(t, resp.Allowed)
}

func TestDatabaseValidator_AllowsMutableFieldChange(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("mydb", false, 1)
	updatedDB := newDB("mydb", false, 1)
	comment := "updated comment"
	updatedDB.Spec.Comment = &comment

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, updatedDB))

	assert.True(t, resp.Allowed)
}

func TestDatabaseValidator_DeniesNameChange(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("mydb", false, 1)
	renamedDB := newDB("renamed", false, 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, renamedDB))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.name is immutable")
}

func TestDatabaseValidator_DeniesTransientChange(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("mydb", false, 1)
	transientDB := newDB("mydb", true, 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, transientDB))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.transient is immutable")
}

func TestDatabaseValidator_DeniesEmptyUseRoleOnCreate(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	db := newDB("mydb", false, 0)
	empty := ""
	db.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeCreateRequest(db))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole must not be an empty string")
}

func TestDatabaseValidator_DeniesEmptyUseRoleOnUpdate(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("mydb", false, 1)
	updatedDB := newDB("mydb", false, 1)
	empty := ""
	updatedDB.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, updatedDB))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole must not be an empty string")
}

func TestDatabaseValidator_AllowsValidUseRole(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	db := newDB("mydb", false, 0)
	role := "SYSADMIN"
	db.Spec.UseRole = &role

	resp := v.Handle(context.Background(), makeCreateRequest(db))

	assert.True(t, resp.Allowed)
}

// --- Schema Validator Tests ---

func TestSchemaValidator_AllowsCreate(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(newSchema("myschema", "mydb", false, 0)))

	assert.True(t, resp.Allowed)
}

func TestSchemaValidator_AllowsUpdateBeforeFirstReconcile(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	oldS := newSchema("myschema", "mydb", false, 0)
	newS := newSchema("renamed", "otherdb", true, 0)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, newS))

	assert.True(t, resp.Allowed)
}

func TestSchemaValidator_AllowsMutableFieldChange(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	oldS := newSchema("myschema", "mydb", false, 1)
	updatedS := newSchema("myschema", "mydb", false, 1)
	comment := "updated"
	updatedS.Spec.Comment = &comment

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, updatedS))

	assert.True(t, resp.Allowed)
}

func TestSchemaValidator_DeniesNameChange(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	oldS := newSchema("myschema", "mydb", false, 1)
	renamedS := newSchema("renamed", "mydb", false, 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, renamedS))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.name is immutable")
}

func TestSchemaValidator_DeniesDatabaseRefChange(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	oldS := newSchema("myschema", "mydb", false, 1)
	changedS := newSchema("myschema", "otherdb", false, 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, changedS))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.databaseRef.name is immutable")
}

func TestSchemaValidator_DeniesTransientChange(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	oldS := newSchema("myschema", "mydb", false, 1)
	transientS := newSchema("myschema", "mydb", true, 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, transientS))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.transient is immutable")
}

func TestSchemaValidator_DeniesEmptyUseRoleOnCreate(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	s := newSchema("myschema", "mydb", false, 0)
	empty := ""
	s.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeCreateRequest(s))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole must not be an empty string")
}

func TestSchemaValidator_DeniesEmptyUseRoleOnUpdate(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	oldS := newSchema("myschema", "mydb", false, 1)
	updatedS := newSchema("myschema", "mydb", false, 1)
	empty := ""
	updatedS.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, updatedS))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole must not be an empty string")
}

func TestSchemaValidator_AllowsValidUseRole(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	s := newSchema("myschema", "mydb", false, 0)
	role := "SYSADMIN"
	s.Spec.UseRole = &role

	resp := v.Handle(context.Background(), makeCreateRequest(s))

	assert.True(t, resp.Allowed)
}

// --- Warehouse Validator Tests ---

func ptrWarehouseType(wt snowplanev1alpha1.WarehouseType) *snowplanev1alpha1.WarehouseType {
	return &wt
}

func newWarehouse(name string, whType *snowplanev1alpha1.WarehouseType, observedGen int64) *snowplanev1alpha1.Warehouse {
	return &snowplanev1alpha1.Warehouse{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "Warehouse",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-wh",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.WarehouseSpec{
			CommonSpec:    snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:          name,
			WarehouseType: whType,
		},
		Status: snowplanev1alpha1.WarehouseStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

func TestWarehouseValidator_AllowsCreate(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(newWarehouse("MYWH", nil, 0)))

	assert.True(t, resp.Allowed)
}

func TestWarehouseValidator_AllowsUpdateBeforeFirstReconcile(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	oldWH := newWarehouse("MYWH", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeStandard), 0)
	newWH := newWarehouse("RENAMED", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeSnowparkOptimized), 0)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldWH, newWH))

	assert.True(t, resp.Allowed)
}

func TestWarehouseValidator_AllowsMutableFieldChange(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	oldWH := newWarehouse("MYWH", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeStandard), 1)
	updatedWH := newWarehouse("MYWH", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeStandard), 1)
	comment := "updated comment"
	updatedWH.Spec.Comment = &comment

	resp := v.Handle(context.Background(), makeUpdateRequest(oldWH, updatedWH))

	assert.True(t, resp.Allowed)
}

func TestWarehouseValidator_DeniesNameChange(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	oldWH := newWarehouse("MYWH", nil, 1)
	renamedWH := newWarehouse("RENAMED", nil, 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldWH, renamedWH))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.name is immutable")
}

func TestWarehouseValidator_AllowsWarehouseTypeChange(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	oldWH := newWarehouse("MYWH", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeStandard), 1)
	changedWH := newWarehouse("MYWH", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeSnowparkOptimized), 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldWH, changedWH))

	assert.True(t, resp.Allowed)
}

func TestWarehouseValidator_AllowsWarehouseTypeSetFromNil(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	oldWH := newWarehouse("MYWH", nil, 1) // created without explicit type
	changedWH := newWarehouse("MYWH", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeSnowparkOptimized), 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldWH, changedWH))

	assert.True(t, resp.Allowed)
}

func TestWarehouseValidator_AllowsWarehouseTypeUnset(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	oldWH := newWarehouse("MYWH", ptrWarehouseType(snowplanev1alpha1.WarehouseTypeStandard), 1)
	changedWH := newWarehouse("MYWH", nil, 1) // trying to unset

	resp := v.Handle(context.Background(), makeUpdateRequest(oldWH, changedWH))

	assert.True(t, resp.Allowed)
}

func TestWarehouseValidator_DeniesEmptyUseRoleOnCreate(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	wh := newWarehouse("MYWH", nil, 0)
	empty := ""
	wh.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeCreateRequest(wh))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole must not be an empty string")
}

func TestWarehouseValidator_DeniesEmptyUseRoleOnUpdate(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	oldWH := newWarehouse("MYWH", nil, 1)
	updatedWH := newWarehouse("MYWH", nil, 1)
	empty := ""
	updatedWH.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeUpdateRequest(oldWH, updatedWH))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole must not be an empty string")
}

func TestWarehouseValidator_AllowsValidUseRole(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	wh := newWarehouse("MYWH", nil, 0)
	role := "SYSADMIN"
	wh.Spec.UseRole = &role

	resp := v.Handle(context.Background(), makeCreateRequest(wh))

	assert.True(t, resp.Allowed)
}

// --- AccountRole Validator Tests ---

func newAccountRole(name string, observedGen int64) *snowplanev1alpha1.AccountRole {
	return &snowplanev1alpha1.AccountRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "AccountRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.AccountRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:       name,
		},
		Status: snowplanev1alpha1.AccountRoleStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

func TestAccountRoleValidator_AllowsCreate(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(newAccountRole("MY_ROLE", 0)))

	require.True(t, resp.Allowed)
}

func TestAccountRoleValidator_AllowsUpdateBeforeFirstReconcile(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleValidator(testScheme())
	oldRole := newAccountRole("MY_ROLE", 0) // not yet reconciled
	newRole := newAccountRole("CHANGED_ROLE", 0)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldRole, newRole))

	require.True(t, resp.Allowed)
}

func TestAccountRoleValidator_AllowsMutableFieldChange(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleValidator(testScheme())
	oldRole := newAccountRole("MY_ROLE", 1)
	newRole := newAccountRole("MY_ROLE", 1)
	comment := "new comment"
	newRole.Spec.Comment = &comment

	resp := v.Handle(context.Background(), makeUpdateRequest(oldRole, newRole))

	require.True(t, resp.Allowed)
}

func TestAccountRoleValidator_DeniesNameChange(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleValidator(testScheme())
	oldRole := newAccountRole("MY_ROLE", 1)
	newRole := newAccountRole("CHANGED_ROLE", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldRole, newRole))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.name is immutable")
}

func TestAccountRoleValidator_DeniesEmptyUseRoleOnCreate(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleValidator(testScheme())
	role := newAccountRole("MY_ROLE", 0)
	empty := ""
	role.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeCreateRequest(role))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole must not be an empty string")
}

func TestAccountRoleValidator_DeniesEmptyUseRoleOnUpdate(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleValidator(testScheme())
	oldRole := newAccountRole("MY_ROLE", 1)
	updatedRole := newAccountRole("MY_ROLE", 1)
	empty := ""
	updatedRole.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeUpdateRequest(oldRole, updatedRole))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole must not be an empty string")
}

func TestAccountRoleValidator_AllowsValidUseRole(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleValidator(testScheme())
	role := newAccountRole("MY_ROLE", 0)
	useRole := "USERADMIN"
	role.Spec.UseRole = &useRole

	resp := v.Handle(context.Background(), makeCreateRequest(role))

	assert.True(t, resp.Allowed)
}

// --- User Validator Tests ---

func newUser(name string, userType *snowplanev1alpha1.UserType, observedGen int64) *snowplanev1alpha1.User {
	return &snowplanev1alpha1.User{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "User",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-user",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.UserSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:       name,
			Type:       userType,
		},
		Status: snowplanev1alpha1.UserStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

func ptrUserType(t snowplanev1alpha1.UserType) *snowplanev1alpha1.UserType {
	return &t
}

func TestUserValidator_AllowsCreate(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(newUser("MY_USER", nil, 0)))

	require.True(t, resp.Allowed)
}

func TestUserValidator_AllowsCreateWithType(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(newUser("MY_USER", ptrUserType(snowplanev1alpha1.UserTypeService), 0)))

	require.True(t, resp.Allowed)
}

func TestUserValidator_DeniesInvalidType(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	bad := snowplanev1alpha1.UserType("INVALID")
	resp := v.Handle(context.Background(), makeCreateRequest(newUser("MY_USER", &bad, 0)))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.type must be one of")
}

func TestUserValidator_AllowsUpdateBeforeFirstReconcile(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	oldUser := newUser("MY_USER", nil, 0) // not yet reconciled
	newU := newUser("CHANGED_USER", nil, 0)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldUser, newU))

	require.True(t, resp.Allowed)
}

func TestUserValidator_AllowsMutableFieldChange(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	oldUser := newUser("MY_USER", nil, 1)
	newU := newUser("MY_USER", nil, 1)
	comment := "new comment"
	newU.Spec.Comment = &comment

	resp := v.Handle(context.Background(), makeUpdateRequest(oldUser, newU))

	require.True(t, resp.Allowed)
}

func TestUserValidator_DeniesNameChange(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	oldUser := newUser("MY_USER", nil, 1)
	newU := newUser("CHANGED_USER", nil, 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldUser, newU))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.name is immutable")
}

func TestUserValidator_DeniesTypeChange(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	oldUser := newUser("MY_USER", ptrUserType(snowplanev1alpha1.UserTypePerson), 1)
	newU := newUser("MY_USER", ptrUserType(snowplanev1alpha1.UserTypeService), 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldUser, newU))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.type is immutable")
}

func TestUserValidator_DeniesTypeSetAfterCreation(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	oldUser := newUser("MY_USER", nil, 1)
	newU := newUser("MY_USER", ptrUserType(snowplanev1alpha1.UserTypeService), 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldUser, newU))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.type is immutable")
}

func TestUserValidator_DeniesTypeUnset(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	oldUser := newUser("MY_USER", ptrUserType(snowplanev1alpha1.UserTypePerson), 1)
	newU := newUser("MY_USER", nil, 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldUser, newU))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.type is immutable")
}

func TestUserValidator_DeniesEmptyUseRoleOnCreate(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	user := newUser("MY_USER", nil, 0)
	empty := ""
	user.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeCreateRequest(user))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole must not be an empty string")
}

func TestUserValidator_DeniesEmptyUseRoleOnUpdate(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	oldUser := newUser("MY_USER", nil, 1)
	updatedUser := newUser("MY_USER", nil, 1)
	empty := ""
	updatedUser.Spec.UseRole = &empty

	resp := v.Handle(context.Background(), makeUpdateRequest(oldUser, updatedUser))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole must not be an empty string")
}

func TestUserValidator_AllowsValidUseRole(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	user := newUser("MY_USER", nil, 0)
	useRole := "USERADMIN"
	user.Spec.UseRole = &useRole

	resp := v.Handle(context.Background(), makeCreateRequest(user))

	assert.True(t, resp.Allowed)
}

// --- UseRole Immutability Tests (all 5 resource types) ---

func ptrString(s string) *string { return &s }

func TestDatabaseValidator_DeniesUseRoleChange(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("MYDB", false, 1)
	oldDB.Spec.UseRole = ptrString("ROLE_A")
	newDB := newDB("MYDB", false, 1)
	newDB.Spec.UseRole = ptrString("ROLE_B")

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, newDB))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole is immutable")
}

func TestDatabaseValidator_AllowsUseRoleUnchanged(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("MYDB", false, 1)
	oldDB.Spec.UseRole = ptrString("DATA_ADMIN")
	newDB := newDB("MYDB", false, 1)
	newDB.Spec.UseRole = ptrString("DATA_ADMIN")

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, newDB))

	assert.True(t, resp.Allowed)
}

func TestSchemaValidator_DeniesUseRoleChange(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	oldS := newSchema("myschema", "mydb", false, 1)
	oldS.Spec.UseRole = ptrString("ROLE_A")
	newS := newSchema("myschema", "mydb", false, 1)
	newS.Spec.UseRole = ptrString("ROLE_B")

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, newS))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole is immutable")
}

func TestWarehouseValidator_DeniesUseRoleChange(t *testing.T) {
	t.Parallel()

	v := NewWarehouseValidator(testScheme())
	oldWH := newWarehouse("MYWH", nil, 1)
	oldWH.Spec.UseRole = ptrString("ROLE_A")
	newWH := newWarehouse("MYWH", nil, 1)
	newWH.Spec.UseRole = ptrString("ROLE_B")

	resp := v.Handle(context.Background(), makeUpdateRequest(oldWH, newWH))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole is immutable")
}

func TestAccountRoleValidator_DeniesUseRoleChange(t *testing.T) {
	t.Parallel()

	v := NewAccountRoleValidator(testScheme())
	oldRole := newAccountRole("MY_ROLE", 1)
	oldRole.Spec.UseRole = ptrString("USERADMIN")
	newRole := newAccountRole("MY_ROLE", 1)
	newRole.Spec.UseRole = ptrString("SECURITYADMIN")

	resp := v.Handle(context.Background(), makeUpdateRequest(oldRole, newRole))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole is immutable")
}

func TestUserValidator_DeniesUseRoleChange(t *testing.T) {
	t.Parallel()

	v := NewUserValidator(testScheme())
	oldUser := newUser("MY_USER", nil, 1)
	oldUser.Spec.UseRole = ptrString("USERADMIN")
	newU := newUser("MY_USER", nil, 1)
	newU.Spec.UseRole = ptrString("SECURITYADMIN")

	resp := v.Handle(context.Background(), makeUpdateRequest(oldUser, newU))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole is immutable")
}

func TestDatabaseValidator_DeniesUseRoleSetFromNil(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("MYDB", false, 1) // no useRole
	newDB := newDB("MYDB", false, 1)
	newDB.Spec.UseRole = ptrString("DATA_ADMIN")

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, newDB))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole is immutable")
}

func TestDatabaseValidator_DeniesUseRoleUnset(t *testing.T) {
	t.Parallel()

	v := NewDatabaseValidator(testScheme())
	oldDB := newDB("MYDB", false, 1)
	oldDB.Spec.UseRole = ptrString("DATA_ADMIN")
	newDB := newDB("MYDB", false, 1) // unset useRole

	resp := v.Handle(context.Background(), makeUpdateRequest(oldDB, newDB))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.useRole is immutable")
}

// --- ProviderConfig Validator Tests ---

func newProviderConfig(account, user string, authType snowplanev1alpha1.AuthenticationType) *snowplanev1alpha1.ProviderConfig {
	creds := snowplanev1alpha1.ProviderCredentials{}
	if authType == snowplanev1alpha1.AuthenticationTypeKeyPair ||
		authType == snowplanev1alpha1.AuthenticationTypeUsernamePassword {
		creds.SecretRef = &snowplanev1alpha1.SecretKeyReference{Name: "cred-secret", Key: "key"}
	}

	return &snowplanev1alpha1.ProviderConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "ProviderConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-provider",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.ProviderConfigSpec{
			Account:            account,
			User:               user,
			AuthenticationType: authType,
			Credentials:        creds,
		},
	}
}

func TestProviderConfigValidator_AllowsValidCreate(t *testing.T) {
	t.Parallel()

	v := NewProviderConfigValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(
		newProviderConfig("xy12345", "admin", snowplanev1alpha1.AuthenticationTypeKeyPair),
	))

	assert.True(t, resp.Allowed)
}

func TestProviderConfigValidator_AllowsEmptyAuthType(t *testing.T) {
	t.Parallel()

	v := NewProviderConfigValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(
		newProviderConfig("xy12345", "admin", ""),
	))

	assert.True(t, resp.Allowed)
}

func TestProviderConfigValidator_DeniesEmptyAccount(t *testing.T) {
	t.Parallel()

	v := NewProviderConfigValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(
		newProviderConfig("", "admin", snowplanev1alpha1.AuthenticationTypeKeyPair),
	))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.account is required")
}

func TestProviderConfigValidator_DeniesEmptyUser(t *testing.T) {
	t.Parallel()

	v := NewProviderConfigValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(
		newProviderConfig("xy12345", "", snowplanev1alpha1.AuthenticationTypeKeyPair),
	))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.user is required")
}

func TestProviderConfigValidator_DeniesInvalidAuthType(t *testing.T) {
	t.Parallel()

	v := NewProviderConfigValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(
		newProviderConfig("xy12345", "admin", snowplanev1alpha1.AuthenticationType("BadAuth")),
	))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.authenticationType must be one of")
}

func TestProviderConfigValidator_DeniesMultipleErrors(t *testing.T) {
	t.Parallel()

	v := NewProviderConfigValidator(testScheme())
	resp := v.Handle(context.Background(), makeCreateRequest(
		newProviderConfig("", "", snowplanev1alpha1.AuthenticationType("BadAuth")),
	))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.account is required")
	assert.Contains(t, resp.Result.Message, "spec.user is required")
	assert.Contains(t, resp.Result.Message, "spec.authenticationType must be one of")
}

func TestProviderConfigValidator_DeniesAccountChange(t *testing.T) {
	t.Parallel()

	v := NewProviderConfigValidator(testScheme())
	oldPC := newProviderConfig("old-account", "admin", snowplanev1alpha1.AuthenticationTypeKeyPair)
	oldPC.Status.ObservedGeneration = 1
	newPC := newProviderConfig("new-account", "admin", snowplanev1alpha1.AuthenticationTypeKeyPair)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldPC, newPC))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.account is immutable")
}

func TestProviderConfigValidator_DeniesUserChange(t *testing.T) {
	t.Parallel()

	v := NewProviderConfigValidator(testScheme())
	oldPC := newProviderConfig("xy12345", "old-user", snowplanev1alpha1.AuthenticationTypeKeyPair)
	oldPC.Status.ObservedGeneration = 1
	newPC := newProviderConfig("xy12345", "new-user", snowplanev1alpha1.AuthenticationTypeKeyPair)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldPC, newPC))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.user is immutable")
}

func TestProviderConfigValidator_AllowsUnchangedUpdate(t *testing.T) {
	t.Parallel()

	v := NewProviderConfigValidator(testScheme())
	pc := newProviderConfig("xy12345", "admin", snowplanev1alpha1.AuthenticationTypeKeyPair)

	resp := v.Handle(context.Background(), makeUpdateRequest(pc, pc.DeepCopy()))
	require.True(t, resp.Allowed)
}

func TestProviderConfigValidator_DeniesAccountAndUserChange(t *testing.T) {
	t.Parallel()

	v := NewProviderConfigValidator(testScheme())
	oldPC := newProviderConfig("old-account", "old-user", snowplanev1alpha1.AuthenticationTypeKeyPair)
	oldPC.Status.ObservedGeneration = 1
	newPC := newProviderConfig("new-account", "new-user", snowplanev1alpha1.AuthenticationTypeKeyPair)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldPC, newPC))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.account is immutable")
	assert.Contains(t, resp.Result.Message, "spec.user is immutable")
}

func TestProviderConfigValidator_AllowsChangeBeforeFirstReconcile(t *testing.T) {
	t.Parallel()

	v := NewProviderConfigValidator(testScheme())
	// ObservedGeneration=0 means never reconciled — allow immutable field changes.
	oldPC := newProviderConfig("old-account", "old-user", snowplanev1alpha1.AuthenticationTypeKeyPair)
	newPC := newProviderConfig("new-account", "new-user", snowplanev1alpha1.AuthenticationTypeKeyPair)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldPC, newPC))
	assert.True(t, resp.Allowed, "should allow account+user change before first reconcile")
}

// --- FieldExport Validator Tests ---

func newFieldExport(sourceKind, sourceName, path string, targetKind snowplanev1alpha1.FieldExportTargetKind, targetName, key string) *snowplanev1alpha1.FieldExport {
	return &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{Name: "test-fe", Namespace: "default"},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{
					Kind: sourceKind,
					Name: sourceName,
				},
				Path: path,
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: targetKind,
				Name: targetName,
				Key:  key,
			},
		},
	}
}

func TestFieldExportValidator_AllowsValidCreate(t *testing.T) {
	t.Parallel()

	v := NewFieldExportValidator(testScheme())
	fe := newFieldExport("Database", "mydb", ".status.showOutput.createdOn", snowplanev1alpha1.FieldExportTargetConfigMap, "my-cm", "db-created")

	resp := v.Handle(context.Background(), makeCreateRequest(fe))
	assert.True(t, resp.Allowed)
}

func TestFieldExportValidator_DeniesInvalidSourceKind(t *testing.T) {
	t.Parallel()

	v := NewFieldExportValidator(testScheme())
	fe := newFieldExport("InvalidKind", "mydb", ".status.showOutput.createdOn", snowplanev1alpha1.FieldExportTargetConfigMap, "my-cm", "key")

	resp := v.Handle(context.Background(), makeCreateRequest(fe))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "not a known Snowplane resource kind")
}

func TestFieldExportValidator_DeniesPathNotStartingWithStatus(t *testing.T) {
	t.Parallel()

	v := NewFieldExportValidator(testScheme())
	fe := newFieldExport("Database", "mydb", ".spec.name", snowplanev1alpha1.FieldExportTargetConfigMap, "my-cm", "key")

	resp := v.Handle(context.Background(), makeCreateRequest(fe))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, ".status.")
}

func TestFieldExportValidator_DeniesPathWithArrayIndex(t *testing.T) {
	t.Parallel()

	v := NewFieldExportValidator(testScheme())
	fe := newFieldExport("Database", "mydb", ".status.conditions[0].message", snowplanev1alpha1.FieldExportTargetConfigMap, "my-cm", "key")

	resp := v.Handle(context.Background(), makeCreateRequest(fe))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "array indexing")
}

func TestFieldExportValidator_DeniesImmutableSourceKindChange(t *testing.T) {
	t.Parallel()

	v := NewFieldExportValidator(testScheme())
	oldFE := newFieldExport("Database", "mydb", ".status.showOutput.name", snowplanev1alpha1.FieldExportTargetConfigMap, "my-cm", "key")
	newFE := newFieldExport("Warehouse", "mydb", ".status.showOutput.name", snowplanev1alpha1.FieldExportTargetConfigMap, "my-cm", "key")

	resp := v.Handle(context.Background(), makeUpdateRequest(oldFE, newFE))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.from.resource.kind is immutable")
}

func TestFieldExportValidator_DeniesImmutableTargetNameChange(t *testing.T) {
	t.Parallel()

	v := NewFieldExportValidator(testScheme())
	oldFE := newFieldExport("Database", "mydb", ".status.showOutput.name", snowplanev1alpha1.FieldExportTargetConfigMap, "my-cm", "key")
	newFE := newFieldExport("Database", "mydb", ".status.showOutput.name", snowplanev1alpha1.FieldExportTargetConfigMap, "other-cm", "key")

	resp := v.Handle(context.Background(), makeUpdateRequest(oldFE, newFE))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.to.name is immutable")
}

func TestFieldExportValidator_AllowsPathChange(t *testing.T) {
	t.Parallel()

	v := NewFieldExportValidator(testScheme())
	oldFE := newFieldExport("Database", "mydb", ".status.showOutput.name", snowplanev1alpha1.FieldExportTargetConfigMap, "my-cm", "key")
	newFE := newFieldExport("Database", "mydb", ".status.showOutput.createdOn", snowplanev1alpha1.FieldExportTargetConfigMap, "my-cm", "key")

	resp := v.Handle(context.Background(), makeUpdateRequest(oldFE, newFE))
	assert.True(t, resp.Allowed, "path should be mutable")
}

func TestFieldExportValidator_AllowsAllKnownSourceKinds(t *testing.T) {
	t.Parallel()

	v := NewFieldExportValidator(testScheme())
	kinds := []string{"Database", "Schema", "Warehouse", "AccountRole", "DatabaseRole", "AccountRoleGrant", "DatabaseRoleGrant", "ShareGrant", "User", "Table", "View", "Stage"}

	for _, kind := range kinds {
		fe := newFieldExport(kind, "test", ".status.showOutput.name", snowplanev1alpha1.FieldExportTargetSecret, "my-secret", "key")
		resp := v.Handle(context.Background(), makeCreateRequest(fe))
		assert.True(t, resp.Allowed, "kind %s should be allowed", kind)
	}
}
