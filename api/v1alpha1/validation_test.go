package v1alpha1

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrStr(s string) *string { return &s }
func ptrInt32(v int32) *int32 { return &v }

// validCommonSpec returns a CommonSpec with the required providerRef.name
// and a valid DeletionPolicy set.
func validCommonSpec() CommonSpec {
	return CommonSpec{ProviderRef: ProviderReference{Name: "default"}, DeletionPolicy: DeletionPolicyDelete}
}

func TestDatabaseSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&DatabaseSpec{CommonSpec: validCommonSpec(), Name: "MYDB"}).Validate())
}

func TestDatabaseSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	err := (&DatabaseSpec{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestDatabaseSpec_Validate_RetentionOutOfRange(t *testing.T) {
	t.Parallel()
	err := (&DatabaseSpec{Name: "MYDB", DataRetentionTimeInDays: ptrInt32(100)}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.dataRetentionTimeInDays must be between 0 and 90")
}

func TestDatabaseSpec_Validate_MaxExtensionOutOfRange(t *testing.T) {
	t.Parallel()
	err := (&DatabaseSpec{Name: "MYDB", MaxDataExtensionTimeInDays: ptrInt32(-1)}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.maxDataExtensionTimeInDays must be between 0 and 90")
}

func TestDatabaseSpec_Validate_EmptyUseRole(t *testing.T) {
	t.Parallel()
	err := (&DatabaseSpec{CommonSpec: CommonSpec{UseRole: ptrStr("")}, Name: "MYDB"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.useRole must not be an empty string")
}

func TestDatabaseSpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	err := (&DatabaseSpec{
		CommonSpec:                 CommonSpec{UseRole: ptrStr("")},
		DataRetentionTimeInDays:    ptrInt32(100),
		MaxDataExtensionTimeInDays: ptrInt32(-5),
	}).Validate()
	require.Error(t, err)
	s := err.Error()
	assert.Contains(t, s, "spec.name is required")
	assert.Contains(t, s, "spec.dataRetentionTimeInDays")
	assert.Contains(t, s, "spec.maxDataExtensionTimeInDays")
	assert.Contains(t, s, "spec.useRole")
}

func TestSchemaSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&SchemaSpec{CommonSpec: validCommonSpec(), Name: "S", DatabaseRef: &LocalObjectReference{Name: "db"}}).Validate())
}

func TestSchemaSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	err := (&SchemaSpec{DatabaseRef: &LocalObjectReference{Name: "db"}}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestSchemaSpec_Validate_EmptyDatabaseRef(t *testing.T) {
	t.Parallel()
	err := (&SchemaSpec{Name: "S"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of spec.databaseRef or spec.databaseName must be set")
}

func TestSchemaSpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	err := (&SchemaSpec{CommonSpec: CommonSpec{UseRole: ptrStr("")}, DataRetentionTimeInDays: ptrInt32(200)}).Validate()
	require.Error(t, err)
	s := err.Error()
	assert.Contains(t, s, "spec.name is required")
	assert.Contains(t, s, "exactly one of spec.databaseRef or spec.databaseName must be set")
	assert.Contains(t, s, "spec.dataRetentionTimeInDays")
	assert.Contains(t, s, "spec.useRole")
}

func TestWarehouseSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH"}).Validate())
}

func TestWarehouseSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	err := (&WarehouseSpec{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestWarehouseSpec_Validate_ScaleFactorOutOfRange(t *testing.T) {
	t.Parallel()
	err := (&WarehouseSpec{Name: "WH", QueryAccelerationMaxScaleFactor: ptrInt32(150)}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.queryAccelerationMaxScaleFactor must be between 0 and 100")
}

func TestWarehouseSpec_Validate_ClusterCountInvalid(t *testing.T) {
	t.Parallel()
	err := (&WarehouseSpec{Name: "WH", MinClusterCount: ptrInt32(5), MaxClusterCount: ptrInt32(3)}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.minClusterCount (5) must not exceed spec.maxClusterCount (3)")
}

func TestWarehouseSpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	err := (&WarehouseSpec{
		CommonSpec:                      CommonSpec{UseRole: ptrStr("")},
		QueryAccelerationMaxScaleFactor: ptrInt32(200),
		MinClusterCount:                 ptrInt32(10),
		MaxClusterCount:                 ptrInt32(2),
	}).Validate()
	require.Error(t, err)
	parts := strings.Split(err.Error(), "\n")
	assert.GreaterOrEqual(t, len(parts), 3)
}

func TestAccountRoleSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&AccountRoleSpec{CommonSpec: validCommonSpec(), Name: "R"}).Validate())
}

func TestAccountRoleSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	err := (&AccountRoleSpec{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestAccountRoleSpec_Validate_EmptyUseRole(t *testing.T) {
	t.Parallel()
	err := (&AccountRoleSpec{CommonSpec: CommonSpec{UseRole: ptrStr("")}, Name: "R"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.useRole")
}

func TestUserSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&UserSpec{CommonSpec: validCommonSpec(), Name: "U"}).Validate())
}

func TestUserSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	err := (&UserSpec{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestUserSpec_Validate_InvalidType(t *testing.T) {
	t.Parallel()
	bad := UserType("INVALID")
	err := (&UserSpec{Name: "U", Type: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.type must be one of")
}

func TestUserSpec_Validate_ValidTypes(t *testing.T) {
	t.Parallel()
	for _, ut := range []UserType{UserTypePerson, UserTypeService, UserTypeLegacyService} {
		ut := ut
		t.Run(string(ut), func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, (&UserSpec{CommonSpec: validCommonSpec(), Name: "U", Type: &ut}).Validate())
		})
	}
}

func TestUserSpec_Validate_PasswordMissingName(t *testing.T) {
	t.Parallel()
	err := (&UserSpec{Name: "U", Password: &SecretKeyReference{Key: "k"}}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.password.name is required")
}

func TestUserSpec_Validate_PasswordMissingKey(t *testing.T) {
	t.Parallel()
	err := (&UserSpec{Name: "U", Password: &SecretKeyReference{Name: "s"}}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.password.key is required")
}

func TestUserSpec_Validate_RSAPublicKeyMissingFields(t *testing.T) {
	t.Parallel()
	err := (&UserSpec{Name: "U", RSAPublicKey: &SecretKeyReference{}}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.rsaPublicKey.name is required")
	assert.Contains(t, err.Error(), "spec.rsaPublicKey.key is required")
}

func TestUserSpec_Validate_RSAPublicKey2MissingFields(t *testing.T) {
	t.Parallel()
	err := (&UserSpec{Name: "U", RSAPublicKey2: &SecretKeyReference{}}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.rsaPublicKey2.name is required")
	assert.Contains(t, err.Error(), "spec.rsaPublicKey2.key is required")
}

func TestUserSpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	bad := UserType("BAD")
	err := (&UserSpec{
		CommonSpec:    CommonSpec{UseRole: ptrStr("")},
		Type:          &bad,
		Password:      &SecretKeyReference{},
		RSAPublicKey:  &SecretKeyReference{},
		RSAPublicKey2: &SecretKeyReference{},
	}).Validate()
	require.Error(t, err)
	parts := strings.Split(err.Error(), "\n")
	assert.GreaterOrEqual(t, len(parts), 9)
}

func TestValidateSecretKeyRef_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validateSecretKeyRef("f", &SecretKeyReference{Name: "s", Key: "k"}))
}

func TestValidateSecretKeyRef_Nil(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validateSecretKeyRef("f", nil))
}

// --- ProviderRef.Name validation (H-3) ---

func TestCommonSpec_Validate_EmptyProviderRef(t *testing.T) {
	t.Parallel()
	err := (&DatabaseSpec{Name: "D"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.providerRef.name is required")
}

func TestCommonSpec_Validate_ProviderRefSet(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&DatabaseSpec{CommonSpec: validCommonSpec(), Name: "D"}).Validate())
}

func TestComputeSpecHash_Deterministic(t *testing.T) {
	t.Parallel()

	spec := DatabaseSpec{Name: "mydb", Comment: ptrStr("hello")}
	h1, err1 := ComputeSpecHash(spec)
	require.NoError(t, err1)
	h2, err2 := ComputeSpecHash(spec)
	require.NoError(t, err2)
	assert.Equal(t, h1, h2, "same spec should produce identical hash")
	assert.Len(t, h1, 64, "SHA-256 hex output is 64 chars")
}

func TestComputeSpecHash_DifferentSpecs(t *testing.T) {
	t.Parallel()

	spec1 := DatabaseSpec{Name: "mydb", Comment: ptrStr("a")}
	spec2 := DatabaseSpec{Name: "mydb", Comment: ptrStr("b")}
	h1, err1 := ComputeSpecHash(spec1)
	require.NoError(t, err1)
	h2, err2 := ComputeSpecHash(spec2)
	require.NoError(t, err2)
	assert.NotEqual(t, h1, h2)
}

func TestComputeSpecHash_DetectsFieldRemoval(t *testing.T) {
	t.Parallel()

	specWithComment := DatabaseSpec{Name: "mydb", Comment: ptrStr("hello")}
	specWithoutComment := DatabaseSpec{Name: "mydb"}
	h1, err1 := ComputeSpecHash(specWithComment)
	require.NoError(t, err1)
	h2, err2 := ComputeSpecHash(specWithoutComment)
	require.NoError(t, err2)
	assert.NotEqual(t, h1, h2)
}

func TestComputeSpecHash_ErrorsOnUnmarshalable(t *testing.T) {
	t.Parallel()
	// Channels cannot be JSON-marshaled; ComputeSpecHash should return an error.
	_, err := ComputeSpecHash(make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "json.Marshal failed")
}

// --- DeletionPolicy validation (shared via CommonSpec) ---

func TestCommonSpec_Validate_InvalidDeletionPolicy(t *testing.T) {
	t.Parallel()
	dp := DeletionPolicy("Deletee")
	err := (&DatabaseSpec{Name: "D", CommonSpec: CommonSpec{DeletionPolicy: dp}}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.deletionPolicy must be one of")
}

func TestCommonSpec_Validate_ValidDeletionPolicies(t *testing.T) {
	t.Parallel()
	for _, dp := range []DeletionPolicy{DeletionPolicyDelete, DeletionPolicyOrphan} {
		dp := dp
		t.Run(string(dp), func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, (&DatabaseSpec{CommonSpec: CommonSpec{ProviderRef: ProviderReference{Name: "default"}, DeletionPolicy: dp}, Name: "D"}).Validate())
		})
	}
}

func TestCommonSpec_Validate_EmptyDeletionPolicyAccepted(t *testing.T) {
	t.Parallel()
	err := (&DatabaseSpec{CommonSpec: CommonSpec{ProviderRef: ProviderReference{Name: "default"}, DeletionPolicy: ""}, Name: "D"}).Validate()
	require.NoError(t, err, "empty deletionPolicy should be accepted (defaults to Delete)")
}

// --- Warehouse enum validation ---

func TestWarehouseSpec_Validate_InvalidWarehouseType(t *testing.T) {
	t.Parallel()
	bad := WarehouseType("INVALID")
	err := (&WarehouseSpec{Name: "WH", WarehouseType: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.warehouseType must be one of")
}

func TestWarehouseSpec_Validate_ValidWarehouseTypes(t *testing.T) {
	t.Parallel()
	for _, wt := range []WarehouseType{WarehouseTypeStandard, WarehouseTypeSnowparkOptimized} {
		wt := wt
		t.Run(string(wt), func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, (&WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH", WarehouseType: &wt}).Validate())
		})
	}
}

func TestWarehouseSpec_Validate_InvalidWarehouseSize(t *testing.T) {
	t.Parallel()
	bad := WarehouseSize("TINY")
	err := (&WarehouseSpec{Name: "WH", WarehouseSize: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.warehouseSize must be one of")
}

func TestWarehouseSpec_Validate_ValidWarehouseSizes(t *testing.T) {
	t.Parallel()
	for _, ws := range []WarehouseSize{
		WarehouseSizeXSmall, WarehouseSizeSmall, WarehouseSizeMedium, WarehouseSizeLarge,
		WarehouseSizeXLarge, WarehouseSize2XLarge, WarehouseSize3XLarge,
		WarehouseSize4XLarge, WarehouseSize5XLarge, WarehouseSize6XLarge,
	} {
		ws := ws
		t.Run(string(ws), func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, (&WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH", WarehouseSize: &ws}).Validate())
		})
	}
}

func TestWarehouseSpec_Validate_InvalidScalingPolicy(t *testing.T) {
	t.Parallel()
	bad := ScalingPolicy("AGGRESSIVE")
	err := (&WarehouseSpec{Name: "WH", ScalingPolicy: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.scalingPolicy must be one of")
}

func TestWarehouseSpec_Validate_ValidScalingPolicies(t *testing.T) {
	t.Parallel()
	for _, sp := range []ScalingPolicy{ScalingPolicyStandard, ScalingPolicyEconomy} {
		sp := sp
		t.Run(string(sp), func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, (&WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH", ScalingPolicy: &sp}).Validate())
		})
	}
}

func TestWarehouseSpec_Validate_InvalidResourceConstraint(t *testing.T) {
	t.Parallel()
	bad := ResourceConstraint("CPU")
	err := (&WarehouseSpec{Name: "WH", ResourceConstraint: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.resourceConstraint must be one of")
}

func TestWarehouseSpec_Validate_ValidResourceConstraints(t *testing.T) {
	t.Parallel()
	rc := ResourceConstraintMemory
	assert.NoError(t, (&WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH", ResourceConstraint: &rc}).Validate())
}

// --- Warehouse range validation (L-6) ---

func TestWarehouseSpec_Validate_ClusterCountRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		min     *int32
		max     *int32
		wantErr string
	}{
		{"min too low", ptrInt32(0), nil, "spec.minClusterCount must be between 1 and 10"},
		{"min too high", ptrInt32(11), nil, "spec.minClusterCount must be between 1 and 10"},
		{"max too low", nil, ptrInt32(0), "spec.maxClusterCount must be between 1 and 10"},
		{"max too high", nil, ptrInt32(11), "spec.maxClusterCount must be between 1 and 10"},
		{"valid min", ptrInt32(1), nil, ""},
		{"valid max", nil, ptrInt32(10), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := &WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH", MinClusterCount: tt.min, MaxClusterCount: tt.max}
			err := spec.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestWarehouseSpec_Validate_AutoSuspendNonNegative(t *testing.T) {
	t.Parallel()
	neg := int32(-1)
	err := (&WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH", AutoSuspend: &neg}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.autoSuspend must be non-negative")
}

func TestWarehouseSpec_Validate_MaxConcurrencyLevelRange(t *testing.T) {
	t.Parallel()

	zero := int32(0)
	err := (&WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH", MaxConcurrencyLevel: &zero}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.maxConcurrencyLevel must be between 1 and 32")

	over := int32(33)
	err = (&WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH", MaxConcurrencyLevel: &over}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.maxConcurrencyLevel must be between 1 and 32")

	valid := int32(16)
	assert.NoError(t, (&WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH", MaxConcurrencyLevel: &valid}).Validate())
}

func TestWarehouseSpec_Validate_TimeoutNonNegative(t *testing.T) {
	t.Parallel()

	neg := int32(-1)
	err := (&WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH", StatementQueuedTimeoutInSeconds: &neg}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.statementQueuedTimeoutInSeconds must be non-negative")

	err = (&WarehouseSpec{CommonSpec: validCommonSpec(), Name: "WH", StatementTimeoutInSeconds: &neg}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.statementTimeoutInSeconds must be non-negative")
}

// --- Database enum validation ---

func TestDatabaseSpec_Validate_InvalidStorageSerializationPolicy(t *testing.T) {
	t.Parallel()
	bad := StorageSerializationPolicy("CUSTOM")
	err := (&DatabaseSpec{Name: "D", StorageSerializationPolicy: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.storageSerializationPolicy must be one of")
}

func TestDatabaseSpec_Validate_ValidStorageSerializationPolicies(t *testing.T) {
	t.Parallel()
	for _, v := range []StorageSerializationPolicy{StorageSerializationPolicyCompatible, StorageSerializationPolicyOptimized} {
		v := v
		t.Run(string(v), func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, (&DatabaseSpec{CommonSpec: validCommonSpec(), Name: "D", StorageSerializationPolicy: &v}).Validate())
		})
	}
}

func TestDatabaseSpec_Validate_InvalidLogLevel(t *testing.T) {
	t.Parallel()
	bad := LogLevel("VERBOSE")
	err := (&DatabaseSpec{Name: "D", LogLevel: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.logLevel must be one of")
}

func TestDatabaseSpec_Validate_ValidLogLevels(t *testing.T) {
	t.Parallel()
	for _, v := range []LogLevel{LogLevelTrace, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelFatal, LogLevelOff} {
		v := v
		t.Run(string(v), func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, (&DatabaseSpec{CommonSpec: validCommonSpec(), Name: "D", LogLevel: &v}).Validate())
		})
	}
}

func TestDatabaseSpec_Validate_InvalidMetricLevel(t *testing.T) {
	t.Parallel()
	bad := MetricLevel("MOST")
	err := (&DatabaseSpec{Name: "D", MetricLevel: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.metricLevel must be one of")
}

func TestDatabaseSpec_Validate_InvalidTraceLevel(t *testing.T) {
	t.Parallel()
	bad := TraceLevel("SOMETIMES")
	err := (&DatabaseSpec{Name: "D", TraceLevel: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.traceLevel must be one of")
}

// --- Schema enum validation (same types as Database) ---

func TestSchemaSpec_Validate_MaxDataExtensionOutOfRange(t *testing.T) {
	t.Parallel()
	err := (&SchemaSpec{Name: "S", DatabaseRef: &LocalObjectReference{Name: "db"}, MaxDataExtensionTimeInDays: ptrInt32(100)}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.maxDataExtensionTimeInDays must be between 0 and 90")
}

func TestSchemaSpec_Validate_InvalidStorageSerializationPolicy(t *testing.T) {
	t.Parallel()
	bad := StorageSerializationPolicy("CUSTOM")
	err := (&SchemaSpec{Name: "S", DatabaseRef: &LocalObjectReference{Name: "db"}, StorageSerializationPolicy: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.storageSerializationPolicy must be one of")
}

func TestSchemaSpec_Validate_InvalidLogLevel(t *testing.T) {
	t.Parallel()
	bad := LogLevel("VERBOSE")
	err := (&SchemaSpec{Name: "S", DatabaseRef: &LocalObjectReference{Name: "db"}, LogLevel: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.logLevel must be one of")
}

func TestSchemaSpec_Validate_InvalidMetricLevel(t *testing.T) {
	t.Parallel()
	bad := MetricLevel("PARTIAL")
	err := (&SchemaSpec{Name: "S", DatabaseRef: &LocalObjectReference{Name: "db"}, MetricLevel: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.metricLevel must be one of")
}

func TestSchemaSpec_Validate_InvalidTraceLevel(t *testing.T) {
	t.Parallel()
	bad := TraceLevel("RARELY")
	err := (&SchemaSpec{Name: "S", DatabaseRef: &LocalObjectReference{Name: "db"}, TraceLevel: &bad}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.traceLevel must be one of")
}

// --- validateEnum helper ---

func TestValidateEnum_Nil(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validateEnum[WarehouseType]("f", nil, WarehouseTypeStandard))
}

func TestValidateEnum_Valid(t *testing.T) {
	t.Parallel()
	v := WarehouseTypeStandard
	assert.NoError(t, validateEnum("f", &v, WarehouseTypeStandard, WarehouseTypeSnowparkOptimized))
}

func TestValidateEnum_Invalid(t *testing.T) {
	t.Parallel()
	v := WarehouseType("BAD")
	err := validateEnum("f", &v, WarehouseTypeStandard, WarehouseTypeSnowparkOptimized)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be one of")
	assert.Contains(t, err.Error(), "BAD")
}

// --- ProviderConfigSpec Validation ---

func TestProviderConfigSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&ProviderConfigSpec{Account: "xy12345", User: "admin"}).Validate())
}

func TestProviderConfigSpec_Validate_ValidWithAuthType(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&ProviderConfigSpec{
		Account:            "xy12345",
		User:               "admin",
		AuthenticationType: AuthenticationTypeKeyPair,
		Credentials:        ProviderCredentials{SecretRef: &SecretKeyReference{Name: "my-secret", Key: "privateKey"}},
	}).Validate())
}

func TestProviderConfigSpec_Validate_EmptyAccount(t *testing.T) {
	t.Parallel()
	err := (&ProviderConfigSpec{User: "admin"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.account is required")
}

func TestProviderConfigSpec_Validate_EmptyUser(t *testing.T) {
	t.Parallel()
	err := (&ProviderConfigSpec{Account: "xy12345"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.user is required")
}

func TestProviderConfigSpec_Validate_InvalidAuthType(t *testing.T) {
	t.Parallel()
	err := (&ProviderConfigSpec{
		Account:            "xy12345",
		User:               "admin",
		AuthenticationType: AuthenticationType("BadAuth"),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.authenticationType must be one of")
}

func TestProviderConfigSpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	err := (&ProviderConfigSpec{
		AuthenticationType: AuthenticationType("BadAuth"),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.account is required")
	assert.Contains(t, err.Error(), "spec.user is required")
	assert.Contains(t, err.Error(), "spec.authenticationType must be one of")
}

func TestProviderConfigSpec_Validate_AllAuthTypes(t *testing.T) {
	t.Parallel()

	secretRef := ProviderCredentials{SecretRef: &SecretKeyReference{Name: "s", Key: "k"}}

	tests := []struct {
		authType         AuthenticationType
		creds            ProviderCredentials
		workloadIdentity *WorkloadIdentitySpec
	}{
		{AuthenticationTypeKeyPair, secretRef, nil},
		{AuthenticationTypeUsernamePassword, secretRef, nil},
		{AuthenticationTypeWorkloadIdentity, ProviderCredentials{}, &WorkloadIdentitySpec{}},
		{"", ProviderCredentials{}, nil}, // empty is allowed (default)
	}

	for _, tt := range tests {
		assert.NoError(t, (&ProviderConfigSpec{
			Account:            "xy12345",
			User:               "admin",
			AuthenticationType: tt.authType,
			Credentials:        tt.creds,
			WorkloadIdentity:   tt.workloadIdentity,
		}).Validate(), "expected valid for auth type %q", tt.authType)
	}
}

// --- Auth-type credential validation (H-2) ---

func TestProviderConfigSpec_Validate_KeyPairMissingSecret(t *testing.T) {
	t.Parallel()
	err := (&ProviderConfigSpec{
		Account:            "xy12345",
		User:               "admin",
		AuthenticationType: AuthenticationTypeKeyPair,
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secretRef (name and key) is required for KeyPair")
}

func TestProviderConfigSpec_Validate_KeyPair_PassphraseKeyAllowed(t *testing.T) {
	t.Parallel()
	err := (&ProviderConfigSpec{
		Account:            "xy12345",
		User:               "admin",
		AuthenticationType: AuthenticationTypeKeyPair,
		Credentials: ProviderCredentials{
			SecretRef:     &SecretKeyReference{Name: "my-secret", Key: "privateKey"},
			PassphraseKey: "passphrase",
		},
	}).Validate()
	assert.NoError(t, err)
}

func TestProviderConfigSpec_Validate_Password_PassphraseKeyRejected(t *testing.T) {
	t.Parallel()
	err := (&ProviderConfigSpec{
		Account:            "xy12345",
		User:               "admin",
		AuthenticationType: AuthenticationTypeUsernamePassword,
		Credentials: ProviderCredentials{
			SecretRef:     &SecretKeyReference{Name: "my-secret", Key: "password"},
			PassphraseKey: "passphrase",
		},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "passphraseKey is only valid for KeyPair")
}

func TestProviderConfigSpec_Validate_WorkloadIdentity_PassphraseKeyRejected(t *testing.T) {
	t.Parallel()
	err := (&ProviderConfigSpec{
		Account:            "xy12345",
		User:               "admin",
		AuthenticationType: AuthenticationTypeWorkloadIdentity,
		WorkloadIdentity:   &WorkloadIdentitySpec{Provider: WIFProviderOIDC},
		Credentials: ProviderCredentials{
			PassphraseKey: "passphrase",
		},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "passphraseKey is only valid for KeyPair")
}

func TestProviderConfigSpec_Validate_PasswordMissingSecret(t *testing.T) {
	t.Parallel()
	err := (&ProviderConfigSpec{
		Account:            "xy12345",
		User:               "admin",
		AuthenticationType: AuthenticationTypeUsernamePassword,
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secretRef (name and key) is required for UsernamePassword")
}

// --- WorkloadIdentity Validation ---

func TestProviderConfigSpec_Validate_WorkloadIdentity_Valid(t *testing.T) {
	t.Parallel()

	err := (&ProviderConfigSpec{
		Account:            "orgname-accountname",
		User:               "svc_snowplane",
		AuthenticationType: AuthenticationTypeWorkloadIdentity,
		Credentials:        ProviderCredentials{},
		WorkloadIdentity: &WorkloadIdentitySpec{
			TokenFilePath: "/var/run/secrets/snowflake/token",
			Audience:      "https://orgname-accountname.snowflakecomputing.com",
		},
	}).Validate()
	assert.NoError(t, err)
}

func TestProviderConfigSpec_Validate_WorkloadIdentity_DefaultTokenPath(t *testing.T) {
	t.Parallel()

	// No tokenFilePath set — defaults to /var/run/secrets/snowflake/token, which is valid.
	err := (&ProviderConfigSpec{
		Account:            "orgname-accountname",
		User:               "svc_snowplane",
		AuthenticationType: AuthenticationTypeWorkloadIdentity,
		Credentials:        ProviderCredentials{},
		WorkloadIdentity:   &WorkloadIdentitySpec{},
	}).Validate()
	assert.NoError(t, err)
}

func TestProviderConfigSpec_Validate_WorkloadIdentity_MissingBlock(t *testing.T) {
	t.Parallel()

	err := (&ProviderConfigSpec{
		Account:            "orgname-accountname",
		User:               "svc_snowplane",
		AuthenticationType: AuthenticationTypeWorkloadIdentity,
		Credentials:        ProviderCredentials{},
	}).Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "spec.workloadIdentity is required")
}

func TestProviderConfigSpec_Validate_WorkloadIdentity_InvalidTokenPath(t *testing.T) {
	t.Parallel()

	err := (&ProviderConfigSpec{
		Account:            "orgname-accountname",
		User:               "svc_snowplane",
		AuthenticationType: AuthenticationTypeWorkloadIdentity,
		Credentials:        ProviderCredentials{},
		WorkloadIdentity: &WorkloadIdentitySpec{
			TokenFilePath: "/etc/passwd",
		},
	}).Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be under /var/run/secrets/")
}

func TestProviderConfigSpec_Validate_WorkloadIdentity_SecretRefMutualExclusion(t *testing.T) {
	t.Parallel()

	err := (&ProviderConfigSpec{
		Account:            "orgname-accountname",
		User:               "svc_snowplane",
		AuthenticationType: AuthenticationTypeWorkloadIdentity,
		Credentials: ProviderCredentials{
			SecretRef: &SecretKeyReference{Name: "some-secret", Key: "key"},
		},
		WorkloadIdentity: &WorkloadIdentitySpec{},
	}).Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must not be set for WorkloadIdentity")
}

func TestProviderConfigSpec_Validate_WorkloadIdentity_AllProviders(t *testing.T) {
	t.Parallel()

	for _, provider := range []WorkloadIdentityProvider{WIFProviderOIDC, WIFProviderAWS, WIFProviderGCP, WIFProviderAzure, ""} {
		t.Run(string(provider), func(t *testing.T) {
			t.Parallel()
			err := (&ProviderConfigSpec{
				Account:            "orgname-accountname",
				User:               "svc_snowplane",
				AuthenticationType: AuthenticationTypeWorkloadIdentity,
				WorkloadIdentity:   &WorkloadIdentitySpec{Provider: provider},
			}).Validate()
			assert.NoError(t, err, "expected valid for provider %q", provider)
		})
	}
}

func TestProviderConfigSpec_Validate_WorkloadIdentity_InvalidProvider(t *testing.T) {
	t.Parallel()

	err := (&ProviderConfigSpec{
		Account:            "orgname-accountname",
		User:               "svc_snowplane",
		AuthenticationType: AuthenticationTypeWorkloadIdentity,
		WorkloadIdentity:   &WorkloadIdentitySpec{Provider: "InvalidCloud"},
	}).Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "spec.workloadIdentity.provider must be one of")
}

func TestProviderConfigSpec_Validate_WorkloadIdentity_AWSNoTokenPathValidation(t *testing.T) {
	t.Parallel()

	// AWS provider uses IAM credentials, not token file — no token path validation needed.
	err := (&ProviderConfigSpec{
		Account:            "orgname-accountname",
		User:               "svc_snowplane",
		AuthenticationType: AuthenticationTypeWorkloadIdentity,
		WorkloadIdentity:   &WorkloadIdentitySpec{Provider: WIFProviderAWS},
	}).Validate()
	assert.NoError(t, err)
}

func TestWorkloadIdentitySpec_GetTokenFilePath(t *testing.T) {
	t.Parallel()

	assert.Equal(t, DefaultTokenFilePath, (&WorkloadIdentitySpec{}).GetTokenFilePath())
	assert.Equal(t, "/var/run/secrets/custom/token", (&WorkloadIdentitySpec{TokenFilePath: "/var/run/secrets/custom/token"}).GetTokenFilePath())
}

func TestWorkloadIdentitySpec_GetProvider(t *testing.T) {
	t.Parallel()

	assert.Equal(t, WIFProviderOIDC, (&WorkloadIdentitySpec{}).GetProvider())
	assert.Equal(t, WIFProviderAWS, (&WorkloadIdentitySpec{Provider: WIFProviderAWS}).GetProvider())
}

// --- IsNamespaceAllowed ---

func TestIsNamespaceAllowed_EmptyList(t *testing.T) {
	t.Parallel()

	spec := &ProviderConfigSpec{AllowedNamespaces: nil}
	assert.True(t, spec.IsNamespaceAllowed("any-namespace"))
	assert.True(t, spec.IsNamespaceAllowed("default"))
}

func TestIsNamespaceAllowed_Wildcard(t *testing.T) {
	t.Parallel()

	spec := &ProviderConfigSpec{AllowedNamespaces: []string{"*"}}
	assert.True(t, spec.IsNamespaceAllowed("team-a"))
	assert.True(t, spec.IsNamespaceAllowed("team-b"))
}

func TestIsNamespaceAllowed_SpecificNamespaces(t *testing.T) {
	t.Parallel()

	spec := &ProviderConfigSpec{AllowedNamespaces: []string{"team-a", "team-b"}}
	assert.True(t, spec.IsNamespaceAllowed("team-a"))
	assert.True(t, spec.IsNamespaceAllowed("team-b"))
	assert.False(t, spec.IsNamespaceAllowed("team-c"))
	assert.False(t, spec.IsNamespaceAllowed("default"))
}

func TestIsNamespaceAllowed_WildcardAmongSpecific(t *testing.T) {
	t.Parallel()

	spec := &ProviderConfigSpec{AllowedNamespaces: []string{"team-a", "*"}}
	assert.True(t, spec.IsNamespaceAllowed("team-a"))
	assert.True(t, spec.IsNamespaceAllowed("any-ns"))
}

func TestIsNamespaceAllowed_SingleNamespace(t *testing.T) {
	t.Parallel()

	spec := &ProviderConfigSpec{AllowedNamespaces: []string{"prod"}}
	assert.True(t, spec.IsNamespaceAllowed("prod"))
	assert.False(t, spec.IsNamespaceAllowed("staging"))
}

// --- Dangerous Grant Validation ---

func TestValidateDangerousAccountRoleGrant_SafeGrant(t *testing.T) {
	t.Parallel()

	spec := &AccountRoleGrantSpec{
		Privilege:   "USAGE",
		On:          GrantOn{Account: true},
		AccountRole: "DATA_READER",
	}

	err := ValidateDangerousAccountRoleGrant(spec)
	assert.NoError(t, err)
}

func TestValidateDangerousAccountRoleGrant_DangerousSystemRoles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		role string
	}{
		{"ACCOUNTADMIN", "ACCOUNTADMIN"},
		{"SECURITYADMIN", "SECURITYADMIN"},
		{"ORGADMIN", "ORGADMIN"},
		{"case_insensitive", "accountadmin"},
		{"with_whitespace", "  ACCOUNTADMIN  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := &AccountRoleGrantSpec{
				Privilege:   "USAGE",
				On:          GrantOn{Account: true},
				AccountRole: tt.role,
			}

			err := ValidateDangerousAccountRoleGrant(spec)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "dangerous and blocked by default")
			assert.Contains(t, err.Error(), AnnotationAllowDangerousGrant)
		})
	}
}

func TestValidateDangerousAccountRoleGrant_DangerousPrivilegesOnAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		privilege string
	}{
		{"MANAGE_GRANTS", "MANAGE GRANTS"},
		{"OWNERSHIP", "OWNERSHIP"},
		{"case_insensitive", "manage grants"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := &AccountRoleGrantSpec{
				Privilege:   tt.privilege,
				On:          GrantOn{Account: true},
				AccountRole: "CUSTOM_ROLE",
			}

			err := ValidateDangerousAccountRoleGrant(spec)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "dangerous and blocked by default")
		})
	}
}

func TestValidateDangerousAccountRoleGrant_DangerousPrivilegeNotOnAccount(t *testing.T) {
	t.Parallel()

	// OWNERSHIP on a specific object is normal — only blocked on ACCOUNT.
	spec := &AccountRoleGrantSpec{
		Privilege: "OWNERSHIP",
		On: GrantOn{AccountObject: &GrantOnAccountObject{
			ObjectType: "DATABASE",
			ObjectName: "MY_DB",
		}},
		AccountRole: "DBA_ROLE",
	}

	err := ValidateDangerousAccountRoleGrant(spec)
	assert.NoError(t, err)
}

func TestValidateDangerousAccountRoleGrant_BothDangerousPrivAndTarget(t *testing.T) {
	t.Parallel()

	spec := &AccountRoleGrantSpec{
		Privilege:   "MANAGE GRANTS",
		On:          GrantOn{Account: true},
		AccountRole: "ACCOUNTADMIN",
	}

	err := ValidateDangerousAccountRoleGrant(spec)
	require.Error(t, err)
	// Should contain errors for both privilege and target.
	assert.Contains(t, err.Error(), "MANAGE GRANTS on ACCOUNT")
	assert.Contains(t, err.Error(), "system role ACCOUNTADMIN")
}

func TestValidateDangerousAccountRoleGrant_NonSystemRoleAllowed(t *testing.T) {
	t.Parallel()

	// Non-system account roles are never blocked by the target check.
	spec := &AccountRoleGrantSpec{
		Privilege:   "USAGE",
		On:          GrantOn{Account: true},
		AccountRole: "MY_CUSTOM_ROLE",
	}

	err := ValidateDangerousAccountRoleGrant(spec)
	assert.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: UserSpec email validation (L-8)
// --------------------------------------------------------------------------

func TestUserSpec_Validate_ValidEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		email string
	}{
		{"simple", "user@example.com"},
		{"plus_tag", "user+tag@example.com"},
		{"subdomain", "admin@mail.example.co.uk"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, (&UserSpec{
				CommonSpec: validCommonSpec(),
				Name:       "U",
				Email:      ptrStr(tt.email),
			}).Validate())
		})
	}
}

func TestUserSpec_Validate_InvalidEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		email string
	}{
		{"no_at", "not-an-email"},
		{"no_domain", "user@"},
		{"no_local", "@example.com"},
		{"spaces", "bad email@example.com"},
		{"double_at", "user@@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := (&UserSpec{
				CommonSpec: validCommonSpec(),
				Name:       "U",
				Email:      ptrStr(tt.email),
			}).Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "spec.email is not a valid email address")
		})
	}
}

func TestUserSpec_Validate_NilEmail(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&UserSpec{CommonSpec: validCommonSpec(), Name: "U", Email: nil}).Validate())
}

func TestUserSpec_Validate_EmptyEmail(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&UserSpec{CommonSpec: validCommonSpec(), Name: "U", Email: ptrStr("")}).Validate())
}

// --- Column Type Validation ---

func TestIsValidColumnType(t *testing.T) {
	t.Parallel()

	valid := []string{
		"VARCHAR", "VARCHAR(100)", "varchar(255)", "CHAR(10)",
		"NUMBER", "NUMBER(38,0)", "DECIMAL(10,2)", "INT", "INTEGER", "BIGINT",
		"FLOAT", "FLOAT4", "FLOAT8", "DOUBLE", "DOUBLE PRECISION", "REAL",
		"BOOLEAN", "DATE", "TIME", "DATETIME",
		"TIMESTAMP", "TIMESTAMP_LTZ", "TIMESTAMP_NTZ", "TIMESTAMP_TZ",
		"TIMESTAMP_LTZ(9)", "TIMESTAMP_NTZ(0)",
		"BINARY", "VARBINARY", "BINARY(16)",
		"VARIANT", "OBJECT", "ARRAY",
		"GEOGRAPHY", "GEOMETRY",
		"STRING", "TEXT",
		"VECTOR",
	}

	for _, typ := range valid {
		t.Run("valid/"+typ, func(t *testing.T) {
			t.Parallel()
			assert.True(t, isValidColumnType(typ), "expected %q to be valid", typ)
		})
	}

	invalid := []string{
		"INVALID_TYPE", "FOO", "VARCHAR2(100)", "NVARCHAR",
		"DROP TABLE", "'; DROP TABLE --", "<script>",
		"", " ",
	}

	for _, typ := range invalid {
		name := typ
		if name == "" {
			name = "empty"
		}
		if name == " " {
			name = "space"
		}

		t.Run("invalid/"+name, func(t *testing.T) {
			t.Parallel()
			assert.False(t, isValidColumnType(typ), "expected %q to be invalid", typ)
		})
	}
}

func TestTableSpec_Validate_ValidColumnType(t *testing.T) {
	t.Parallel()

	spec := TableSpec{
		CommonSpec:  validCommonSpec(),
		Name:        "T",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
		Columns: []ColumnDefinition{
			{Name: "id", Type: "NUMBER(38,0)"},
			{Name: "name", Type: "VARCHAR(100)"},
			{Name: "data", Type: "VARIANT"},
		},
	}
	assert.NoError(t, spec.Validate())
}

func TestTableSpec_Validate_InvalidColumnType(t *testing.T) {
	t.Parallel()

	spec := TableSpec{
		CommonSpec:  validCommonSpec(),
		Name:        "T",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
		Columns: []ColumnDefinition{
			{Name: "id", Type: "NUMBER"},
			{Name: "bad", Type: "INVALID_TYPE"},
		},
	}

	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `spec.columns[1].type "INVALID_TYPE" is not a recognized Snowflake data type`)
}

func TestTableSpec_Validate_SQLInjectionColumnType(t *testing.T) {
	t.Parallel()

	spec := TableSpec{
		CommonSpec:  validCommonSpec(),
		Name:        "T",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
		Columns: []ColumnDefinition{
			{Name: "col", Type: "'; DROP TABLE --"},
		},
	}

	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a recognized Snowflake data type")
}

// ---------------------------------------------------------------------------
// TableSpec — missing required fields
// ---------------------------------------------------------------------------

func TestTableSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	err := (&TableSpec{
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
		Columns:     []ColumnDefinition{{Name: "id", Type: "INT"}},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestTableSpec_Validate_EmptyDatabaseRef(t *testing.T) {
	t.Parallel()
	err := (&TableSpec{
		Name:      "T",
		SchemaRef: &LocalObjectReference{Name: "sch"},
		Columns:   []ColumnDefinition{{Name: "id", Type: "INT"}},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of spec.databaseRef or spec.databaseName must be set")
}

func TestTableSpec_Validate_EmptySchemaRef(t *testing.T) {
	t.Parallel()
	err := (&TableSpec{
		Name:        "T",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		Columns:     []ColumnDefinition{{Name: "id", Type: "INT"}},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of spec.schemaRef or spec.schemaName must be set")
}

func TestTableSpec_Validate_EmptyColumns(t *testing.T) {
	t.Parallel()
	err := (&TableSpec{
		Name:        "T",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
		Columns:     []ColumnDefinition{},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.columns must have at least one column")
}

func TestTableSpec_Validate_ColumnMissingName(t *testing.T) {
	t.Parallel()
	err := (&TableSpec{
		Name:        "T",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
		Columns:     []ColumnDefinition{{Type: "INT"}},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.columns[0].name is required")
}

func TestTableSpec_Validate_ColumnMissingType(t *testing.T) {
	t.Parallel()
	err := (&TableSpec{
		Name:        "T",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
		Columns:     []ColumnDefinition{{Name: "id"}},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.columns[0].type is required")
}

func TestTableSpec_Validate_DataRetentionOutOfRange(t *testing.T) {
	t.Parallel()
	err := (&TableSpec{
		CommonSpec:              validCommonSpec(),
		Name:                    "T",
		DatabaseRef:             &LocalObjectReference{Name: "db"},
		SchemaRef:               &LocalObjectReference{Name: "sch"},
		Columns:                 []ColumnDefinition{{Name: "id", Type: "INT"}},
		DataRetentionTimeInDays: ptrInt32(100),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.dataRetentionTimeInDays must be between 0 and 90")
}

func TestTableSpec_Validate_MaxExtensionOutOfRange(t *testing.T) {
	t.Parallel()
	err := (&TableSpec{
		CommonSpec:                 validCommonSpec(),
		Name:                       "T",
		DatabaseRef:                &LocalObjectReference{Name: "db"},
		SchemaRef:                  &LocalObjectReference{Name: "sch"},
		Columns:                    []ColumnDefinition{{Name: "id", Type: "INT"}},
		MaxDataExtensionTimeInDays: ptrInt32(-1),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.maxDataExtensionTimeInDays must be between 0 and 90")
}

func TestTableSpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	err := (&TableSpec{
		Columns:                 []ColumnDefinition{},
		DataRetentionTimeInDays: ptrInt32(200),
	}).Validate()
	require.Error(t, err)
	s := err.Error()
	assert.Contains(t, s, "spec.name is required")
	assert.Contains(t, s, "exactly one of spec.databaseRef or spec.databaseName must be set")
	assert.Contains(t, s, "exactly one of spec.schemaRef or spec.schemaName must be set")
	assert.Contains(t, s, "spec.columns must have at least one column")
	assert.Contains(t, s, "spec.dataRetentionTimeInDays")
}

func TestTableSpec_Validate_DuplicateColumnNames(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns: []ColumnDefinition{
			{Name: "ID", Type: "NUMBER"},
			{Name: "name", Type: "VARCHAR"},
			{Name: "id", Type: "VARCHAR"}, // duplicate (case-insensitive)
		},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicates spec.columns[0]")
}

func TestTableSpec_Validate_NoDuplicateColumns(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns: []ColumnDefinition{
			{Name: "ID", Type: "NUMBER"},
			{Name: "NAME", Type: "VARCHAR"},
		},
	}).Validate()
	require.NoError(t, err)
}

func TestTableSpec_Validate_ConstraintPrimaryKey(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns: []ColumnDefinition{
			{Name: "ID", Type: "NUMBER"},
			{Name: "NAME", Type: "VARCHAR"},
		},
		Constraints: []TableConstraint{
			{Name: "pk_id", Type: TableConstraintPrimaryKey, Columns: []string{"ID"}},
		},
	}).Validate()
	require.NoError(t, err)
}

func TestTableSpec_Validate_ConstraintForeignKey(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns: []ColumnDefinition{
			{Name: "ID", Type: "NUMBER"},
			{Name: "PARENT_ID", Type: "NUMBER"},
		},
		Constraints: []TableConstraint{
			{
				Name:    "fk_parent",
				Type:    TableConstraintForeignKey,
				Columns: []string{"PARENT_ID"},
				ForeignKey: &ForeignKeyReference{
					Table:   "PARENT",
					Columns: []string{"ID"},
				},
			},
		},
	}).Validate()
	require.NoError(t, err)
}

func TestTableSpec_Validate_ConstraintUndefinedColumn(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns: []ColumnDefinition{
			{Name: "ID", Type: "NUMBER"},
		},
		Constraints: []TableConstraint{
			{Name: "pk_bad", Type: TableConstraintPrimaryKey, Columns: []string{"NONEXISTENT"}},
		},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "references undefined column")
}

func TestTableSpec_Validate_ConstraintNoColumns(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns: []ColumnDefinition{
			{Name: "ID", Type: "NUMBER"},
		},
		Constraints: []TableConstraint{
			{Name: "pk", Type: TableConstraintPrimaryKey, Columns: []string{}},
		},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must have at least one column")
}

func TestTableSpec_Validate_ConstraintFKMissingRef(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns: []ColumnDefinition{
			{Name: "ID", Type: "NUMBER"},
		},
		Constraints: []TableConstraint{
			{Name: "fk", Type: TableConstraintForeignKey, Columns: []string{"ID"}},
		},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foreignKey is required")
}

func TestTableSpec_Validate_ConstraintFKEmptyTable(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns:      []ColumnDefinition{{Name: "ID", Type: "NUMBER"}},
		Constraints: []TableConstraint{
			{
				Name:       "fk",
				Type:       TableConstraintForeignKey,
				Columns:    []string{"ID"},
				ForeignKey: &ForeignKeyReference{Table: "", Columns: []string{"ID"}},
			},
		},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foreignKey.table is required")
}

func TestTableSpec_Validate_ConstraintFKEmptyColumns(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns:      []ColumnDefinition{{Name: "ID", Type: "NUMBER"}},
		Constraints: []TableConstraint{
			{
				Name:       "fk",
				Type:       TableConstraintForeignKey,
				Columns:    []string{"ID"},
				ForeignKey: &ForeignKeyReference{Table: "PARENT", Columns: nil},
			},
		},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foreignKey.columns must have at least one column")
}

func TestTableSpec_Validate_ConstraintFKColumnMismatch(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns: []ColumnDefinition{
			{Name: "A", Type: "NUMBER"},
			{Name: "B", Type: "NUMBER"},
		},
		Constraints: []TableConstraint{
			{
				Name:    "fk",
				Type:    TableConstraintForeignKey,
				Columns: []string{"A", "B"},
				ForeignKey: &ForeignKeyReference{
					Table:   "OTHER",
					Columns: []string{"X"},
				},
			},
		},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "column count")
}

func TestTableSpec_Validate_ConstraintInvalidType(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns: []ColumnDefinition{
			{Name: "ID", Type: "NUMBER"},
		},
		Constraints: []TableConstraint{
			{Name: "c", Type: "INVALID", Columns: []string{"ID"}},
		},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not valid")
}

func TestTableSpec_Validate_ConstraintPKWithForeignKey(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns: []ColumnDefinition{
			{Name: "ID", Type: "NUMBER"},
		},
		Constraints: []TableConstraint{
			{
				Name:    "pk",
				Type:    TableConstraintPrimaryKey,
				Columns: []string{"ID"},
				ForeignKey: &ForeignKeyReference{
					Table:   "OTHER",
					Columns: []string{"X"},
				},
			},
		},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foreignKey must not be set")
}

func TestTableSpec_Validate_ConstraintCaseInsensitiveColumnRef(t *testing.T) {
	t.Parallel()
	dbName := "DB"
	schemaName := "SCH"
	// Constraint references column with different case — should still pass.
	err := (&TableSpec{
		CommonSpec:   CommonSpec{ProviderRef: ProviderReference{Name: "default"}},
		Name:         "t",
		DatabaseName: &dbName,
		SchemaName:   &schemaName,
		Columns: []ColumnDefinition{
			{Name: "MyColumn", Type: "VARCHAR"},
		},
		Constraints: []TableConstraint{
			{Name: "uq", Type: TableConstraintUnique, Columns: []string{"mycolumn"}},
		},
	}).Validate()
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// ViewSpec — validation tests
// ---------------------------------------------------------------------------

func TestViewSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&ViewSpec{
		CommonSpec:  validCommonSpec(),
		Name:        "V",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
		Statement:   "SELECT 1",
	}).Validate())
}

func TestViewSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	err := (&ViewSpec{
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
		Statement:   "SELECT 1",
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestViewSpec_Validate_EmptyDatabaseRef(t *testing.T) {
	t.Parallel()
	err := (&ViewSpec{
		Name:      "V",
		SchemaRef: &LocalObjectReference{Name: "sch"},
		Statement: "SELECT 1",
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of spec.databaseRef or spec.databaseName must be set")
}

func TestViewSpec_Validate_EmptySchemaRef(t *testing.T) {
	t.Parallel()
	err := (&ViewSpec{
		Name:        "V",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		Statement:   "SELECT 1",
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of spec.schemaRef or spec.schemaName must be set")
}

func TestViewSpec_Validate_EmptyStatement(t *testing.T) {
	t.Parallel()
	err := (&ViewSpec{
		Name:        "V",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.statement is required")
}

func TestViewSpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	err := (&ViewSpec{}).Validate()
	require.Error(t, err)
	s := err.Error()
	assert.Contains(t, s, "spec.name is required")
	assert.Contains(t, s, "exactly one of spec.databaseRef or spec.databaseName must be set")
	assert.Contains(t, s, "exactly one of spec.schemaRef or spec.schemaName must be set")
	assert.Contains(t, s, "spec.statement is required")
}

// ---------------------------------------------------------------------------
// StageSpec — validation tests
// ---------------------------------------------------------------------------

func TestStageSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&StageSpec{
		CommonSpec:  validCommonSpec(),
		Name:        "S",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
	}).Validate())
}

func TestStageSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	err := (&StageSpec{
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestStageSpec_Validate_EmptyDatabaseRef(t *testing.T) {
	t.Parallel()
	err := (&StageSpec{
		Name:      "S",
		SchemaRef: &LocalObjectReference{Name: "sch"},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of spec.databaseRef or spec.databaseName must be set")
}

func TestStageSpec_Validate_EmptySchemaRef(t *testing.T) {
	t.Parallel()
	err := (&StageSpec{
		Name:        "S",
		DatabaseRef: &LocalObjectReference{Name: "db"},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of spec.schemaRef or spec.schemaName must be set")
}

func TestStageSpec_Validate_StorageIntegrationWithoutURL(t *testing.T) {
	t.Parallel()
	err := (&StageSpec{
		CommonSpec:         validCommonSpec(),
		Name:               "S",
		DatabaseRef:        &LocalObjectReference{Name: "db"},
		SchemaRef:          &LocalObjectReference{Name: "sch"},
		StorageIntegration: ptrStr("MY_INT"),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.storageIntegration requires spec.url")
}

func TestStageSpec_Validate_StorageIntegrationWithURL(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&StageSpec{
		CommonSpec:         validCommonSpec(),
		Name:               "S",
		DatabaseRef:        &LocalObjectReference{Name: "db"},
		SchemaRef:          &LocalObjectReference{Name: "sch"},
		URL:                ptrStr("s3://bucket/path"),
		StorageIntegration: ptrStr("MY_INT"),
	}).Validate())
}

func TestStageSpec_Validate_StorageIntegrationWithEmptyURL(t *testing.T) {
	t.Parallel()
	err := (&StageSpec{
		CommonSpec:         validCommonSpec(),
		Name:               "S",
		DatabaseRef:        &LocalObjectReference{Name: "db"},
		SchemaRef:          &LocalObjectReference{Name: "sch"},
		URL:                ptrStr(""),
		StorageIntegration: ptrStr("MY_INT"),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.storageIntegration requires spec.url")
}

func TestStageSpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	err := (&StageSpec{StorageIntegration: ptrStr("I")}).Validate()
	require.Error(t, err)
	s := err.Error()
	assert.Contains(t, s, "spec.name is required")
	assert.Contains(t, s, "exactly one of spec.databaseRef or spec.databaseName must be set")
	assert.Contains(t, s, "exactly one of spec.schemaRef or spec.schemaName must be set")
	assert.Contains(t, s, "spec.storageIntegration requires spec.url")
}

func TestStageSpec_Validate_EncryptionInternalOnly(t *testing.T) {
	t.Parallel()

	for _, encType := range []string{"SNOWFLAKE_FULL", "SNOWFLAKE_SSE"} {
		spec := &StageSpec{
			CommonSpec:  validCommonSpec(),
			Name:        "S",
			DatabaseRef: &LocalObjectReference{Name: "db"},
			SchemaRef:   &LocalObjectReference{Name: "sch"},
			URL:         ptrStr("s3://bucket/path"),
			Encryption:  &StageEncryption{Type: encType},
		}
		err := spec.Validate()
		require.Error(t, err, "encryption type %s should fail on external stage", encType)
		assert.Contains(t, err.Error(), "only valid for internal stages")
	}
}

func TestStageSpec_Validate_EncryptionExternalOnly(t *testing.T) {
	t.Parallel()

	for _, encType := range []string{"AWS_CSE", "AWS_SSE_S3", "AWS_SSE_KMS", "GCS_SSE_KMS", "AZURE_CSE", "NONE"} {
		spec := &StageSpec{
			CommonSpec:  validCommonSpec(),
			Name:        "S",
			DatabaseRef: &LocalObjectReference{Name: "db"},
			SchemaRef:   &LocalObjectReference{Name: "sch"},
			Encryption:  &StageEncryption{Type: encType},
		}
		err := spec.Validate()
		require.Error(t, err, "encryption type %s should fail on internal stage", encType)
		assert.Contains(t, err.Error(), "only valid for external stages")
	}
}

func TestStageSpec_Validate_EncryptionValidInternal(t *testing.T) {
	t.Parallel()
	spec := &StageSpec{
		CommonSpec:  validCommonSpec(),
		Name:        "S",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
		Encryption:  &StageEncryption{Type: "SNOWFLAKE_FULL"},
	}
	assert.NoError(t, spec.Validate())
}

func TestStageSpec_Validate_EncryptionValidExternal(t *testing.T) {
	t.Parallel()
	spec := &StageSpec{
		CommonSpec:  validCommonSpec(),
		Name:        "S",
		DatabaseRef: &LocalObjectReference{Name: "db"},
		SchemaRef:   &LocalObjectReference{Name: "sch"},
		URL:         ptrStr("s3://bucket/path"),
		Encryption:  &StageEncryption{Type: "AWS_SSE_S3"},
	}
	assert.NoError(t, spec.Validate())
}

// ---------------------------------------------------------------------------
// DatabaseRoleSpec — validation tests
// ---------------------------------------------------------------------------

func TestDatabaseRoleSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&DatabaseRoleSpec{
		CommonSpec:  validCommonSpec(),
		Name:        "R",
		DatabaseRef: &LocalObjectReference{Name: "db"},
	}).Validate())
}

func TestDatabaseRoleSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	err := (&DatabaseRoleSpec{
		DatabaseRef: &LocalObjectReference{Name: "db"},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestDatabaseRoleSpec_Validate_EmptyDatabaseRef(t *testing.T) {
	t.Parallel()
	err := (&DatabaseRoleSpec{
		Name: "R",
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of spec.databaseRef or spec.databaseName must be set")
}

func TestDatabaseRoleSpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	err := (&DatabaseRoleSpec{CommonSpec: CommonSpec{UseRole: ptrStr("")}}).Validate()
	require.Error(t, err)
	s := err.Error()
	assert.Contains(t, s, "spec.name is required")
	assert.Contains(t, s, "exactly one of spec.databaseRef or spec.databaseName must be set")
	assert.Contains(t, s, "spec.useRole")
}

// ---------------------------------------------------------------------------
// AccountRoleGrantSpec — validation tests
// ---------------------------------------------------------------------------

// validAccountRoleGrantSpec returns a minimal valid AccountRoleGrantSpec.
func validAccountRoleGrantSpec() *AccountRoleGrantSpec {
	return &AccountRoleGrantSpec{
		CommonSpec:  validCommonSpec(),
		Privilege:   "CREATE DATABASE",
		On:          GrantOn{Account: true},
		AccountRole: "MY_ROLE",
	}
}

// validDatabaseRoleGrantSpec returns a minimal valid DatabaseRoleGrantSpec.
func validDatabaseRoleGrantSpec() *DatabaseRoleGrantSpec {
	return &DatabaseRoleGrantSpec{
		CommonSpec:   validCommonSpec(),
		Privilege:    "USAGE",
		On:           GrantOn{AccountObject: &GrantOnAccountObject{ObjectType: "DATABASE", ObjectName: "MY_DB"}},
		DatabaseRole: "MY_DB.MY_ROLE",
	}
}

// validShareGrantSpec returns a minimal valid ShareGrantSpec.
func validShareGrantSpec() *ShareGrantSpec {
	return &ShareGrantSpec{
		CommonSpec: validCommonSpec(),
		Privilege:  "USAGE",
		ObjectType: "DATABASE",
		ObjectName: "MY_DB",
		Share:      "MY_SHARE",
	}
}

func TestAccountRoleGrantSpec_Validate_Valid_AccountGrant(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validAccountRoleGrantSpec().Validate())
}

func TestAccountRoleGrantSpec_Validate_Valid_AccountObjectGrant(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.Privilege = "USAGE"
	spec.On = GrantOn{AccountObject: &GrantOnAccountObject{ObjectType: "DATABASE", ObjectName: "MY_DB"}}
	assert.NoError(t, spec.Validate())
}

func TestAccountRoleGrantSpec_Validate_Valid_SchemaGrant_SchemaName(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.Privilege = "USAGE"
	spec.On = GrantOn{Schema: &GrantOnSchema{SchemaName: `"DB"."SCH"`}}
	assert.NoError(t, spec.Validate())
}

func TestAccountRoleGrantSpec_Validate_Valid_SchemaGrant_SchemaRef(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.Privilege = "USAGE"
	spec.On = GrantOn{Schema: &GrantOnSchema{SchemaRef: &LocalObjectReference{Name: "my-schema"}}}
	assert.NoError(t, spec.Validate())
}

func TestAccountRoleGrantSpec_Validate_Valid_SchemaGrant_AllInDatabase(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.Privilege = "USAGE"
	spec.On = GrantOn{Schema: &GrantOnSchema{AllInDatabase: "MY_DB"}}
	assert.NoError(t, spec.Validate())
}

func TestAccountRoleGrantSpec_Validate_Valid_SchemaGrant_AllInDatabaseRef(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.Privilege = "USAGE"
	spec.On = GrantOn{Schema: &GrantOnSchema{AllInDatabaseRef: &LocalObjectReference{Name: "my-db"}}}
	assert.NoError(t, spec.Validate())
}

func TestAccountRoleGrantSpec_Validate_Valid_SchemaGrant_FutureInDatabase(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.Privilege = "USAGE"
	spec.On = GrantOn{Schema: &GrantOnSchema{FutureInDatabase: "MY_DB"}}
	assert.NoError(t, spec.Validate())
}

func TestAccountRoleGrantSpec_Validate_Valid_SchemaGrant_FutureInDatabaseRef(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.Privilege = "USAGE"
	spec.On = GrantOn{Schema: &GrantOnSchema{FutureInDatabaseRef: &LocalObjectReference{Name: "my-db"}}}
	assert.NoError(t, spec.Validate())
}

func TestAccountRoleGrantSpec_Validate_Valid_SchemaObjectGrant(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.Privilege = "SELECT"
	spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{ObjectType: "TABLE", ObjectName: `"DB"."SCH"."T"`}}
	assert.NoError(t, spec.Validate())
}

func TestAccountRoleGrantSpec_Validate_Valid_SchemaObjectAllGrant(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.Privilege = "SELECT"
	spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{
		All: &GrantOnBulk{ObjectTypePlural: "TABLES", InDatabase: "MY_DB"},
	}}
	assert.NoError(t, spec.Validate())
}

func TestAccountRoleGrantSpec_Validate_Valid_SchemaObjectFutureGrant(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.Privilege = "SELECT"
	spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{
		Future: &GrantOnBulk{ObjectTypePlural: "TABLES", InSchema: `"DB"."SCH"`},
	}}
	assert.NoError(t, spec.Validate())
}

func TestAccountRoleGrantSpec_Validate_Valid_AccountRoleRef(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.AccountRole = ""
	spec.AccountRoleRef = &LocalObjectReference{Name: "my-role"}
	assert.NoError(t, spec.Validate())
}

// --- AccountRoleGrantSpec: empty privilege ---

func TestAccountRoleGrantSpec_Validate_EmptyPrivilege(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.Privilege = ""
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.privilege is required")
}

// --- AccountRoleGrantSpec: On mutual exclusivity ---

func TestAccountRoleGrantSpec_Validate_OnNoneSet(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.on: exactly one of account, accountObject, schema, or schemaObject must be set")
}

func TestAccountRoleGrantSpec_Validate_OnMultipleSet(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{
		Account:       true,
		AccountObject: &GrantOnAccountObject{ObjectType: "DATABASE", ObjectName: "DB"},
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.on: exactly one of")
}

// --- AccountRoleGrantSpec: AccountObject missing fields ---

func TestAccountRoleGrantSpec_Validate_AccountObject_MissingObjectType(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{AccountObject: &GrantOnAccountObject{ObjectName: "MY_DB"}}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.on.accountObject.objectType is required")
}

func TestAccountRoleGrantSpec_Validate_AccountObject_MissingObjectName(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{AccountObject: &GrantOnAccountObject{ObjectType: "DATABASE"}}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.on.accountObject.objectName is required")
}

// --- AccountRoleGrantSpec: Schema mutual exclusivity ---

func TestAccountRoleGrantSpec_Validate_Schema_NoneSet(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{Schema: &GrantOnSchema{}}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.on.schema: exactly one of")
}

func TestAccountRoleGrantSpec_Validate_Schema_MultipleSet(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{Schema: &GrantOnSchema{
		SchemaName:    "DB.SCH",
		AllInDatabase: "DB",
	}}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.on.schema: exactly one of")
}

func TestAccountRoleGrantSpec_Validate_Schema_RawAndRefMutuallyExclusive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		schema  GrantOnSchema
		wantErr string
	}{
		{
			"schemaName+schemaRef",
			GrantOnSchema{SchemaName: "DB.SCH", SchemaRef: &LocalObjectReference{Name: "s"}},
			"schemaName and schemaRef are mutually exclusive",
		},
		{
			"allInDatabase+allInDatabaseRef",
			GrantOnSchema{AllInDatabase: "DB", AllInDatabaseRef: &LocalObjectReference{Name: "d"}},
			"allInDatabase and allInDatabaseRef are mutually exclusive",
		},
		{
			"futureInDatabase+futureInDatabaseRef",
			GrantOnSchema{FutureInDatabase: "DB", FutureInDatabaseRef: &LocalObjectReference{Name: "d"}},
			"futureInDatabase and futureInDatabaseRef are mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := validAccountRoleGrantSpec()
			spec.On = GrantOn{Schema: &tt.schema}
			err := spec.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// --- AccountRoleGrantSpec: SchemaObject mutual exclusivity ---

func TestAccountRoleGrantSpec_Validate_SchemaObject_NoneSet(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{}}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.on.schemaObject: exactly one of")
}

func TestAccountRoleGrantSpec_Validate_SchemaObject_MultipleSet(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{
		ObjectType: "TABLE",
		ObjectName: "T",
		All:        &GrantOnBulk{ObjectTypePlural: "TABLES", InDatabase: "DB"},
	}}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.on.schemaObject: exactly one of")
}

func TestAccountRoleGrantSpec_Validate_SchemaObject_ObjectTypeMissingName(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{ObjectType: "TABLE"}}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.on.schemaObject.objectName is required when objectType is set")
}

func TestAccountRoleGrantSpec_Validate_SchemaObject_ObjectNameMissingType(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{ObjectName: "T"}}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.on.schemaObject.objectType is required when objectName is set")
}

// --- AccountRoleGrantSpec: Bulk grant (All/Future) validation ---

func TestAccountRoleGrantSpec_Validate_BulkGrant_MissingObjectTypePlural(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{
		All: &GrantOnBulk{InDatabase: "DB"},
	}}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "objectTypePlural is required")
}

func TestAccountRoleGrantSpec_Validate_BulkGrant_NoScope(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{
		All: &GrantOnBulk{ObjectTypePlural: "TABLES"},
	}}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of inDatabase, inDatabaseRef, inSchema, or inSchemaRef must be set")
}

func TestAccountRoleGrantSpec_Validate_BulkGrant_MultipleScope(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{
		All: &GrantOnBulk{ObjectTypePlural: "TABLES", InDatabase: "DB", InSchema: "DB.SCH"},
	}}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of inDatabase, inDatabaseRef, inSchema, or inSchemaRef must be set")
}

func TestAccountRoleGrantSpec_Validate_BulkGrant_RawAndRefMutuallyExclusive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		bulk    GrantOnBulk
		wantErr string
	}{
		{
			"inDatabase+inDatabaseRef",
			GrantOnBulk{ObjectTypePlural: "TABLES", InDatabase: "DB", InDatabaseRef: &LocalObjectReference{Name: "d"}},
			"inDatabase and inDatabaseRef are mutually exclusive",
		},
		{
			"inSchema+inSchemaRef",
			GrantOnBulk{ObjectTypePlural: "TABLES", InSchema: "DB.SCH", InSchemaRef: &LocalObjectReference{Name: "s"}},
			"inSchema and inSchemaRef are mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := validAccountRoleGrantSpec()
			spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{All: &tt.bulk}}
			err := spec.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAccountRoleGrantSpec_Validate_BulkGrant_ValidScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		bulk GrantOnBulk
	}{
		{"inDatabase", GrantOnBulk{ObjectTypePlural: "TABLES", InDatabase: "DB"}},
		{"inDatabaseRef", GrantOnBulk{ObjectTypePlural: "TABLES", InDatabaseRef: &LocalObjectReference{Name: "d"}}},
		{"inSchema", GrantOnBulk{ObjectTypePlural: "TABLES", InSchema: "DB.SCH"}},
		{"inSchemaRef", GrantOnBulk{ObjectTypePlural: "TABLES", InSchemaRef: &LocalObjectReference{Name: "s"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := validAccountRoleGrantSpec()
			spec.Privilege = "SELECT"
			spec.On = GrantOn{SchemaObject: &GrantOnSchemaObject{All: &tt.bulk}}
			assert.NoError(t, spec.Validate())
		})
	}
}

// --- AccountRoleGrantSpec: AccountRole/AccountRoleRef mutual exclusivity ---

func TestAccountRoleGrantSpec_Validate_AccountRoleAndRefMutuallyExclusive(t *testing.T) {
	t.Parallel()
	spec := validAccountRoleGrantSpec()
	spec.AccountRole = "R"
	spec.AccountRoleRef = &LocalObjectReference{Name: "r"}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accountRole and accountRoleRef are mutually exclusive")
}

// --- AccountRoleGrantSpec: Multiple errors aggregation ---

func TestAccountRoleGrantSpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	err := (&AccountRoleGrantSpec{
		// Empty privilege, empty On, no accountRole/accountRoleRef, missing providerRef.
		On: GrantOn{},
	}).Validate()
	require.Error(t, err)
	s := err.Error()
	assert.Contains(t, s, "spec.privilege is required")
	assert.Contains(t, s, "spec.on: exactly one of")
	assert.Contains(t, s, "spec: exactly one of accountRole or accountRoleRef must be set")
	assert.Contains(t, s, "spec.providerRef.name is required")
}

// ---------------------------------------------------------------------------
// AccountRoleGrantSpec.ResolveKind — tests
// ---------------------------------------------------------------------------

func TestAccountRoleGrantSpec_ResolveKind_Regular(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		on   GrantOn
	}{
		{"account", GrantOn{Account: true}},
		{"accountObject", GrantOn{AccountObject: &GrantOnAccountObject{ObjectType: "DATABASE", ObjectName: "DB"}}},
		{"schemaName", GrantOn{Schema: &GrantOnSchema{SchemaName: "DB.SCH"}}},
		{"schemaRef", GrantOn{Schema: &GrantOnSchema{SchemaRef: &LocalObjectReference{Name: "s"}}}},
		{"schemaObject", GrantOn{SchemaObject: &GrantOnSchemaObject{ObjectType: "TABLE", ObjectName: "T"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := &AccountRoleGrantSpec{On: tt.on}
			assert.Equal(t, GrantKindRegular, spec.ResolveKind())
		})
	}
}

func TestAccountRoleGrantSpec_ResolveKind_Future(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		on   GrantOn
	}{
		{"futureInDatabase", GrantOn{Schema: &GrantOnSchema{FutureInDatabase: "DB"}}},
		{"futureInDatabaseRef", GrantOn{Schema: &GrantOnSchema{FutureInDatabaseRef: &LocalObjectReference{Name: "d"}}}},
		{"futureSchemaObject", GrantOn{SchemaObject: &GrantOnSchemaObject{Future: &GrantOnBulk{ObjectTypePlural: "TABLES", InDatabase: "DB"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := &AccountRoleGrantSpec{On: tt.on}
			assert.Equal(t, GrantKindFuture, spec.ResolveKind())
		})
	}
}

func TestAccountRoleGrantSpec_ResolveKind_All(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		on   GrantOn
	}{
		{"allInDatabase", GrantOn{Schema: &GrantOnSchema{AllInDatabase: "DB"}}},
		{"allInDatabaseRef", GrantOn{Schema: &GrantOnSchema{AllInDatabaseRef: &LocalObjectReference{Name: "d"}}}},
		{"allSchemaObject", GrantOn{SchemaObject: &GrantOnSchemaObject{All: &GrantOnBulk{ObjectTypePlural: "TABLES", InDatabase: "DB"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := &AccountRoleGrantSpec{On: tt.on}
			assert.Equal(t, GrantKindAll, spec.ResolveKind())
		})
	}
}

// ---------------------------------------------------------------------------
// GrantOn.Description — tests
// ---------------------------------------------------------------------------

func TestGrantOn_Description(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		on   GrantOn
		want string
	}{
		{"account", GrantOn{Account: true}, "ON ACCOUNT"},
		{"accountObject", GrantOn{AccountObject: &GrantOnAccountObject{ObjectType: "DATABASE", ObjectName: "DB"}}, "ON DATABASE DB"},
		{"schemaName", GrantOn{Schema: &GrantOnSchema{SchemaName: "DB.SCH"}}, "ON SCHEMA DB.SCH"},
		{"schemaRef", GrantOn{Schema: &GrantOnSchema{SchemaRef: &LocalObjectReference{Name: "s"}}}, "ON SCHEMA (ref: s)"},
		{"allInDatabase", GrantOn{Schema: &GrantOnSchema{AllInDatabase: "DB"}}, "ON ALL SCHEMAS IN DATABASE DB"},
		{"allInDatabaseRef", GrantOn{Schema: &GrantOnSchema{AllInDatabaseRef: &LocalObjectReference{Name: "d"}}}, "ON ALL SCHEMAS IN DATABASE (ref: d)"},
		{"futureInDatabase", GrantOn{Schema: &GrantOnSchema{FutureInDatabase: "DB"}}, "ON FUTURE SCHEMAS IN DATABASE DB"},
		{"futureInDatabaseRef", GrantOn{Schema: &GrantOnSchema{FutureInDatabaseRef: &LocalObjectReference{Name: "d"}}}, "ON FUTURE SCHEMAS IN DATABASE (ref: d)"},
		{"schemaObject", GrantOn{SchemaObject: &GrantOnSchemaObject{ObjectType: "TABLE", ObjectName: "T"}}, "ON TABLE T"},
		{"allInDB", GrantOn{SchemaObject: &GrantOnSchemaObject{All: &GrantOnBulk{ObjectTypePlural: "TABLES", InDatabase: "DB"}}}, "ON ALL TABLES IN DATABASE DB"},
		{"allInDBRef", GrantOn{SchemaObject: &GrantOnSchemaObject{All: &GrantOnBulk{ObjectTypePlural: "TABLES", InDatabaseRef: &LocalObjectReference{Name: "d"}}}}, "ON ALL TABLES IN DATABASE (ref: d)"},
		{"allInSchema", GrantOn{SchemaObject: &GrantOnSchemaObject{All: &GrantOnBulk{ObjectTypePlural: "TABLES", InSchema: "DB.SCH"}}}, "ON ALL TABLES IN SCHEMA DB.SCH"},
		{"allInSchemaRef", GrantOn{SchemaObject: &GrantOnSchemaObject{All: &GrantOnBulk{ObjectTypePlural: "TABLES", InSchemaRef: &LocalObjectReference{Name: "s"}}}}, "ON ALL TABLES IN SCHEMA (ref: s)"},
		{"futureInDB", GrantOn{SchemaObject: &GrantOnSchemaObject{Future: &GrantOnBulk{ObjectTypePlural: "VIEWS", InDatabase: "DB"}}}, "ON FUTURE VIEWS IN DATABASE DB"},
		{"unknown", GrantOn{}, "ON <unknown>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.on.Description())
		})
	}
}

// ---------------------------------------------------------------------------
// AccountRoleAssignmentSpec.Validate — tests
// ---------------------------------------------------------------------------

func TestAccountRoleAssignmentSpec_Validate_Valid_RoleName_ToRole(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&AccountRoleAssignmentSpec{
		CommonSpec: validCommonSpec(), RoleName: "ANALYST", ToRole: "SYSADMIN",
	}).Validate())
}

func TestAccountRoleAssignmentSpec_Validate_Valid_RoleRef_ToUserRef(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&AccountRoleAssignmentSpec{
		CommonSpec: validCommonSpec(),
		RoleRef:    &LocalObjectReference{Name: "my-role"},
		ToUserRef:  &LocalObjectReference{Name: "my-user"},
	}).Validate())
}

func TestAccountRoleAssignmentSpec_Validate_NoRole(t *testing.T) {
	t.Parallel()
	err := (&AccountRoleAssignmentSpec{CommonSpec: validCommonSpec(), ToRole: "SYSADMIN"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of roleName or roleRef")
}

func TestAccountRoleAssignmentSpec_Validate_BothRoleAndRef(t *testing.T) {
	t.Parallel()
	err := (&AccountRoleAssignmentSpec{
		CommonSpec: validCommonSpec(),
		RoleName:   "X",
		RoleRef:    &LocalObjectReference{Name: "y"},
		ToRole:     "SYSADMIN",
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of roleName or roleRef")
}

func TestAccountRoleAssignmentSpec_Validate_NoTarget(t *testing.T) {
	t.Parallel()
	err := (&AccountRoleAssignmentSpec{CommonSpec: validCommonSpec(), RoleName: "R"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of toRole")
}

func TestAccountRoleAssignmentSpec_Validate_TwoTargets(t *testing.T) {
	t.Parallel()
	err := (&AccountRoleAssignmentSpec{
		CommonSpec: validCommonSpec(), RoleName: "R", ToRole: "A", ToUser: "B",
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of toRole")
}

// ---------------------------------------------------------------------------
// DatabaseRoleAssignmentSpec.Validate — tests
// ---------------------------------------------------------------------------

func TestDatabaseRoleAssignmentSpec_Validate_Valid_ToRole(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&DatabaseRoleAssignmentSpec{
		CommonSpec: validCommonSpec(), DatabaseRoleName: "MY_DB.READER", ToRole: "SYSADMIN",
	}).Validate())
}

func TestDatabaseRoleAssignmentSpec_Validate_Valid_ToDatabaseRole(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&DatabaseRoleAssignmentSpec{
		CommonSpec:      validCommonSpec(),
		DatabaseRoleRef: &LocalObjectReference{Name: "my-dr"},
		ToDatabaseRole:  "MY_DB.WRITER",
	}).Validate())
}

func TestDatabaseRoleAssignmentSpec_Validate_NoRole(t *testing.T) {
	t.Parallel()
	err := (&DatabaseRoleAssignmentSpec{CommonSpec: validCommonSpec(), ToRole: "SYSADMIN"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of databaseRoleName or databaseRoleRef")
}

func TestDatabaseRoleAssignmentSpec_Validate_BothRoleAndRef(t *testing.T) {
	t.Parallel()
	err := (&DatabaseRoleAssignmentSpec{
		CommonSpec:       validCommonSpec(),
		DatabaseRoleName: "X",
		DatabaseRoleRef:  &LocalObjectReference{Name: "y"},
		ToRole:           "SYSADMIN",
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of databaseRoleName or databaseRoleRef")
}

func TestDatabaseRoleAssignmentSpec_Validate_NoTarget(t *testing.T) {
	t.Parallel()
	err := (&DatabaseRoleAssignmentSpec{CommonSpec: validCommonSpec(), DatabaseRoleName: "D.R"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of toRole")
}

func TestDatabaseRoleAssignmentSpec_Validate_TwoTargets(t *testing.T) {
	t.Parallel()
	err := (&DatabaseRoleAssignmentSpec{
		CommonSpec: validCommonSpec(), DatabaseRoleName: "D.R", ToRole: "A", ToDatabaseRole: "D.B",
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of toRole")
}

// ---------------------------------------------------------------------------
// NotificationIntegrationSpec.Validate — tests
// ---------------------------------------------------------------------------

func TestNotificationIntegrationSpec_Validate_ValidEmail(t *testing.T) {
	t.Parallel()

	spec := NotificationIntegrationSpec{
		CommonSpec: validCommonSpec(),
		Name:       "MY_NI",
		Type:       NotificationIntegrationTypeEmail,
		Email:      &EmailNotificationConfig{AllowedRecipients: []string{"a@b.com"}},
	}
	require.NoError(t, spec.Validate())
}

func TestNotificationIntegrationSpec_Validate_ValidQueue(t *testing.T) {
	t.Parallel()

	spec := NotificationIntegrationSpec{
		CommonSpec: validCommonSpec(),
		Name:       "MY_NI",
		Type:       NotificationIntegrationTypeQueue,
		Queue:      &QueueNotificationConfig{NotificationProvider: "AWS_SNS", Direction: "OUTBOUND"},
	}
	require.NoError(t, spec.Validate())
}

func TestNotificationIntegrationSpec_Validate_ValidWebhook(t *testing.T) {
	t.Parallel()

	spec := NotificationIntegrationSpec{
		CommonSpec: validCommonSpec(),
		Name:       "MY_NI",
		Type:       NotificationIntegrationTypeWebhook,
		Webhook:    &WebhookNotificationConfig{WebhookURL: "https://example.com/hook"},
	}
	require.NoError(t, spec.Validate())
}

func TestNotificationIntegrationSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	spec := NotificationIntegrationSpec{
		CommonSpec: validCommonSpec(),
		Type:       NotificationIntegrationTypeEmail,
		Email:      &EmailNotificationConfig{AllowedRecipients: []string{"a@b.com"}},
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestNotificationIntegrationSpec_Validate_EmptyType(t *testing.T) {
	t.Parallel()

	spec := NotificationIntegrationSpec{
		CommonSpec: validCommonSpec(),
		Name:       "MY_NI",
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.type is required")
}

func TestNotificationIntegrationSpec_Validate_EmailMissingConfig(t *testing.T) {
	t.Parallel()

	spec := NotificationIntegrationSpec{
		CommonSpec: validCommonSpec(),
		Name:       "MY_NI",
		Type:       NotificationIntegrationTypeEmail,
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.email is required when type is EMAIL")
}

func TestNotificationIntegrationSpec_Validate_EmailMissingRecipients(t *testing.T) {
	t.Parallel()

	spec := NotificationIntegrationSpec{
		CommonSpec: validCommonSpec(),
		Name:       "MY_NI",
		Type:       NotificationIntegrationTypeEmail,
		Email:      &EmailNotificationConfig{},
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.email.allowedRecipients is required")
}

func TestNotificationIntegrationSpec_Validate_QueueMissingConfig(t *testing.T) {
	t.Parallel()

	spec := NotificationIntegrationSpec{
		CommonSpec: validCommonSpec(),
		Name:       "MY_NI",
		Type:       NotificationIntegrationTypeQueue,
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.queue is required when type is QUEUE")
}

func TestNotificationIntegrationSpec_Validate_QueueMissingProvider(t *testing.T) {
	t.Parallel()

	spec := NotificationIntegrationSpec{
		CommonSpec: validCommonSpec(),
		Name:       "MY_NI",
		Type:       NotificationIntegrationTypeQueue,
		Queue:      &QueueNotificationConfig{Direction: "OUTBOUND"},
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.queue.notificationProvider is required")
}

func TestNotificationIntegrationSpec_Validate_WebhookMissingConfig(t *testing.T) {
	t.Parallel()

	spec := NotificationIntegrationSpec{
		CommonSpec: validCommonSpec(),
		Name:       "MY_NI",
		Type:       NotificationIntegrationTypeWebhook,
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.webhook is required when type is WEBHOOK")
}

func TestNotificationIntegrationSpec_Validate_WebhookMissingURL(t *testing.T) {
	t.Parallel()

	spec := NotificationIntegrationSpec{
		CommonSpec: validCommonSpec(),
		Name:       "MY_NI",
		Type:       NotificationIntegrationTypeWebhook,
		Webhook:    &WebhookNotificationConfig{},
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.webhook.webhookURL is required")
}

func TestNotificationIntegrationSpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()

	err := (&NotificationIntegrationSpec{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
	assert.Contains(t, err.Error(), "spec.type is required")
}

// ---------------------------------------------------------------------------
// FieldExportSpec.Validate — tests
// ---------------------------------------------------------------------------

func TestFieldExportSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	spec := FieldExportSpec{
		From: FieldExportSource{
			Resource: FieldExportResourceRef{Kind: "Database", Name: "my-db"},
			Path:     ".status.showOutput.name",
		},
		To: FieldExportTarget{
			Kind: FieldExportTargetConfigMap,
			Name: "my-cm",
			Key:  "db-name",
		},
	}
	assert.NoError(t, spec.Validate())
}

func TestFieldExportSpec_Validate_UnsupportedKind(t *testing.T) {
	t.Parallel()
	spec := FieldExportSpec{
		From: FieldExportSource{
			Resource: FieldExportResourceRef{Kind: "Deployment", Name: "my-deploy"},
			Path:     ".status.replicas",
		},
		To: FieldExportTarget{
			Kind: FieldExportTargetConfigMap,
			Name: "my-cm",
			Key:  "replicas",
		},
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a supported Snowplane resource kind")
}

func TestFieldExportSpec_Validate_EmptyFields(t *testing.T) {
	t.Parallel()
	err := (&FieldExportSpec{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.from.resource.kind is required")
	assert.Contains(t, err.Error(), "spec.from.resource.name is required")
	assert.Contains(t, err.Error(), "spec.from.path is required")
	assert.Contains(t, err.Error(), "spec.to.kind is required")
	assert.Contains(t, err.Error(), "spec.to.name is required")
	assert.Contains(t, err.Error(), "spec.to.key is required")
}

func TestFieldExportSpec_Validate_InvalidTargetKind(t *testing.T) {
	t.Parallel()
	spec := FieldExportSpec{
		From: FieldExportSource{
			Resource: FieldExportResourceRef{Kind: "Database", Name: "my-db"},
			Path:     ".status.showOutput.name",
		},
		To: FieldExportTarget{
			Kind: "Deployment",
			Name: "my-cm",
			Key:  "db-name",
		},
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"Deployment" is invalid`)
	assert.Contains(t, err.Error(), `"ConfigMap"`)
	assert.Contains(t, err.Error(), `"Secret"`)
}

func TestFieldExportSpec_Validate_SecretTargetKind(t *testing.T) {
	t.Parallel()
	spec := FieldExportSpec{
		From: FieldExportSource{
			Resource: FieldExportResourceRef{Kind: "Database", Name: "my-db"},
			Path:     ".status.showOutput.name",
		},
		To: FieldExportTarget{
			Kind: FieldExportTargetSecret,
			Name: "my-secret",
			Key:  "db-name",
		},
	}
	assert.NoError(t, spec.Validate())
}

func TestFieldExportSpec_Validate_PathNoLeadingDot(t *testing.T) {
	t.Parallel()
	spec := FieldExportSpec{
		From: FieldExportSource{
			Resource: FieldExportResourceRef{Kind: "Database", Name: "my-db"},
			Path:     "status.showOutput.name",
		},
		To: FieldExportTarget{
			Kind: FieldExportTargetConfigMap,
			Name: "my-cm",
			Key:  "db-name",
		},
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `must start with "."`)
}

func TestFieldExportSpec_Validate_PathArrayIndexing(t *testing.T) {
	t.Parallel()
	spec := FieldExportSpec{
		From: FieldExportSource{
			Resource: FieldExportResourceRef{Kind: "Database", Name: "my-db"},
			Path:     ".status.conditions[0].message",
		},
		To: FieldExportTarget{
			Kind: FieldExportTargetConfigMap,
			Name: "my-cm",
			Key:  "msg",
		},
	}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support array indexing")
}

func TestFieldExportSpec_Validate_AllSourceKinds(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"Database", "Schema", "Warehouse", "User", "AccountRole", "DatabaseRole", "AccountRoleGrant", "DatabaseRoleGrant", "ShareGrant", "Table", "View", "Stage", "Task", "StreamOnTable", "StreamOnView", "StreamOnExternalTable", "StreamOnDirectoryTable", "StreamOnDynamicTable", "Tag", "NetworkPolicy", "ResourceMonitor", "MaskingPolicy", "RowAccessPolicy", "GrantOwnership"} {
		spec := FieldExportSpec{
			From: FieldExportSource{
				Resource: FieldExportResourceRef{Kind: kind, Name: "test"},
				Path:     ".status.showOutput.name",
			},
			To: FieldExportTarget{
				Kind: FieldExportTargetConfigMap,
				Name: "cm",
				Key:  "key",
			},
		}
		assert.NoError(t, spec.Validate(), "kind %s should be valid", kind)
	}
}

// ---------------------------------------------------------------------------
// ShareGrantSpec — validation tests
// ---------------------------------------------------------------------------

func TestShareGrantSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validShareGrantSpec().Validate())
}

func TestShareGrantSpec_Validate_EmptyFields(t *testing.T) {
	t.Parallel()
	err := (&ShareGrantSpec{}).Validate()
	require.Error(t, err)
	s := err.Error()
	assert.Contains(t, s, "spec.privilege is required")
	assert.Contains(t, s, "spec.objectType is required")
	assert.Contains(t, s, "spec.objectName is required")
	assert.Contains(t, s, "spec.share is required")
}

// ---------------------------------------------------------------------------
// DatabaseRoleGrantSpec — validation tests
// ---------------------------------------------------------------------------

func TestDatabaseRoleGrantSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validDatabaseRoleGrantSpec().Validate())
}

func TestDatabaseRoleGrantSpec_Validate_RoleAndRefMutuallyExclusive(t *testing.T) {
	t.Parallel()
	spec := validDatabaseRoleGrantSpec()
	spec.DatabaseRoleRef = &LocalObjectReference{Name: "r"}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "databaseRole and databaseRoleRef are mutually exclusive")
}

func TestDatabaseRoleGrantSpec_Validate_EmptyPrivilege(t *testing.T) {
	t.Parallel()
	spec := validDatabaseRoleGrantSpec()
	spec.Privilege = ""
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.privilege is required")
}

func TestDatabaseRoleGrantSpec_Validate_NoRoleOrRef(t *testing.T) {
	t.Parallel()
	spec := validDatabaseRoleGrantSpec()
	spec.DatabaseRole = ""
	spec.DatabaseRoleRef = nil
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of databaseRole or databaseRoleRef must be set")
}

func TestDatabaseRoleGrantSpec_Validate_OnNoneSet(t *testing.T) {
	t.Parallel()
	spec := validDatabaseRoleGrantSpec()
	spec.On = GrantOn{}
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of")
}

func TestDatabaseRoleGrantSpec_Validate_ValidWithRef(t *testing.T) {
	t.Parallel()
	spec := validDatabaseRoleGrantSpec()
	spec.DatabaseRole = ""
	spec.DatabaseRoleRef = &LocalObjectReference{Name: "my-role"}
	assert.NoError(t, spec.Validate())
}

// --------------------------------------------------------------------------
// Tests: validatePrivilegeObjectCompat (L-15)
// --------------------------------------------------------------------------

func TestPrivilegeObjectCompat_Account_Valid(t *testing.T) {
	t.Parallel()

	for _, priv := range []string{
		"CREATE DATABASE", "MANAGE GRANTS", "MONITOR USAGE", "EXECUTE TASK",
		"ALL", "ALL PRIVILEGES",
	} {
		t.Run(priv, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validatePrivilegeObjectCompat(
				&GrantOn{Account: true}, priv))
		})
	}
}

func TestPrivilegeObjectCompat_Account_Invalid(t *testing.T) {
	t.Parallel()

	for _, priv := range []string{"SELECT", "INSERT", "USAGE", "READ", "WRITE"} {
		t.Run(priv, func(t *testing.T) {
			t.Parallel()
			err := validatePrivilegeObjectCompat(&GrantOn{Account: true}, priv)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not a valid global account privilege")
		})
	}
}

func TestPrivilegeObjectCompat_Account_CaseInsensitive(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validatePrivilegeObjectCompat(
		&GrantOn{Account: true}, "create database"))
}

func TestPrivilegeObjectCompat_Database_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{AccountObject: &GrantOnAccountObject{ObjectType: "DATABASE", ObjectName: "DB"}}

	for _, priv := range []string{"USAGE", "MONITOR", "CREATE SCHEMA", "MODIFY", "ALL"} {
		t.Run(priv, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validatePrivilegeObjectCompat(on, priv))
		})
	}
}

func TestPrivilegeObjectCompat_Database_Invalid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{AccountObject: &GrantOnAccountObject{ObjectType: "DATABASE", ObjectName: "DB"}}
	err := validatePrivilegeObjectCompat(on, "SELECT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for DATABASE")
}

func TestPrivilegeObjectCompat_Warehouse_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{AccountObject: &GrantOnAccountObject{ObjectType: "WAREHOUSE", ObjectName: "WH"}}

	for _, priv := range []string{"USAGE", "OPERATE", "MONITOR", "MODIFY", "ALL"} {
		t.Run(priv, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validatePrivilegeObjectCompat(on, priv))
		})
	}
}

func TestPrivilegeObjectCompat_Warehouse_Invalid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{AccountObject: &GrantOnAccountObject{ObjectType: "WAREHOUSE", ObjectName: "WH"}}
	err := validatePrivilegeObjectCompat(on, "SELECT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for WAREHOUSE")
}

func TestPrivilegeObjectCompat_Table_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{ObjectType: "TABLE", ObjectName: `"DB"."SCH"."T"`}}

	for _, priv := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "ALL"} {
		t.Run(priv, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validatePrivilegeObjectCompat(on, priv))
		})
	}
}

func TestPrivilegeObjectCompat_Table_Invalid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{ObjectType: "TABLE", ObjectName: `"DB"."SCH"."T"`}}
	err := validatePrivilegeObjectCompat(on, "CREATE SCHEMA")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for TABLE")
}

func TestPrivilegeObjectCompat_View_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{ObjectType: "VIEW", ObjectName: `"DB"."SCH"."V"`}}
	assert.NoError(t, validatePrivilegeObjectCompat(on, "SELECT"))
}

func TestPrivilegeObjectCompat_View_Invalid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{ObjectType: "VIEW", ObjectName: `"DB"."SCH"."V"`}}
	err := validatePrivilegeObjectCompat(on, "INSERT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for VIEW")
}

func TestPrivilegeObjectCompat_Stage_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{ObjectType: "STAGE", ObjectName: `"DB"."SCH"."S"`}}
	assert.NoError(t, validatePrivilegeObjectCompat(on, "READ"))
}

func TestPrivilegeObjectCompat_Stage_Invalid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{ObjectType: "STAGE", ObjectName: `"DB"."SCH"."S"`}}
	err := validatePrivilegeObjectCompat(on, "SELECT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for STAGE")
}

func TestPrivilegeObjectCompat_Schema_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{Schema: &GrantOnSchema{SchemaName: `"DB"."SCH"`}}

	for _, priv := range []string{"USAGE", "MONITOR", "CREATE TABLE", "CREATE VIEW", "MODIFY", "ALL"} {
		t.Run(priv, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validatePrivilegeObjectCompat(on, priv))
		})
	}
}

func TestPrivilegeObjectCompat_Schema_Invalid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{Schema: &GrantOnSchema{SchemaName: `"DB"."SCH"`}}
	err := validatePrivilegeObjectCompat(on, "SELECT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for SCHEMA")
}

func TestPrivilegeObjectCompat_BulkTables_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{
		All: &GrantOnBulk{ObjectTypePlural: "TABLES", InDatabase: "DB"},
	}}
	assert.NoError(t, validatePrivilegeObjectCompat(on, "SELECT"))
}

func TestPrivilegeObjectCompat_BulkTables_Invalid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{
		All: &GrantOnBulk{ObjectTypePlural: "TABLES", InDatabase: "DB"},
	}}
	err := validatePrivilegeObjectCompat(on, "CREATE SCHEMA")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for TABLES")
}

// --------------------------------------------------------------------------
// Tests: V-2 — EXTERNAL TABLE, ALERT, DYNAMIC TABLE privilege maps
// --------------------------------------------------------------------------

func TestPrivilegeObjectCompat_ExternalTable_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{
		ObjectType: "EXTERNAL TABLE", ObjectName: `"DB"."SCH"."ET"`,
	}}
	for _, priv := range []string{"SELECT", "REFERENCES", "ALL", "ALL PRIVILEGES", "OWNERSHIP"} {
		t.Run(priv, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validatePrivilegeObjectCompat(on, priv))
		})
	}
}

func TestPrivilegeObjectCompat_ExternalTable_Invalid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{
		ObjectType: "EXTERNAL TABLE", ObjectName: `"DB"."SCH"."ET"`,
	}}
	err := validatePrivilegeObjectCompat(on, "INSERT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for EXTERNAL TABLE")
}

func TestPrivilegeObjectCompat_BulkExternalTables_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{
		All: &GrantOnBulk{ObjectTypePlural: "EXTERNAL TABLES", InDatabase: "DB"},
	}}
	assert.NoError(t, validatePrivilegeObjectCompat(on, "SELECT"))
}

func TestPrivilegeObjectCompat_Alert_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{
		ObjectType: "ALERT", ObjectName: `"DB"."SCH"."A"`,
	}}
	for _, priv := range []string{"MONITOR", "OPERATE", "ALL", "ALL PRIVILEGES", "OWNERSHIP"} {
		t.Run(priv, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validatePrivilegeObjectCompat(on, priv))
		})
	}
}

func TestPrivilegeObjectCompat_Alert_Invalid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{
		ObjectType: "ALERT", ObjectName: `"DB"."SCH"."A"`,
	}}
	err := validatePrivilegeObjectCompat(on, "SELECT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for ALERT")
}

func TestPrivilegeObjectCompat_DynamicTable_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{
		ObjectType: "DYNAMIC TABLE", ObjectName: `"DB"."SCH"."DT"`,
	}}
	for _, priv := range []string{"SELECT", "MONITOR", "OPERATE", "ALL", "ALL PRIVILEGES", "OWNERSHIP"} {
		t.Run(priv, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validatePrivilegeObjectCompat(on, priv))
		})
	}
}

func TestPrivilegeObjectCompat_DynamicTable_Invalid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{
		ObjectType: "DYNAMIC TABLE", ObjectName: `"DB"."SCH"."DT"`,
	}}
	err := validatePrivilegeObjectCompat(on, "INSERT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for DYNAMIC TABLE")
}

func TestPrivilegeObjectCompat_FutureTables_Valid(t *testing.T) {
	t.Parallel()

	on := &GrantOn{SchemaObject: &GrantOnSchemaObject{
		Future: &GrantOnBulk{ObjectTypePlural: "TABLES", InSchema: `"DB"."SCH"`},
	}}
	assert.NoError(t, validatePrivilegeObjectCompat(on, "INSERT"))
}

func TestPrivilegeObjectCompat_UnknownObjectType_Allowed(t *testing.T) {
	t.Parallel()

	// Unknown object types should pass — defer validation to Snowflake.
	on := &GrantOn{AccountObject: &GrantOnAccountObject{ObjectType: "CUSTOM_THING", ObjectName: "X"}}
	assert.NoError(t, validatePrivilegeObjectCompat(on, "WHATEVER"))
}

func TestPrivilegeObjectCompat_EmptyOn_NoError(t *testing.T) {
	t.Parallel()

	// No target set (will fail other validation) — should not panic.
	assert.NoError(t, validatePrivilegeObjectCompat(&GrantOn{}, "SELECT"))
}

func TestJoinPrivileges_Sorted(t *testing.T) {
	t.Parallel()

	privs := map[string]struct{}{
		"ZEBRA":  {},
		"ALPHA":  {},
		"MIDDLE": {},
	}
	result := joinPrivileges(privs)
	assert.Equal(t, "ALPHA, MIDDLE, ZEBRA", result)
}

// ---------------------------------------------------------------------------
// MaskingPolicySpec
// ---------------------------------------------------------------------------

func validMaskingPolicySpec() *MaskingPolicySpec {
	return &MaskingPolicySpec{
		CommonSpec:   validCommonSpec(),
		Name:         "MY_POLICY",
		DatabaseName: ptrStr("DB"),
		SchemaName:   ptrStr("SCH"),
		Signature:    []MaskingPolicyArgument{{Name: "val", Type: "VARCHAR"}},
		Body:         "CASE WHEN true THEN val END",
	}
}

func TestMaskingPolicySpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validMaskingPolicySpec().Validate())
}

func TestMaskingPolicySpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	s := validMaskingPolicySpec()
	s.Name = ""
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestMaskingPolicySpec_Validate_EmptySignature(t *testing.T) {
	t.Parallel()
	s := validMaskingPolicySpec()
	s.Signature = nil
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.signature requires at least one argument")
}

func TestMaskingPolicySpec_Validate_EmptyBody(t *testing.T) {
	t.Parallel()
	s := validMaskingPolicySpec()
	s.Body = ""
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.body is required")
}

func TestMaskingPolicySpec_Validate_NoDatabaseSource(t *testing.T) {
	t.Parallel()
	s := validMaskingPolicySpec()
	s.DatabaseName = nil
	s.DatabaseRef = nil
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "databaseRef")
}

func TestMaskingPolicySpec_Validate_NoSchemaSource(t *testing.T) {
	t.Parallel()
	s := validMaskingPolicySpec()
	s.SchemaName = nil
	s.SchemaRef = nil
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schemaRef")
}

func TestMaskingPolicySpec_Validate_BothDatabaseSources(t *testing.T) {
	t.Parallel()
	s := validMaskingPolicySpec()
	s.DatabaseRef = &LocalObjectReference{Name: "db-cr"}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "databaseRef")
}

func TestMaskingPolicySpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	err := (&MaskingPolicySpec{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
	assert.Contains(t, err.Error(), "spec.signature requires at least one argument")
	assert.Contains(t, err.Error(), "spec.body is required")
}

// ---------------------------------------------------------------------------
// RowAccessPolicySpec
// ---------------------------------------------------------------------------

func validRowAccessPolicySpec() *RowAccessPolicySpec {
	return &RowAccessPolicySpec{
		CommonSpec:   validCommonSpec(),
		Name:         "MY_RAP",
		DatabaseName: ptrStr("DB"),
		SchemaName:   ptrStr("SCH"),
		Signature:    []RowAccessPolicyArgument{{Name: "col", Type: "VARCHAR"}},
		Body:         "true",
	}
}

func TestRowAccessPolicySpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validRowAccessPolicySpec().Validate())
}

func TestRowAccessPolicySpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	s := validRowAccessPolicySpec()
	s.Name = ""
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestRowAccessPolicySpec_Validate_EmptySignature(t *testing.T) {
	t.Parallel()
	s := validRowAccessPolicySpec()
	s.Signature = nil
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.signature requires at least one argument")
}

func TestRowAccessPolicySpec_Validate_EmptyBody(t *testing.T) {
	t.Parallel()
	s := validRowAccessPolicySpec()
	s.Body = ""
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.body is required")
}

func TestRowAccessPolicySpec_Validate_NoDatabaseSource(t *testing.T) {
	t.Parallel()
	s := validRowAccessPolicySpec()
	s.DatabaseName = nil
	s.DatabaseRef = nil
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "databaseRef")
}

func TestRowAccessPolicySpec_Validate_NoSchemaSource(t *testing.T) {
	t.Parallel()
	s := validRowAccessPolicySpec()
	s.SchemaName = nil
	s.SchemaRef = nil
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schemaRef")
}

func TestRowAccessPolicySpec_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	err := (&RowAccessPolicySpec{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
	assert.Contains(t, err.Error(), "spec.signature requires at least one argument")
	assert.Contains(t, err.Error(), "spec.body is required")
}

// ---------------------------------------------------------------------------
// TagSpec
// ---------------------------------------------------------------------------

func validTagSpec() *TagSpec {
	return &TagSpec{
		CommonSpec:   validCommonSpec(),
		Name:         "MY_TAG",
		DatabaseName: ptrStr("DB"),
		SchemaName:   ptrStr("SCH"),
	}
}

func TestTagSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validTagSpec().Validate())
}

func TestTagSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	s := validTagSpec()
	s.Name = ""
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestTagSpec_Validate_NoDatabaseSource(t *testing.T) {
	t.Parallel()
	s := validTagSpec()
	s.DatabaseName = nil
	s.DatabaseRef = nil
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "databaseRef")
}

func TestTagSpec_Validate_NoSchemaSource(t *testing.T) {
	t.Parallel()
	s := validTagSpec()
	s.SchemaName = nil
	s.SchemaRef = nil
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schemaRef")
}

func TestTagSpec_Validate_BothDatabaseSources(t *testing.T) {
	t.Parallel()
	s := validTagSpec()
	s.DatabaseRef = &LocalObjectReference{Name: "db-cr"}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "databaseRef")
}

func TestValidateDatabaseSource_EmptyRefName(t *testing.T) {
	t.Parallel()
	err := validateDatabaseSource(&LocalObjectReference{Name: ""}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of spec.databaseRef or spec.databaseName must be set")
}

func TestValidateSchemaSource_EmptyRefName(t *testing.T) {
	t.Parallel()
	err := validateSchemaSource(&LocalObjectReference{Name: ""}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of spec.schemaRef or spec.schemaName must be set")
}

// ---------------------------------------------------------------------------
// TaskSpec
// ---------------------------------------------------------------------------

func validTaskSpec() *TaskSpec {
	return &TaskSpec{
		CommonSpec:   validCommonSpec(),
		Name:         "MY_TASK",
		DatabaseName: ptrStr("DB"),
		SchemaName:   ptrStr("SCH"),
		SQLStatement: "SELECT 1",
	}
}

func TestTaskSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validTaskSpec().Validate())
}

func TestTaskSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	s := validTaskSpec()
	s.Name = ""
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestTaskSpec_Validate_EmptySQLStatement(t *testing.T) {
	t.Parallel()
	s := validTaskSpec()
	s.SQLStatement = ""
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.sqlStatement is required")
}

func TestTaskSpec_Validate_NoDatabaseSource(t *testing.T) {
	t.Parallel()
	s := validTaskSpec()
	s.DatabaseName = nil
	s.DatabaseRef = nil
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "databaseRef")
}

func TestTaskSpec_Validate_NoSchemaSource(t *testing.T) {
	t.Parallel()
	s := validTaskSpec()
	s.SchemaName = nil
	s.SchemaRef = nil
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schemaRef")
}

func TestTaskSpec_Validate_WarehouseMutualExclusion(t *testing.T) {
	t.Parallel()
	s := validTaskSpec()
	s.WarehouseName = ptrStr("WH")
	size := "SMALL"
	s.UserTaskManagedInitialWarehouseSize = &size
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.warehouseRef/warehouseName and spec.userTaskManagedInitialWarehouseSize are mutually exclusive")
}

func TestTaskSpec_Validate_TimeoutTooLarge(t *testing.T) {
	t.Parallel()
	s := validTaskSpec()
	s.UserTaskTimeoutMs = ptrInt32(700000000)
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.userTaskTimeoutMs must be between 0 and 604800000")
}

func TestTaskSpec_Validate_TimeoutNegative(t *testing.T) {
	t.Parallel()
	s := validTaskSpec()
	s.UserTaskTimeoutMs = ptrInt32(-1)
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.userTaskTimeoutMs must be between 0 and 604800000")
}

func TestTaskSpec_Validate_RetryAttemptsTooLarge(t *testing.T) {
	t.Parallel()
	s := validTaskSpec()
	s.TaskAutoRetryAttempts = ptrInt32(31)
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.taskAutoRetryAttempts must be between 0 and 30")
}

func TestTaskSpec_Validate_RetryAttemptsNegative(t *testing.T) {
	t.Parallel()
	s := validTaskSpec()
	s.TaskAutoRetryAttempts = ptrInt32(-1)
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.taskAutoRetryAttempts must be between 0 and 30")
}

func TestTaskSpec_Validate_ValidTimeoutAndRetry(t *testing.T) {
	t.Parallel()
	s := validTaskSpec()
	s.UserTaskTimeoutMs = ptrInt32(60000)
	s.TaskAutoRetryAttempts = ptrInt32(3)
	assert.NoError(t, s.Validate())
}

// ---------------------------------------------------------------------------
// NetworkPolicySpec
// ---------------------------------------------------------------------------

func TestNetworkPolicySpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&NetworkPolicySpec{CommonSpec: validCommonSpec(), Name: "MY_NP"}).Validate())
}

func TestNetworkPolicySpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	err := (&NetworkPolicySpec{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

// ---------------------------------------------------------------------------
// ResourceMonitorSpec
// ---------------------------------------------------------------------------

func TestResourceMonitorSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	freq := ResourceMonitorFrequencyMonthly
	assert.NoError(t, (&ResourceMonitorSpec{
		CommonSpec:     validCommonSpec(),
		Name:           "MY_MONITOR",
		Frequency:      &freq,
		StartTimestamp: ptrStr("IMMEDIATELY"),
	}).Validate())
}

func TestResourceMonitorSpec_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	err := (&ResourceMonitorSpec{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is required")
}

func TestResourceMonitorSpec_Validate_FrequencyWithoutStartTimestamp(t *testing.T) {
	t.Parallel()
	freq := ResourceMonitorFrequencyDaily
	err := (&ResourceMonitorSpec{
		CommonSpec: validCommonSpec(),
		Name:       "MY_MONITOR",
		Frequency:  &freq,
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.frequency and spec.startTimestamp must both be set or both be omitted")
}

func TestResourceMonitorSpec_Validate_StartTimestampWithoutFrequency(t *testing.T) {
	t.Parallel()
	err := (&ResourceMonitorSpec{
		CommonSpec:     validCommonSpec(),
		Name:           "MY_MONITOR",
		StartTimestamp: ptrStr("2024-01-01"),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.frequency and spec.startTimestamp must both be set or both be omitted")
}

func TestResourceMonitorSpec_Validate_NeitherFrequencyNorStartTimestamp(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&ResourceMonitorSpec{CommonSpec: validCommonSpec(), Name: "MY_MONITOR"}).Validate())
}

// ---------------------------------------------------------------------------
// GrantOwnershipSpec
// ---------------------------------------------------------------------------

func validGrantOwnershipSpec() *GrantOwnershipSpec {
	return &GrantOwnershipSpec{
		CommonSpec:  validCommonSpec(),
		ObjectType:  "TABLE",
		ObjectName:  `"DB"."SCH"."MY_TABLE"`,
		AccountRole: "SYSADMIN",
	}
}

func TestGrantOwnershipSpec_Validate_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validGrantOwnershipSpec().Validate())
}

func TestGrantOwnershipSpec_Validate_EmptyObjectType(t *testing.T) {
	t.Parallel()
	s := validGrantOwnershipSpec()
	s.ObjectType = ""
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.objectType is required")
}

func TestGrantOwnershipSpec_Validate_EmptyObjectName(t *testing.T) {
	t.Parallel()
	s := validGrantOwnershipSpec()
	s.ObjectName = ""
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.objectName is required")
}

func TestGrantOwnershipSpec_Validate_NoRoleSet(t *testing.T) {
	t.Parallel()
	s := validGrantOwnershipSpec()
	s.AccountRole = ""
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of accountRole, accountRoleRef, databaseRole, or databaseRoleRef must be set")
}

func TestGrantOwnershipSpec_Validate_MultipleRolesSet(t *testing.T) {
	t.Parallel()
	s := validGrantOwnershipSpec()
	s.DatabaseRole = "DB_ROLE"
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of accountRole, accountRoleRef, databaseRole, or databaseRoleRef must be set")
}

func TestGrantOwnershipSpec_Validate_AccountRoleRef(t *testing.T) {
	t.Parallel()
	s := validGrantOwnershipSpec()
	s.AccountRole = ""
	s.AccountRoleRef = &LocalObjectReference{Name: "my-role"}
	assert.NoError(t, s.Validate())
}

func TestGrantOwnershipSpec_Validate_DatabaseRoleRef(t *testing.T) {
	t.Parallel()
	s := validGrantOwnershipSpec()
	s.AccountRole = ""
	s.DatabaseRoleRef = &LocalObjectReference{Name: "my-db-role"}
	assert.NoError(t, s.Validate())
}

// TestFieldExportCELListMatchesGoMap verifies that the CEL whitelist in
// FieldExportSpec's kubebuilder marker stays in sync with the Go-level
// ValidFieldExportSourceKinds map. If you add a new resource kind to the
// Go map, this test will fail until you also add it to the CEL rule in
// fieldexport_types.go (and vice versa).
func TestFieldExportCELListMatchesGoMap(t *testing.T) {
	t.Parallel()

	// Read the CEL rule source from fieldexport_types.go.
	// The CEL rule contains: self.from.resource.kind in ['Database','Schema',...]
	// We parse the kinds from the file to avoid a fragile hardcoded duplicate.
	src, err := os.ReadFile("fieldexport_types.go")
	require.NoError(t, err, "reading fieldexport_types.go")

	// Extract the list from the CEL rule:  in ['Kind1','Kind2',...]
	re := regexp.MustCompile(`self\.from\.resource\.kind in \[([^\]]+)\]`)
	matches := re.FindSubmatch(src)
	require.NotNil(t, matches, "could not find CEL kind-in-list rule in fieldexport_types.go")

	// Parse individual kinds from the captured group.
	kindRe := regexp.MustCompile(`'([^']+)'`)
	kindMatches := kindRe.FindAllSubmatch(matches[1], -1)
	celKinds := make(map[string]struct{}, len(kindMatches))
	for _, m := range kindMatches {
		celKinds[string(m[1])] = struct{}{}
	}

	// Compare CEL kinds against Go map.
	for kind := range ValidFieldExportSourceKinds {
		_, ok := celKinds[kind]
		assert.True(t, ok, "kind %q is in ValidFieldExportSourceKinds but missing from the CEL rule in fieldexport_types.go", kind)
	}

	for kind := range celKinds {
		_, ok := ValidFieldExportSourceKinds[kind]
		assert.True(t, ok, "kind %q is in the CEL rule but missing from ValidFieldExportSourceKinds in validation.go", kind)
	}

	// Ensure exact count match as a safety net.
	assert.Equal(t, len(ValidFieldExportSourceKinds), len(celKinds),
		"CEL whitelist has %d kinds but Go map has %d", len(celKinds), len(ValidFieldExportSourceKinds))
}

// ---------------------------------------------------------------------------
// L-15 Sync Tests: Verify CEL and Go validation stay in sync
// ---------------------------------------------------------------------------

// TestCELAndGoMutualExclusivityInSync verifies that every mutual exclusivity
// check in Go Validate() has a corresponding CEL XValidation rule in the types
// file. The CEL rule provides fast admission-time rejection; the Go check
// provides defense-in-depth for programmatic callers and CRD version skew.
//
// If you add a new exactly-one-of check in Go, this test will fail until you
// also add the matching CEL XValidation rule (and vice versa).
//
// Validation policy:
//   - CEL = admission gate (runs on CREATE/UPDATE, instant user feedback)
//   - Go Validate() = defense-in-depth (runs in reconciler, catches API skew)
//   - Go checks that CEL cannot express (email parsing, lookup tables, regex)
//     are Go-only by design and are NOT covered by this sync test.
func TestCELAndGoMutualExclusivityInSync(t *testing.T) {
	t.Parallel()

	// All resources that have databaseRef/databaseName exactly-one-of in Go
	// must have a matching CEL rule in their types file.
	dbRefResources := []string{
		"schema_types.go",
		"databaserole_types.go",
		"table_types.go",
		"view_types.go",
		"alert_types.go",
		"task_types.go",
		"stream_on_table_types.go",
		"stream_on_view_types.go",
		"stream_on_external_table_types.go",
		"stream_on_directory_table_types.go",
		"stream_on_dynamic_table_types.go",
		"tag_types.go",
		"stage_types.go",
		"fileformat_types.go",
		"pipe_types.go",
		"dynamictable_types.go",
		"passwordpolicy_types.go",
		"networkrule_types.go",
		"maskingpolicy_types.go",
		"rowaccesspolicy_types.go",
	}

	for _, typesFile := range dbRefResources {
		t.Run("databaseRef_"+typesFile, func(t *testing.T) {
			assertCELMutualExclusivity(t, typesFile, "databaseRef", "databaseName")
		})
	}

	// All schema-scoped resources with schemaRef/schemaName.
	schemaRefResources := []string{
		"table_types.go",
		"view_types.go",
		"alert_types.go",
		"task_types.go",
		"stream_on_table_types.go",
		"stream_on_view_types.go",
		"stream_on_external_table_types.go",
		"stream_on_directory_table_types.go",
		"stream_on_dynamic_table_types.go",
		"tag_types.go",
		"stage_types.go",
		"fileformat_types.go",
		"pipe_types.go",
		"dynamictable_types.go",
		"passwordpolicy_types.go",
		"networkrule_types.go",
		"maskingpolicy_types.go",
		"rowaccesspolicy_types.go",
	}

	for _, typesFile := range schemaRefResources {
		t.Run("schemaRef_"+typesFile, func(t *testing.T) {
			assertCELMutualExclusivity(t, typesFile, "schemaRef", "schemaName")
		})
	}

	// Grant role/ref mutual exclusivity.
	t.Run("accountRoleGrant_roleRef", func(t *testing.T) {
		assertCELMutualExclusivity(t, "accountrolegrant_types.go", "accountRole", "accountRoleRef")
	})
	t.Run("databaseRoleGrant_roleRef", func(t *testing.T) {
		assertCELMutualExclusivity(t, "databaserolegrant_types.go", "databaseRole", "databaseRoleRef")
	})

	// RoleAssignment mutual exclusivity.
	t.Run("accountRoleAssignment_role", func(t *testing.T) {
		assertCELMutualExclusivity(t, "accountroleassignment_types.go", "roleName", "roleRef")
	})
	t.Run("accountRoleAssignment_target", func(t *testing.T) {
		assertCELMutualExclusivity(t, "accountroleassignment_types.go", "toRole", "toRoleRef", "toUser", "toUserRef")
	})
	t.Run("databaseRoleAssignment_role", func(t *testing.T) {
		assertCELMutualExclusivity(t, "databaseroleassignment_types.go", "databaseRoleName", "databaseRoleRef")
	})
	t.Run("databaseRoleAssignment_target", func(t *testing.T) {
		assertCELMutualExclusivity(t, "databaseroleassignment_types.go", "toRole", "toRoleRef", "toDatabaseRole", "toDatabaseRoleRef")
	})

	// GrantOwnership target mutual exclusivity.
	t.Run("grantOwnership_target", func(t *testing.T) {
		assertCELMutualExclusivity(t, "grantownership_types.go", "accountRole", "accountRoleRef", "databaseRole", "databaseRoleRef")
	})

	// Task warehouse mutual exclusivity.
	t.Run("task_warehouse", func(t *testing.T) {
		assertCELMutualExclusivity(t, "task_types.go", "warehouse", "userTaskManagedInitialWarehouseSize")
	})
}

// assertCELMutualExclusivity verifies that the given types file contains a CEL
// XValidation rule that references ALL specified fields. This covers both
// "exactly one of A or B" and "exactly one of A, B, C, or D" patterns.
func assertCELMutualExclusivity(t *testing.T, typesFile string, fields ...string) {
	t.Helper()

	src, err := os.ReadFile(typesFile)
	require.NoError(t, err, "reading %s", typesFile)

	content := string(src)

	// Look for a single CEL XValidation rule line that mentions ALL fields.
	// This catches patterns like:
	//   (has(self.databaseRef) ? 1 : 0) + (has(self.databaseName) ? 1 : 0) == 1
	//   has(self.databaseRef) != has(self.databaseName)
	//   !(has(self.databaseRef) && has(self.databaseName))
	lines := strings.Split(content, "\n")
	found := false

	for _, line := range lines {
		if !strings.Contains(line, "XValidation") {
			continue
		}

		allPresent := true
		for _, field := range fields {
			if !strings.Contains(line, "self."+field) {
				allPresent = false
				break
			}
		}

		if allPresent {
			found = true
			break
		}
	}

	assert.True(t, found,
		"%s: missing CEL XValidation rule for mutual exclusivity of [%s]. "+
			"Go Validate() has this check — add a matching CEL rule for admission-time rejection.",
		typesFile, strings.Join(fields, ", "))
}
