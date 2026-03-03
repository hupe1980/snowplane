package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests — CREATE
// --------------------------------------------------------------------------

func TestBuildCreateSecretSQL(t *testing.T) {
	t.Parallel()

	t.Run("OAuth2ClientCredentials", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecretOptions{
			Name:              NewSchemaObjectIdentifier("DB", "SCH", "MY_SECRET"),
			SecretType:        SecretTypeOAuth2,
			APIAuthentication: "MY_AUTH",
			OAuthScopes:       []string{"session:scope:read", "session:scope:write"},
			Comment:           ptr("test secret"),
		}

		got, err := buildCreateSecretSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE SECRET IF NOT EXISTS "DB"."SCH"."MY_SECRET"`)
		assert.Contains(t, got, "TYPE = OAUTH2")
		assert.Contains(t, got, `API_AUTHENTICATION = "MY_AUTH"`)
		assert.Contains(t, got, "OAUTH_SCOPES")
		assert.Contains(t, got, "'session:scope:read'")
		assert.Contains(t, got, "'session:scope:write'")
		assert.Contains(t, got, "COMMENT = 'test secret'")
	})

	t.Run("OAuth2AuthorizationCodeGrant", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecretOptions{
			Name:                        NewSchemaObjectIdentifier("DB", "SCH", "ACG_SECRET"),
			SecretType:                  SecretTypeOAuth2,
			APIAuthentication:           "MY_AUTH",
			OAuthRefreshToken:           "abc123",
			OAuthRefreshTokenExpiryTime: "2025-01-06 20:00:00",
		}

		got, err := buildCreateSecretSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE SECRET IF NOT EXISTS "DB"."SCH"."ACG_SECRET"`)
		assert.Contains(t, got, "TYPE = OAUTH2")
		assert.Contains(t, got, "OAUTH_REFRESH_TOKEN = 'abc123'")
		assert.Contains(t, got, "OAUTH_REFRESH_TOKEN_EXPIRY_TIME = '2025-01-06 20:00:00'")
	})

	t.Run("Password", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecretOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "BA_SECRET"),
			SecretType: SecretTypePassword,
			Username:   "myuser",
			Password:   "mypass",
		}

		got, err := buildCreateSecretSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE SECRET IF NOT EXISTS "DB"."SCH"."BA_SECRET"`)
		assert.Contains(t, got, "TYPE = PASSWORD")
		assert.Contains(t, got, "USERNAME = 'myuser'")
		assert.Contains(t, got, "PASSWORD = 'mypass'")
	})

	t.Run("GenericString", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecretOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "GS_SECRET"),
			SecretType:   SecretTypeGenericString,
			SecretString: "my-api-token",
		}

		got, err := buildCreateSecretSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE SECRET IF NOT EXISTS "DB"."SCH"."GS_SECRET"`)
		assert.Contains(t, got, "TYPE = GENERIC_STRING")
		assert.Contains(t, got, "SECRET_STRING = 'my-api-token'")
	})
}

// --------------------------------------------------------------------------
// SQL generation tests — ALTER
// --------------------------------------------------------------------------

func TestBuildAlterSecretStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecretOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "MY_SECRET"),
			Comment: ptr("updated comment"),
		}

		stmts, err := buildAlterSecretStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `ALTER SECRET "DB"."SCH"."MY_SECRET" SET`)
		assert.Contains(t, stmts[0], "COMMENT = 'updated comment'")
	})

	t.Run("SetOAuthScopes", func(t *testing.T) {
		t.Parallel()
		scopes := []string{"read", "write"}
		opts := AlterSecretOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "MY_SECRET"),
			SecretType:  SecretTypeOAuth2,
			OAuthScopes: &scopes,
		}

		stmts, err := buildAlterSecretStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `ALTER SECRET "DB"."SCH"."MY_SECRET" SET OAUTH_SCOPES`)
		assert.Contains(t, stmts[0], "'read'")
		assert.Contains(t, stmts[0], "'write'")
	})

	t.Run("SetOAuthRefreshToken", func(t *testing.T) {
		t.Parallel()
		token := "new-token-123"
		opts := AlterSecretOptions{
			Name:              NewSchemaObjectIdentifier("DB", "SCH", "ACG_SECRET"),
			SecretType:        SecretTypeOAuth2,
			OAuthRefreshToken: &token,
		}

		stmts, err := buildAlterSecretStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "OAUTH_REFRESH_TOKEN = 'new-token-123'")
	})

	t.Run("SetOAuthRefreshTokenExpiryTime", func(t *testing.T) {
		t.Parallel()
		expiry := "2026-06-01 00:00:00"
		opts := AlterSecretOptions{
			Name:                        NewSchemaObjectIdentifier("DB", "SCH", "ACG_SECRET"),
			SecretType:                  SecretTypeOAuth2,
			OAuthRefreshTokenExpiryTime: &expiry,
		}

		stmts, err := buildAlterSecretStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "OAUTH_REFRESH_TOKEN_EXPIRY_TIME = '2026-06-01 00:00:00'")
	})

	t.Run("SetUsernamePassword", func(t *testing.T) {
		t.Parallel()
		user := "newuser"
		pass := "newpass"
		opts := AlterSecretOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "BA_SECRET"),
			SecretType: SecretTypePassword,
			Username:   &user,
			Password:   &pass,
		}

		stmts, err := buildAlterSecretStatements(opts)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(stmts), 1)

		combined := ""
		for _, s := range stmts {
			combined += s + " "
		}

		assert.Contains(t, combined, "USERNAME = 'newuser'")
		assert.Contains(t, combined, "PASSWORD = 'newpass'")
	})

	t.Run("SetSecretString", func(t *testing.T) {
		t.Parallel()
		ss := "new-api-token"
		opts := AlterSecretOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "GS_SECRET"),
			SecretType:   SecretTypeGenericString,
			SecretString: &ss,
		}

		stmts, err := buildAlterSecretStatements(opts)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(stmts), 1)

		combined := ""
		for _, s := range stmts {
			combined += s + " "
		}

		assert.Contains(t, combined, "SECRET_STRING = 'new-api-token'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecretOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "MY_SECRET"),
			UnsetFields: []string{"COMMENT"},
		}

		stmts, err := buildAlterSecretStatements(opts)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(stmts), 1)

		combined := ""
		for _, s := range stmts {
			combined += s + " "
		}

		assert.Contains(t, combined, "UNSET")
		assert.Contains(t, combined, "COMMENT")
	})

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecretOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_SECRET"),
		}

		stmts, err := buildAlterSecretStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})
}

// --------------------------------------------------------------------------
// SQL generation tests — SHOW
// --------------------------------------------------------------------------

func TestBuildShowSecretByIDSQL(t *testing.T) {
	t.Parallel()

	id := NewSchemaObjectIdentifier("DB", "SCH", "MY_SECRET")
	got := buildShowSecretByIDSQL(id)
	assert.Contains(t, got, `SHOW SECRETS LIKE 'MY\_SECRET' IN SCHEMA "DB"."SCH"`)
}

// --------------------------------------------------------------------------
// Validate
// --------------------------------------------------------------------------

func TestCreateSecretOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("ValidOAuth2", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecretOptions{
			Name:              NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SecretType:        SecretTypeOAuth2,
			APIAuthentication: "AUTH",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecretOptions{
			SecretType: SecretTypeOAuth2,
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingType", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecretOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "S"),
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("OAuth2MissingAPIAuth", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecretOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SecretType: SecretTypeOAuth2,
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("PasswordMissingUsername", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecretOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SecretType: SecretTypePassword,
			Password:   "pass",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("PasswordMissingPassword", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecretOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SecretType: SecretTypePassword,
			Username:   "user",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("GenericStringMissingSecretString", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecretOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SecretType: SecretTypeGenericString,
		}
		assert.Error(t, opts.Validate())
	})
}

// --------------------------------------------------------------------------
// HasChanges
// --------------------------------------------------------------------------

func TestAlterSecretOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecretOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "S"),
			Comment: ptr("updated"),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecretOptions{
			Comment: ptr("updated"),
		}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterSecretOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecretOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "S"),
		}
		assert.False(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecretOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "S"),
			Comment: ptr("hi"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecretOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "S"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
