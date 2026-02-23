//go:build integration

package snowflake_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

func TestGrant_Lifecycle(t *testing.T) {
	// Create a test role and a test database to grant on.
	roleClient := snowflake.NewAccountRoleClient(sfClient)
	roleName := snowflake.NewAccountObjectIdentifier(uniqueName("GRANT_ROLE"))

	require.NoError(t, roleClient.Create(testCtx, snowflake.CreateAccountRoleOptions{
		Name: roleName,
	}))
	t.Cleanup(func() { _ = roleClient.Drop(testCtx, roleName) })

	dbClient := snowflake.NewDatabaseClient(sfClient)
	dbName := snowflake.NewAccountObjectIdentifier(uniqueName("GRANT_DB"))

	require.NoError(t, dbClient.Create(testCtx, snowflake.CreateDatabaseOptions{Name: dbName}))
	t.Cleanup(func() { _ = dbClient.Drop(testCtx, dbName) })

	grantClient := snowflake.NewGrantClient(sfClient)

	onParams := snowflake.OnClauseParams{
		AccountObjectType: "DATABASE",
		AccountObjectName: dbName.Name(),
	}
	onClause := snowflake.BuildOnClause(onParams)
	toClause := snowflake.BuildToClause(roleName.Name(), "", "")
	fromClause := snowflake.BuildFromClause(roleName.Name(), "", "")
	showTarget, _ := snowflake.BuildShowGrantsTarget(onParams, "")

	grantID := snowflake.GrantIdentifier{
		Privilege:        "USAGE",
		OnClause:         onClause,
		ToClause:         toClause,
		GranteeName:      roleName.Name(),
		ShowGrantsTarget: showTarget,
	}

	// --- Grant ---
	err := grantClient.Grant(testCtx, snowflake.CreateGrantOptions{
		Privilege: "USAGE",
		OnClause:  onClause,
		ToClause:  toClause,
	})
	require.NoError(t, err, "GRANT USAGE ON DATABASE")

	// --- Observe ---
	obs, err := grantClient.Observe(testCtx, grantID)
	require.NoError(t, err)
	require.True(t, obs.Exists, "grant should be observable")
	assert.Equal(t, "USAGE", obs.ShowOutput.Privilege)

	// --- Revoke ---
	err = grantClient.Revoke(testCtx, snowflake.RevokeGrantOptions{
		Privilege:  "USAGE",
		OnClause:   onClause,
		FromClause: fromClause,
	})
	require.NoError(t, err, "REVOKE USAGE ON DATABASE")

	obs, err = grantClient.Observe(testCtx, grantID)
	require.NoError(t, err)
	assert.False(t, obs.Exists, "grant should not exist after REVOKE")
}
