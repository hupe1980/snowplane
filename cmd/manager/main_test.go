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

// --------------------------------------------------------------------------
// Tests: validControllerNames completeness (L-19)
// --------------------------------------------------------------------------

// controllerRegistrationNames returns the set of controller names that appear
// in the registration table (controllers slice) plus the standalone
// "fieldexport" controller.  This duplicates the names from main() so that the
// test below can catch any drift between the two lists.
//
// Keep this list in sync with the controllers slice in main().
var controllerRegistrationNames = map[string]bool{
	"alert":                   true,
	"database":                true,
	"schema":                  true,
	"warehouse":               true,
	"accountrole":             true,
	"databaserole":            true,
	"accountrolegrant":        true,
	"databaserolegrant":       true,
	"sharegrant":              true,
	"user":                    true,
	"table":                   true,
	"view":                    true,
	"stage":                   true,
	"task":                    true,
	"streamontable":           true,
	"streamonview":            true,
	"streamonexternaltable":   true,
	"streamondirectorytable":  true,
	"streamondynamictable":    true,
	"tag":                     true,
	"networkpolicy":           true,
	"resourcemonitor":         true,
	"maskingpolicy":           true,
	"rowaccesspolicy":         true,
	"grantownership":          true,
	"storageintegration":      true,
	"fileformat":              true,
	"pipe":                    true,
	"dynamictable":            true,
	"notificationintegration": true,
	"securityintegration":     true,
	"passwordpolicy":          true,
	"networkrule":             true,
	"accountroleassignment":   true,
	"databaseroleassignment":  true,
	// standalone controller (not in GenericReconciler registration loop)
	"fieldexport": true,
}

func TestValidControllerNames_MatchesRegistrationTable(t *testing.T) {
	t.Parallel()

	// Every name that can be disabled must appear in the registration table.
	for name := range validControllerNames {
		assert.True(t, controllerRegistrationNames[name],
			"validControllerNames contains %q which is not in the controller registration table", name)
	}

	// Every registered controller must be disableable.
	for name := range controllerRegistrationNames {
		assert.True(t, validControllerNames[name],
			"controller %q is registered but missing from validControllerNames", name)
	}

	// Counts must match (belt-and-suspenders).
	assert.Equal(t, len(validControllerNames), len(controllerRegistrationNames),
		"validControllerNames and controllerRegistrationNames have different lengths")
}
