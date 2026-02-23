package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL builder tests
// --------------------------------------------------------------------------

func TestBuildCreateWarehouseSQL_Minimal(t *testing.T) {
	t.Parallel()

	sql := buildCreateWarehouseSQL(CreateWarehouseOptions{
		Name: NewAccountObjectIdentifier("MY_WH"),
	})
	assert.Equal(t, `CREATE WAREHOUSE IF NOT EXISTS "MY_WH"`, sql)
}

func TestBuildCreateWarehouseSQL_AllOptions(t *testing.T) {
	t.Parallel()

	whType := "STANDARD"
	whSize := "LARGE"
	scalingPolicy := "ECONOMY"
	resourceConstraint := "MEMORY"

	sql := buildCreateWarehouseSQL(CreateWarehouseOptions{
		Name:                            NewAccountObjectIdentifier("ANALYTICS_WH"),
		WarehouseType:                   &whType,
		WarehouseSize:                   &whSize,
		MinClusterCount:                 ptrInt32(1),
		MaxClusterCount:                 ptrInt32(3),
		ScalingPolicy:                   &scalingPolicy,
		AutoSuspend:                     ptrInt32(300),
		AutoResume:                      ptrBool(true),
		InitiallySuspended:              true,
		ResourceMonitor:                 ptrString("my_monitor"),
		Comment:                         ptrString("analytics warehouse"),
		EnableQueryAcceleration:         ptrBool(true),
		QueryAccelerationMaxScaleFactor: ptrInt32(8),
		MaxConcurrencyLevel:             ptrInt32(16),
		StatementQueuedTimeoutInSeconds: ptrInt32(60),
		StatementTimeoutInSeconds:       ptrInt32(3600),
		ResourceConstraint:              &resourceConstraint,
	})

	expected := `CREATE WAREHOUSE IF NOT EXISTS "ANALYTICS_WH"` +
		` WAREHOUSE_TYPE = 'STANDARD'` +
		` WAREHOUSE_SIZE = 'LARGE'` +
		` MIN_CLUSTER_COUNT = 1` +
		` MAX_CLUSTER_COUNT = 3` +
		` SCALING_POLICY = 'ECONOMY'` +
		` AUTO_SUSPEND = 300` +
		` AUTO_RESUME = TRUE` +
		` INITIALLY_SUSPENDED = TRUE` +
		` RESOURCE_MONITOR = 'my_monitor'` +
		` COMMENT = 'analytics warehouse'` +
		` ENABLE_QUERY_ACCELERATION = TRUE` +
		` QUERY_ACCELERATION_MAX_SCALE_FACTOR = 8` +
		` MAX_CONCURRENCY_LEVEL = 16` +
		` STATEMENT_QUEUED_TIMEOUT_IN_SECONDS = 60` +
		` STATEMENT_TIMEOUT_IN_SECONDS = 3600` +
		` RESOURCE_CONSTRAINT = 'MEMORY'`
	assert.Equal(t, expected, sql)
}

func TestBuildCreateWarehouseSQL_NotInitiallySuspended(t *testing.T) {
	t.Parallel()

	sql := buildCreateWarehouseSQL(CreateWarehouseOptions{
		Name:               NewAccountObjectIdentifier("WH"),
		InitiallySuspended: false,
	})
	assert.NotContains(t, sql, "INITIALLY_SUSPENDED")
}

func TestBuildCreateWarehouseSQL_CreateOrAlter(t *testing.T) {
	t.Parallel()

	sql := buildCreateWarehouseSQL(CreateWarehouseOptions{
		Name:             NewAccountObjectIdentifier("WH"),
		Comment:          ptrString("managed"),
		UseCreateOrAlter: true,
	})
	assert.Equal(t, `CREATE OR ALTER WAREHOUSE "WH" COMMENT = 'managed'`, sql)
}

func TestBuildAlterWarehouseStatements_SetOnly(t *testing.T) {
	t.Parallel()

	size := "XLARGE"
	stmts, err := buildAlterWarehouseStatements(AlterWarehouseOptions{
		Name:          NewAccountObjectIdentifier("MY_WH"),
		WarehouseSize: &size,
		AutoSuspend:   ptrInt32(600),
	})
	require.NoError(t, err)

	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER WAREHOUSE "MY_WH" SET WAREHOUSE_SIZE = 'XLARGE' AUTO_SUSPEND = 600`, stmts[0])
}

func TestBuildAlterWarehouseStatements_WarehouseTypeChange(t *testing.T) {
	t.Parallel()

	whType := "SNOWPARK-OPTIMIZED"
	stmts, err := buildAlterWarehouseStatements(AlterWarehouseOptions{
		Name:          NewAccountObjectIdentifier("MY_WH"),
		WarehouseType: &whType,
	})
	require.NoError(t, err)

	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER WAREHOUSE "MY_WH" SET WAREHOUSE_TYPE = 'SNOWPARK-OPTIMIZED'`, stmts[0])
}

func TestBuildAlterWarehouseStatements_UnsetOnly(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterWarehouseStatements(AlterWarehouseOptions{
		Name:        NewAccountObjectIdentifier("MY_WH"),
		UnsetFields: []string{"COMMENT", "AUTO_SUSPEND"},
	})
	require.NoError(t, err)

	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER WAREHOUSE "MY_WH" UNSET COMMENT, AUTO_SUSPEND`, stmts[0])
}

func TestBuildAlterWarehouseStatements_SetAndUnset(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterWarehouseStatements(AlterWarehouseOptions{
		Name:        NewAccountObjectIdentifier("MY_WH"),
		Comment:     ptrString("new comment"),
		UnsetFields: []string{"AUTO_SUSPEND"},
	})
	require.NoError(t, err)

	require.Len(t, stmts, 2)
	assert.Equal(t, `ALTER WAREHOUSE "MY_WH" SET COMMENT = 'new comment'`, stmts[0])
	assert.Equal(t, `ALTER WAREHOUSE "MY_WH" UNSET AUTO_SUSPEND`, stmts[1])
}

func TestBuildAlterWarehouseStatements_Empty(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterWarehouseStatements(AlterWarehouseOptions{
		Name: NewAccountObjectIdentifier("MY_WH"),
	})
	require.NoError(t, err)

	assert.Empty(t, stmts)
}

func TestBuildAlterWarehouseStatements_AllSetOptions(t *testing.T) {
	t.Parallel()

	whType := "SNOWPARK-OPTIMIZED"
	size := "MEDIUM"
	scalingPolicy := "STANDARD"
	constraint := "MEMORY"

	stmts, err := buildAlterWarehouseStatements(AlterWarehouseOptions{
		Name:                            NewAccountObjectIdentifier("WH"),
		WarehouseType:                   &whType,
		WarehouseSize:                   &size,
		MinClusterCount:                 ptrInt32(2),
		MaxClusterCount:                 ptrInt32(5),
		ScalingPolicy:                   &scalingPolicy,
		AutoSuspend:                     ptrInt32(120),
		AutoResume:                      ptrBool(false),
		ResourceMonitor:                 ptrString("rm1"),
		Comment:                         ptrString("test"),
		EnableQueryAcceleration:         ptrBool(true),
		QueryAccelerationMaxScaleFactor: ptrInt32(10),
		MaxConcurrencyLevel:             ptrInt32(4),
		StatementQueuedTimeoutInSeconds: ptrInt32(30),
		StatementTimeoutInSeconds:       ptrInt32(1800),
		ResourceConstraint:              &constraint,
	})
	require.NoError(t, err)

	require.Len(t, stmts, 1)
	expected := `ALTER WAREHOUSE "WH" SET` +
		` WAREHOUSE_TYPE = 'SNOWPARK-OPTIMIZED'` +
		` WAREHOUSE_SIZE = 'MEDIUM'` +
		` MIN_CLUSTER_COUNT = 2` +
		` MAX_CLUSTER_COUNT = 5` +
		` SCALING_POLICY = 'STANDARD'` +
		` AUTO_SUSPEND = 120` +
		` AUTO_RESUME = FALSE` +
		` RESOURCE_MONITOR = 'rm1'` +
		` COMMENT = 'test'` +
		` ENABLE_QUERY_ACCELERATION = TRUE` +
		` QUERY_ACCELERATION_MAX_SCALE_FACTOR = 10` +
		` MAX_CONCURRENCY_LEVEL = 4` +
		` STATEMENT_QUEUED_TIMEOUT_IN_SECONDS = 30` +
		` STATEMENT_TIMEOUT_IN_SECONDS = 1800` +
		` RESOURCE_CONSTRAINT = 'MEMORY'`
	assert.Equal(t, expected, stmts[0])
}

func TestBuildDropWarehouseSQL(t *testing.T) {
	t.Parallel()

	sql := buildDropWarehouseSQL(NewAccountObjectIdentifier("MY_WH"))
	assert.Equal(t, `DROP WAREHOUSE IF EXISTS "MY_WH"`, sql)
}

func TestBuildShowWarehouseByIDSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowWarehouseByIDSQL(NewAccountObjectIdentifier("MYWH"))
	assert.Equal(t, `SHOW WAREHOUSES LIKE 'MYWH'`, sql)
}

func TestBuildShowWarehouseByIDSQL_EscapesPattern(t *testing.T) {
	t.Parallel()

	sql := buildShowWarehouseByIDSQL(NewAccountObjectIdentifier("MY%WH"))
	assert.Equal(t, `SHOW WAREHOUSES LIKE 'MY\%WH'`, sql)
}

func TestBuildShowWarehouseByIDSQL_EscapesUnderscore(t *testing.T) {
	t.Parallel()

	sql := buildShowWarehouseByIDSQL(NewAccountObjectIdentifier("MY_WH"))
	assert.Equal(t, `SHOW WAREHOUSES LIKE 'MY\_WH'`, sql)
}

func TestBuildShowWarehouseParametersSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowWarehouseParametersSQL(NewAccountObjectIdentifier("MY_WH"))
	assert.Equal(t, `SHOW PARAMETERS IN WAREHOUSE "MY_WH"`, sql)
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateWarehouseOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	err := (&CreateWarehouseOptions{
		Name: NewAccountObjectIdentifier(""),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "warehouse name is required")
}

func TestCreateWarehouseOptions_Validate_InvalidQAMaxScaleFactor(t *testing.T) {
	t.Parallel()

	err := (&CreateWarehouseOptions{
		Name:                            NewAccountObjectIdentifier("WH"),
		QueryAccelerationMaxScaleFactor: ptrInt32(101),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "queryAccelerationMaxScaleFactor must be between 0 and 100")
}

func TestCreateWarehouseOptions_Validate_MinGTMax(t *testing.T) {
	t.Parallel()

	err := (&CreateWarehouseOptions{
		Name:            NewAccountObjectIdentifier("WH"),
		MinClusterCount: ptrInt32(5),
		MaxClusterCount: ptrInt32(3),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "minClusterCount (5) must be <= maxClusterCount (3)")
}

func TestCreateWarehouseOptions_Validate_Valid(t *testing.T) {
	t.Parallel()

	err := (&CreateWarehouseOptions{
		Name:            NewAccountObjectIdentifier("WH"),
		MinClusterCount: ptrInt32(1),
		MaxClusterCount: ptrInt32(3),
	}).Validate()
	require.NoError(t, err)
}

func TestAlterWarehouseOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	err := (&AlterWarehouseOptions{
		Name: NewAccountObjectIdentifier(""),
	}).Validate()
	require.Error(t, err)
}

func TestAlterWarehouseOptions_Validate_MinGTMax(t *testing.T) {
	t.Parallel()

	minCluster := int32(5)
	maxCluster := int32(2)
	err := (&AlterWarehouseOptions{
		Name:            NewAccountObjectIdentifier("WH"),
		MinClusterCount: &minCluster,
		MaxClusterCount: &maxCluster,
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "minClusterCount")
}

func TestAlterWarehouseOptions_Validate_InvalidQAMaxScaleFactor(t *testing.T) {
	t.Parallel()

	bad := int32(200)
	err := (&AlterWarehouseOptions{
		Name:                            NewAccountObjectIdentifier("WH"),
		QueryAccelerationMaxScaleFactor: &bad,
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "queryAccelerationMaxScaleFactor")
}

func TestAlterWarehouseOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("no changes", func(t *testing.T) {
		t.Parallel()
		assert.False(t, (&AlterWarehouseOptions{Name: NewAccountObjectIdentifier("WH")}).HasChanges())
	})

	t.Run("with size", func(t *testing.T) {
		t.Parallel()
		size := "LARGE"
		assert.True(t, (&AlterWarehouseOptions{Name: NewAccountObjectIdentifier("WH"), WarehouseSize: &size}).HasChanges())
	})

	t.Run("with type", func(t *testing.T) {
		t.Parallel()
		whType := "SNOWPARK-OPTIMIZED"
		assert.True(t, (&AlterWarehouseOptions{Name: NewAccountObjectIdentifier("WH"), WarehouseType: &whType}).HasChanges())
	})

	t.Run("with unset", func(t *testing.T) {
		t.Parallel()
		assert.True(t, (&AlterWarehouseOptions{Name: NewAccountObjectIdentifier("WH"), UnsetFields: []string{"COMMENT"}}).HasChanges())
	})
}

// --------------------------------------------------------------------------
// Helper function tests
// --------------------------------------------------------------------------

func TestSqlString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", sqlString(nil))
	assert.Equal(t, "hello", sqlString("hello"))
	assert.Equal(t, "bytes", sqlString([]byte("bytes")))
	assert.Equal(t, "42", sqlString(42))
}

func ptrString(s string) *string { return &s }
func ptrInt32(i int32) *int32    { return &i }
func ptrBool(b bool) *bool       { return &b }
