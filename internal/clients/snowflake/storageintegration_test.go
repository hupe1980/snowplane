package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateStorageIntegrationSQL(t *testing.T) {
	t.Parallel()

	t.Run("S3Basic", func(t *testing.T) {
		t.Parallel()
		roleARN := "arn:aws:iam::123456789012:role/myrole"
		opts := CreateStorageIntegrationOptions{
			Name:                    NewAccountObjectIdentifier("MY_INT"),
			Type:                    "EXTERNAL_STAGE",
			StorageProvider:         "S3",
			StorageAWSRoleARN:       &roleARN,
			StorageAllowedLocations: []string{"s3://mybucket/"},
		}
		got := buildCreateStorageIntegrationSQL(opts)
		assert.Contains(t, got, `CREATE STORAGE INTEGRATION IF NOT EXISTS "MY_INT"`)
		assert.Contains(t, got, "TYPE = 'EXTERNAL_STAGE'")
		assert.Contains(t, got, "STORAGE_PROVIDER = 'S3'")
		assert.Contains(t, got, "STORAGE_AWS_ROLE_ARN = 'arn:aws:iam::123456789012:role/myrole'")
		assert.Contains(t, got, "STORAGE_ALLOWED_LOCATIONS = ('s3://mybucket/')")
	})

	t.Run("GCSWithOptions", func(t *testing.T) {
		t.Parallel()
		comment := "test integration"
		enabled := false
		opts := CreateStorageIntegrationOptions{
			Name:                    NewAccountObjectIdentifier("GCS_INT"),
			Type:                    "EXTERNAL_STAGE",
			StorageProvider:         "GCS",
			StorageAllowedLocations: []string{"gcs://bucket1/path1/", "gcs://bucket2/"},
			StorageBlockedLocations: []string{"gcs://bucket1/sensitive/"},
			Enabled:                 &enabled,
			Comment:                 &comment,
		}
		got := buildCreateStorageIntegrationSQL(opts)
		assert.Contains(t, got, "STORAGE_PROVIDER = 'GCS'")
		assert.Contains(t, got, "ENABLED = FALSE")
		assert.Contains(t, got, "COMMENT = 'test integration'")
		assert.Contains(t, got, "STORAGE_BLOCKED_LOCATIONS = ('gcs://bucket1/sensitive/')")
	})

	t.Run("AzureWithTenantID", func(t *testing.T) {
		t.Parallel()
		tenantID := "my-tenant-id"
		opts := CreateStorageIntegrationOptions{
			Name:                    NewAccountObjectIdentifier("AZURE_INT"),
			Type:                    "EXTERNAL_STAGE",
			StorageProvider:         "AZURE",
			AzureTenantID:           &tenantID,
			StorageAllowedLocations: []string{"azure://myaccount.blob.core.windows.net/mycontainer/"},
		}
		got := buildCreateStorageIntegrationSQL(opts)
		assert.Contains(t, got, "STORAGE_PROVIDER = 'AZURE'")
		assert.Contains(t, got, "AZURE_TENANT_ID = 'my-tenant-id'")
	})

	t.Run("S3WithExternalID", func(t *testing.T) {
		t.Parallel()
		roleARN := "arn:aws:iam::123456789012:role/myrole"
		extID := "my-custom-external-id"
		opts := CreateStorageIntegrationOptions{
			Name:                    NewAccountObjectIdentifier("S3_EXT_INT"),
			Type:                    "EXTERNAL_STAGE",
			StorageProvider:         "S3",
			StorageAWSRoleARN:       &roleARN,
			StorageAWSExternalID:    &extID,
			StorageAllowedLocations: []string{"s3://mybucket/"},
		}
		got := buildCreateStorageIntegrationSQL(opts)
		assert.Contains(t, got, "STORAGE_AWS_EXTERNAL_ID = 'my-custom-external-id'")
		assert.Contains(t, got, "STORAGE_AWS_ROLE_ARN = 'arn:aws:iam::123456789012:role/myrole'")
	})
}

func TestCreateStorageIntegrationOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid_S3", func(t *testing.T) {
		t.Parallel()
		roleARN := "arn:aws:iam::123:role/r"
		opts := CreateStorageIntegrationOptions{
			Name:                    NewAccountObjectIdentifier("INT"),
			Type:                    "EXTERNAL_STAGE",
			StorageProvider:         "S3",
			StorageAWSRoleARN:       &roleARN,
			StorageAllowedLocations: []string{"s3://b/"},
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateStorageIntegrationOptions{
			Type:                    "EXTERNAL_STAGE",
			StorageProvider:         "GCS",
			StorageAllowedLocations: []string{"gcs://b/"},
		}
		require.Error(t, opts.Validate())
	})

	t.Run("S3MissingRoleARN", func(t *testing.T) {
		t.Parallel()
		opts := CreateStorageIntegrationOptions{
			Name:                    NewAccountObjectIdentifier("INT"),
			Type:                    "EXTERNAL_STAGE",
			StorageProvider:         "S3",
			StorageAllowedLocations: []string{"s3://b/"},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storageAWSRoleARN")
	})

	t.Run("AzureMissingTenantID", func(t *testing.T) {
		t.Parallel()
		opts := CreateStorageIntegrationOptions{
			Name:                    NewAccountObjectIdentifier("INT"),
			Type:                    "EXTERNAL_STAGE",
			StorageProvider:         "AZURE",
			StorageAllowedLocations: []string{"azure://a/"},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "azureTenantID")
	})

	t.Run("MissingLocations", func(t *testing.T) {
		t.Parallel()
		opts := CreateStorageIntegrationOptions{
			Name:            NewAccountObjectIdentifier("INT"),
			Type:            "EXTERNAL_STAGE",
			StorageProvider: "GCS",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storage allowed location")
	})
}

func TestBuildAlterStorageIntegrationStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationOptions{
			Name: NewAccountObjectIdentifier("INT"),
		}
		stmts, err := buildAlterStorageIntegrationStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterStorageIntegrationOptions{
			Name:    NewAccountObjectIdentifier("INT"),
			Comment: &comment,
		}
		stmts, err := buildAlterStorageIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET")
		assert.Contains(t, stmts[0], "COMMENT = 'updated'")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationOptions{
			Name:        NewAccountObjectIdentifier("INT"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterStorageIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("UpdateLocations", func(t *testing.T) {
		t.Parallel()
		locs := []string{"s3://a/", "s3://b/"}
		opts := AlterStorageIntegrationOptions{
			Name:                    NewAccountObjectIdentifier("INT"),
			StorageAllowedLocations: &locs,
		}
		stmts, err := buildAlterStorageIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "STORAGE_ALLOWED_LOCATIONS")
	})

	t.Run("SetExternalID", func(t *testing.T) {
		t.Parallel()
		extID := "my-ext-id"
		opts := AlterStorageIntegrationOptions{
			Name:                 NewAccountObjectIdentifier("INT"),
			StorageAWSExternalID: &extID,
		}
		stmts, err := buildAlterStorageIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "STORAGE_AWS_EXTERNAL_ID = 'my-ext-id'")
	})
}

func TestBuildShowStorageIntegrationByIDSQL(t *testing.T) {
	t.Parallel()
	got := buildShowStorageIntegrationByIDSQL(NewAccountObjectIdentifier("MY_INT"))
	assert.Contains(t, got, "SHOW STORAGE INTEGRATIONS LIKE 'MY\\_INT'")
}

func TestDropSQL(t *testing.T) {
	t.Parallel()
	stmt := sqlbuilder.DropIfExists("STORAGE INTEGRATION", sqlbuilder.QuoteIdentifier("MY_INT"))
	assert.Contains(t, stmt, `DROP STORAGE INTEGRATION IF EXISTS "MY_INT"`)
}

func TestAlterStorageIntegrationOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationOptions{Name: NewAccountObjectIdentifier("I")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterStorageIntegrationOptions{Name: NewAccountObjectIdentifier("I"), Comment: &c}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationOptions{Name: NewAccountObjectIdentifier("I"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithExternalID", func(t *testing.T) {
		t.Parallel()
		extID := "ext"
		opts := AlterStorageIntegrationOptions{Name: NewAccountObjectIdentifier("I"), StorageAWSExternalID: &extID}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterStorageIntegrationOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationOptions{Name: NewAccountObjectIdentifier("INT")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}
