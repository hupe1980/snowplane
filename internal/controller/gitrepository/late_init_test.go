package gitrepository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newGitRepository() *snowplanev1alpha1.GitRepository {
	return &snowplanev1alpha1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.GitRepositorySpec{
			Name:           "MY_REPO",
			Origin:         "https://github.com/example/repo.git",
			APIIntegration: "MY_API",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills nil fields from observation", func(t *testing.T) {
		obj := newGitRepository()
		obs := &reconciler.Observation[*snowflake.GitRepositoryObservation]{
			Exists: true,
			Detail: &snowflake.GitRepositoryObservation{
				ShowOutput: &snowplanev1alpha1.GitRepositoryShowOutput{
					Comment:        "adopted",
					GitCredentials: "MY_CREDS",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "adopted", *obj.Spec.Comment)
		assert.Equal(t, "MY_CREDS", *obj.Spec.GitCredentials)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newGitRepository()
		obj.Spec.Comment = testutil.Ptr("user comment")
		obj.Spec.GitCredentials = testutil.Ptr("USER_CREDS")

		obs := &reconciler.Observation[*snowflake.GitRepositoryObservation]{
			Exists: true,
			Detail: &snowflake.GitRepositoryObservation{
				ShowOutput: &snowplanev1alpha1.GitRepositoryShowOutput{
					Comment:        "sf comment",
					GitCredentials: "SF_CREDS",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, "USER_CREDS", *obj.Spec.GitCredentials)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newGitRepository()
		obs := &reconciler.Observation[*snowflake.GitRepositoryObservation]{Exists: true, Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newGitRepository()
		obs := &reconciler.Observation[*snowflake.GitRepositoryObservation]{
			Exists: true,
			Detail: &snowflake.GitRepositoryObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("skips zero-value strings", func(t *testing.T) {
		obj := newGitRepository()
		obs := &reconciler.Observation[*snowflake.GitRepositoryObservation]{
			Exists: true,
			Detail: &snowflake.GitRepositoryObservation{
				ShowOutput: &snowplanev1alpha1.GitRepositoryShowOutput{
					Comment:        "",
					GitCredentials: "",
				},
			},
		}
		assert.False(t, lateInitialize(obj, obs))
	})
}
