//go:build integration

package snowflake_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

func TestDatabase_Lifecycle(t *testing.T) {
	dbClient := snowflake.NewDatabaseClient(sfClient)
	dbName := snowflake.NewAccountObjectIdentifier(uniqueName("DB"))

	t.Cleanup(func() {
		_ = dbClient.Drop(testCtx, dbName)
	})

	// --- Create ---
	err := dbClient.Create(testCtx, snowflake.CreateDatabaseOptions{
		Name:    dbName,
		Comment: ptrString("snowplane integration test"),
	})
	require.NoError(t, err, "CREATE DATABASE")

	// --- Observe ---
	obs, err := dbClient.Observe(testCtx, dbName)
	require.NoError(t, err)
	require.True(t, obs.Exists, "database should exist after CREATE")
	assert.Equal(t, dbName.Name(), obs.ShowOutput.Name)
	assert.Equal(t, "snowplane integration test", obs.ShowOutput.Comment)

	// --- Alter ---
	err = dbClient.Alter(testCtx, snowflake.AlterDatabaseOptions{
		Name:    dbName,
		Comment: ptrString("updated comment"),
	})
	require.NoError(t, err, "ALTER DATABASE")

	obs, err = dbClient.Observe(testCtx, dbName)
	require.NoError(t, err)
	assert.Equal(t, "updated comment", obs.ShowOutput.Comment)

	// --- Drop ---
	err = dbClient.Drop(testCtx, dbName)
	require.NoError(t, err, "DROP DATABASE")

	obs, err = dbClient.Observe(testCtx, dbName)
	require.NoError(t, err)
	assert.False(t, obs.Exists, "database should not exist after DROP")
}

func TestSchema_Lifecycle(t *testing.T) {
	// Create a parent database first.
	dbClient := snowflake.NewDatabaseClient(sfClient)
	dbName := snowflake.NewAccountObjectIdentifier(uniqueName("SCHEMA_DB"))

	require.NoError(t, dbClient.Create(testCtx, snowflake.CreateDatabaseOptions{Name: dbName}))
	t.Cleanup(func() { _ = dbClient.Drop(testCtx, dbName) })

	schemaClient := snowflake.NewSchemaClient(sfClient)
	schemaName := snowflake.NewDatabaseObjectIdentifier(dbName.Name(), uniqueName("SCH"))

	// --- Create ---
	err := schemaClient.Create(testCtx, snowflake.CreateSchemaOptions{
		Name:    schemaName,
		Comment: ptrString("test schema"),
	})
	require.NoError(t, err, "CREATE SCHEMA")

	// --- Observe ---
	obs, err := schemaClient.Observe(testCtx, schemaName)
	require.NoError(t, err)
	require.True(t, obs.Exists)
	assert.Equal(t, schemaName.Name(), obs.ShowOutput.Name)
	assert.Equal(t, "test schema", obs.ShowOutput.Comment)

	// --- Alter ---
	err = schemaClient.Alter(testCtx, snowflake.AlterSchemaOptions{
		Name:    schemaName,
		Comment: ptrString("altered schema"),
	})
	require.NoError(t, err, "ALTER SCHEMA")

	obs, err = schemaClient.Observe(testCtx, schemaName)
	require.NoError(t, err)
	assert.Equal(t, "altered schema", obs.ShowOutput.Comment)

	// --- Drop ---
	err = schemaClient.Drop(testCtx, schemaName)
	require.NoError(t, err, "DROP SCHEMA")

	obs, err = schemaClient.Observe(testCtx, schemaName)
	require.NoError(t, err)
	assert.False(t, obs.Exists)
}

func TestTable_Lifecycle(t *testing.T) {
	// Setup parent database (uses PUBLIC schema).
	dbClient := snowflake.NewDatabaseClient(sfClient)
	dbName := snowflake.NewAccountObjectIdentifier(uniqueName("TBL_DB"))

	require.NoError(t, dbClient.Create(testCtx, snowflake.CreateDatabaseOptions{Name: dbName}))
	t.Cleanup(func() { _ = dbClient.Drop(testCtx, dbName) })

	schemaClient := snowflake.NewSchemaClient(sfClient)
	schemaName := snowflake.NewDatabaseObjectIdentifier(dbName.Name(), "PUBLIC")

	tableClient := snowflake.NewTableClient(sfClient)
	tblName := snowflake.NewSchemaObjectIdentifier(dbName.Name(), "PUBLIC", uniqueName("TBL"))

	// --- Create ---
	err := tableClient.Create(testCtx, snowflake.CreateTableOptions{
		Name: tblName,
		Columns: []snowflake.CreateTableColumn{
			{Name: "ID", Type: "NUMBER(38,0)", Nullable: ptrBool(false)},
			{Name: "NAME", Type: "VARCHAR(100)"},
		},
		Comment: ptrString("test table"),
	})
	require.NoError(t, err, "CREATE TABLE")

	// --- Observe ---
	obs, err := tableClient.Observe(testCtx, tblName)
	require.NoError(t, err)
	require.True(t, obs.Exists)
	assert.Equal(t, tblName.Name(), obs.ShowOutput.Name)
	assert.Equal(t, "test table", obs.ShowOutput.Comment)

	// --- Alter ---
	err = tableClient.Alter(testCtx, snowflake.AlterTableOptions{
		Name:    tblName,
		Comment: ptrString("altered table"),
	})
	require.NoError(t, err, "ALTER TABLE")

	obs, err = tableClient.Observe(testCtx, tblName)
	require.NoError(t, err)
	assert.Equal(t, "altered table", obs.ShowOutput.Comment)

	// --- Drop ---
	err = tableClient.Drop(testCtx, tblName)
	require.NoError(t, err, "DROP TABLE")

	obs, err = tableClient.Observe(testCtx, tblName)
	require.NoError(t, err)
	assert.False(t, obs.Exists)

	// Also verify schema observe works (schema came for free with the database).
	schemaObs, err := schemaClient.Observe(testCtx, schemaName)
	require.NoError(t, err)
	assert.True(t, schemaObs.Exists, "PUBLIC schema should exist")
}

func TestView_Lifecycle(t *testing.T) {
	// Setup parent database (uses PUBLIC schema).
	dbClient := snowflake.NewDatabaseClient(sfClient)
	dbName := snowflake.NewAccountObjectIdentifier(uniqueName("VIEW_DB"))

	require.NoError(t, dbClient.Create(testCtx, snowflake.CreateDatabaseOptions{Name: dbName}))
	t.Cleanup(func() { _ = dbClient.Drop(testCtx, dbName) })

	viewClient := snowflake.NewViewClient(sfClient)
	viewName := snowflake.NewSchemaObjectIdentifier(dbName.Name(), "PUBLIC", uniqueName("VW"))

	// --- Create ---
	err := viewClient.Create(testCtx, snowflake.CreateViewOptions{
		Name:      viewName,
		Statement: "SELECT 1 AS VAL",
		Comment:   ptrString("test view"),
	})
	require.NoError(t, err, "CREATE VIEW")

	// --- Observe ---
	obs, err := viewClient.Observe(testCtx, viewName)
	require.NoError(t, err)
	require.True(t, obs.Exists)
	assert.Equal(t, viewName.Name(), obs.ShowOutput.Name)
	assert.Equal(t, "test view", obs.ShowOutput.Comment)
	assert.False(t, obs.ShowOutput.IsSecure)

	// --- Alter (set secure) ---
	err = viewClient.Alter(testCtx, snowflake.AlterViewOptions{
		Name:   viewName,
		Secure: ptrBool(true),
	})
	require.NoError(t, err, "ALTER VIEW SET SECURE")

	obs, err = viewClient.Observe(testCtx, viewName)
	require.NoError(t, err)
	assert.True(t, obs.ShowOutput.IsSecure, "view should be secure after ALTER")

	// --- Drop ---
	err = viewClient.Drop(testCtx, viewName)
	require.NoError(t, err, "DROP VIEW")

	obs, err = viewClient.Observe(testCtx, viewName)
	require.NoError(t, err)
	assert.False(t, obs.Exists)
}

func TestStage_InternalLifecycle(t *testing.T) {
	// Setup parent database (uses PUBLIC schema).
	dbClient := snowflake.NewDatabaseClient(sfClient)
	dbName := snowflake.NewAccountObjectIdentifier(uniqueName("STG_DB"))

	require.NoError(t, dbClient.Create(testCtx, snowflake.CreateDatabaseOptions{Name: dbName}))
	t.Cleanup(func() { _ = dbClient.Drop(testCtx, dbName) })

	stageClient := snowflake.NewStageClient(sfClient)
	stageName := snowflake.NewSchemaObjectIdentifier(dbName.Name(), "PUBLIC", uniqueName("STG"))

	// --- Create (internal stage) ---
	err := stageClient.Create(testCtx, snowflake.CreateStageOptions{
		Name:    stageName,
		Comment: ptrString("test internal stage"),
	})
	require.NoError(t, err, "CREATE STAGE")

	// --- Observe ---
	obs, err := stageClient.Observe(testCtx, stageName)
	require.NoError(t, err)
	require.True(t, obs.Exists)
	assert.Equal(t, stageName.Name(), obs.ShowOutput.Name)
	assert.Equal(t, "INTERNAL", obs.ShowOutput.Type)

	// --- Drop ---
	err = stageClient.Drop(testCtx, stageName)
	require.NoError(t, err, "DROP STAGE")

	obs, err = stageClient.Observe(testCtx, stageName)
	require.NoError(t, err)
	assert.False(t, obs.Exists)
}
