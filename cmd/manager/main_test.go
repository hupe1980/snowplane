package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDisabledControllers_Empty(t *testing.T) {
	t.Parallel()
	result, err := parseDisabledControllers("")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestParseDisabledControllers_Single(t *testing.T) {
	t.Parallel()
	result, err := parseDisabledControllers("accountrolegrant")
	require.NoError(t, err)
	assert.True(t, result["accountrolegrant"])
	assert.False(t, result["database"])
}

func TestParseDisabledControllers_Multiple(t *testing.T) {
	t.Parallel()
	result, err := parseDisabledControllers("accountrolegrant,stage,view")
	require.NoError(t, err)
	assert.True(t, result["accountrolegrant"])
	assert.True(t, result["stage"])
	assert.True(t, result["view"])
	assert.False(t, result["database"])
}

func TestParseDisabledControllers_WithSpaces(t *testing.T) {
	t.Parallel()
	result, err := parseDisabledControllers(" accountrolegrant , stage , view ")
	require.NoError(t, err)
	assert.True(t, result["accountrolegrant"])
	assert.True(t, result["stage"])
	assert.True(t, result["view"])
}

func TestParseDisabledControllers_TrailingComma(t *testing.T) {
	t.Parallel()
	result, err := parseDisabledControllers("accountrolegrant,")
	require.NoError(t, err)
	assert.True(t, result["accountrolegrant"])
	assert.Equal(t, 1, len(result))
}

func TestParseDisabledControllers_InvalidName(t *testing.T) {
	t.Parallel()
	_, err := parseDisabledControllers("accountrolegrant,foobar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown controller name \"foobar\"")
}

func TestParseDisabledControllers_AllValid(t *testing.T) {
	t.Parallel()
	result, err := parseDisabledControllers("database,schema,warehouse,accountrole,databaserole,accountrolegrant,databaserolegrant,sharegrant,user,table,view,stage,fieldexport")
	require.NoError(t, err)
	assert.Equal(t, 13, len(result))
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
