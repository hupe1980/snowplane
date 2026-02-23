package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateTableSQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     CreateTableOptions
		expected string
	}{
		{
			name: "basic table",
			opts: CreateTableOptions{
				Name: NewSchemaObjectIdentifier("DB", "SCHEMA", "USERS"),
				Columns: []CreateTableColumn{
					{Name: "ID", Type: "NUMBER(38,0)"},
					{Name: "NAME", Type: "VARCHAR(256)"},
				},
			},
			expected: `CREATE TABLE IF NOT EXISTS "DB"."SCHEMA"."USERS" ("ID" NUMBER(38,0), "NAME" VARCHAR(256))`,
		},
		{
			name: "transient table with options",
			opts: CreateTableOptions{
				Name:      NewSchemaObjectIdentifier("DB", "SCHEMA", "EVENTS"),
				Transient: true,
				Columns: []CreateTableColumn{
					{Name: "ID", Type: "NUMBER"},
					{Name: "TS", Type: "TIMESTAMP_NTZ"},
				},
				DataRetentionTimeInDays: ptrInt32(1),
				ChangeTracking:          ptrBool(true),
				Comment:                 ptrString("events table"),
			},
			expected: `CREATE TRANSIENT TABLE IF NOT EXISTS "DB"."SCHEMA"."EVENTS" ("ID" NUMBER, "TS" TIMESTAMP_NTZ) DATA_RETENTION_TIME_IN_DAYS = 1 CHANGE_TRACKING = TRUE COMMENT = 'events table'`,
		},
		{
			name: "table with not null and default",
			opts: CreateTableOptions{
				Name: NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns: []CreateTableColumn{
					{Name: "ID", Type: "NUMBER", Nullable: ptrBool(false)},
					{Name: "STATUS", Type: "VARCHAR", Default: ptrString("'ACTIVE'"), Comment: ptrString("status col")},
				},
			},
			expected: `CREATE TABLE IF NOT EXISTS "DB"."S"."T" ("ID" NUMBER NOT NULL, "STATUS" VARCHAR DEFAULT 'ACTIVE' COMMENT 'status col')`,
		},
		{
			name: "table with cluster by",
			opts: CreateTableOptions{
				Name: NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns: []CreateTableColumn{
					{Name: "DATE_COL", Type: "DATE"},
					{Name: "ID", Type: "NUMBER"},
				},
				ClusterBy: []string{"DATE_COL", "ID"},
			},
			expected: `CREATE TABLE IF NOT EXISTS "DB"."S"."T" ("DATE_COL" DATE, "ID" NUMBER) CLUSTER BY ("DATE_COL", "ID")`,
		},
		{
			name: "table with schema evolution",
			opts: CreateTableOptions{
				Name: NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns: []CreateTableColumn{
					{Name: "V", Type: "VARIANT"},
				},
				EnableSchemaEvolution: ptrBool(true),
			},
			expected: `CREATE TABLE IF NOT EXISTS "DB"."S"."T" ("V" VARIANT) ENABLE_SCHEMA_EVOLUTION = TRUE`,
		},
		{
			name: "table with primary key constraint",
			opts: CreateTableOptions{
				Name: NewSchemaObjectIdentifier("DB", "S", "ORDERS"),
				Columns: []CreateTableColumn{
					{Name: "ID", Type: "NUMBER(38,0)", Nullable: ptrBool(false)},
					{Name: "AMOUNT", Type: "NUMBER(10,2)"},
				},
				Constraints: []CreateTableConstraint{
					{Name: "pk_orders", Type: "PRIMARY KEY", Columns: []string{"ID"}},
				},
			},
			expected: `CREATE TABLE IF NOT EXISTS "DB"."S"."ORDERS" ("ID" NUMBER(38,0) NOT NULL, "AMOUNT" NUMBER(10,2), CONSTRAINT "pk_orders" PRIMARY KEY ("ID"))`,
		},
		{
			name: "table with unique constraint",
			opts: CreateTableOptions{
				Name: NewSchemaObjectIdentifier("DB", "S", "USERS"),
				Columns: []CreateTableColumn{
					{Name: "ID", Type: "NUMBER"},
					{Name: "EMAIL", Type: "VARCHAR(256)"},
				},
				Constraints: []CreateTableConstraint{
					{Type: "UNIQUE", Columns: []string{"EMAIL"}},
				},
			},
			expected: `CREATE TABLE IF NOT EXISTS "DB"."S"."USERS" ("ID" NUMBER, "EMAIL" VARCHAR(256), UNIQUE ("EMAIL"))`,
		},
		{
			name: "table with foreign key constraint",
			opts: CreateTableOptions{
				Name: NewSchemaObjectIdentifier("DB", "S", "ORDER_ITEMS"),
				Columns: []CreateTableColumn{
					{Name: "ID", Type: "NUMBER"},
					{Name: "ORDER_ID", Type: "NUMBER"},
				},
				Constraints: []CreateTableConstraint{
					{
						Name:              "fk_order",
						Type:              "FOREIGN KEY",
						Columns:           []string{"ORDER_ID"},
						ForeignKeyTable:   `"DB"."S"."ORDERS"`,
						ForeignKeyColumns: []string{"ID"},
					},
				},
			},
			expected: `CREATE TABLE IF NOT EXISTS "DB"."S"."ORDER_ITEMS" ("ID" NUMBER, "ORDER_ID" NUMBER, CONSTRAINT "fk_order" FOREIGN KEY ("ORDER_ID") REFERENCES "DB"."S"."ORDERS" ("ID"))`,
		},
		{
			name: "table with multiple constraints",
			opts: CreateTableOptions{
				Name: NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns: []CreateTableColumn{
					{Name: "ID", Type: "NUMBER", Nullable: ptrBool(false)},
					{Name: "CODE", Type: "VARCHAR(10)"},
					{Name: "REF_ID", Type: "NUMBER"},
				},
				Constraints: []CreateTableConstraint{
					{Name: "pk_t", Type: "PRIMARY KEY", Columns: []string{"ID"}},
					{Name: "uq_code", Type: "UNIQUE", Columns: []string{"CODE"}},
					{Name: "fk_ref", Type: "FOREIGN KEY", Columns: []string{"REF_ID"}, ForeignKeyTable: `"DB"."S"."OTHER"`, ForeignKeyColumns: []string{"ID"}},
				},
			},
			expected: `CREATE TABLE IF NOT EXISTS "DB"."S"."T" ("ID" NUMBER NOT NULL, "CODE" VARCHAR(10), "REF_ID" NUMBER, CONSTRAINT "pk_t" PRIMARY KEY ("ID"), CONSTRAINT "uq_code" UNIQUE ("CODE"), CONSTRAINT "fk_ref" FOREIGN KEY ("REF_ID") REFERENCES "DB"."S"."OTHER" ("ID"))`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildCreateTableSQL(tc.opts)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestBuildAlterTableStatements(t *testing.T) {
	t.Parallel()

	id := NewSchemaObjectIdentifier("DB", "S", "T")

	tests := []struct {
		name     string
		opts     AlterTableOptions
		expected []string
	}{
		{
			name: "set comment and retention",
			opts: AlterTableOptions{
				Name:                    id,
				Comment:                 ptrString("updated"),
				DataRetentionTimeInDays: ptrInt32(7),
			},
			expected: []string{
				`ALTER TABLE "DB"."S"."T" SET COMMENT = 'updated' DATA_RETENTION_TIME_IN_DAYS = 7`,
			},
		},
		{
			name: "change clustering key",
			opts: AlterTableOptions{
				Name:      id,
				ClusterBy: []string{"COL_A", "COL_B"},
			},
			expected: []string{
				`ALTER TABLE "DB"."S"."T" CLUSTER BY ("COL_A", "COL_B")`,
			},
		},
		{
			name: "drop clustering key",
			opts: AlterTableOptions{
				Name:              id,
				DropClusteringKey: true,
			},
			expected: []string{
				`ALTER TABLE "DB"."S"."T" DROP CLUSTERING KEY`,
			},
		},
		{
			name: "unset fields",
			opts: AlterTableOptions{
				Name:        id,
				UnsetFields: []string{"COMMENT", "CHANGE_TRACKING"},
			},
			expected: []string{
				`ALTER TABLE "DB"."S"."T" UNSET COMMENT, CHANGE_TRACKING`,
			},
		},
		{
			name: "set change tracking and schema evolution",
			opts: AlterTableOptions{
				Name:                  id,
				ChangeTracking:        ptrBool(true),
				EnableSchemaEvolution: ptrBool(false),
			},
			expected: []string{
				`ALTER TABLE "DB"."S"."T" SET CHANGE_TRACKING = TRUE ENABLE_SCHEMA_EVOLUTION = FALSE`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := buildAlterTableStatements(tc.opts)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestBuildDropTableSQL(t *testing.T) {
	t.Parallel()

	got := buildDropTableSQL(NewSchemaObjectIdentifier("DB", "S", "T"))
	assert.Equal(t, `DROP TABLE IF EXISTS "DB"."S"."T"`, got)
}

func TestBuildShowTableByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowTableByIDSQL(NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_TABLE"))
	assert.Equal(t, `SHOW TABLES LIKE 'MY\_TABLE' IN SCHEMA "MY_DB"."PUBLIC"`, got)
}

func TestCreateTableOptionsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    CreateTableOptions
		wantErr bool
	}{
		{
			name: "valid",
			opts: CreateTableOptions{
				Name:    NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns: []CreateTableColumn{{Name: "ID", Type: "NUMBER"}},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			opts: CreateTableOptions{
				Name:    NewSchemaObjectIdentifier("", "", ""),
				Columns: []CreateTableColumn{{Name: "ID", Type: "NUMBER"}},
			},
			wantErr: true,
		},
		{
			name: "no columns",
			opts: CreateTableOptions{
				Name: NewSchemaObjectIdentifier("DB", "S", "T"),
			},
			wantErr: true,
		},
		{
			name: "column missing type",
			opts: CreateTableOptions{
				Name:    NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns: []CreateTableColumn{{Name: "ID"}},
			},
			wantErr: true,
		},
		{
			name: "retention out of range",
			opts: CreateTableOptions{
				Name:                    NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns:                 []CreateTableColumn{{Name: "ID", Type: "NUMBER"}},
				DataRetentionTimeInDays: ptrInt32(100),
			},
			wantErr: true,
		},
		{
			name: "valid with constraints",
			opts: CreateTableOptions{
				Name:    NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns: []CreateTableColumn{{Name: "ID", Type: "NUMBER"}},
				Constraints: []CreateTableConstraint{
					{Type: "PRIMARY KEY", Columns: []string{"ID"}},
				},
			},
			wantErr: false,
		},
		{
			name: "constraint with no columns",
			opts: CreateTableOptions{
				Name:    NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns: []CreateTableColumn{{Name: "ID", Type: "NUMBER"}},
				Constraints: []CreateTableConstraint{
					{Type: "PRIMARY KEY"},
				},
			},
			wantErr: true,
		},
		{
			name: "constraint with unknown type",
			opts: CreateTableOptions{
				Name:    NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns: []CreateTableColumn{{Name: "ID", Type: "NUMBER"}},
				Constraints: []CreateTableConstraint{
					{Type: "CHECK", Columns: []string{"ID"}},
				},
			},
			wantErr: true,
		},
		{
			name: "foreign key missing table",
			opts: CreateTableOptions{
				Name:    NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns: []CreateTableColumn{{Name: "REF_ID", Type: "NUMBER"}},
				Constraints: []CreateTableConstraint{
					{Type: "FOREIGN KEY", Columns: []string{"REF_ID"}, ForeignKeyColumns: []string{"ID"}},
				},
			},
			wantErr: true,
		},
		{
			name: "foreign key column count mismatch",
			opts: CreateTableOptions{
				Name:    NewSchemaObjectIdentifier("DB", "S", "T"),
				Columns: []CreateTableColumn{{Name: "A", Type: "NUMBER"}, {Name: "B", Type: "NUMBER"}},
				Constraints: []CreateTableConstraint{
					{Type: "FOREIGN KEY", Columns: []string{"A", "B"}, ForeignKeyTable: "OTHER", ForeignKeyColumns: []string{"X"}},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.opts.Validate()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAlterTableOptionsValidation(t *testing.T) {
	t.Parallel()

	err := (&AlterTableOptions{
		Name:    NewSchemaObjectIdentifier("DB", "S", "T"),
		Comment: ptrString("ok"),
	}).Validate()
	require.NoError(t, err)

	err = (&AlterTableOptions{
		Name:                    NewSchemaObjectIdentifier("DB", "S", "T"),
		DataRetentionTimeInDays: ptrInt32(-1),
	}).Validate()
	require.Error(t, err)
}

func TestAlterTableOptionsHasChanges(t *testing.T) {
	t.Parallel()

	assert.False(t, (&AlterTableOptions{Name: NewSchemaObjectIdentifier("DB", "S", "T")}).HasChanges())
	assert.True(t, (&AlterTableOptions{Name: NewSchemaObjectIdentifier("DB", "S", "T"), Comment: ptrString("x")}).HasChanges())
	assert.True(t, (&AlterTableOptions{Name: NewSchemaObjectIdentifier("DB", "S", "T"), DropClusteringKey: true}).HasChanges())
	assert.True(t, (&AlterTableOptions{Name: NewSchemaObjectIdentifier("DB", "S", "T"), ClusterBy: []string{"A"}}).HasChanges())
}
