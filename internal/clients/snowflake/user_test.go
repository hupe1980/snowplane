package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateUserSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicUser", func(t *testing.T) {
		t.Parallel()
		opts := CreateUserOptions{
			Name: NewAccountObjectIdentifier("ALICE"),
		}
		got, err := buildCreateUserSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE USER IF NOT EXISTS "ALICE"`, got)
	})

	t.Run("WithPassword", func(t *testing.T) {
		t.Parallel()
		pwd := "s3cret"
		opts := CreateUserOptions{
			Name:     NewAccountObjectIdentifier("ALICE"),
			Password: &pwd,
		}
		got, err := buildCreateUserSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE USER IF NOT EXISTS "ALICE" PASSWORD = 's3cret'`, got)
	})

	t.Run("AllFields", func(t *testing.T) {
		t.Parallel()
		pwd := "pass"
		login := "alice_login"
		display := "Alice Smith"
		email := "alice@example.com"
		first := "Alice"
		last := "Smith"
		comment := "Test user"
		rsaKey := "MIIBIjAN..."
		rsaKey2 := "MIIBIjBB..."
		userType := "SERVICE"
		defRole := "DATA_READER"
		defSecRoles := "ALL"
		defWH := "COMPUTE_WH"
		defNS := "MYDB.PUBLIC"
		mustChange := true
		disabled := false

		opts := CreateUserOptions{
			Name:                  NewAccountObjectIdentifier("ALICE"),
			Password:              &pwd,
			LoginName:             &login,
			DisplayName:           &display,
			Email:                 &email,
			FirstName:             &first,
			LastName:              &last,
			Comment:               &comment,
			RSAPublicKey:          &rsaKey,
			RSAPublicKey2:         &rsaKey2,
			Type:                  &userType,
			DefaultRole:           &defRole,
			DefaultSecondaryRoles: &defSecRoles,
			DefaultWarehouse:      &defWH,
			DefaultNamespace:      &defNS,
			MustChangePassword:    &mustChange,
			Disabled:              &disabled,
		}
		got, err := buildCreateUserSQL(opts)
		require.NoError(t, err)

		expected := `CREATE USER IF NOT EXISTS "ALICE"` +
			` PASSWORD = 'pass'` +
			` LOGIN_NAME = 'alice_login'` +
			` DISPLAY_NAME = 'Alice Smith'` +
			` EMAIL = 'alice@example.com'` +
			` FIRST_NAME = 'Alice'` +
			` LAST_NAME = 'Smith'` +
			` COMMENT = 'Test user'` +
			` RSA_PUBLIC_KEY = 'MIIBIjAN...'` +
			` RSA_PUBLIC_KEY_2 = 'MIIBIjBB...'` +
			` TYPE = SERVICE` +
			` DEFAULT_ROLE = 'DATA_READER'` +
			` DEFAULT_SECONDARY_ROLES = ('ALL')` +
			` DEFAULT_WAREHOUSE = 'COMPUTE_WH'` +
			` DEFAULT_NAMESPACE = 'MYDB.PUBLIC'` +
			` MUST_CHANGE_PASSWORD = TRUE` +
			` DISABLED = FALSE`
		assert.Equal(t, expected, got)
	})

	t.Run("PasswordWithQuotes", func(t *testing.T) {
		t.Parallel()
		pwd := "it's secret"
		opts := CreateUserOptions{
			Name:     NewAccountObjectIdentifier("BOB"),
			Password: &pwd,
		}
		got, err := buildCreateUserSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE USER IF NOT EXISTS "BOB" PASSWORD = 'it''s secret'`, got)
	})

	t.Run("DefaultSecondaryRolesNonAll", func(t *testing.T) {
		t.Parallel()
		secRoles := "READER"
		opts := CreateUserOptions{
			Name:                  NewAccountObjectIdentifier("BOB"),
			DefaultSecondaryRoles: &secRoles,
		}
		got, err := buildCreateUserSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE USER IF NOT EXISTS "BOB" DEFAULT_SECONDARY_ROLES = ('READER')`, got)
	})
}

func TestBuildAlterUserStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterUserOptions{
			Name: NewAccountObjectIdentifier("ALICE"),
		}
		stmts, err := buildAlterUserStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetEmail", func(t *testing.T) {
		t.Parallel()
		email := "alice@new.com"
		opts := AlterUserOptions{
			Name:  NewAccountObjectIdentifier("ALICE"),
			Email: &email,
		}
		stmts, err := buildAlterUserStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Equal(t, `ALTER USER "ALICE" SET EMAIL = 'alice@new.com'`, stmts[0])
	})

	t.Run("SetMultipleFields", func(t *testing.T) {
		t.Parallel()
		email := "alice@new.com"
		comment := "updated"
		disabled := true
		opts := AlterUserOptions{
			Name:     NewAccountObjectIdentifier("ALICE"),
			Email:    &email,
			Comment:  &comment,
			Disabled: &disabled,
		}
		stmts, err := buildAlterUserStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Equal(t, `ALTER USER "ALICE" SET EMAIL = 'alice@new.com' COMMENT = 'updated' DISABLED = TRUE`, stmts[0])
	})

	t.Run("UnsetOnly", func(t *testing.T) {
		t.Parallel()
		opts := AlterUserOptions{
			Name:        NewAccountObjectIdentifier("ALICE"),
			UnsetFields: []string{"COMMENT", "EMAIL"},
		}
		stmts, err := buildAlterUserStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Equal(t, `ALTER USER "ALICE" UNSET COMMENT, EMAIL`, stmts[0])
	})

	t.Run("SetAndUnset", func(t *testing.T) {
		t.Parallel()
		email := "new@example.com"
		opts := AlterUserOptions{
			Name:        NewAccountObjectIdentifier("ALICE"),
			Email:       &email,
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterUserStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 2)
		assert.Equal(t, `ALTER USER "ALICE" SET EMAIL = 'new@example.com'`, stmts[0])
		assert.Equal(t, `ALTER USER "ALICE" UNSET COMMENT`, stmts[1])
	})

	t.Run("SetPassword", func(t *testing.T) {
		t.Parallel()
		pwd := "newpass"
		opts := AlterUserOptions{
			Name:     NewAccountObjectIdentifier("ALICE"),
			Password: &pwd,
		}
		stmts, err := buildAlterUserStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Equal(t, `ALTER USER "ALICE" SET PASSWORD = 'newpass'`, stmts[0])
	})

	t.Run("SetDefaultSecondaryRolesAll", func(t *testing.T) {
		t.Parallel()
		secRoles := "ALL"
		opts := AlterUserOptions{
			Name:                  NewAccountObjectIdentifier("ALICE"),
			DefaultSecondaryRoles: &secRoles,
		}
		stmts, err := buildAlterUserStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Equal(t, `ALTER USER "ALICE" SET DEFAULT_SECONDARY_ROLES = ('ALL')`, stmts[0])
	})
}

func TestBuildDropUserSQL(t *testing.T) {
	t.Parallel()

	got := buildDropUserSQL(NewAccountObjectIdentifier("OLD_USER"))
	assert.Equal(t, `DROP USER IF EXISTS "OLD_USER"`, got)
}

func TestBuildShowUserByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowUserByIDSQL(NewAccountObjectIdentifier("MY_USER"))
	assert.Equal(t, `SHOW USERS LIKE 'MY\_USER'`, got)
}

func TestBuildDescribeUserSQL(t *testing.T) {
	t.Parallel()

	got := buildDescribeUserSQL(NewAccountObjectIdentifier("MY_USER"))
	assert.Equal(t, `DESCRIBE USER "MY_USER"`, got)
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateUserOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateUserOptions{Name: NewAccountObjectIdentifier("ALICE")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateUserOptions{}
		assert.Error(t, opts.Validate())
	})

	t.Run("InvalidType", func(t *testing.T) {
		t.Parallel()
		badType := "ROBOT"
		opts := CreateUserOptions{
			Name: NewAccountObjectIdentifier("ALICE"),
			Type: &badType,
		}
		assert.Error(t, opts.Validate())
		assert.Contains(t, opts.Validate().Error(), "ROBOT")
	})

	t.Run("ValidServiceType", func(t *testing.T) {
		t.Parallel()
		svcType := "SERVICE"
		opts := CreateUserOptions{
			Name: NewAccountObjectIdentifier("SVC_USER"),
			Type: &svcType,
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("ValidLegacyServiceType", func(t *testing.T) {
		t.Parallel()
		legType := "LEGACY_SERVICE"
		opts := CreateUserOptions{
			Name: NewAccountObjectIdentifier("SVC_USER"),
			Type: &legType,
		}
		assert.NoError(t, opts.Validate())
	})
}

func TestAlterUserOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("ALICE")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterUserOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterUserOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("U")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("LoginNameSet", func(t *testing.T) {
		t.Parallel()
		v := "x"
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("U"), LoginName: &v}
		assert.True(t, opts.HasChanges())
	})

	t.Run("EmailSet", func(t *testing.T) {
		t.Parallel()
		v := "a@b.com"
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("U"), Email: &v}
		assert.True(t, opts.HasChanges())
	})

	t.Run("PasswordSet", func(t *testing.T) {
		t.Parallel()
		v := "secret"
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("U"), Password: &v}
		assert.True(t, opts.HasChanges())
	})

	t.Run("DisabledSet", func(t *testing.T) {
		t.Parallel()
		v := true
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("U"), Disabled: &v}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFieldsSet", func(t *testing.T) {
		t.Parallel()
		opts := AlterUserOptions{
			Name:        NewAccountObjectIdentifier("U"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("RSAPublicKeySet", func(t *testing.T) {
		t.Parallel()
		v := "MII..."
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("U"), RSAPublicKey: &v}
		assert.True(t, opts.HasChanges())
	})

	t.Run("RSAPublicKey2Set", func(t *testing.T) {
		t.Parallel()
		v := "MII..."
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("U"), RSAPublicKey2: &v}
		assert.True(t, opts.HasChanges())
	})

	t.Run("DefaultSecondaryRolesSet", func(t *testing.T) {
		t.Parallel()
		v := "ALL"
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("U"), DefaultSecondaryRoles: &v}
		assert.True(t, opts.HasChanges())
	})

	t.Run("DefaultWarehouseSet", func(t *testing.T) {
		t.Parallel()
		v := "WH1"
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("U"), DefaultWarehouse: &v}
		assert.True(t, opts.HasChanges())
	})

	t.Run("DefaultNamespaceSet", func(t *testing.T) {
		t.Parallel()
		v := "DB.SCHEMA"
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("U"), DefaultNamespace: &v}
		assert.True(t, opts.HasChanges())
	})

	t.Run("MustChangePasswordSet", func(t *testing.T) {
		t.Parallel()
		v := true
		opts := AlterUserOptions{Name: NewAccountObjectIdentifier("U"), MustChangePassword: &v}
		assert.True(t, opts.HasChanges())
	})
}

func TestValidateUserType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"PERSON", "PERSON", false},
		{"SERVICE", "SERVICE", false},
		{"LEGACY_SERVICE", "LEGACY_SERVICE", false},
		{"Invalid", "ADMIN", true},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateUserType(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
