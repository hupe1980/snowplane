package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T { return &v }

// TestDeepCopy_DatabaseSpec_PointerIsolation proves that mutating a deep-copied
// DatabaseSpec does not affect the original (all pointer fields are independent).
func TestDeepCopy_DatabaseSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := DatabaseSpec{
		CommonSpec:                 CommonSpec{UseRole: ptr("SYSADMIN")},
		Name:                       "DB1",
		Comment:                    ptr("original"),
		DataRetentionTimeInDays:    ptr(int32(10)),
		MaxDataExtensionTimeInDays: ptr(int32(20)),
		Catalog:                    ptr("cat"),
		ExternalVolume:             ptr("vol"),
		ReplaceInvalidCharacters:   ptr(true),
		DefaultDDLCollation:        ptr("en-ci"),
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
		CommonSpec:                 CommonSpec{UseRole: ptr("SYSADMIN")},
		Name:                       "SCH",
		Comment:                    ptr("original"),
		DataRetentionTimeInDays:    ptr(int32(5)),
		MaxDataExtensionTimeInDays: ptr(int32(10)),
		DefaultDDLCollation:        ptr("en"),
		ReplaceInvalidCharacters:   ptr(false),
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
		MinClusterCount:                 ptr(int32(1)),
		MaxClusterCount:                 ptr(int32(3)),
		AutoSuspend:                     ptr(int32(300)),
		AutoResume:                      ptr(true),
		ResourceMonitor:                 ptr("monitor"),
		Comment:                         ptr("original"),
		EnableQueryAcceleration:         ptr(true),
		QueryAccelerationMaxScaleFactor: ptr(int32(8)),
		MaxConcurrencyLevel:             ptr(int32(10)),
		StatementQueuedTimeoutInSeconds: ptr(int32(60)),
		StatementTimeoutInSeconds:       ptr(int32(120)),
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

// TestDeepCopy_UserSpec_PointerIsolation tests all pointer fields on UserSpec.
func TestDeepCopy_UserSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := UserSpec{
		Name:                  "USER1",
		LoginName:             ptr("login"),
		DisplayName:           ptr("display"),
		Email:                 ptr("a@b.com"),
		FirstName:             ptr("first"),
		LastName:              ptr("last"),
		Comment:               ptr("original"),
		DefaultRole:           ptr("ROLE1"),
		DefaultSecondaryRoles: ptr("ALL"),
		DefaultWarehouse:      ptr("WH"),
		DefaultNamespace:      ptr("NS"),
		MustChangePassword:    ptr(false),
		Disabled:              ptr(false),
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
// GrantOn → GrantOnSchemaObject → *GrantOnBulk chain.
func TestDeepCopy_GrantOn_NestedPointerIsolation(t *testing.T) {
	t.Parallel()

	orig := GrantOn{
		AccountObject: &GrantOnAccountObject{
			ObjectType: "DATABASE",
			ObjectName: "DB1",
		},
		Schema: &GrantOnSchema{
			SchemaName: ptr("DB1.PUBLIC"),
		},
		SchemaObject: &GrantOnSchemaObject{
			ObjectType: "TABLE",
			ObjectName: "DB1.PUBLIC.T1",
			All: &GrantOnBulk{
				InDatabase: ptr("DB1"),
			},
			Future: &GrantOnBulk{
				InSchema: ptr("DB1.PUBLIC"),
			},
		},
	}

	copied := orig.DeepCopy()
	require.NotNil(t, copied)

	// Mutate level 1 struct pointers.
	copied.AccountObject.ObjectName = "MUTATED"
	copied.Schema.SchemaName = ptr("MUTATED")
	copied.SchemaObject.ObjectName = "MUTATED"

	// Mutate level 2 nested pointers (GrantOnBulk).
	copied.SchemaObject.All.InDatabase = ptr("MUTATED")
	copied.SchemaObject.Future.InSchema = ptr("MUTATED")

	// Original level 1 unchanged.
	assert.Equal(t, "DB1", orig.AccountObject.ObjectName)
	assert.Equal(t, "DB1.PUBLIC", *orig.Schema.SchemaName)
	assert.Equal(t, "DB1.PUBLIC.T1", orig.SchemaObject.ObjectName)

	// Original level 2 unchanged.
	assert.Equal(t, "DB1", *orig.SchemaObject.All.InDatabase)
	assert.Equal(t, "DB1.PUBLIC", *orig.SchemaObject.Future.InSchema)
}

// TestDeepCopy_GrantPrivilegesToAccountRoleSpec_FullIsolation tests the complete GrantPrivilegesToAccountRoleSpec with all
// nested pointer types to catch any missing deep copy in the chain.
func TestDeepCopy_GrantPrivilegesToAccountRoleSpec_FullIsolation(t *testing.T) {
	t.Parallel()

	orig := GrantPrivilegesToAccountRoleSpec{
		Privilege: "SELECT",
		On: GrantOn{
			SchemaObject: &GrantOnSchemaObject{
				ObjectType: "TABLE",
				ObjectName: "DB.SCH.T",
				All: &GrantOnBulk{
					InSchema: ptr("DB.SCH"),
				},
			},
		},
		AccountRole:     ptr("ANALYST"),
		WithGrantOption: true,
	}

	copied := orig.DeepCopy()
	copied.On.SchemaObject.ObjectName = "MUTATED"
	copied.On.SchemaObject.All.InSchema = ptr("MUTATED")
	copied.AccountRole = ptr("MUTATED")

	assert.Equal(t, "DB.SCH.T", orig.On.SchemaObject.ObjectName)
	assert.Equal(t, "DB.SCH", *orig.On.SchemaObject.All.InSchema)
	assert.Equal(t, "ANALYST", *orig.AccountRole)
}

// TestDeepCopy_TableSpec_ColumnPointerIsolation tests that column pointer
// fields (Nullable, Default, Comment) are independently deep-copied.
func TestDeepCopy_TableSpec_ColumnPointerIsolation(t *testing.T) {
	t.Parallel()

	orig := TableSpec{
		Name: "T1",
		Columns: []ColumnDefinition{
			{Name: "id", Type: "NUMBER", Nullable: ptr(false), Default: ptr("0"), Comment: ptr("pk")},
			{Name: "name", Type: "VARCHAR", Nullable: ptr(true), Comment: ptr("label")},
		},
		Comment:                    ptr("table comment"),
		DataRetentionTimeInDays:    ptr(int32(30)),
		MaxDataExtensionTimeInDays: ptr(int32(60)),
		ChangeTracking:             ptr(false),
		DefaultDDLCollation:        ptr("en"),
		EnableSchemaEvolution:      ptr(true),
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

// TestDeepCopy_ExternalStageSpec_NestedStructPointerIsolation tests ExternalStageSpec's
// nested struct pointers (Encryption, Directory).
func TestDeepCopy_ExternalStageSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := ExternalStageSpec{
		Name:               "STG",
		URL:                "s3://bucket/path",
		StorageIntegration: ptr("MY_INT"),
		FileFormat:         ptr("CSV"),
		Comment:            ptr("original"),
		Encryption: &ExternalStageEncryption{
			Type: "AWS_SSE_S3",
		},
		Directory: &ExternalStageDirectoryOptions{
			Enable:      true,
			AutoRefresh: ptr(false),
		},
	}

	copied := orig.DeepCopy()

	// Mutate primitive pointers.
	*copied.StorageIntegration = "MUTATED"
	*copied.FileFormat = "MUTATED"
	*copied.Comment = "MUTATED"

	// Mutate nested struct pointers.
	copied.Encryption.Type = "MUTATED"
	copied.Directory.Enable = false
	*copied.Directory.AutoRefresh = true

	// Original unchanged.
	assert.Equal(t, "s3://bucket/path", orig.URL)
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
		Comment:        ptr("original"),
		ChangeTracking: ptr(false),
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
			CommonSpec: CommonSpec{UseRole: ptr("SYSADMIN")},
			Name:       "DB1",
			Comment:    ptr("original"),
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
			{Spec: DatabaseSpec{Name: "DB1", Comment: ptr("one")}},
			{Spec: DatabaseSpec{Name: "DB2", Comment: ptr("two")}},
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
		Comment: ptr("original"),
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
		Comment: ptr("original"),
	}

	copied := orig.DeepCopy()
	*copied.Comment = "MUTATED"

	assert.Equal(t, "original", *orig.Comment)
}

// TestDeepCopy_OAuthIntegrationForCustomClientsSpec_PointerIsolation tests all
// pointer, slice-of-string, and nested fields for deep-copy isolation.
func TestDeepCopy_OAuthIntegrationForCustomClientsSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := OAuthIntegrationForCustomClientsSpec{
		CommonSpec:                  CommonSpec{UseRole: ptr("SYSADMIN")},
		Name:                        "MY_OAUTH",
		Enabled:                     ptr(true),
		OAuthClientType:             "CONFIDENTIAL",
		OAuthRedirectURI:            "https://example.com/callback",
		OAuthAllowNonTLSRedirectURI: ptr(false),
		OAuthEnforcePKCE:            ptr(true),
		OAuthUseSecondaryRoles:      ptr("IMPLICIT"),
		PreAuthorizedRolesList:      []string{"ROLE_A", "ROLE_B"},
		BlockedRolesList:            []string{"BLOCKED_ROLE"},
		OAuthIssueRefreshTokens:     ptr(true),
		OAuthRefreshTokenValidity:   ptr(int64(86400)),
		NetworkPolicy:               ptr("MY_POLICY"),
		OAuthClientRSAPublicKey:     ptr("public-key-1"),
		OAuthClientRSAPublicKey2:    ptr("public-key-2"),
		Comment:                     ptr("original"),
	}

	copied := orig.DeepCopy()
	require.NotNil(t, copied)

	// Mutate all pointers in the copy.
	*copied.UseRole = "HACKER"
	*copied.Enabled = false
	*copied.OAuthAllowNonTLSRedirectURI = true
	*copied.OAuthEnforcePKCE = false
	*copied.OAuthUseSecondaryRoles = "NONE"
	*copied.OAuthIssueRefreshTokens = false
	*copied.OAuthRefreshTokenValidity = 0
	*copied.NetworkPolicy = "MUTATED"
	*copied.OAuthClientRSAPublicKey = "MUTATED"
	*copied.OAuthClientRSAPublicKey2 = "MUTATED"
	*copied.Comment = "MUTATED"

	// Mutate slices in the copy.
	copied.PreAuthorizedRolesList[0] = "MUTATED"
	copied.BlockedRolesList = append(copied.BlockedRolesList, "EXTRA")

	// Original must be unchanged.
	assert.Equal(t, "SYSADMIN", *orig.UseRole)
	assert.Equal(t, true, *orig.Enabled)
	assert.Equal(t, false, *orig.OAuthAllowNonTLSRedirectURI)
	assert.Equal(t, true, *orig.OAuthEnforcePKCE)
	assert.Equal(t, "IMPLICIT", *orig.OAuthUseSecondaryRoles)
	assert.Equal(t, true, *orig.OAuthIssueRefreshTokens)
	assert.Equal(t, int64(86400), *orig.OAuthRefreshTokenValidity)
	assert.Equal(t, "MY_POLICY", *orig.NetworkPolicy)
	assert.Equal(t, "public-key-1", *orig.OAuthClientRSAPublicKey)
	assert.Equal(t, "public-key-2", *orig.OAuthClientRSAPublicKey2)
	assert.Equal(t, "original", *orig.Comment)
	assert.Equal(t, []string{"ROLE_A", "ROLE_B"}, orig.PreAuthorizedRolesList)
	assert.Equal(t, []string{"BLOCKED_ROLE"}, orig.BlockedRolesList)
}

// TestDeepCopy_FunctionPythonSpec_PointerIsolation tests complex nested types
// including []CallableArgument, []SecretBinding, and *ObjectReference.
func TestDeepCopy_FunctionPythonSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := FunctionPythonSpec{
		CommonSpec:   CommonSpec{UseRole: ptr("SYSADMIN")},
		Name:         "MY_FUNC",
		DatabaseRef:  &ObjectReference{Name: "DB_REF"},
		DatabaseName: ptr("MY_DB"),
		SchemaRef:    &ObjectReference{Name: "SCHEMA_REF"},
		SchemaName:   ptr("MY_SCHEMA"),
		Arguments: []CallableArgument{
			{Name: "arg1", Type: "VARCHAR", DefaultValue: ptr("hello")},
			{Name: "arg2", Type: "NUMBER"},
		},
		Returns:                    "VARCHAR",
		Handler:                    "handler",
		RuntimeVersion:             "3.8",
		SnowparkPackage:            "snowflake-snowpark-python==1.0.0",
		Body:                       ptr("def handler(arg1, arg2): return arg1"),
		Packages:                   []string{"pkg1", "pkg2"},
		Imports:                    []string{"@stage/file.py"},
		ExternalAccessIntegrations: []string{"EAI1"},
		Secrets: []SecretBinding{
			{SecretName: "secret1", VariableName: "var1"},
		},
		NullInputBehavior: ptr("RETURNS NULL ON NULL INPUT"),
		Volatility:        ptr("IMMUTABLE"),
		Secure:            ptr(true),
		Comment:           ptr("original"),
	}

	copied := orig.DeepCopy()
	require.NotNil(t, copied)

	// Mutate pointers.
	*copied.UseRole = "HACKER"
	*copied.DatabaseName = "MUTATED"
	*copied.SchemaName = "MUTATED"
	*copied.Body = "MUTATED"
	*copied.NullInputBehavior = "MUTATED"
	*copied.Volatility = "MUTATED"
	*copied.Secure = false
	*copied.Comment = "MUTATED"

	// Mutate nested struct pointers.
	copied.DatabaseRef.Name = "MUTATED"
	copied.SchemaRef.Name = "MUTATED"

	// Mutate slice of structs.
	copied.Arguments[0].Name = "MUTATED"
	*copied.Arguments[0].DefaultValue = "MUTATED"
	copied.Secrets[0].SecretName = "MUTATED"

	// Mutate string slices.
	copied.Packages[0] = "MUTATED"
	copied.Imports[0] = "MUTATED"
	copied.ExternalAccessIntegrations[0] = "MUTATED"

	// Original must be unchanged.
	assert.Equal(t, "SYSADMIN", *orig.UseRole)
	assert.Equal(t, "MY_DB", *orig.DatabaseName)
	assert.Equal(t, "MY_SCHEMA", *orig.SchemaName)
	assert.Equal(t, "def handler(arg1, arg2): return arg1", *orig.Body)
	assert.Equal(t, "RETURNS NULL ON NULL INPUT", *orig.NullInputBehavior)
	assert.Equal(t, "IMMUTABLE", *orig.Volatility)
	assert.Equal(t, true, *orig.Secure)
	assert.Equal(t, "original", *orig.Comment)
	assert.Equal(t, "DB_REF", orig.DatabaseRef.Name)
	assert.Equal(t, "SCHEMA_REF", orig.SchemaRef.Name)
	assert.Equal(t, "arg1", orig.Arguments[0].Name)
	assert.Equal(t, "hello", *orig.Arguments[0].DefaultValue)
	assert.Equal(t, "secret1", orig.Secrets[0].SecretName)
	assert.Equal(t, []string{"pkg1", "pkg2"}, orig.Packages)
	assert.Equal(t, []string{"@stage/file.py"}, orig.Imports)
	assert.Equal(t, []string{"EAI1"}, orig.ExternalAccessIntegrations)
}

// TestDeepCopy_NetworkRuleSpec_PointerIsolation tests NetworkRuleSpec with
// *ObjectReference pointers and []string slice isolation.
func TestDeepCopy_NetworkRuleSpec_PointerIsolation(t *testing.T) {
	t.Parallel()

	orig := NetworkRuleSpec{
		CommonSpec:   CommonSpec{UseRole: ptr("SYSADMIN")},
		Name:         "MY_RULE",
		DatabaseRef:  &ObjectReference{Name: "DB_REF"},
		DatabaseName: ptr("MY_DB"),
		SchemaRef:    &ObjectReference{Name: "SCHEMA_REF"},
		SchemaName:   ptr("MY_SCHEMA"),
		Type:         "IPV4",
		Mode:         "INGRESS",
		ValueList:    []string{"10.0.0.1", "10.0.0.2"},
		Comment:      ptr("original"),
	}

	copied := orig.DeepCopy()
	require.NotNil(t, copied)

	// Mutate pointers and nested structs.
	*copied.UseRole = "HACKER"
	*copied.DatabaseName = "MUTATED"
	*copied.SchemaName = "MUTATED"
	copied.DatabaseRef.Name = "MUTATED"
	copied.SchemaRef.Name = "MUTATED"
	*copied.Comment = "MUTATED"
	copied.ValueList[0] = "MUTATED"

	// Original must be unchanged.
	assert.Equal(t, "SYSADMIN", *orig.UseRole)
	assert.Equal(t, "MY_DB", *orig.DatabaseName)
	assert.Equal(t, "MY_SCHEMA", *orig.SchemaName)
	assert.Equal(t, "DB_REF", orig.DatabaseRef.Name)
	assert.Equal(t, "SCHEMA_REF", orig.SchemaRef.Name)
	assert.Equal(t, "original", *orig.Comment)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, orig.ValueList)
}
