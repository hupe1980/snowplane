package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateAPIIntegrationSQL(t *testing.T) {
	t.Parallel()

	t.Run("AWSBasic", func(t *testing.T) {
		t.Parallel()

		roleARN := "arn:aws:iam::123456789012:role/my-role"

		opts := CreateAPIIntegrationOptions{
			Name:               NewAccountObjectIdentifier("my_api"),
			APIProvider:        "aws_api_gateway",
			Enabled:            ptr(true),
			APIAllowedPrefixes: []string{"https://api.example.com/v1/"},
			APIAWSRoleARN:      &roleARN,
		}

		sql, err := buildCreateAPIIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `CREATE API INTEGRATION IF NOT EXISTS "my_api"`)
		assert.Contains(t, sql, `API_PROVIDER = aws_api_gateway`)
		assert.Contains(t, sql, `API_AWS_ROLE_ARN = '`+roleARN+`'`)
		assert.Contains(t, sql, `API_ALLOWED_PREFIXES = ('https://api.example.com/v1/')`)
		assert.Contains(t, sql, `ENABLED = TRUE`)
	})

	t.Run("AzureWithAllOptions", func(t *testing.T) {
		t.Parallel()

		tenantID := "my-tenant-id"
		appID := "my-app-id"
		apiKey := "my-api-key"
		comment := "integration for tests"

		opts := CreateAPIIntegrationOptions{
			Name:               NewAccountObjectIdentifier("azure_api"),
			APIProvider:        "azure_api_management",
			Enabled:            ptr(true),
			APIAllowedPrefixes: []string{"https://azure.example.com/v1/", "https://azure.example.com/v2/"},
			APIBlockedPrefixes: []string{"https://azure.example.com/v1/admin"},
			AzureTenantID:      &tenantID,
			AzureADAppID:       &appID,
			APIKey:             &apiKey,
			Comment:            &comment,
		}

		sql, err := buildCreateAPIIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `CREATE API INTEGRATION IF NOT EXISTS "azure_api"`)
		assert.Contains(t, sql, `API_PROVIDER = azure_api_management`)
		assert.Contains(t, sql, `AZURE_TENANT_ID = '`+tenantID+`'`)
		assert.Contains(t, sql, `AZURE_AD_APPLICATION_ID = '`+appID+`'`)
		assert.Contains(t, sql, `API_ALLOWED_PREFIXES = ('https://azure.example.com/v1/', 'https://azure.example.com/v2/')`)
		assert.Contains(t, sql, `API_BLOCKED_PREFIXES = ('https://azure.example.com/v1/admin')`)
		assert.Contains(t, sql, `API_KEY = '`+apiKey+`'`)
		assert.Contains(t, sql, `COMMENT = '`+comment+`'`)
	})

	t.Run("GoogleBasic", func(t *testing.T) {
		t.Parallel()

		audience := "my-project-id"

		opts := CreateAPIIntegrationOptions{
			Name:               NewAccountObjectIdentifier("google_api"),
			APIProvider:        "google_api_gateway",
			Enabled:            ptr(true),
			APIAllowedPrefixes: []string{"https://gateway.example.com/"},
			GoogleAudience:     &audience,
		}

		sql, err := buildCreateAPIIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `API_PROVIDER = google_api_gateway`)
		assert.Contains(t, sql, `GOOGLE_AUDIENCE = '`+audience+`'`)
	})

	t.Run("GitHTTPS", func(t *testing.T) {
		t.Parallel()

		opts := CreateAPIIntegrationOptions{
			Name:               NewAccountObjectIdentifier("git_api"),
			APIProvider:        "git_https_api",
			Enabled:            ptr(true),
			APIAllowedPrefixes: []string{"https://github.com/my-org/"},
		}

		sql, err := buildCreateAPIIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `API_PROVIDER = git_https_api`)
		assert.Contains(t, sql, `API_ALLOWED_PREFIXES = ('https://github.com/my-org/')`)
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			opts    CreateAPIIntegrationOptions
			wantErr string
		}{
			{
				name:    "EmptyName",
				opts:    CreateAPIIntegrationOptions{APIProvider: "aws_api_gateway", APIAllowedPrefixes: []string{"https://x"}, APIAWSRoleARN: ptr("arn")},
				wantErr: "name is required",
			},
			{
				name:    "EmptyProvider",
				opts:    CreateAPIIntegrationOptions{Name: NewAccountObjectIdentifier("x"), APIAllowedPrefixes: []string{"https://x"}},
				wantErr: "api_provider is required",
			},
			{
				name:    "InvalidProvider",
				opts:    CreateAPIIntegrationOptions{Name: NewAccountObjectIdentifier("x"), APIProvider: "invalid", APIAllowedPrefixes: []string{"https://x"}},
				wantErr: "invalid api_provider",
			},
			{
				name:    "EmptyPrefixes",
				opts:    CreateAPIIntegrationOptions{Name: NewAccountObjectIdentifier("x"), APIProvider: "aws_api_gateway", APIAWSRoleARN: ptr("arn")},
				wantErr: "api_allowed_prefixes is required",
			},
			{
				name:    "AWSMissingRoleARN",
				opts:    CreateAPIIntegrationOptions{Name: NewAccountObjectIdentifier("x"), APIProvider: "aws_api_gateway", APIAllowedPrefixes: []string{"https://x"}},
				wantErr: "api_aws_role_arn is required",
			},
			{
				name:    "AzureMissingTenantID",
				opts:    CreateAPIIntegrationOptions{Name: NewAccountObjectIdentifier("x"), APIProvider: "azure_api_management", APIAllowedPrefixes: []string{"https://x"}, AzureADAppID: ptr("app")},
				wantErr: "azure_tenant_id is required",
			},
			{
				name:    "AzureMissingAppID",
				opts:    CreateAPIIntegrationOptions{Name: NewAccountObjectIdentifier("x"), APIProvider: "azure_api_management", APIAllowedPrefixes: []string{"https://x"}, AzureTenantID: ptr("tid")},
				wantErr: "azure_ad_application_id is required",
			},
			{
				name:    "GoogleMissingAudience",
				opts:    CreateAPIIntegrationOptions{Name: NewAccountObjectIdentifier("x"), APIProvider: "google_api_gateway", APIAllowedPrefixes: []string{"https://x"}},
				wantErr: "google_audience is required",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				_, err := buildCreateAPIIntegrationSQL(tt.opts)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			})
		}
	})
}

func TestBuildAlterAPIIntegrationStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		comment := "new comment"
		opts := AlterAPIIntegrationOptions{
			Name:    NewAccountObjectIdentifier("my_api"),
			Comment: &comment,
		}

		stmts, err := buildAlterAPIIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `ALTER API INTEGRATION "my_api" SET`)
		assert.Contains(t, stmts[0], `COMMENT = '`+comment+`'`)
	})

	t.Run("SetAllowedPrefixes", func(t *testing.T) {
		t.Parallel()

		prefixes := []string{"https://api.example.com/v1/", "https://api.example.com/v2/"}
		opts := AlterAPIIntegrationOptions{
			Name:               NewAccountObjectIdentifier("my_api"),
			APIAllowedPrefixes: &prefixes,
		}

		stmts, err := buildAlterAPIIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `API_ALLOWED_PREFIXES = ('https://api.example.com/v1/', 'https://api.example.com/v2/')`)
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterAPIIntegrationOptions{
			Name:        NewAccountObjectIdentifier("my_api"),
			UnsetFields: []string{"COMMENT"},
		}

		stmts, err := buildAlterAPIIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `ALTER API INTEGRATION "my_api" UNSET COMMENT`)
	})

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterAPIIntegrationOptions{
			Name: NewAccountObjectIdentifier("my_api"),
		}

		stmts, err := buildAlterAPIIntegrationStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("MultipleChanges", func(t *testing.T) {
		t.Parallel()

		comment := "updated"
		roleARN := "arn:aws:iam::999:role/new"
		blocked := []string{"https://bad.com/"}
		opts := AlterAPIIntegrationOptions{
			Name:               NewAccountObjectIdentifier("my_api"),
			Comment:            &comment,
			APIAWSRoleARN:      &roleARN,
			APIBlockedPrefixes: &blocked,
			Enabled:            ptr(false),
			UnsetFields:        []string{"API_KEY"},
		}

		stmts, err := buildAlterAPIIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 2) // SET + UNSET
		assert.Contains(t, stmts[0], `COMMENT = '`+comment+`'`)
		assert.Contains(t, stmts[0], `API_AWS_ROLE_ARN = '`+roleARN+`'`)
		assert.Contains(t, stmts[0], `API_BLOCKED_PREFIXES = ('https://bad.com/')`)
		assert.Contains(t, stmts[0], `ENABLED = FALSE`)
		assert.Contains(t, stmts[1], `UNSET API_KEY`)
	})

	t.Run("ValidationError_EmptyAllowedPrefixes", func(t *testing.T) {
		t.Parallel()

		empty := []string{}
		opts := AlterAPIIntegrationOptions{
			Name:               NewAccountObjectIdentifier("my_api"),
			APIAllowedPrefixes: &empty,
		}

		_, err := buildAlterAPIIntegrationStatements(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "api_allowed_prefixes cannot be set to empty")
	})
}

func TestAlterAPIIntegrationOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterAPIIntegrationOptions{Name: NewAccountObjectIdentifier("x")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterAPIIntegrationOptions{Name: NewAccountObjectIdentifier("x"), Comment: ptr("c")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()

		opts := AlterAPIIntegrationOptions{Name: NewAccountObjectIdentifier("x"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithEnabled", func(t *testing.T) {
		t.Parallel()

		opts := AlterAPIIntegrationOptions{Name: NewAccountObjectIdentifier("x"), Enabled: ptr(true)}
		assert.True(t, opts.HasChanges())
	})
}

func TestCreateAPIIntegrationOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("ValidAWS", func(t *testing.T) {
		t.Parallel()

		opts := CreateAPIIntegrationOptions{
			Name:               NewAccountObjectIdentifier("x"),
			APIProvider:        "aws_api_gateway",
			APIAllowedPrefixes: []string{"https://x"},
			APIAWSRoleARN:      ptr("arn"),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("ValidAzure", func(t *testing.T) {
		t.Parallel()

		opts := CreateAPIIntegrationOptions{
			Name:               NewAccountObjectIdentifier("x"),
			APIProvider:        "azure_api_management",
			APIAllowedPrefixes: []string{"https://x"},
			AzureTenantID:      ptr("tid"),
			AzureADAppID:       ptr("aid"),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("ValidGoogle", func(t *testing.T) {
		t.Parallel()

		opts := CreateAPIIntegrationOptions{
			Name:               NewAccountObjectIdentifier("x"),
			APIProvider:        "google_api_gateway",
			APIAllowedPrefixes: []string{"https://x"},
			GoogleAudience:     ptr("aud"),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("ValidGit", func(t *testing.T) {
		t.Parallel()

		opts := CreateAPIIntegrationOptions{
			Name:               NewAccountObjectIdentifier("x"),
			APIProvider:        "git_https_api",
			APIAllowedPrefixes: []string{"https://github.com/"},
		}
		assert.NoError(t, opts.Validate())
	})
}
