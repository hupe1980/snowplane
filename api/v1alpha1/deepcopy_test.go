package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// deepCopyPtr helpers for constructing test values.
func strPtr(s string) *string { return &s }
func int32Ptr(v int32) *int32 { return &v }
func boolPtr(v bool) *bool    { return &v }

// TestDeepCopy_DatabaseSpec_PointerIsolation proves that mutating a deep-copied
// DatabaseSpec does not affect the original (all pointer fields are independent).
func TestDeepCopy_DatabaseSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := DatabaseSpec{
		CommonSpec:                 CommonSpec{UseRole: strPtr("SYSADMIN")},
		Name:                       "DB1",
		Comment:                    strPtr("original"),
		DataRetentionTimeInDays:    int32Ptr(10),
		MaxDataExtensionTimeInDays: int32Ptr(20),
		Catalog:                    strPtr("cat"),
		ExternalVolume:             strPtr("vol"),
		ReplaceInvalidCharacters:   boolPtr(true),
		DefaultDDLCollation:        strPtr("en-ci"),
	}

	copied := orig.DeepCopy()
	require.NotNil(t, copied)

	// Mutate every pointer in the copy.
	*copied.UseRole = "HACKER"
	*copied.Comment = "mutated"
	*copied.DataRetentionTimeInDays = 99
	*copied.MaxDataExtensionTimeInDays = 99
	*copied.Catalog = "mutated"
	*copied.ExternalVolume = "mutated"
	*copied.ReplaceInvalidCharacters = false
	*copied.DefaultDDLCollation = "mutated"

	// Original must be unchanged.
	assert.Equal(t, "SYSADMIN", *orig.UseRole)
	assert.Equal(t, "original", *orig.Comment)
	assert.Equal(t, int32(10), *orig.DataRetentionTimeInDays)
	assert.Equal(t, int32(20), *orig.MaxDataExtensionTimeInDays)
	assert.Equal(t, "cat", *orig.Catalog)
	assert.Equal(t, "vol", *orig.ExternalVolume)
	assert.Equal(t, true, *orig.ReplaceInvalidCharacters)
	assert.Equal(t, "en-ci", *orig.DefaultDDLCollation)
}

// TestDeepCopy_SchemaSpec_PointerIsolation tests SchemaSpec pointer independence.
func TestDeepCopy_SchemaSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := SchemaSpec{
		CommonSpec:                 CommonSpec{UseRole: strPtr("SYSADMIN")},
		Name:                       "SCH",
		Comment:                    strPtr("original"),
		DataRetentionTimeInDays:    int32Ptr(5),
		MaxDataExtensionTimeInDays: int32Ptr(10),
		DefaultDDLCollation:        strPtr("en"),
		ReplaceInvalidCharacters:   boolPtr(false),
	}

	copied := orig.DeepCopy()
	*copied.Comment = "mutated"
	*copied.DataRetentionTimeInDays = 99
	*copied.MaxDataExtensionTimeInDays = 99
	*copied.DefaultDDLCollation = "mutated"
	*copied.ReplaceInvalidCharacters = true

	assert.Equal(t, "original", *orig.Comment)
	assert.Equal(t, int32(5), *orig.DataRetentionTimeInDays)
	assert.Equal(t, int32(10), *orig.MaxDataExtensionTimeInDays)
	assert.Equal(t, "en", *orig.DefaultDDLCollation)
	assert.Equal(t, false, *orig.ReplaceInvalidCharacters)
}

// TestDeepCopy_WarehouseSpec_PointerIsolation tests the most pointer-dense spec type.
func TestDeepCopy_WarehouseSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := WarehouseSpec{
		Name:                            "WH",
		MinClusterCount:                 int32Ptr(1),
		MaxClusterCount:                 int32Ptr(3),
		AutoSuspend:                     int32Ptr(300),
		AutoResume:                      boolPtr(true),
		ResourceMonitor:                 strPtr("monitor"),
		Comment:                         strPtr("original"),
		EnableQueryAcceleration:         boolPtr(true),
		QueryAccelerationMaxScaleFactor: int32Ptr(8),
		MaxConcurrencyLevel:             int32Ptr(10),
		StatementQueuedTimeoutInSeconds: int32Ptr(60),
		StatementTimeoutInSeconds:       int32Ptr(120),
	}

	copied := orig.DeepCopy()
	*copied.MinClusterCount = 99
	*copied.MaxClusterCount = 99
	*copied.AutoSuspend = 99
	*copied.AutoResume = false
	*copied.ResourceMonitor = "mutated"
	*copied.Comment = "mutated"
	*copied.EnableQueryAcceleration = false
	*copied.QueryAccelerationMaxScaleFactor = 99
	*copied.MaxConcurrencyLevel = 99
	*copied.StatementQueuedTimeoutInSeconds = 99
	*copied.StatementTimeoutInSeconds = 99

	assert.Equal(t, int32(1), *orig.MinClusterCount)
	assert.Equal(t, int32(3), *orig.MaxClusterCount)
	assert.Equal(t, int32(300), *orig.AutoSuspend)
	assert.Equal(t, true, *orig.AutoResume)
	assert.Equal(t, "monitor", *orig.ResourceMonitor)
	assert.Equal(t, "original", *orig.Comment)
	assert.Equal(t, true, *orig.EnableQueryAcceleration)
	assert.Equal(t, int32(8), *orig.QueryAccelerationMaxScaleFactor)
	assert.Equal(t, int32(10), *orig.MaxConcurrencyLevel)
	assert.Equal(t, int32(60), *orig.StatementQueuedTimeoutInSeconds)
	assert.Equal(t, int32(120), *orig.StatementTimeoutInSeconds)
}

// TestDeepCopy_UserSpec_PointerIsolation tests all 15 pointer fields on UserSpec.
func TestDeepCopy_UserSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := UserSpec{
		Name:                  "USER1",
		LoginName:             strPtr("login"),
		DisplayName:           strPtr("display"),
		Email:                 strPtr("a@b.com"),
		FirstName:             strPtr("first"),
		LastName:              strPtr("last"),
		Comment:               strPtr("original"),
		DefaultRole:           strPtr("ROLE1"),
		DefaultSecondaryRoles: strPtr("ALL"),
		DefaultWarehouse:      strPtr("WH"),
		DefaultNamespace:      strPtr("NS"),
		MustChangePassword:    boolPtr(false),
		Disabled:              boolPtr(false),
		Password: &SecretKeyReference{
			Name: "secret",
			Key:  "password",
		},
		RSAPublicKey: &SecretKeyReference{
			Name: "secret",
			Key:  "pubkey",
		},
		RSAPublicKey2: &SecretKeyReference{
			Name: "secret2",
			Key:  "pubkey2",
		},
	}

	copied := orig.DeepCopy()

	// Mutate all primitive pointers in the copy.
	*copied.LoginName = "mutated"
	*copied.DisplayName = "mutated"
	*copied.Email = "mutated@z.com"
	*copied.FirstName = "mutated"
	*copied.LastName = "mutated"
	*copied.Comment = "mutated"
	*copied.DefaultRole = "mutated"
	*copied.DefaultSecondaryRoles = "mutated"
	*copied.DefaultWarehouse = "mutated"
	*copied.DefaultNamespace = "mutated"
	*copied.MustChangePassword = true
	*copied.Disabled = true

	// Mutate struct pointers.
	copied.Password.Name = "mutated-secret"
	copied.RSAPublicKey.Name = "mutated-secret"
	copied.RSAPublicKey2.Name = "mutated-secret"

	// Original must be unchanged.
	assert.Equal(t, "login", *orig.LoginName)
	assert.Equal(t, "display", *orig.DisplayName)
	assert.Equal(t, "a@b.com", *orig.Email)
	assert.Equal(t, "first", *orig.FirstName)
	assert.Equal(t, "last", *orig.LastName)
	assert.Equal(t, "original", *orig.Comment)
	assert.Equal(t, "ROLE1", *orig.DefaultRole)
	assert.Equal(t, "ALL", *orig.DefaultSecondaryRoles)
	assert.Equal(t, "WH", *orig.DefaultWarehouse)
	assert.Equal(t, "NS", *orig.DefaultNamespace)
	assert.Equal(t, false, *orig.MustChangePassword)
	assert.Equal(t, false, *orig.Disabled)
	assert.Equal(t, "secret", orig.Password.Name)
	assert.Equal(t, "secret", orig.RSAPublicKey.Name)
	assert.Equal(t, "secret2", orig.RSAPublicKey2.Name)
}

// TestDeepCopy_GrantOn_NestedPointerIsolation tests the deeply nested
// GrantOn → GrantOnSchemaObject → *GrantOnBulk chain (L-22).
func TestDeepCopy_GrantOn_NestedPointerIsolation(t *testing.T) {
	t.Parallel()

	orig := GrantOn{
		AccountObject: &GrantOnAccountObject{
			ObjectType: "DATABASE",
			ObjectName: "DB1",
		},
		Schema: &GrantOnSchema{
			SchemaName: "DB1.PUBLIC",
		},
		SchemaObject: &GrantOnSchemaObject{
			ObjectType: "TABLE",
			ObjectName: "DB1.PUBLIC.T1",
			All: &GrantOnBulk{
				InDatabase: "DB1",
			},
			Future: &GrantOnBulk{
				InSchema: "DB1.PUBLIC",
			},
		},
	}

	copied := orig.DeepCopy()
	require.NotNil(t, copied)

	// Mutate level 1 struct pointers.
	copied.AccountObject.ObjectName = "MUTATED"
	copied.Schema.SchemaName = "MUTATED"
	copied.SchemaObject.ObjectName = "MUTATED"

	// Mutate level 2 nested pointers (GrantOnBulk).
	copied.SchemaObject.All.InDatabase = "MUTATED"
	copied.SchemaObject.Future.InSchema = "MUTATED"

	// Original level 1 unchanged.
	assert.Equal(t, "DB1", orig.AccountObject.ObjectName)
	assert.Equal(t, "DB1.PUBLIC", orig.Schema.SchemaName)
	assert.Equal(t, "DB1.PUBLIC.T1", orig.SchemaObject.ObjectName)

	// Original level 2 unchanged.
	assert.Equal(t, "DB1", orig.SchemaObject.All.InDatabase)
	assert.Equal(t, "DB1.PUBLIC", orig.SchemaObject.Future.InSchema)
}

// TestDeepCopy_AccountRoleGrantSpec_FullIsolation tests the complete AccountRoleGrantSpec with all
// nested pointer types to catch any missing deep copy in the chain.
func TestDeepCopy_AccountRoleGrantSpec_FullIsolation(t *testing.T) {
	t.Parallel()

	orig := AccountRoleGrantSpec{
		Privilege: "SELECT",
		On: GrantOn{
			SchemaObject: &GrantOnSchemaObject{
				ObjectType: "TABLE",
				ObjectName: "DB.SCH.T",
				All: &GrantOnBulk{
					InSchema: "DB.SCH",
				},
			},
		},
		AccountRole:     "ANALYST",
		WithGrantOption: true,
	}

	copied := orig.DeepCopy()
	copied.On.SchemaObject.ObjectName = "MUTATED"
	copied.On.SchemaObject.All.InSchema = "MUTATED"
	copied.AccountRole = "MUTATED"

	assert.Equal(t, "DB.SCH.T", orig.On.SchemaObject.ObjectName)
	assert.Equal(t, "DB.SCH", orig.On.SchemaObject.All.InSchema)
	assert.Equal(t, "ANALYST", orig.AccountRole)
}

// TestDeepCopy_TableSpec_ColumnPointerIsolation tests that column pointer
// fields (Nullable, Default, Comment) are independently deep-copied.
func TestDeepCopy_TableSpec_ColumnPointerIsolation(t *testing.T) {
	t.Parallel()

	orig := TableSpec{
		Name: "T1",
		Columns: []ColumnDefinition{
			{Name: "id", Type: "NUMBER", Nullable: boolPtr(false), Default: strPtr("0"), Comment: strPtr("pk")},
			{Name: "name", Type: "VARCHAR", Nullable: boolPtr(true), Comment: strPtr("label")},
		},
		Comment:                    strPtr("table comment"),
		DataRetentionTimeInDays:    int32Ptr(30),
		MaxDataExtensionTimeInDays: int32Ptr(60),
		ChangeTracking:             boolPtr(false),
		DefaultDDLCollation:        strPtr("en"),
		EnableSchemaEvolution:      boolPtr(true),
	}

	copied := orig.DeepCopy()

	// Mutate column pointers.
	*copied.Columns[0].Nullable = true
	*copied.Columns[0].Default = "MUTATED"
	*copied.Columns[0].Comment = "MUTATED"
	*copied.Columns[1].Nullable = false
	*copied.Columns[1].Comment = "MUTATED"

	// Mutate table-level pointers.
	*copied.Comment = "MUTATED"
	*copied.ChangeTracking = true
	*copied.EnableSchemaEvolution = false

	// Original columns unchanged.
	assert.Equal(t, false, *orig.Columns[0].Nullable)
	assert.Equal(t, "0", *orig.Columns[0].Default)
	assert.Equal(t, "pk", *orig.Columns[0].Comment)
	assert.Equal(t, true, *orig.Columns[1].Nullable)
	assert.Equal(t, "label", *orig.Columns[1].Comment)

	// Original table pointers unchanged.
	assert.Equal(t, "table comment", *orig.Comment)
	assert.Equal(t, false, *orig.ChangeTracking)
	assert.Equal(t, true, *orig.EnableSchemaEvolution)
}

// TestDeepCopy_StageSpec_NestedStructPointerIsolation tests StageSpec's
// nested struct pointers (Encryption, Directory).
func TestDeepCopy_StageSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := StageSpec{
		Name:               "STG",
		URL:                strPtr("s3://bucket/path"),
		StorageIntegration: strPtr("MY_INT"),
		FileFormat:         strPtr("CSV"),
		Comment:            strPtr("original"),
		Encryption: &StageEncryption{
			Type: "AWS_SSE_S3",
		},
		Directory: &StageDirectoryOptions{
			Enable:      true,
			AutoRefresh: boolPtr(false),
		},
	}

	copied := orig.DeepCopy()

	// Mutate primitive pointers.
	*copied.URL = "MUTATED"
	*copied.StorageIntegration = "MUTATED"
	*copied.FileFormat = "MUTATED"
	*copied.Comment = "MUTATED"

	// Mutate nested struct pointers.
	copied.Encryption.Type = "MUTATED"
	copied.Directory.Enable = false
	*copied.Directory.AutoRefresh = true

	// Original unchanged.
	assert.Equal(t, "s3://bucket/path", *orig.URL)
	assert.Equal(t, "MY_INT", *orig.StorageIntegration)
	assert.Equal(t, "CSV", *orig.FileFormat)
	assert.Equal(t, "original", *orig.Comment)
	assert.Equal(t, "AWS_SSE_S3", orig.Encryption.Type)
	assert.Equal(t, true, orig.Directory.Enable)
	assert.Equal(t, false, *orig.Directory.AutoRefresh)
}

// TestDeepCopy_ViewSpec_PointerIsolation tests ViewSpec pointer fields.
func TestDeepCopy_ViewSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := ViewSpec{
		Name:           "V1",
		Statement:      "SELECT 1",
		Comment:        strPtr("original"),
		ChangeTracking: boolPtr(false),
	}

	copied := orig.DeepCopy()
	*copied.Comment = "MUTATED"
	*copied.ChangeTracking = true

	assert.Equal(t, "original", *orig.Comment)
	assert.Equal(t, false, *orig.ChangeTracking)
}

// TestDeepCopy_CommonStatus_ConditionsIsolation tests that the Conditions
// slice is independently deep-copied (slice header AND elements).
func TestDeepCopy_CommonStatus_ConditionsIsolation(t *testing.T) {
	t.Parallel()

	orig := CommonStatus{
		Conditions: []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"},
			{Type: "Synced", Status: metav1.ConditionTrue, Reason: "OK"},
		},
	}

	copied := orig.DeepCopy()

	// Mutate copy's conditions.
	copied.Conditions[0].Status = metav1.ConditionFalse
	copied.Conditions[0].Reason = "MUTATED"
	copied.Conditions = append(copied.Conditions, metav1.Condition{Type: "Extra"})

	// Original unchanged.
	assert.Equal(t, metav1.ConditionTrue, orig.Conditions[0].Status)
	assert.Equal(t, "OK", orig.Conditions[0].Reason)
	assert.Equal(t, 2, len(orig.Conditions))
}

// TestDeepCopy_Database_FullObject tests DeepCopy on a complete Database CR,
// including ObjectMeta, Spec, and Status.
func TestDeepCopy_Database_FullObject(t *testing.T) {
	t.Parallel()

	orig := &Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "db1",
			Namespace:   "ns1",
			Labels:      map[string]string{"env": "prod"},
			Annotations: map[string]string{"key": "value"},
		},
		Spec: DatabaseSpec{
			CommonSpec: CommonSpec{UseRole: strPtr("SYSADMIN")},
			Name:       "DB1",
			Comment:    strPtr("original"),
		},
		Status: DatabaseStatus{
			CommonStatus: CommonStatus{
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue},
				},
			},
		},
	}

	copied := orig.DeepCopy()

	// Mutate copy.
	copied.Labels["env"] = "MUTATED"
	copied.Annotations["key"] = "MUTATED"
	*copied.Spec.Comment = "MUTATED"
	copied.Status.Conditions[0].Status = metav1.ConditionFalse

	// Original unchanged.
	assert.Equal(t, "prod", orig.Labels["env"])
	assert.Equal(t, "value", orig.Annotations["key"])
	assert.Equal(t, "original", *orig.Spec.Comment)
	assert.Equal(t, metav1.ConditionTrue, orig.Status.Conditions[0].Status)
}

// TestDeepCopy_DatabaseList_ItemsIsolation tests that list items are independent.
func TestDeepCopy_DatabaseList_ItemsIsolation(t *testing.T) {
	t.Parallel()

	orig := &DatabaseList{
		Items: []Database{
			{Spec: DatabaseSpec{Name: "DB1", Comment: strPtr("one")}},
			{Spec: DatabaseSpec{Name: "DB2", Comment: strPtr("two")}},
		},
	}

	copied := orig.DeepCopy()
	*copied.Items[0].Spec.Comment = "MUTATED"
	copied.Items = append(copied.Items, Database{Spec: DatabaseSpec{Name: "DB3"}})

	assert.Equal(t, "one", *orig.Items[0].Spec.Comment)
	assert.Equal(t, 2, len(orig.Items))
}

// TestDeepCopy_NilHandling tests that DeepCopy of nil returns nil.
func TestDeepCopy_NilHandling(t *testing.T) {
	t.Parallel()

	var db *Database
	assert.Nil(t, db.DeepCopy())

	var spec *DatabaseSpec
	assert.Nil(t, spec.DeepCopy())

	var grantOn *GrantOn
	assert.Nil(t, grantOn.DeepCopy())

	var grantSO *GrantOnSchemaObject
	assert.Nil(t, grantSO.DeepCopy())
}

// TestDeepCopy_ProviderCredentials_PointerIsolation verifies that SecretRef
// (now a pointer) is deep-copied correctly and the original is not mutated.
func TestDeepCopy_ProviderCredentials_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := ProviderCredentials{
		SecretRef: &SecretKeyReference{
			Name:      "secret",
			Namespace: "ns",
			Key:       "key",
		},
	}

	copied := orig.DeepCopy()

	// Mutate the copy's pointer field.
	copied.SecretRef.Name = "MUTATED"
	copied.SecretRef.Key = "MUTATED"

	// Original is unchanged because SecretRef is now deep-copied.
	assert.Equal(t, "secret", orig.SecretRef.Name)
	assert.Equal(t, "key", orig.SecretRef.Key)
}

// TestDeepCopy_ProviderCredentials_NilSecretRef verifies that DeepCopy
// handles a nil SecretRef correctly (WorkloadIdentity, no Secret).
func TestDeepCopy_ProviderCredentials_NilSecretRef(t *testing.T) {
	t.Parallel()

	orig := ProviderCredentials{}

	copied := orig.DeepCopy()
	require.Nil(t, copied.SecretRef)
}

// TestDeepCopy_AccountRoleSpec_PointerIsolation tests AccountRoleSpec.
func TestDeepCopy_AccountRoleSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := AccountRoleSpec{
		Name:    "ROLE1",
		Comment: strPtr("original"),
	}

	copied := orig.DeepCopy()
	*copied.Comment = "MUTATED"

	assert.Equal(t, "original", *orig.Comment)
}

// TestDeepCopy_DatabaseRoleSpec_PointerIsolation tests DatabaseRoleSpec.
func TestDeepCopy_DatabaseRoleSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := DatabaseRoleSpec{
		Name:    "DBROLE1",
		Comment: strPtr("original"),
	}

	copied := orig.DeepCopy()
	*copied.Comment = "MUTATED"

	assert.Equal(t, "original", *orig.Comment)
}
