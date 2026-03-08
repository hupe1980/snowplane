package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateExternalVolumeSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicS3", func(t *testing.T) {
		t.Parallel()

		roleARN := "arn:aws:iam::123456789012:role/myrole"
		opts := CreateExternalVolumeOptions{
			Name: NewAccountObjectIdentifier("MY_EV"),
			StorageLocations: []ExternalVolumeStorageLocationOption{
				{
					Name:              "my-s3-loc",
					StorageProvider:   "S3",
					StorageBaseURL:    "s3://mybucket/path/",
					StorageAWSRoleARN: &roleARN,
				},
			},
		}

		got, err := buildCreateExternalVolumeSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE EXTERNAL VOLUME IF NOT EXISTS "MY_EV"`)
		assert.Contains(t, got, "STORAGE_LOCATIONS = (")
		assert.Contains(t, got, "NAME = 'my-s3-loc'")
		assert.Contains(t, got, "STORAGE_PROVIDER = 'S3'")
		assert.Contains(t, got, "STORAGE_BASE_URL = 's3://mybucket/path/'")
		assert.Contains(t, got, "STORAGE_AWS_ROLE_ARN = 'arn:aws:iam::123456789012:role/myrole'")
	})

	t.Run("MultipleLocations", func(t *testing.T) {
		t.Parallel()

		roleARN := "arn:aws:iam::123456789012:role/myrole"
		tenantID := "my-tenant-id"
		opts := CreateExternalVolumeOptions{
			Name: NewAccountObjectIdentifier("MULTI_EV"),
			StorageLocations: []ExternalVolumeStorageLocationOption{
				{
					Name:              "s3-loc",
					StorageProvider:   "S3",
					StorageBaseURL:    "s3://bucket1/",
					StorageAWSRoleARN: &roleARN,
				},
				{
					Name:            "azure-loc",
					StorageProvider: "AZURE",
					StorageBaseURL:  "azure://myaccount.blob.core.windows.net/container/",
					AzureTenantID:   &tenantID,
				},
			},
			AllowWrites: ptr(false),
			Comment:     ptr("multi-cloud volume"),
		}

		got, err := buildCreateExternalVolumeSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "NAME = 's3-loc'")
		assert.Contains(t, got, "NAME = 'azure-loc'")
		assert.Contains(t, got, "STORAGE_PROVIDER = 'AZURE'")
		assert.Contains(t, got, "AZURE_TENANT_ID = 'my-tenant-id'")
		assert.Contains(t, got, "ALLOW_WRITES = FALSE")
		assert.Contains(t, got, "COMMENT = 'multi-cloud volume'")
	})

	t.Run("WithEncryption", func(t *testing.T) {
		t.Parallel()

		roleARN := "arn:aws:iam::123456789012:role/test"
		encType := "AWS_SSE_KMS"
		kmsKey := "1234abcd-12ab-34cd-56ef-1234567890ab"
		opts := CreateExternalVolumeOptions{
			Name: NewAccountObjectIdentifier("ENC_EV"),
			StorageLocations: []ExternalVolumeStorageLocationOption{
				{
					Name:               "enc-loc",
					StorageProvider:    "S3",
					StorageBaseURL:     "s3://mybucket/",
					StorageAWSRoleARN:  &roleARN,
					EncryptionType:     &encType,
					EncryptionKMSKeyID: &kmsKey,
				},
			},
		}

		got, err := buildCreateExternalVolumeSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "ENCRYPTION = ( TYPE = 'AWS_SSE_KMS' KMS_KEY_ID = '1234abcd-12ab-34cd-56ef-1234567890ab' )")
	})

	t.Run("GCSLocation", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalVolumeOptions{
			Name: NewAccountObjectIdentifier("GCS_EV"),
			StorageLocations: []ExternalVolumeStorageLocationOption{
				{
					Name:            "gcs-loc",
					StorageProvider: "GCS",
					StorageBaseURL:  "gcs://mybucket/data/",
				},
			},
		}

		got, err := buildCreateExternalVolumeSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "STORAGE_PROVIDER = 'GCS'")
		assert.Contains(t, got, "STORAGE_BASE_URL = 'gcs://mybucket/data/'")
	})
}

func TestCreateExternalVolumeOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalVolumeOptions{
			Name: NewAccountObjectIdentifier("MY_EV"),
			StorageLocations: []ExternalVolumeStorageLocationOption{
				{Name: "loc1", StorageProvider: "S3", StorageBaseURL: "s3://bucket/"},
			},
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalVolumeOptions{
			StorageLocations: []ExternalVolumeStorageLocationOption{
				{Name: "loc1", StorageProvider: "S3", StorageBaseURL: "s3://bucket/"},
			},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingLocations", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalVolumeOptions{
			Name: NewAccountObjectIdentifier("MY_EV"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one storage location")
	})

	t.Run("LocationMissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalVolumeOptions{
			Name: NewAccountObjectIdentifier("MY_EV"),
			StorageLocations: []ExternalVolumeStorageLocationOption{
				{Name: "", StorageProvider: "S3", StorageBaseURL: "s3://bucket/"},
			},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("LocationMissingProvider", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalVolumeOptions{
			Name: NewAccountObjectIdentifier("MY_EV"),
			StorageLocations: []ExternalVolumeStorageLocationOption{
				{Name: "loc1", StorageProvider: "", StorageBaseURL: "s3://bucket/"},
			},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storageProvider")
	})

	t.Run("LocationMissingURL", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalVolumeOptions{
			Name: NewAccountObjectIdentifier("MY_EV"),
			StorageLocations: []ExternalVolumeStorageLocationOption{
				{Name: "loc1", StorageProvider: "S3", StorageBaseURL: ""},
			},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storageBaseURL")
	})
}

func TestBuildAlterExternalVolumeStatements(t *testing.T) {
	t.Parallel()

	name := NewAccountObjectIdentifier("MY_EV")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalVolumeOptions{Name: name}
		stmts, err := buildAlterExternalVolumeStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetAllowWrites", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalVolumeOptions{
			Name:        name,
			AllowWrites: ptr(false),
		}
		stmts, err := buildAlterExternalVolumeStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ALLOW_WRITES = FALSE")
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalVolumeOptions{
			Name:    name,
			Comment: ptr("updated comment"),
		}
		stmts, err := buildAlterExternalVolumeStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated comment'")
	})

	t.Run("AddStorageLocation", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalVolumeOptions{
			Name: name,
			AddLocations: []ExternalVolumeStorageLocationOption{
				{
					Name:            "new-gcs",
					StorageProvider: "GCS",
					StorageBaseURL:  "gcs://newbucket/",
				},
			},
		}
		stmts, err := buildAlterExternalVolumeStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ADD STORAGE_LOCATION")
		assert.Contains(t, stmts[0], "NAME = 'new-gcs'")
		assert.Contains(t, stmts[0], "STORAGE_PROVIDER = 'GCS'")
	})

	t.Run("RemoveStorageLocation", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalVolumeOptions{
			Name:                name,
			RemoveLocationNames: []string{"old-loc"},
		}
		stmts, err := buildAlterExternalVolumeStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "REMOVE STORAGE_LOCATION 'old-loc'")
	})

	t.Run("RemoveBeforeAdd", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalVolumeOptions{
			Name:                name,
			RemoveLocationNames: []string{"old-loc"},
			AddLocations: []ExternalVolumeStorageLocationOption{
				{Name: "new-loc", StorageProvider: "S3", StorageBaseURL: "s3://bucket/"},
			},
		}
		stmts, err := buildAlterExternalVolumeStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 2)
		// REMOVE should come before ADD.
		assert.Contains(t, stmts[0], "REMOVE STORAGE_LOCATION")
		assert.Contains(t, stmts[1], "ADD STORAGE_LOCATION")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalVolumeOptions{
			Name:        name,
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterExternalVolumeStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("MixedOperations", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalVolumeOptions{
			Name:                name,
			AllowWrites:         ptr(true),
			Comment:             ptr("new comment"),
			RemoveLocationNames: []string{"old-loc"},
			AddLocations: []ExternalVolumeStorageLocationOption{
				{Name: "new-loc", StorageProvider: "GCS", StorageBaseURL: "gcs://bucket/"},
			},
		}
		stmts, err := buildAlterExternalVolumeStatements(opts)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(stmts), 3) // REMOVE + ADD + SET
	})
}

func TestAlterExternalVolumeOptions_HasChanges(t *testing.T) {
	t.Parallel()

	name := NewAccountObjectIdentifier("MY_EV")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalVolumeOptions{Name: name}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithAllowWrites", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalVolumeOptions{Name: name, AllowWrites: ptr(true)}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalVolumeOptions{Name: name, Comment: ptr("x")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithAddLocation", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalVolumeOptions{
			Name:         name,
			AddLocations: []ExternalVolumeStorageLocationOption{{Name: "loc", StorageProvider: "S3"}},
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithRemoveLocation", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalVolumeOptions{
			Name:                name,
			RemoveLocationNames: []string{"loc"},
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalVolumeOptions{Name: name, UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterExternalVolumeOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalVolumeOptions{Name: NewAccountObjectIdentifier("MY_EV")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalVolumeOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}

func TestBuildStorageLocationSQL(t *testing.T) {
	t.Parallel()

	t.Run("S3WithAllOptions", func(t *testing.T) {
		t.Parallel()

		roleARN := "arn:aws:iam::123456789012:role/myrole"
		extID := "my-ext-id"
		encType := "AWS_SSE_KMS"
		kmsKey := "key-id"
		got := buildStorageLocationSQL(ExternalVolumeStorageLocationOption{
			Name:                 "s3-loc",
			StorageProvider:      "S3",
			StorageBaseURL:       "s3://bucket/path/",
			StorageAWSRoleARN:    &roleARN,
			StorageAWSExternalID: &extID,
			EncryptionType:       &encType,
			EncryptionKMSKeyID:   &kmsKey,
		})
		assert.Contains(t, got, "NAME = 's3-loc'")
		assert.Contains(t, got, "STORAGE_PROVIDER = 'S3'")
		assert.Contains(t, got, "STORAGE_BASE_URL = 's3://bucket/path/'")
		assert.Contains(t, got, "STORAGE_AWS_ROLE_ARN = 'arn:aws:iam::123456789012:role/myrole'")
		assert.Contains(t, got, "STORAGE_AWS_EXTERNAL_ID = 'my-ext-id'")
		assert.Contains(t, got, "TYPE = 'AWS_SSE_KMS'")
		assert.Contains(t, got, "KMS_KEY_ID = 'key-id'")
	})

	t.Run("AzureWithTenantID", func(t *testing.T) {
		t.Parallel()

		tenantID := "my-tenant-id"
		got := buildStorageLocationSQL(ExternalVolumeStorageLocationOption{
			Name:            "azure-loc",
			StorageProvider: "AZURE",
			StorageBaseURL:  "azure://myaccount.blob.core.windows.net/container/",
			AzureTenantID:   &tenantID,
		})
		assert.Contains(t, got, "STORAGE_PROVIDER = 'AZURE'")
		assert.Contains(t, got, "AZURE_TENANT_ID = 'my-tenant-id'")
	})

	t.Run("GCSMinimal", func(t *testing.T) {
		t.Parallel()

		got := buildStorageLocationSQL(ExternalVolumeStorageLocationOption{
			Name:            "gcs-loc",
			StorageProvider: "GCS",
			StorageBaseURL:  "gcs://bucket/",
		})
		assert.Contains(t, got, "STORAGE_PROVIDER = 'GCS'")
		assert.NotContains(t, got, "STORAGE_AWS_ROLE_ARN")
		assert.NotContains(t, got, "AZURE_TENANT_ID")
		assert.NotContains(t, got, "ENCRYPTION")
	})
}
