package streamlit

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

func TestParseCommaList(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, parseCommaList(""))
	})

	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"EAI_1"}, parseCommaList("EAI_1"))
	})

	t.Run("Multiple", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"EAI_1", "EAI_2", "EAI_3"}, parseCommaList("EAI_1, EAI_2, EAI_3"))
	})

	t.Run("WhitespaceHandling", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"A", "B"}, parseCommaList("  A , B  "))
	})

	t.Run("TrailingComma", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"A", "B"}, parseCommaList("A,B,"))
	})
}

func TestStringSliceEqualFold(t *testing.T) {
	t.Parallel()

	t.Run("Equal", func(t *testing.T) {
		t.Parallel()
		assert.True(t, stringSliceEqualFold([]string{"A", "B"}, []string{"A", "B"}))
	})

	t.Run("CaseInsensitive", func(t *testing.T) {
		t.Parallel()
		assert.True(t, stringSliceEqualFold([]string{"eai_1", "eai_2"}, []string{"EAI_1", "EAI_2"}))
	})

	t.Run("OrderIndependent", func(t *testing.T) {
		t.Parallel()
		assert.True(t, stringSliceEqualFold([]string{"B", "A"}, []string{"A", "B"}))
	})

	t.Run("DifferentLength", func(t *testing.T) {
		t.Parallel()
		assert.False(t, stringSliceEqualFold([]string{"A"}, []string{"A", "B"}))
	})

	t.Run("DifferentValues", func(t *testing.T) {
		t.Parallel()
		assert.False(t, stringSliceEqualFold([]string{"A", "B"}, []string{"A", "C"}))
	})

	t.Run("BothEmpty", func(t *testing.T) {
		t.Parallel()
		assert.True(t, stringSliceEqualFold([]string{}, []string{}))
	})

	t.Run("BothNil", func(t *testing.T) {
		t.Parallel()
		assert.True(t, stringSliceEqualFold(nil, nil))
	})
}

func TestBuildAlterOptions_ExternalAccessIntegrations(t *testing.T) {
	t.Parallel()

	id := snowflake.NewSchemaObjectIdentifier("DB", "SCH", "MY_STREAMLIT")

	t.Run("NoEAI_NoChange", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.Streamlit{}
		obs := &snowflake.StreamlitObservation{
			ShowOutput: &snowplanev1alpha1.StreamlitShowOutput{},
		}

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.ExternalAccessIntegrations)
	})

	t.Run("EAI_MatchesDescribe_NoChange", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.Streamlit{
			Spec: snowplanev1alpha1.StreamlitSpec{
				ExternalAccessIntegrations: []string{"EAI_1", "EAI_2"},
			},
		}
		obs := &snowflake.StreamlitObservation{
			ShowOutput:     &snowplanev1alpha1.StreamlitShowOutput{},
			DescribeOutput: &snowplanev1alpha1.StreamlitDescribeOutput{ExternalAccessIntegrations: "EAI_1, EAI_2"},
		}

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.ExternalAccessIntegrations)
	})

	t.Run("EAI_DiffersFromDescribe_SetsAlter", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.Streamlit{
			Spec: snowplanev1alpha1.StreamlitSpec{
				ExternalAccessIntegrations: []string{"EAI_1", "EAI_NEW"},
			},
		}
		obs := &snowflake.StreamlitObservation{
			ShowOutput:     &snowplanev1alpha1.StreamlitShowOutput{},
			DescribeOutput: &snowplanev1alpha1.StreamlitDescribeOutput{ExternalAccessIntegrations: "EAI_1, EAI_2"},
		}

		opts := buildAlterOptions(obj, id, obs)
		require(t, opts.ExternalAccessIntegrations != nil, "ExternalAccessIntegrations should be set")
		assert.Equal(t, []string{"EAI_1", "EAI_NEW"}, *opts.ExternalAccessIntegrations)
	})

	t.Run("EAI_NoDescribeOutput_SetsAlter", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.Streamlit{
			Spec: snowplanev1alpha1.StreamlitSpec{
				ExternalAccessIntegrations: []string{"EAI_1"},
			},
		}
		obs := &snowflake.StreamlitObservation{
			ShowOutput: &snowplanev1alpha1.StreamlitShowOutput{},
		}

		opts := buildAlterOptions(obj, id, obs)
		require(t, opts.ExternalAccessIntegrations != nil, "ExternalAccessIntegrations should be set")
		assert.Equal(t, []string{"EAI_1"}, *opts.ExternalAccessIntegrations)
	})
}

// require is a simple helper that fails the test if the condition is false.
func require(t *testing.T, condition bool, msg string) {
	t.Helper()

	if !condition {
		t.Fatal(msg)
	}
}
