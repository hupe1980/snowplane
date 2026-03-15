package streamlit

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/testutil"
)

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

func TestBuildAlterOptions_MainFile(t *testing.T) {
	t.Parallel()

	id := snowflake.NewSchemaObjectIdentifier("DB", "SCH", "MY_STREAMLIT")

	t.Run("SkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.Streamlit{
			Spec: snowplanev1alpha1.StreamlitSpec{
				MainFile: testutil.Ptr("app.py"),
			},
		}
		obs := &snowflake.StreamlitObservation{
			ShowOutput:     &snowplanev1alpha1.StreamlitShowOutput{},
			DescribeOutput: &snowplanev1alpha1.StreamlitDescribeOutput{MainFile: "app.py"},
		}
		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.MainFile, "Should skip when MainFile matches")
	})

	t.Run("SentWhenChanged", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.Streamlit{
			Spec: snowplanev1alpha1.StreamlitSpec{
				MainFile: testutil.Ptr("new_app.py"),
			},
		}
		obs := &snowflake.StreamlitObservation{
			ShowOutput:     &snowplanev1alpha1.StreamlitShowOutput{},
			DescribeOutput: &snowplanev1alpha1.StreamlitDescribeOutput{MainFile: "old_app.py"},
		}
		opts := buildAlterOptions(obj, id, obs)
		require(t, opts.MainFile != nil, "MainFile should be set when changed")
		assert.Equal(t, "new_app.py", *opts.MainFile)
	})
}

func TestDetectDrift_MainFileDrift(t *testing.T) {
	t.Parallel()

	obj := &snowplanev1alpha1.Streamlit{
		Spec: snowplanev1alpha1.StreamlitSpec{
			Name:     "MY_STREAMLIT",
			MainFile: testutil.Ptr("new_app.py"),
		},
		Status: snowplanev1alpha1.StreamlitStatus{
			DatabaseName: "DB",
			SchemaName:   "SCH",
		},
	}

	obs := &snowflake.StreamlitObservation{
		ShowOutput: &snowplanev1alpha1.StreamlitShowOutput{
			Name:         "MY_STREAMLIT",
			DatabaseName: "DB",
			SchemaName:   "SCH",
		},
		DescribeOutput: &snowplanev1alpha1.StreamlitDescribeOutput{
			MainFile: "old_app.py",
		},
	}

	result := detectDrift(obj, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "MAIN_FILE")
}
