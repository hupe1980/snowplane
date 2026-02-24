package snowflake

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateFileFormatSQL(t *testing.T) {
	t.Parallel()

	t.Run("CSVBasic", func(t *testing.T) {
		t.Parallel()
		opts := CreateFileFormatOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_CSV"),
			Type: "CSV",
		}
		got := buildCreateFileFormatSQL(opts)
		assert.Contains(t, got, `CREATE FILE FORMAT IF NOT EXISTS "DB"."SCH"."MY_CSV"`)
		assert.Contains(t, got, "TYPE = 'CSV'")
	})

	t.Run("CSVWithOptions", func(t *testing.T) {
		t.Parallel()
		delim := "|"
		recDelim := "\\n"
		skipHeader := int32(1)
		enclosed := "\""
		comment := "my csv format"
		opts := CreateFileFormatOptions{
			Name:                      NewSchemaObjectIdentifier("DB", "SCH", "MY_CSV"),
			Type:                      "CSV",
			FieldDelimiter:            &delim,
			RecordDelimiter:           &recDelim,
			SkipHeader:                &skipHeader,
			FieldOptionallyEnclosedBy: &enclosed,
			Comment:                   &comment,
		}
		got := buildCreateFileFormatSQL(opts)
		assert.Contains(t, got, "FIELD_DELIMITER = '|'")
		assert.Contains(t, got, "SKIP_HEADER = 1")
		assert.Contains(t, got, "COMMENT = 'my csv format'")
	})

	t.Run("JSONWithOptions", func(t *testing.T) {
		t.Parallel()
		stripOuter := true
		stripNull := true
		opts := CreateFileFormatOptions{
			Name:            NewSchemaObjectIdentifier("DB", "SCH", "MY_JSON"),
			Type:            "JSON",
			StripOuterArray: &stripOuter,
			StripNullValues: &stripNull,
		}
		got := buildCreateFileFormatSQL(opts)
		assert.Contains(t, got, "TYPE = 'JSON'")
		assert.Contains(t, got, "STRIP_OUTER_ARRAY = TRUE")
		assert.Contains(t, got, "STRIP_NULL_VALUES = TRUE")
	})

	t.Run("WithNullIf", func(t *testing.T) {
		t.Parallel()
		nullIf := []string{"NULL", ""}
		opts := CreateFileFormatOptions{
			Name:   NewSchemaObjectIdentifier("DB", "SCH", "FF"),
			Type:   "CSV",
			NullIf: nullIf,
		}
		got := buildCreateFileFormatSQL(opts)
		assert.Contains(t, got, "NULL_IF = ('NULL', '')")
	})

	t.Run("WithCompression", func(t *testing.T) {
		t.Parallel()
		compression := "GZIP"
		opts := CreateFileFormatOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "FF"),
			Type:        "CSV",
			Compression: &compression,
		}
		got := buildCreateFileFormatSQL(opts)
		assert.Contains(t, got, "COMPRESSION = GZIP")
	})
}

func TestCreateFileFormatOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateFileFormatOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "FF"),
			Type: "CSV",
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateFileFormatOptions{Type: "CSV"}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingType", func(t *testing.T) {
		t.Parallel()
		opts := CreateFileFormatOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "FF"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type is required")
	})
}

func TestBuildAlterFileFormatStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterFileFormatOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "FF"),
		}
		stmts, err := buildAlterFileFormatStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterFileFormatOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "FF"),
			Comment: &comment,
		}
		stmts, err := buildAlterFileFormatStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterFileFormatOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "FF"),
			UnsetFields: []string{"COMMENT", "FIELD_DELIMITER"},
		}
		stmts, err := buildAlterFileFormatStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT, FIELD_DELIMITER")
	})

	t.Run("SetNullIf", func(t *testing.T) {
		t.Parallel()
		nullIf := []string{"NA", ""}
		opts := AlterFileFormatOptions{
			Name:   NewSchemaObjectIdentifier("DB", "SCH", "FF"),
			NullIf: &nullIf,
		}
		stmts, err := buildAlterFileFormatStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "NULL_IF = ('NA', '')")
	})
}

func TestShowFileFormatByIDSQL(t *testing.T) {
	t.Parallel()
	scope := fmt.Sprintf("SCHEMA %s", sqlbuilder.QuoteIdentifier("DB")+"."+sqlbuilder.QuoteIdentifier("SCH"))
	got := sqlbuilder.ShowLikeIn("FILE FORMATS", "MY_FF", scope)
	assert.Contains(t, got, "SHOW FILE FORMATS LIKE 'MY\\_FF'")
	assert.Contains(t, got, `IN SCHEMA "DB"."SCH"`)
}

func TestDropFileFormatSQL(t *testing.T) {
	t.Parallel()
	id := NewSchemaObjectIdentifier("DB", "SCH", "MY_FF")
	stmt := sqlbuilder.DropIfExists("FILE FORMAT", id.FullyQualifiedName())
	assert.Contains(t, stmt, `DROP FILE FORMAT IF EXISTS "DB"."SCH"."MY_FF"`)
}

func TestAlterFileFormatOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterFileFormatOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "FF")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterFileFormatOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "FF"), Comment: &c}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterFileFormatOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterFileFormatOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "FF")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterFileFormatOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}
