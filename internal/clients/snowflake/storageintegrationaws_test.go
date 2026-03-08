package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateStorageIntegrationAWSSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAWSOptions{
			Name:                    NewAccountObjectIdentifier("MY_AWS_INT"),
			StorageAWSRoleARN:       "arn:aws:iam::123456789012:role/myrole",
			StorageAllowedLocations: []string{"s3://mybucket/"},
		}

		got, err := buildCreateStorageIntegrationAWSSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE STORAGE INTEGRATION IF NOT EXISTS "MY_AWS_INT"`)
		assert.Contains(t, got, "TYPE = 'EXTERNAL_STAGE'")
		assert.Contains(t, got, "STORAGE_PROVIDER = 'S3'")
		assert.Contains(t, got, "STORAGE_AWS_ROLE_ARN")
		assert.Contains(t, got, "STORAGE_ALLOWED_LOCATIONS")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAWSOptions{
			Name:                    NewAccountObjectIdentifier("MY_AWS_INT"),
			StorageAWSRoleARN:       "arn:aws:iam::123456789012:role/myrole",
			StorageAllowedLocations: []string{"s3://mybucket/"},
			StorageBlockedLocations: []string{"s3://mybucket/sensitive/"},
			Enabled:                 ptr(false),
			StorageAWSExternalID:    ptr("ext-id"),
			Comment:                 ptr("test"),
		}

		got, err := buildCreateStorageIntegrationAWSSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "ENABLED = FALSE")
		assert.Contains(t, got, "STORAGE_BLOCKED_LOCATIONS")
		assert.Contains(t, got, "STORAGE_AWS_EXTERNAL_ID = 'ext-id'")
		assert.Contains(t, got, "COMMENT = 'test'")
	})
}

func TestCreateStorageIntegrationAWSOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAWSOptions{
			Name:                    NewAccountObjectIdentifier("MY_AWS_INT"),
			StorageAWSRoleARN:       "arn:aws:iam::123456789012:role/myrole",
			StorageAllowedLocations: []string{"s3://mybucket/"},
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAWSOptions{
			StorageAWSRoleARN:       "arn:aws:iam::123456789012:role/myrole",
			StorageAllowedLocations: []string{"s3://mybucket/"},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingRoleARN", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAWSOptions{
			Name:                    NewAccountObjectIdentifier("MY_AWS_INT"),
			StorageAWSRoleARN:       "",
			StorageAllowedLocations: []string{"s3://mybucket/"},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storageAWSRoleARN")
	})

	t.Run("MissingLocations", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAWSOptions{
			Name:              NewAccountObjectIdentifier("MY_AWS_INT"),
			StorageAWSRoleARN: "arn:aws:iam::123456789012:role/myrole",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storage allowed location")
	})
}

func TestBuildAlterStorageIntegrationAWSStatements(t *testing.T) {
	t.Parallel()

	name := NewAccountObjectIdentifier("MY_AWS_INT")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterStorageIntegrationAWSOptions{Name: name}
		stmts, err := buildAlterStorageIntegrationAWSStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterStorageIntegrationAWSOptions{
			Name:    name,
			Comment: ptr("updated"),
		}
		stmts, err := buildAlterStorageIntegrationAWSStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated'")
	})

	t.Run("UpdateLocations", func(t *testing.T) {
		t.Parallel()

		locs := []string{"s3://a/", "s3://b/"}
		opts := AlterStorageIntegrationAWSOptions{
			Name:                    name,
			StorageAllowedLocations: &locs,
		}
		stmts, err := buildAlterStorageIntegrationAWSStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "STORAGE_ALLOWED_LOCATIONS")
	})

	t.Run("SetExternalID", func(t *testing.T) {
		t.Parallel()

		opts := AlterStorageIntegrationAWSOptions{
			Name:                 name,
			StorageAWSExternalID: ptr("my-ext-id"),
		}
		stmts, err := buildAlterStorageIntegrationAWSStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "STORAGE_AWS_EXTERNAL_ID = 'my-ext-id'")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterStorageIntegrationAWSOptions{
			Name:        name,
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterStorageIntegrationAWSStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})
}

func TestAlterStorageIntegrationAWSOptions_HasChanges(t *testing.T) {
	t.Parallel()

	name := NewAccountObjectIdentifier("MY_AWS_INT")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationAWSOptions{Name: name}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationAWSOptions{Name: name, Comment: ptr("x")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationAWSOptions{Name: name, UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterStorageIntegrationAWSOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationAWSOptions{Name: NewAccountObjectIdentifier("MY_AWS_INT")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationAWSOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}
