//go:build integration

package snowflake_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

func TestWarehouse_Lifecycle(t *testing.T) {
	whClient := snowflake.NewWarehouseClient(sfClient)
	whName := snowflake.NewAccountObjectIdentifier(uniqueName("WH"))

	t.Cleanup(func() {
		_ = whClient.Drop(testCtx, whName)
	})

	// --- Create ---
	err := whClient.Create(testCtx, snowflake.CreateWarehouseOptions{
		Name:               whName,
		WarehouseSize:      ptrString("X-Small"),
		AutoSuspend:        ptrInt32(60),
		AutoResume:         ptrBool(true),
		InitiallySuspended: true,
		Comment:            ptrString("snowplane integration test"),
	})
	require.NoError(t, err, "CREATE WAREHOUSE")

	// --- Observe ---
	obs, err := whClient.Observe(testCtx, whName)
	require.NoError(t, err)
	require.True(t, obs.Exists)
	assert.Equal(t, whName.Name(), obs.ShowOutput.Name)
	assert.Equal(t, "snowplane integration test", obs.ShowOutput.Comment)
	assert.Equal(t, "X-Small", obs.ShowOutput.Size)

	// --- Alter ---
	err = whClient.Alter(testCtx, snowflake.AlterWarehouseOptions{
		Name:    whName,
		Comment: ptrString("updated warehouse"),
	})
	require.NoError(t, err, "ALTER WAREHOUSE")

	obs, err = whClient.Observe(testCtx, whName)
	require.NoError(t, err)
	assert.Equal(t, "updated warehouse", obs.ShowOutput.Comment)

	// --- Drop ---
	err = whClient.Drop(testCtx, whName)
	require.NoError(t, err, "DROP WAREHOUSE")

	obs, err = whClient.Observe(testCtx, whName)
	require.NoError(t, err)
	assert.False(t, obs.Exists)
}

func TestUser_Lifecycle(t *testing.T) {
	userClient := snowflake.NewUserClient(sfClient)
	userName := snowflake.NewAccountObjectIdentifier(uniqueName("USR"))

	t.Cleanup(func() {
		_ = userClient.Drop(testCtx, userName)
	})

	// --- Create ---
	err := userClient.Create(testCtx, snowflake.CreateUserOptions{
		Name:    userName,
		Comment: ptrString("snowplane integration test user"),
		Type:    ptrString("PERSON"),
	})
	require.NoError(t, err, "CREATE USER")

	// --- Observe ---
	obs, err := userClient.Observe(testCtx, userName)
	require.NoError(t, err)
	require.True(t, obs.Exists)
	assert.Equal(t, userName.Name(), obs.ShowOutput.Name)
	assert.Equal(t, "snowplane integration test user", obs.ShowOutput.Comment)

	// --- Alter ---
	err = userClient.Alter(testCtx, snowflake.AlterUserOptions{
		Name:    userName,
		Comment: ptrString("altered user"),
	})
	require.NoError(t, err, "ALTER USER")

	obs, err = userClient.Observe(testCtx, userName)
	require.NoError(t, err)
	assert.Equal(t, "altered user", obs.ShowOutput.Comment)

	// --- Drop ---
	err = userClient.Drop(testCtx, userName)
	require.NoError(t, err, "DROP USER")

	obs, err = userClient.Observe(testCtx, userName)
	require.NoError(t, err)
	assert.False(t, obs.Exists)
}

func TestAccountRole_Lifecycle(t *testing.T) {
	roleClient := snowflake.NewAccountRoleClient(sfClient)
	roleName := snowflake.NewAccountObjectIdentifier(uniqueName("ROLE"))

	t.Cleanup(func() {
		_ = roleClient.Drop(testCtx, roleName)
	})

	// --- Create ---
	err := roleClient.Create(testCtx, snowflake.CreateAccountRoleOptions{
		Name:    roleName,
		Comment: ptrString("snowplane integration test role"),
	})
	require.NoError(t, err, "CREATE ROLE")

	// --- Observe ---
	obs, err := roleClient.Observe(testCtx, roleName)
	require.NoError(t, err)
	require.True(t, obs.Exists)
	assert.Equal(t, roleName.Name(), obs.ShowOutput.Name)
	assert.Equal(t, "snowplane integration test role", obs.ShowOutput.Comment)

	// --- Alter ---
	err = roleClient.Alter(testCtx, snowflake.AlterAccountRoleOptions{
		Name:    roleName,
		Comment: ptrString("altered role"),
	})
	require.NoError(t, err, "ALTER ROLE")

	obs, err = roleClient.Observe(testCtx, roleName)
	require.NoError(t, err)
	assert.Equal(t, "altered role", obs.ShowOutput.Comment)

	// --- Drop ---
	err = roleClient.Drop(testCtx, roleName)
	require.NoError(t, err, "DROP ROLE")

	obs, err = roleClient.Observe(testCtx, roleName)
	require.NoError(t, err)
	assert.False(t, obs.Exists)
}

func TestDatabaseRole_Lifecycle(t *testing.T) {
	// Create parent database.
	dbClient := snowflake.NewDatabaseClient(sfClient)
	dbName := snowflake.NewAccountObjectIdentifier(uniqueName("DBROLE_DB"))

	require.NoError(t, dbClient.Create(testCtx, snowflake.CreateDatabaseOptions{Name: dbName}))
	t.Cleanup(func() { _ = dbClient.Drop(testCtx, dbName) })

	roleClient := snowflake.NewDatabaseRoleClient(sfClient)
	roleName := snowflake.NewDatabaseObjectIdentifier(dbName.Name(), uniqueName("DBROLE"))

	// --- Create ---
	err := roleClient.Create(testCtx, snowflake.CreateDatabaseRoleOptions{
		Name:    roleName,
		Comment: ptrString("test database role"),
	})
	require.NoError(t, err, "CREATE DATABASE ROLE")

	// --- Observe ---
	obs, err := roleClient.Observe(testCtx, roleName)
	require.NoError(t, err)
	require.True(t, obs.Exists)
	assert.Equal(t, roleName.Name(), obs.ShowOutput.Name)
	assert.Contains(t, []string{dbName.Name(), ""}, obs.ShowOutput.DatabaseName, "database name should match or be empty")
	assert.Equal(t, "test database role", obs.ShowOutput.Comment)

	// --- Alter ---
	err = roleClient.Alter(testCtx, snowflake.AlterDatabaseRoleOptions{
		Name:    roleName,
		Comment: ptrString("altered database role"),
	})
	require.NoError(t, err, "ALTER DATABASE ROLE")

	obs, err = roleClient.Observe(testCtx, roleName)
	require.NoError(t, err)
	assert.Equal(t, "altered database role", obs.ShowOutput.Comment)

	// --- Drop ---
	err = roleClient.Drop(testCtx, roleName)
	require.NoError(t, err, "DROP DATABASE ROLE")

	obs, err = roleClient.Observe(testCtx, roleName)
	require.NoError(t, err)
	assert.False(t, obs.Exists)
}
