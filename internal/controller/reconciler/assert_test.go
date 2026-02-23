package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

func TestAssertIdentifier_Success(t *testing.T) {
	t.Parallel()

	id := snowflake.NewAccountObjectIdentifier("MY_DB")
	got, err := AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	require.NoError(t, err)
	assert.Contains(t, got.FullyQualifiedName(), "MY_DB")
}

func TestAssertIdentifier_Failure(t *testing.T) {
	t.Parallel()

	id := snowflake.NewAccountObjectIdentifier("MY_DB")
	_, err := AssertIdentifier[snowflake.DatabaseObjectIdentifier](id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identifier type mismatch")
	assert.Contains(t, err.Error(), "AccountObjectIdentifier")
}

func TestAssertDetail_Success(t *testing.T) {
	t.Parallel()

	obs := &Observation{
		Exists: true,
		Detail: &snowflake.DatabaseObservation{},
	}

	got, err := AssertDetail[*snowflake.DatabaseObservation](obs)
	require.NoError(t, err)
	assert.NotNil(t, got)
}

func TestAssertDetail_Failure(t *testing.T) {
	t.Parallel()

	obs := &Observation{
		Exists: true,
		Detail: &snowflake.WarehouseObservation{},
	}

	_, err := AssertDetail[*snowflake.DatabaseObservation](obs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "observation detail type mismatch")
}

func TestAssertDetail_NilDetail(t *testing.T) {
	t.Parallel()

	obs := &Observation{
		Exists: false,
		Detail: nil,
	}

	_, err := AssertDetail[*snowflake.DatabaseObservation](obs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "observation detail type mismatch")
}

type assertTestAlterOpts struct{ hasChanges bool }

func (m *assertTestAlterOpts) HasChanges() bool { return m.hasChanges }

type otherTestAlterOpts struct{ hasChanges bool }

func (m *otherTestAlterOpts) HasChanges() bool { return m.hasChanges }

func TestAssertAlterOptions_Success(t *testing.T) {
	t.Parallel()

	opts := &assertTestAlterOpts{hasChanges: true}
	got, err := AssertAlterOptions[*assertTestAlterOpts](opts)
	require.NoError(t, err)
	assert.True(t, got.HasChanges())
}

func TestAssertAlterOptions_Failure(t *testing.T) {
	t.Parallel()

	opts := &assertTestAlterOpts{hasChanges: true}
	_, err := AssertAlterOptions[*otherTestAlterOpts](opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alter options type mismatch")
}
