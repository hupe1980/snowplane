package imagerepository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newImageRepository() *snowplanev1alpha1.ImageRepository {
	return &snowplanev1alpha1.ImageRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-imgrepo",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.ImageRepositorySpec{
			Name:         "MY_REPO",
			DatabaseName: ptr("DB"),
			SchemaName:   ptr("SCH"),
		},
	}
}

func TestLateInitialize_NoOp(t *testing.T) {
	t.Parallel()

	obj := newImageRepository()
	obs := &reconciler.Observation[*snowflake.ImageRepositoryObservation]{
		Exists: true,
		Detail: &snowflake.ImageRepositoryObservation{
			Exists: true,
			ShowOutput: &snowplanev1alpha1.ImageRepositoryShowOutput{
				Name:          "MY_REPO",
				DatabaseName:  "DB",
				SchemaName:    "SCH",
				RepositoryURL: "orgname-acctname.registry.snowflakecomputing.com/db/sch/my_repo",
			},
		},
	}

	modified := lateInitialize(obj, obs)
	assert.False(t, modified, "lateInitialize should be a no-op for ImageRepository")
}

func TestLateInitialize_DoesNotModifySpec(t *testing.T) {
	t.Parallel()

	obj := newImageRepository()
	specBefore := obj.Spec.DeepCopy()

	obs := &reconciler.Observation[*snowflake.ImageRepositoryObservation]{
		Exists: true,
		Detail: &snowflake.ImageRepositoryObservation{
			Exists: true,
			ShowOutput: &snowplanev1alpha1.ImageRepositoryShowOutput{
				Name:         "MY_REPO",
				DatabaseName: "DB",
				SchemaName:   "SCH",
			},
		},
	}

	lateInitialize(obj, obs)
	assert.Equal(t, specBefore, &obj.Spec, "spec should not be modified")
}
