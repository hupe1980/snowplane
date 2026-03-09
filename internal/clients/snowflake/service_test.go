package snowflake

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// Tests: CreateServiceOptions.Validate
// --------------------------------------------------------------------------

func TestCreateServiceOptions_Validate_Valid_Specification(t *testing.T) {
	t.Parallel()

	spec := "containers:\n- name: main\n  image: myimage"
	opts := CreateServiceOptions{
		Name:          NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
		ComputePool:   "MY_POOL",
		Specification: &spec,
	}
	assert.NoError(t, opts.Validate())
}

func TestCreateServiceOptions_Validate_Valid_SpecificationReference(t *testing.T) {
	t.Parallel()

	ref := "@db.schema.stage/spec.yaml"
	opts := CreateServiceOptions{
		Name:                   NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
		ComputePool:            "MY_POOL",
		SpecificationReference: &ref,
	}
	assert.NoError(t, opts.Validate())
}

func TestCreateServiceOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	spec := "containers: []"
	opts := CreateServiceOptions{
		Name:          NewSchemaObjectIdentifier("DB", "SCH", ""),
		ComputePool:   "MY_POOL",
		Specification: &spec,
	}
	assert.Error(t, opts.Validate())
}

func TestCreateServiceOptions_Validate_EmptyComputePool(t *testing.T) {
	t.Parallel()

	spec := "containers: []"
	opts := CreateServiceOptions{
		Name:          NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
		ComputePool:   "",
		Specification: &spec,
	}
	assert.Error(t, opts.Validate())
}

func TestCreateServiceOptions_Validate_BothSpecifications(t *testing.T) {
	t.Parallel()

	spec := "containers: []"
	ref := "@db.schema.stage/spec.yaml"
	opts := CreateServiceOptions{
		Name:                   NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
		ComputePool:            "MY_POOL",
		Specification:          &spec,
		SpecificationReference: &ref,
	}
	assert.Error(t, opts.Validate())
}

func TestCreateServiceOptions_Validate_NeitherSpecification(t *testing.T) {
	t.Parallel()

	opts := CreateServiceOptions{
		Name:        NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
		ComputePool: "MY_POOL",
	}
	assert.Error(t, opts.Validate())
}

// --------------------------------------------------------------------------
// Tests: AlterServiceOptions.Validate
// --------------------------------------------------------------------------

func TestAlterServiceOptions_Validate_Valid(t *testing.T) {
	t.Parallel()

	opts := AlterServiceOptions{
		Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
	}
	assert.NoError(t, opts.Validate())
}

func TestAlterServiceOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	opts := AlterServiceOptions{
		Name: NewSchemaObjectIdentifier("DB", "SCH", ""),
	}
	assert.Error(t, opts.Validate())
}

// --------------------------------------------------------------------------
// Tests: AlterServiceOptions.HasChanges
// --------------------------------------------------------------------------

func TestAlterServiceOptions_HasChanges_None(t *testing.T) {
	t.Parallel()

	opts := AlterServiceOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "S")}
	assert.False(t, opts.HasChanges())
}

func TestAlterServiceOptions_HasChanges_MinInstances(t *testing.T) {
	t.Parallel()

	v := int32(2)
	opts := AlterServiceOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "S"), MinInstances: &v}
	assert.True(t, opts.HasChanges())
}

func TestAlterServiceOptions_HasChanges_MaxInstances(t *testing.T) {
	t.Parallel()

	v := int32(5)
	opts := AlterServiceOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "S"), MaxInstances: &v}
	assert.True(t, opts.HasChanges())
}

func TestAlterServiceOptions_HasChanges_Comment(t *testing.T) {
	t.Parallel()

	c := "hello"
	opts := AlterServiceOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "S"), Comment: &c}
	assert.True(t, opts.HasChanges())
}

func TestAlterServiceOptions_HasChanges_UnsetFields(t *testing.T) {
	t.Parallel()

	opts := AlterServiceOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "S"), UnsetFields: []string{"COMMENT"}}
	assert.True(t, opts.HasChanges())
}

// --------------------------------------------------------------------------
// Tests: buildShowServiceByIDSQL
// --------------------------------------------------------------------------

func TestBuildShowServiceByIDSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowServiceByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"))
	assert.Contains(t, sql, "SHOW SERVICES")
	assert.Contains(t, sql, "LIKE")
}

// --------------------------------------------------------------------------
// Tests: ServiceClient (validation only, no DB)
// --------------------------------------------------------------------------

func TestServiceClient_Create_InvalidName(t *testing.T) {
	t.Parallel()

	spec := "containers: []"
	client := NewServiceClient(nil)
	err := client.Create(t.Context(), CreateServiceOptions{
		Name:          NewSchemaObjectIdentifier("DB", "SCH", ""),
		ComputePool:   "MY_POOL",
		Specification: &spec,
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestServiceClient_Create_InvalidSpecificationReference(t *testing.T) {
	t.Parallel()

	ref := "@stage; DROP DATABASE x; --"
	client := NewServiceClient(nil)
	err := client.Create(t.Context(), CreateServiceOptions{
		Name:                   NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
		ComputePool:            "MY_POOL",
		SpecificationReference: &ref,
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestServiceClient_Drop_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewServiceClient(nil)
	err := client.Drop(t.Context(), NewSchemaObjectIdentifier("DB", "SCH", ""))
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestServiceClient_ShowByID_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewServiceClient(nil)
	_, err := client.ShowByID(t.Context(), NewSchemaObjectIdentifier("DB", "SCH", ""))
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestServiceClient_Alter_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewServiceClient(nil)
	err := client.Alter(t.Context(), AlterServiceOptions{
		Name: NewSchemaObjectIdentifier("DB", "SCH", ""),
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

// --------------------------------------------------------------------------
// Tests: Create SQL generation (verifies BoolToSQL fix)
// --------------------------------------------------------------------------

func TestServiceClient_Create_AutoResumeSQL(t *testing.T) {
	t.Parallel()

	t.Run("AutoResumeTrue", func(t *testing.T) {
		t.Parallel()

		var captured string
		mock := &testSQLExec{
			execFn: func(_ context.Context, sql string, _ ...any) error {
				captured = sql
				return nil
			},
		}

		spec := "containers:\n- name: main\n  image: myimage"
		client := NewServiceClient(mock)
		err := client.Create(t.Context(), CreateServiceOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
			ComputePool:   "MY_POOL",
			Specification: &spec,
			AutoResume:    ptr(true),
		})
		require.NoError(t, err)
		assert.Contains(t, captured, "AUTO_RESUME = TRUE")
		assert.NotContains(t, captured, "AUTO_RESUME = true")
	})

	t.Run("AutoResumeFalse", func(t *testing.T) {
		t.Parallel()

		var captured string
		mock := &testSQLExec{
			execFn: func(_ context.Context, sql string, _ ...any) error {
				captured = sql
				return nil
			},
		}

		spec := "containers:\n- name: main\n  image: myimage"
		client := NewServiceClient(mock)
		err := client.Create(t.Context(), CreateServiceOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
			ComputePool:   "MY_POOL",
			Specification: &spec,
			AutoResume:    ptr(false),
		})
		require.NoError(t, err)
		assert.Contains(t, captured, "AUTO_RESUME = FALSE")
		assert.NotContains(t, captured, "AUTO_RESUME = false")
	})
}

func TestServiceClient_Create_FullSQL(t *testing.T) {
	t.Parallel()

	var captured string
	mock := &testSQLExec{
		execFn: func(_ context.Context, sql string, _ ...any) error {
			captured = sql
			return nil
		},
	}

	spec := "containers:\n- name: main\n  image: myimage"
	minInst := int32(1)
	maxInst := int32(3)
	comment := "test service"

	client := NewServiceClient(mock)
	err := client.Create(t.Context(), CreateServiceOptions{
		Name:                       NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
		ComputePool:                "MY_POOL",
		Specification:              &spec,
		MinInstances:               &minInst,
		MaxInstances:               &maxInst,
		AutoResume:                 ptr(true),
		ExternalAccessIntegrations: []string{"MY_EAI"},
		Comment:                    &comment,
	})
	require.NoError(t, err)

	assert.Contains(t, captured, `CREATE SERVICE "DB"."SCH"."MY_SVC"`)
	assert.Contains(t, captured, `IN COMPUTE POOL "MY_POOL"`)
	assert.Contains(t, captured, "FROM SPECIFICATION $$")
	assert.Contains(t, captured, "MIN_INSTANCES = 1")
	assert.Contains(t, captured, "MAX_INSTANCES = 3")
	assert.Contains(t, captured, "AUTO_RESUME = TRUE")
	assert.Contains(t, captured, `EXTERNAL_ACCESS_INTEGRATIONS = ("MY_EAI")`)
	assert.Contains(t, captured, "COMMENT = 'test service'")
}

func TestServiceClient_Create_SpecificationReference(t *testing.T) {
	t.Parallel()

	var captured string
	mock := &testSQLExec{
		execFn: func(_ context.Context, sql string, _ ...any) error {
			captured = sql
			return nil
		},
	}

	ref := "@db.schema.stage/spec.yaml"
	client := NewServiceClient(mock)
	err := client.Create(t.Context(), CreateServiceOptions{
		Name:                   NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
		ComputePool:            "MY_POOL",
		SpecificationReference: &ref,
	})
	require.NoError(t, err)

	assert.Contains(t, captured, "FROM @db.schema.stage/spec.yaml")
	assert.NotContains(t, captured, "SPECIFICATION $$")
}
