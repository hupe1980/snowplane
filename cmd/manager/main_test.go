package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testControllerNames is a representative set of valid controller names for tests.
var testControllerNames = []string{
	"alert", "authenticationpolicy", "database", "schema", "warehouse",
	"accountrole", "databaserole", "grantprivilegestoaccountrole", "grantprivilegestodatabaserole",
	"grantprivilegestoshare", "user", "table", "view", "stage", "task", "fieldexport",
}

func TestParseDisabledControllers_Empty(t *testing.T) {
	t.Parallel()
	result := parseDisabledControllers("")
	assert.Empty(t, result)
}

func TestParseDisabledControllers_Single(t *testing.T) {
	t.Parallel()
	result := parseDisabledControllers("grantprivilegestoaccountrole")
	assert.True(t, result["grantprivilegestoaccountrole"])
	assert.False(t, result["database"])
}

func TestParseDisabledControllers_Multiple(t *testing.T) {
	t.Parallel()
	result := parseDisabledControllers("grantprivilegestoaccountrole,stage,view")
	assert.True(t, result["grantprivilegestoaccountrole"])
	assert.True(t, result["stage"])
	assert.True(t, result["view"])
	assert.False(t, result["database"])
}

func TestParseDisabledControllers_WithSpaces(t *testing.T) {
	t.Parallel()
	result := parseDisabledControllers(" grantprivilegestoaccountrole , stage , view ")
	assert.True(t, result["grantprivilegestoaccountrole"])
	assert.True(t, result["stage"])
	assert.True(t, result["view"])
}

func TestParseDisabledControllers_TrailingComma(t *testing.T) {
	t.Parallel()
	result := parseDisabledControllers("grantprivilegestoaccountrole,")
	assert.True(t, result["grantprivilegestoaccountrole"])
	assert.Equal(t, 1, len(result))
}

func TestValidateDisabledControllers_InvalidName(t *testing.T) {
	t.Parallel()
	disabled := parseDisabledControllers("grantprivilegestoaccountrole,foobar")
	err := validateDisabledControllers(disabled, testControllerNames)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown controller name \"foobar\"")
}

func TestValidateDisabledControllers_AllValid(t *testing.T) {
	t.Parallel()
	disabled := parseDisabledControllers("database,schema,warehouse,accountrole,databaserole,grantprivilegestoaccountrole,grantprivilegestodatabaserole,grantprivilegestoshare,user,table,view,stage,fieldexport")
	err := validateDisabledControllers(disabled, testControllerNames)
	require.NoError(t, err)
	assert.Equal(t, 13, len(disabled))
}

// --------------------------------------------------------------------------
// Tests: parseAllowedRoles (M-4)
// --------------------------------------------------------------------------

func TestParseAllowedRoles_Empty(t *testing.T) {
	t.Parallel()
	result := parseAllowedRoles("")
	assert.Nil(t, result, "empty string should return nil (all roles allowed)")
}

func TestParseAllowedRoles_Single(t *testing.T) {
	t.Parallel()
	result := parseAllowedRoles("SYSADMIN")
	require.NotNil(t, result)
	assert.True(t, result["SYSADMIN"])
	assert.False(t, result["ACCOUNTADMIN"])
}

func TestParseAllowedRoles_Multiple(t *testing.T) {
	t.Parallel()
	result := parseAllowedRoles("SYSADMIN,USERADMIN,DATA_ENGINEER")
	require.NotNil(t, result)
	assert.True(t, result["SYSADMIN"])
	assert.True(t, result["USERADMIN"])
	assert.True(t, result["DATA_ENGINEER"])
	assert.False(t, result["ACCOUNTADMIN"])
}

func TestParseAllowedRoles_NormalizesToUppercase(t *testing.T) {
	t.Parallel()
	result := parseAllowedRoles("sysadmin,Useradmin,data_ENGINEER")
	require.NotNil(t, result)
	assert.True(t, result["SYSADMIN"])
	assert.True(t, result["USERADMIN"])
	assert.True(t, result["DATA_ENGINEER"])
}

func TestParseAllowedRoles_TrimsSpaces(t *testing.T) {
	t.Parallel()
	result := parseAllowedRoles(" SYSADMIN , USERADMIN , DATA_ENGINEER ")
	require.NotNil(t, result)
	assert.True(t, result["SYSADMIN"])
	assert.True(t, result["USERADMIN"])
	assert.True(t, result["DATA_ENGINEER"])
}

func TestParseAllowedRoles_TrailingComma(t *testing.T) {
	t.Parallel()
	result := parseAllowedRoles("SYSADMIN,")
	require.NotNil(t, result)
	assert.True(t, result["SYSADMIN"])
	assert.Equal(t, 1, len(result))
}

func TestParseAllowedRoles_OnlyCommasAndSpaces(t *testing.T) {
	t.Parallel()
	result := parseAllowedRoles(", , ,")
	assert.Nil(t, result, "only whitespace/commas should return nil")
}

// --------------------------------------------------------------------------
// Tests: validateDisabledControllers — completeness is now guaranteed by
// design since valid names are derived from the controllers registration
// table rather than a manually maintained map (M-5).
// --------------------------------------------------------------------------
