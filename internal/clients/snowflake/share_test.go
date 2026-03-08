package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// Tests: validateAccountIdentifier
// --------------------------------------------------------------------------

func TestValidateAccountIdentifier_Valid(t *testing.T) {
	t.Parallel()

	valid := []string{
		"MYORG.MYACCOUNT",
		"my_org.my_account",
		"ORG-NAME.ACCT-NAME",
		"org123.acct456",
		"SIMPLE",
	}

	for _, v := range valid {
		assert.NoError(t, validateAccountIdentifier(v), "expected %q to be valid", v)
	}
}

func TestValidateAccountIdentifier_Invalid(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"",
		"ORG;DROP TABLE",
		"ORG'INJECT",
		"ORG ACCOUNT",
		"ORG\nACCOUNT",
		"ORG=ACCOUNT",
		`ORG"ACCOUNT`,
	}

	for _, v := range invalid {
		assert.Error(t, validateAccountIdentifier(v), "expected %q to be invalid", v)
	}
}

// --------------------------------------------------------------------------
// Tests: CreateShareOptions.Validate
// --------------------------------------------------------------------------

func TestCreateShareOptions_Validate_Valid(t *testing.T) {
	t.Parallel()

	opts := CreateShareOptions{
		Name: NewAccountObjectIdentifier("MY_SHARE"),
	}
	assert.NoError(t, opts.Validate())
}

func TestCreateShareOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	opts := CreateShareOptions{
		Name: NewAccountObjectIdentifier(""),
	}
	assert.Error(t, opts.Validate())
}

// --------------------------------------------------------------------------
// Tests: AlterShareOptions.Validate
// --------------------------------------------------------------------------

func TestAlterShareOptions_Validate_Valid(t *testing.T) {
	t.Parallel()

	opts := AlterShareOptions{
		Name: NewAccountObjectIdentifier("MY_SHARE"),
	}
	assert.NoError(t, opts.Validate())
}

func TestAlterShareOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	opts := AlterShareOptions{
		Name: NewAccountObjectIdentifier(""),
	}
	assert.Error(t, opts.Validate())
}

// --------------------------------------------------------------------------
// Tests: AlterShareOptions.HasChanges
// --------------------------------------------------------------------------

func TestAlterShareOptions_HasChanges_None(t *testing.T) {
	t.Parallel()

	opts := AlterShareOptions{Name: NewAccountObjectIdentifier("S")}
	assert.False(t, opts.HasChanges())
}

func TestAlterShareOptions_HasChanges_Comment(t *testing.T) {
	t.Parallel()

	c := "hello"
	opts := AlterShareOptions{Name: NewAccountObjectIdentifier("S"), Comment: &c}
	assert.True(t, opts.HasChanges())
}

func TestAlterShareOptions_HasChanges_AddAccounts(t *testing.T) {
	t.Parallel()

	opts := AlterShareOptions{Name: NewAccountObjectIdentifier("S"), AddAccounts: []string{"ORG.ACCT"}}
	assert.True(t, opts.HasChanges())
}

// --------------------------------------------------------------------------
// Tests: buildShowShareByIDSQL
// --------------------------------------------------------------------------

func TestBuildShowShareByIDSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowShareByIDSQL(NewAccountObjectIdentifier("MY_SHARE"))
	assert.Contains(t, sql, "SHOW SHARES")
	assert.Contains(t, sql, "LIKE")
}

// --------------------------------------------------------------------------
// Tests: ShareClient.Create (validation only, no DB)
// --------------------------------------------------------------------------

func TestShareClient_Create_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewShareClient(nil) // nil executor — validation fires before SQL
	err := client.Create(t.Context(), CreateShareOptions{
		Name: NewAccountObjectIdentifier(""),
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

// --------------------------------------------------------------------------
// Tests: ShareClient.Drop (validation only, no DB)
// --------------------------------------------------------------------------

func TestShareClient_Drop_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewShareClient(nil)
	err := client.Drop(t.Context(), NewAccountObjectIdentifier(""))
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

// --------------------------------------------------------------------------
// Tests: ShareClient.ShowByID (validation only, no DB)
// --------------------------------------------------------------------------

func TestShareClient_ShowByID_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewShareClient(nil)
	_, err := client.ShowByID(t.Context(), NewAccountObjectIdentifier(""))
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

// --------------------------------------------------------------------------
// Tests: ShareClient.Alter (validation only, no DB)
// --------------------------------------------------------------------------

func TestShareClient_Alter_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewShareClient(nil)
	err := client.Alter(t.Context(), AlterShareOptions{
		Name: NewAccountObjectIdentifier(""),
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestShareClient_Alter_InvalidAccountIdentifier(t *testing.T) {
	t.Parallel()

	client := NewShareClient(nil)
	err := client.Alter(t.Context(), AlterShareOptions{
		Name:        NewAccountObjectIdentifier("MY_SHARE"),
		AddAccounts: []string{"ORG;DROP TABLE"},
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}
