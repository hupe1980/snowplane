package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateStageSQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     CreateStageOptions
		expected string
	}{
		{
			name: "internal stage",
			opts: CreateStageOptions{
				Name: NewSchemaObjectIdentifier("DB", "S", "MY_STAGE"),
			},
			expected: `CREATE STAGE IF NOT EXISTS "DB"."S"."MY_STAGE"`,
		},
		{
			name: "internal with comment and encryption",
			opts: CreateStageOptions{
				Name:       NewSchemaObjectIdentifier("DB", "S", "ENCRYPTED"),
				Comment:    ptr("encrypted stage"),
				Encryption: &StageEncryptionOptions{Type: "SNOWFLAKE_FULL"},
			},
			expected: `CREATE STAGE IF NOT EXISTS "DB"."S"."ENCRYPTED" ENCRYPTION = (TYPE = 'SNOWFLAKE_FULL') COMMENT = 'encrypted stage'`,
		},
		{
			name: "external stage with URL and integration",
			opts: CreateStageOptions{
				Name:               NewSchemaObjectIdentifier("DB", "S", "EXT"),
				URL:                ptr("s3://my-bucket/path/"),
				StorageIntegration: ptr("MY_INT"),
			},
			expected: `CREATE STAGE IF NOT EXISTS "DB"."S"."EXT" URL = 's3://my-bucket/path/' STORAGE_INTEGRATION = "MY_INT"`,
		},
		{
			name: "stage with directory enabled",
			opts: CreateStageOptions{
				Name: NewSchemaObjectIdentifier("DB", "S", "DIR"),
				Directory: &StageDirectoryCreateOptions{
					Enable:      true,
					AutoRefresh: ptr(true),
				},
			},
			expected: `CREATE STAGE IF NOT EXISTS "DB"."S"."DIR" DIRECTORY = (ENABLE = TRUE AUTO_REFRESH = TRUE)`,
		},
		{
			name: "stage with file format",
			opts: CreateStageOptions{
				Name:       NewSchemaObjectIdentifier("DB", "S", "FF"),
				FileFormat: ptr("FORMAT_NAME = 'MY_FORMAT'"),
			},
			expected: `CREATE STAGE IF NOT EXISTS "DB"."S"."FF" FILE_FORMAT = (FORMAT_NAME = 'MY_FORMAT')`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := buildCreateStageSQL(tc.opts)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestBuildAlterStageStatements(t *testing.T) {
	t.Parallel()

	id := NewSchemaObjectIdentifier("DB", "S", "STG")

	tests := []struct {
		name     string
		opts     AlterStageOptions
		expected []string
	}{
		{
			name: "set comment",
			opts: AlterStageOptions{
				Name:    id,
				Comment: ptr("updated"),
			},
			expected: []string{
				`ALTER STAGE "DB"."S"."STG" SET COMMENT = 'updated'`,
			},
		},
		{
			name: "set URL and integration",
			opts: AlterStageOptions{
				Name:               id,
				URL:                ptr("s3://new-bucket/"),
				StorageIntegration: ptr("NEW_INT"),
			},
			expected: []string{
				`ALTER STAGE "DB"."S"."STG" SET URL = 's3://new-bucket/' STORAGE_INTEGRATION = "NEW_INT"`,
			},
		},
		{
			name: "enable directory",
			opts: AlterStageOptions{
				Name: id,
				Directory: &StageDirectoryCreateOptions{
					Enable: true,
				},
			},
			expected: []string{
				`ALTER STAGE "DB"."S"."STG" SET DIRECTORY = (ENABLE = TRUE)`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := buildAlterStageStatements(tc.opts)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestBuildDropStageSQL(t *testing.T) {
	t.Parallel()

	got := buildDropStageSQL(NewSchemaObjectIdentifier("DB", "S", "STG"))
	assert.Equal(t, `DROP STAGE IF EXISTS "DB"."S"."STG"`, got)
}

func TestBuildShowStageByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowStageByIDSQL(NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_STAGE"))
	assert.Equal(t, `SHOW STAGES LIKE 'MY\_STAGE' IN SCHEMA "MY_DB"."PUBLIC"`, got)
}

func TestCreateStageOptionsValidation(t *testing.T) {
	t.Parallel()

	err := (&CreateStageOptions{
		Name: NewSchemaObjectIdentifier("DB", "S", "STG"),
	}).Validate()
	require.NoError(t, err)

	err = (&CreateStageOptions{
		Name: NewSchemaObjectIdentifier("", "", ""),
	}).Validate()
	require.Error(t, err)

	// Valid encryption type
	err = (&CreateStageOptions{
		Name:       NewSchemaObjectIdentifier("DB", "S", "STG"),
		Encryption: &StageEncryptionOptions{Type: "SNOWFLAKE_FULL"},
	}).Validate()
	require.NoError(t, err)

	// Invalid encryption type
	err = (&CreateStageOptions{
		Name:       NewSchemaObjectIdentifier("DB", "S", "STG"),
		Encryption: &StageEncryptionOptions{Type: "SNOWFLAKE_FULL'; DROP DATABASE x;--"},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encryption")

	// Valid file format
	err = (&CreateStageOptions{
		Name:       NewSchemaObjectIdentifier("DB", "S", "STG"),
		FileFormat: ptr("FORMAT_NAME = 'MY_FORMAT'"),
	}).Validate()
	require.NoError(t, err)

	// Invalid file format with semicolons
	err = (&CreateStageOptions{
		Name:       NewSchemaObjectIdentifier("DB", "S", "STG"),
		FileFormat: ptr("TYPE = CSV; DROP DATABASE x"),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file format")
}

func TestAlterStageOptionsValidation(t *testing.T) {
	t.Parallel()

	// Valid alter with file format
	err := (&AlterStageOptions{
		Name:       NewSchemaObjectIdentifier("DB", "S", "STG"),
		FileFormat: ptr("FORMAT_NAME = 'MY_FORMAT'"),
	}).Validate()
	require.NoError(t, err)

	// Invalid file format
	err = (&AlterStageOptions{
		Name:       NewSchemaObjectIdentifier("DB", "S", "STG"),
		FileFormat: ptr("TYPE = CSV; DROP TABLE x"),
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file format")
}

func TestAlterStageOptionsHasChanges(t *testing.T) {
	t.Parallel()

	id := NewSchemaObjectIdentifier("DB", "S", "STG")
	assert.False(t, (&AlterStageOptions{Name: id}).HasChanges())
	assert.True(t, (&AlterStageOptions{Name: id, Comment: ptr("x")}).HasChanges())
	assert.True(t, (&AlterStageOptions{Name: id, URL: ptr("s3://x/")}).HasChanges())
}
