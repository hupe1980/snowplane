package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// Tests: CreateComputePoolOptions.Validate
// --------------------------------------------------------------------------

func TestCreateComputePoolOptions_Validate_Valid(t *testing.T) {
	t.Parallel()

	opts := CreateComputePoolOptions{
		Name:           NewAccountObjectIdentifier("MY_POOL"),
		MinNodes:       1,
		MaxNodes:       3,
		InstanceFamily: "CPU_X64_XS",
	}
	assert.NoError(t, opts.Validate())
}

func TestCreateComputePoolOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	opts := CreateComputePoolOptions{
		Name:           NewAccountObjectIdentifier(""),
		MinNodes:       1,
		MaxNodes:       3,
		InstanceFamily: "CPU_X64_XS",
	}
	assert.Error(t, opts.Validate())
}

func TestCreateComputePoolOptions_Validate_MinNodesZero(t *testing.T) {
	t.Parallel()

	opts := CreateComputePoolOptions{
		Name:           NewAccountObjectIdentifier("MY_POOL"),
		MinNodes:       0,
		MaxNodes:       3,
		InstanceFamily: "CPU_X64_XS",
	}
	assert.Error(t, opts.Validate())
}

func TestCreateComputePoolOptions_Validate_MaxNodesZero(t *testing.T) {
	t.Parallel()

	opts := CreateComputePoolOptions{
		Name:           NewAccountObjectIdentifier("MY_POOL"),
		MinNodes:       1,
		MaxNodes:       0,
		InstanceFamily: "CPU_X64_XS",
	}
	assert.Error(t, opts.Validate())
}

func TestCreateComputePoolOptions_Validate_MinExceedsMax(t *testing.T) {
	t.Parallel()

	opts := CreateComputePoolOptions{
		Name:           NewAccountObjectIdentifier("MY_POOL"),
		MinNodes:       5,
		MaxNodes:       3,
		InstanceFamily: "CPU_X64_XS",
	}
	assert.Error(t, opts.Validate())
}

func TestCreateComputePoolOptions_Validate_EmptyInstanceFamily(t *testing.T) {
	t.Parallel()

	opts := CreateComputePoolOptions{
		Name:           NewAccountObjectIdentifier("MY_POOL"),
		MinNodes:       1,
		MaxNodes:       3,
		InstanceFamily: "",
	}
	assert.Error(t, opts.Validate())
}

// --------------------------------------------------------------------------
// Tests: AlterComputePoolOptions.Validate
// --------------------------------------------------------------------------

func TestAlterComputePoolOptions_Validate_Valid(t *testing.T) {
	t.Parallel()

	opts := AlterComputePoolOptions{
		Name: NewAccountObjectIdentifier("MY_POOL"),
	}
	assert.NoError(t, opts.Validate())
}

func TestAlterComputePoolOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	opts := AlterComputePoolOptions{
		Name: NewAccountObjectIdentifier(""),
	}
	assert.Error(t, opts.Validate())
}

// --------------------------------------------------------------------------
// Tests: AlterComputePoolOptions.HasChanges
// --------------------------------------------------------------------------

func TestAlterComputePoolOptions_HasChanges_None(t *testing.T) {
	t.Parallel()

	opts := AlterComputePoolOptions{Name: NewAccountObjectIdentifier("P")}
	assert.False(t, opts.HasChanges())
}

func TestAlterComputePoolOptions_HasChanges_MinNodes(t *testing.T) {
	t.Parallel()

	v := int32(2)
	opts := AlterComputePoolOptions{Name: NewAccountObjectIdentifier("P"), MinNodes: &v}
	assert.True(t, opts.HasChanges())
}

func TestAlterComputePoolOptions_HasChanges_MaxNodes(t *testing.T) {
	t.Parallel()

	v := int32(5)
	opts := AlterComputePoolOptions{Name: NewAccountObjectIdentifier("P"), MaxNodes: &v}
	assert.True(t, opts.HasChanges())
}

func TestAlterComputePoolOptions_HasChanges_Comment(t *testing.T) {
	t.Parallel()

	c := "hello"
	opts := AlterComputePoolOptions{Name: NewAccountObjectIdentifier("P"), Comment: &c}
	assert.True(t, opts.HasChanges())
}

func TestAlterComputePoolOptions_HasChanges_UnsetFields(t *testing.T) {
	t.Parallel()

	opts := AlterComputePoolOptions{Name: NewAccountObjectIdentifier("P"), UnsetFields: []string{"COMMENT"}}
	assert.True(t, opts.HasChanges())
}

// --------------------------------------------------------------------------
// Tests: buildShowComputePoolByIDSQL
// --------------------------------------------------------------------------

func TestBuildShowComputePoolByIDSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowComputePoolByIDSQL(NewAccountObjectIdentifier("MY_POOL"))
	assert.Contains(t, sql, "SHOW COMPUTE POOLS")
	assert.Contains(t, sql, "LIKE")
}

// --------------------------------------------------------------------------
// Tests: ComputePoolClient (validation only, no DB)
// --------------------------------------------------------------------------

func TestComputePoolClient_Create_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewComputePoolClient(nil)
	err := client.Create(t.Context(), CreateComputePoolOptions{
		Name:           NewAccountObjectIdentifier(""),
		MinNodes:       1,
		MaxNodes:       3,
		InstanceFamily: "CPU_X64_XS",
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestComputePoolClient_Drop_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewComputePoolClient(nil)
	err := client.Drop(t.Context(), NewAccountObjectIdentifier(""))
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestComputePoolClient_ShowByID_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewComputePoolClient(nil)
	_, err := client.ShowByID(t.Context(), NewAccountObjectIdentifier(""))
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestComputePoolClient_Alter_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewComputePoolClient(nil)
	err := client.Alter(t.Context(), AlterComputePoolOptions{
		Name: NewAccountObjectIdentifier(""),
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}
