package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateExternalTableSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		opts := CreateExternalTableOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			Location:   "@DB.SCH.MY_STAGE/path/",
			FileFormat: "TYPE = PARQUET",
		}
		got := buildCreateExternalTableSQL(opts)
		assert.Contains(t, got, `CREATE EXTERNAL TABLE IF NOT EXISTS "DB"."SCH"."MY_EXT"`)
		assert.Contains(t, got, `LOCATION = @DB.SCH.MY_STAGE/path/`)
		assert.Contains(t, got, `FILE_FORMAT = (TYPE = PARQUET)`)
	})

	t.Run("WithColumns", func(t *testing.T) {
		t.Parallel()
		opts := CreateExternalTableOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			Location:   "@DB.SCH.MY_STAGE/",
			FileFormat: "TYPE = CSV",
			Columns: []ExternalTableColumnOpt{
				{Name: "col1", Type: "VARCHAR", As: "value:col1::varchar"},
				{Name: "col2", Type: "NUMBER", As: "value:col2::number"},
			},
		}
		got := buildCreateExternalTableSQL(opts)
		assert.Contains(t, got, `"col1" VARCHAR AS (value:col1::varchar)`)
		assert.Contains(t, got, `"col2" NUMBER AS (value:col2::number)`)
	})

	t.Run("WithPartitions", func(t *testing.T) {
		t.Parallel()
		partType := "USER_SPECIFIED"
		opts := CreateExternalTableOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			Location:      "@DB.SCH.MY_STAGE/",
			FileFormat:    "TYPE = PARQUET",
			PartitionBy:   []string{"date_part", "region"},
			PartitionType: &partType,
		}
		got := buildCreateExternalTableSQL(opts)
		assert.Contains(t, got, `PARTITION BY ("date_part", "region")`)
		assert.Contains(t, got, `PARTITION_TYPE = USER_SPECIFIED`)
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		refreshOnCreate := false
		autoRefresh := true
		pattern := ".*[.]parquet"
		snsTopic := "arn:aws:sns:us-east-1:123456789:my-topic"
		tableFormat := "DELTA"
		integration := "my_integration"
		comment := "my external table"
		opts := CreateExternalTableOptions{
			Name:            NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			Location:        "@DB.SCH.MY_STAGE/data/",
			FileFormat:      "TYPE = PARQUET",
			RefreshOnCreate: &refreshOnCreate,
			AutoRefresh:     &autoRefresh,
			Pattern:         &pattern,
			AwsSnsTopic:     &snsTopic,
			TableFormat:     &tableFormat,
			Integration:     &integration,
			Comment:         &comment,
		}
		got := buildCreateExternalTableSQL(opts)
		assert.Contains(t, got, `INTEGRATION = 'my_integration'`)
		assert.Contains(t, got, `LOCATION = @DB.SCH.MY_STAGE/data/`)
		assert.Contains(t, got, `REFRESH_ON_CREATE = FALSE`)
		assert.Contains(t, got, `AUTO_REFRESH = TRUE`)
		assert.Contains(t, got, `PATTERN = '.*[.]parquet'`)
		assert.Contains(t, got, `FILE_FORMAT = (TYPE = PARQUET)`)
		assert.Contains(t, got, `AWS_SNS_TOPIC = 'arn:aws:sns:us-east-1:123456789:my-topic'`)
		assert.Contains(t, got, `TABLE_FORMAT = DELTA`)
		assert.Contains(t, got, `COMMENT = 'my external table'`)
	})

	t.Run("WithNamedFileFormat", func(t *testing.T) {
		t.Parallel()
		opts := CreateExternalTableOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			Location:   "@DB.SCH.MY_STAGE/",
			FileFormat: "FORMAT_NAME = 'DB.SCH.MY_FORMAT'",
		}
		got := buildCreateExternalTableSQL(opts)
		assert.Contains(t, got, `FILE_FORMAT = (FORMAT_NAME = 'DB.SCH.MY_FORMAT')`)
	})
}

func TestCreateExternalTableOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateExternalTableOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			Location:   "@DB.SCH.STAGE/",
			FileFormat: "TYPE = PARQUET",
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateExternalTableOptions{
			Location:   "@DB.SCH.STAGE/",
			FileFormat: "TYPE = PARQUET",
		}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingLocation", func(t *testing.T) {
		t.Parallel()
		opts := CreateExternalTableOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			FileFormat: "TYPE = PARQUET",
		}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingFileFormat", func(t *testing.T) {
		t.Parallel()
		opts := CreateExternalTableOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			Location: "@DB.SCH.STAGE/",
		}
		require.Error(t, opts.Validate())
	})

	t.Run("InvalidColumn", func(t *testing.T) {
		t.Parallel()
		opts := CreateExternalTableOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			Location:   "@DB.SCH.STAGE/",
			FileFormat: "TYPE = PARQUET",
			Columns: []ExternalTableColumnOpt{
				{Name: "", Type: "VARCHAR", As: "value:c::varchar"},
			},
		}
		require.Error(t, opts.Validate())
	})
}

func TestBuildAlterExternalTableSQL(t *testing.T) {
	t.Parallel()

	t.Run("SetAutoRefreshTrue", func(t *testing.T) {
		t.Parallel()
		autoRefresh := true
		opts := AlterExternalTableOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			AutoRefresh: &autoRefresh,
		}
		got := buildAlterExternalTableSQL(opts)
		assert.Equal(t, `ALTER EXTERNAL TABLE IF EXISTS "DB"."SCH"."MY_EXT" SET AUTO_REFRESH = TRUE`, got)
	})

	t.Run("SetAutoRefreshFalse", func(t *testing.T) {
		t.Parallel()
		autoRefresh := false
		opts := AlterExternalTableOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			AutoRefresh: &autoRefresh,
		}
		got := buildAlterExternalTableSQL(opts)
		assert.Equal(t, `ALTER EXTERNAL TABLE IF EXISTS "DB"."SCH"."MY_EXT" SET AUTO_REFRESH = FALSE`, got)
	})

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalTableOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
		}
		got := buildAlterExternalTableSQL(opts)
		assert.Empty(t, got)
	})
}

func TestAlterExternalTableOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalTableOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalTableOptions{}
		require.Error(t, opts.Validate())
	})
}

func TestAlterExternalTableOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalTableOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
		}
		assert.False(t, opts.HasChanges())
	})

	t.Run("AutoRefreshChange", func(t *testing.T) {
		t.Parallel()
		autoRefresh := true
		opts := AlterExternalTableOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "MY_EXT"),
			AutoRefresh: &autoRefresh,
		}
		assert.True(t, opts.HasChanges())
	})
}
