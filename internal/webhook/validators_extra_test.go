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

// --- C-1 fix: test helpers for inline-name resources ---

// newSchemaInline creates a Schema with inline databaseName (no DatabaseRef).
func newSchemaInline(name, dbName string, transient bool, observedGen int64) *snowplanev1alpha1.Schema {
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
			CommonSpec:   snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:         name,
			DatabaseName: &dbName,
			Transient:    transient,
		},
		Status: snowplanev1alpha1.SchemaStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

// newDatabaseRole creates a DatabaseRole with a databaseRef.
func newDatabaseRole(name, dbRef string, observedGen int64) *snowplanev1alpha1.DatabaseRole {
	return &snowplanev1alpha1.DatabaseRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "DatabaseRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dbrole",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseRoleSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:        name,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRef},
		},
		Status: snowplanev1alpha1.DatabaseRoleStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

// newDatabaseRoleInline creates a DatabaseRole with inline databaseName.
func newDatabaseRoleInline(name, dbName string, observedGen int64) *snowplanev1alpha1.DatabaseRole {
	return &snowplanev1alpha1.DatabaseRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "DatabaseRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dbrole",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseRoleSpec{
			CommonSpec:   snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:         name,
			DatabaseName: &dbName,
		},
		Status: snowplanev1alpha1.DatabaseRoleStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

// newTable creates a Table with databaseRef and schemaRef.
func newTable(name, dbRef, schemaRef string, observedGen int64) *snowplanev1alpha1.Table {
	return &snowplanev1alpha1.Table{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "Table",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-table",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.TableSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:        name,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRef},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: schemaRef},
			Columns:     []snowplanev1alpha1.ColumnDefinition{{Name: "id", Type: "NUMBER"}},
		},
		Status: snowplanev1alpha1.TableStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

// newTableInline creates a Table with inline databaseName and schemaName.
func newTableInline(name, dbName, schemaName string, observedGen int64) *snowplanev1alpha1.Table {
	return &snowplanev1alpha1.Table{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "Table",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-table",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.TableSpec{
			CommonSpec:   snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:         name,
			DatabaseName: &dbName,
			SchemaName:   &schemaName,
			Columns:      []snowplanev1alpha1.ColumnDefinition{{Name: "id", Type: "NUMBER"}},
		},
		Status: snowplanev1alpha1.TableStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

// newViewInline creates a View with inline databaseName and schemaName.
func newViewInline(name, dbName, schemaName, stmt string, observedGen int64) *snowplanev1alpha1.View {
	return &snowplanev1alpha1.View{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "View",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-view",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.ViewSpec{
			CommonSpec:   snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:         name,
			DatabaseName: &dbName,
			SchemaName:   &schemaName,
			Statement:    stmt,
		},
		Status: snowplanev1alpha1.ViewStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

// newStage creates a Stage with databaseRef and schemaRef.
func newStage(name, dbRef, schemaRef string, observedGen int64) *snowplanev1alpha1.Stage {
	return &snowplanev1alpha1.Stage{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "Stage",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-stage",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.StageSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:        name,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRef},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: schemaRef},
		},
		Status: snowplanev1alpha1.StageStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

// newStageInline creates a Stage with inline databaseName and schemaName.
func newStageInline(name, dbName, schemaName string, observedGen int64) *snowplanev1alpha1.Stage {
	return &snowplanev1alpha1.Stage{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "Stage",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-stage",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.StageSpec{
			CommonSpec:   snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete, ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default"}},
			Name:         name,
			DatabaseName: &dbName,
			SchemaName:   &schemaName,
		},
		Status: snowplanev1alpha1.StageStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: observedGen,
			},
		},
	}
}

// ---- C-1 fix verification: inline name update must not panic ----

func TestSchemaValidator_InlineName_UpdateDoesNotPanic(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	oldS := newSchemaInline("myschema", "MY_DB", false, 1)
	newS := newSchemaInline("myschema", "MY_DB", false, 1)
	comment := "updated"
	newS.Spec.Comment = &comment

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, newS))
	assert.True(t, resp.Allowed, "mutable field change on inline-name Schema should be allowed")
}

func TestSchemaValidator_InlineName_DeniesDbNameChange(t *testing.T) {
	t.Parallel()

	v := NewSchemaValidator(testScheme())
	oldS := newSchemaInline("myschema", "DB_A", false, 1)
	newS := newSchemaInline("myschema", "DB_B", false, 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, newS))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.databaseName is immutable")
}

func TestDatabaseRoleValidator_InlineName_UpdateDoesNotPanic(t *testing.T) {
	t.Parallel()

	v := NewDatabaseRoleValidator(testScheme())
	oldR := newDatabaseRoleInline("myrole", "MY_DB", 1)
	newR := newDatabaseRoleInline("myrole", "MY_DB", 1)
	comment := "updated"
	newR.Spec.Comment = &comment

	resp := v.Handle(context.Background(), makeUpdateRequest(oldR, newR))
	assert.True(t, resp.Allowed, "mutable field change on inline-name DatabaseRole should be allowed")
}

func TestDatabaseRoleValidator_InlineName_DeniesDbNameChange(t *testing.T) {
	t.Parallel()

	v := NewDatabaseRoleValidator(testScheme())
	oldR := newDatabaseRoleInline("myrole", "DB_A", 1)
	newR := newDatabaseRoleInline("myrole", "DB_B", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldR, newR))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.databaseName is immutable")
}

func TestDatabaseRoleValidator_Ref_DeniesDbRefChange(t *testing.T) {
	t.Parallel()

	v := NewDatabaseRoleValidator(testScheme())
	oldR := newDatabaseRole("myrole", "db-a", 1)
	newR := newDatabaseRole("myrole", "db-b", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldR, newR))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.databaseRef is immutable")
}

func TestTableValidator_InlineName_UpdateDoesNotPanic(t *testing.T) {
	t.Parallel()

	v := NewTableValidator(testScheme())
	oldT := newTableInline("mytable", "MY_DB", "MY_SCHEMA", 1)
	newT := newTableInline("mytable", "MY_DB", "MY_SCHEMA", 1)
	comment := "updated"
	newT.Spec.Comment = &comment

	resp := v.Handle(context.Background(), makeUpdateRequest(oldT, newT))
	assert.True(t, resp.Allowed, "mutable field change on inline-name Table should be allowed")
}

func TestTableValidator_InlineName_DeniesDbNameChange(t *testing.T) {
	t.Parallel()

	v := NewTableValidator(testScheme())
	oldT := newTableInline("mytable", "DB_A", "MY_SCHEMA", 1)
	newT := newTableInline("mytable", "DB_B", "MY_SCHEMA", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldT, newT))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.databaseName is immutable")
}

func TestTableValidator_InlineName_DeniesSchemaNameChange(t *testing.T) {
	t.Parallel()

	v := NewTableValidator(testScheme())
	oldT := newTableInline("mytable", "MY_DB", "SCHEMA_A", 1)
	newT := newTableInline("mytable", "MY_DB", "SCHEMA_B", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldT, newT))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.schemaName is immutable")
}

func TestTableValidator_Ref_DeniesDbRefChange(t *testing.T) {
	t.Parallel()

	v := NewTableValidator(testScheme())
	oldT := newTable("mytable", "db-a", "sch", 1)
	newT := newTable("mytable", "db-b", "sch", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldT, newT))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.databaseRef is immutable")
}

func TestTableValidator_Ref_DeniesSchemaRefChange(t *testing.T) {
	t.Parallel()

	v := NewTableValidator(testScheme())
	oldT := newTable("mytable", "db", "sch-a", 1)
	newT := newTable("mytable", "db", "sch-b", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldT, newT))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.schemaRef is immutable")
}

func TestViewValidator_InlineName_UpdateDoesNotPanic(t *testing.T) {
	t.Parallel()

	v := NewViewValidator(testScheme())
	oldV := newViewInline("myview", "MY_DB", "MY_SCHEMA", "SELECT 1", 1)
	newV := newViewInline("myview", "MY_DB", "MY_SCHEMA", "SELECT 2", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldV, newV))
	assert.True(t, resp.Allowed, "mutable field change on inline-name View should be allowed")
}

func TestViewValidator_InlineName_DeniesDbNameChange(t *testing.T) {
	t.Parallel()

	v := NewViewValidator(testScheme())
	oldV := newViewInline("myview", "DB_A", "MY_SCHEMA", "SELECT 1", 1)
	newV := newViewInline("myview", "DB_B", "MY_SCHEMA", "SELECT 1", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldV, newV))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.databaseName is immutable")
}

func TestViewValidator_InlineName_DeniesSchemaNameChange(t *testing.T) {
	t.Parallel()

	v := NewViewValidator(testScheme())
	oldV := newViewInline("myview", "MY_DB", "SCHEMA_A", "SELECT 1", 1)
	newV := newViewInline("myview", "MY_DB", "SCHEMA_B", "SELECT 1", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldV, newV))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.schemaName is immutable")
}

func TestStageValidator_InlineName_UpdateDoesNotPanic(t *testing.T) {
	t.Parallel()

	v := NewStageValidator(testScheme())
	oldS := newStageInline("mystage", "MY_DB", "MY_SCHEMA", 1)
	newS := newStageInline("mystage", "MY_DB", "MY_SCHEMA", 1)
	comment := "updated"
	newS.Spec.Comment = &comment

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, newS))
	assert.True(t, resp.Allowed, "mutable field change on inline-name Stage should be allowed")
}

func TestStageValidator_InlineName_DeniesDbNameChange(t *testing.T) {
	t.Parallel()

	v := NewStageValidator(testScheme())
	oldS := newStageInline("mystage", "DB_A", "MY_SCHEMA", 1)
	newS := newStageInline("mystage", "DB_B", "MY_SCHEMA", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, newS))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.databaseName is immutable")
}

func TestStageValidator_InlineName_DeniesSchemaNameChange(t *testing.T) {
	t.Parallel()

	v := NewStageValidator(testScheme())
	oldS := newStageInline("mystage", "MY_DB", "SCHEMA_A", 1)
	newS := newStageInline("mystage", "MY_DB", "SCHEMA_B", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, newS))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.schemaName is immutable")
}

func TestStageValidator_Ref_DeniesDbRefChange(t *testing.T) {
	t.Parallel()

	v := NewStageValidator(testScheme())
	oldS := newStage("mystage", "db-a", "sch", 1)
	newS := newStage("mystage", "db-b", "sch", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, newS))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.databaseRef is immutable")
}

func TestStageValidator_Ref_DeniesSchemaRefChange(t *testing.T) {
	t.Parallel()

	v := NewStageValidator(testScheme())
	oldS := newStage("mystage", "db", "sch-a", 1)
	newS := newStage("mystage", "db", "sch-b", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(oldS, newS))
	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "spec.schemaRef is immutable")
}

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
	assert.Contains(t, msg, "spec.databaseRef is immutable")
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

// --------------------------------------------------------------------------
// H-2: Policy body SQL injection blocklist tests
// --------------------------------------------------------------------------

func TestValidatePolicyBody_SafeBody(t *testing.T) {
	t.Parallel()

	errs := validatePolicyBody("CASE WHEN current_role() IN ('ANALYST') THEN val ELSE '***' END")
	assert.Empty(t, errs)
}

func TestValidatePolicyBody_Semicolon(t *testing.T) {
	t.Parallel()

	errs := validatePolicyBody("val; DROP TABLE foo")
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "semicolons")
}

func TestValidatePolicyBody_SystemFunction(t *testing.T) {
	t.Parallel()

	errs := validatePolicyBody("CASE WHEN true THEN val ELSE SYSTEM$CANCEL_ALL_QUERIES(current_session()) END")
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "SYSTEM$")
}

func TestValidatePolicyBody_ExecuteImmediate(t *testing.T) {
	t.Parallel()

	errs := validatePolicyBody("EXECUTE IMMEDIATE 'DROP TABLE foo'")
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "EXECUTE IMMEDIATE")
}

func TestValidatePolicyBody_DDLKeywords(t *testing.T) {
	t.Parallel()

	bodies := []string{
		"CASE WHEN true THEN val ELSE (CREATE TABLE x(a INT)) END",
		"CASE WHEN true THEN val ELSE (ALTER TABLE x DROP COLUMN a) END",
		"CASE WHEN true THEN val ELSE (DROP TABLE x) END",
		"CASE WHEN true THEN val ELSE (GRANT SELECT ON TABLE x TO ROLE y) END",
	}

	for _, body := range bodies {
		errs := validatePolicyBody(body)
		assert.NotEmpty(t, errs, "body should be rejected: %s", body)
	}
}

func TestValidatePolicyBody_CaseInsensitive(t *testing.T) {
	t.Parallel()

	errs := validatePolicyBody("case when true then val else system$cancel_all_queries(current_session()) end")
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "SYSTEM$")
}

func newMaskingPolicy(body string) *snowplanev1alpha1.MaskingPolicy {
	return &snowplanev1alpha1.MaskingPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "MaskingPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mp",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.MaskingPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default"},
			},
			Name:         "TEST_MP",
			DatabaseName: strPtr("DB"),
			SchemaName:   strPtr("PUBLIC"),
			Body:         body,
			Signature: []snowplanev1alpha1.MaskingPolicyArgument{
				{Name: "val", Type: "VARCHAR"},
			},
		},
	}
}

func newRowAccessPolicy(body string) *snowplanev1alpha1.RowAccessPolicy {
	return &snowplanev1alpha1.RowAccessPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "RowAccessPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rap",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.RowAccessPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default"},
			},
			Name:         "TEST_RAP",
			DatabaseName: strPtr("DB"),
			SchemaName:   strPtr("PUBLIC"),
			Body:         body,
			Signature: []snowplanev1alpha1.RowAccessPolicyArgument{
				{Name: "val", Type: "VARCHAR"},
			},
		},
	}
}

func strPtr(s string) *string { return &s }

func TestMaskingPolicyValidator_RejectsInjection(t *testing.T) {
	t.Parallel()

	v := NewMaskingPolicyValidator(testScheme())
	mp := newMaskingPolicy("CASE WHEN true THEN val ELSE SYSTEM$CANCEL_ALL_QUERIES(current_session()) END")

	resp := v.Handle(context.Background(), makeCreateRequest(mp))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "SYSTEM$")
}

func TestMaskingPolicyValidator_AllowsSafeBody(t *testing.T) {
	t.Parallel()

	v := NewMaskingPolicyValidator(testScheme())
	mp := newMaskingPolicy("CASE WHEN current_role() IN ('ANALYST') THEN val ELSE '***' END")

	resp := v.Handle(context.Background(), makeCreateRequest(mp))

	assert.True(t, resp.Allowed)
}

func TestRowAccessPolicyValidator_RejectsInjection(t *testing.T) {
	t.Parallel()

	v := NewRowAccessPolicyValidator(testScheme())
	rap := newRowAccessPolicy("DROP TABLE foo")

	resp := v.Handle(context.Background(), makeCreateRequest(rap))

	require.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, "DROP ")
}

func TestRowAccessPolicyValidator_AllowsSafeBody(t *testing.T) {
	t.Parallel()

	v := NewRowAccessPolicyValidator(testScheme())
	rap := newRowAccessPolicy("current_role() IN ('ANALYST')")

	resp := v.Handle(context.Background(), makeCreateRequest(rap))

	assert.True(t, resp.Allowed)
}
