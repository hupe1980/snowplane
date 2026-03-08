//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func TestGitRepository_CreateLifecycle(t *testing.T) {
	resetMocks()

	dbK8s := "gitrepo-create-db"
	sfDB := "GITREPO_CREATE_DB"
	schemaK8s := "gitrepo-create-schema"
	sfSchema := "GITREPO_CREATE_SCHEMA"
	repoK8s := "gitrepo-create-test"
	sfRepo := "GITREPO_CREATE_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var created atomic.Bool

	gitRepositoryMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.GitRepositoryObservation, error) {
		if created.Load() {
			return gitRepositoryObservation(sfRepo, sfDB, sfSchema, "https://github.com/example/repo.git", "my_api_integration", "SYSADMIN"), nil
		}

		return &snowflake.GitRepositoryObservation{Exists: false}, nil
	})

	gitRepositoryMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateGitRepositoryOptions) error {
		assert.Equal(t, sfRepo, opts.Name.Name())
		assert.Equal(t, sfDB, opts.Name.DatabaseName())
		assert.Equal(t, sfSchema, opts.Name.SchemaName())
		assert.Equal(t, "https://github.com/example/repo.git", opts.Origin)
		assert.Equal(t, "my_api_integration", opts.APIIntegration)
		created.Store(true)

		return nil
	})

	repo := newTestGitRepository(repoK8s, sfRepo, dbK8s, schemaK8s)
	require.NoError(t, k8sClient.Create(ctx, repo))

	key := types.NamespacedName{Name: repoK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GitRepository
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "git repository should become Ready")

	var result snowplanev1alpha1.GitRepository
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, created.Load())
	assert.Equal(t, sfRepo, result.Status.ShowOutput.Name)
	assert.Equal(t, sfDB, result.Status.DatabaseName)
	assert.Equal(t, sfSchema, result.Status.SchemaName)
	assert.Equal(t, "SYSADMIN", result.Status.ShowOutput.Owner)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/gitrepository")

	refCond := conditions.Get(&result, snowplanev1alpha1.TypeReferencesResolved)
	assert.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionTrue, refCond.Status)

	// Cleanup
	gitRepositoryMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GitRepository
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval, "git repository should be cleaned up")
}

func TestGitRepository_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	dbK8s := "gitrepo-orphan-db"
	sfDB := "GITREPO_ORPHAN_DB"
	schemaK8s := "gitrepo-orphan-schema"
	sfSchema := "GITREPO_ORPHAN_SCHEMA"
	repoK8s := "gitrepo-orphan-test"
	sfRepo := "GITREPO_ORPHAN_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var (
		created atomic.Bool
		dropped atomic.Bool
	)

	gitRepositoryMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.GitRepositoryObservation, error) {
		if created.Load() {
			return gitRepositoryObservation(sfRepo, sfDB, sfSchema, "https://github.com/example/repo.git", "my_api_integration", "SYSADMIN"), nil
		}

		return &snowflake.GitRepositoryObservation{Exists: false}, nil
	})

	gitRepositoryMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateGitRepositoryOptions) error {
		created.Store(true)
		return nil
	})

	gitRepositoryMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropped.Store(true)
		return nil
	})

	repo := newTestGitRepository(repoK8s, sfRepo, dbK8s, schemaK8s)
	repo.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, repo))

	key := types.NamespacedName{Name: repoK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GitRepository
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.GitRepository
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GitRepository
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)

	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}

func TestGitRepository_UpdateComment(t *testing.T) {
	resetMocks()

	dbK8s := "gitrepo-alter-db"
	sfDB := "GITREPO_ALTER_DB"
	schemaK8s := "gitrepo-alter-schema"
	sfSchema := "GITREPO_ALTER_SCHEMA"
	repoK8s := "gitrepo-alter-test"
	sfRepo := "GITREPO_ALTER_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	gitRepositoryMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.GitRepositoryObservation, error) {
		if created.Load() {
			obs := gitRepositoryObservation(sfRepo, sfDB, sfSchema, "https://github.com/example/repo.git", "my_api_integration", "SYSADMIN")
			obs.ShowOutput.Comment = curComment.Load().(string)

			return obs, nil
		}

		return &snowflake.GitRepositoryObservation{Exists: false}, nil
	})

	gitRepositoryMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateGitRepositoryOptions) error {
		created.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	gitRepositoryMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterGitRepositoryOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}

		return nil
	})

	repo := newTestGitRepository(repoK8s, sfRepo, dbK8s, schemaK8s)
	initComment := "initial comment"
	repo.Spec.Comment = &initComment
	f := false
	repo.Spec.ManagementPolicies.CreateOrAlter = &f

	require.NoError(t, k8sClient.Create(ctx, repo))

	key := types.NamespacedName{Name: repoK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GitRepository
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// Update comment
	var current snowplanev1alpha1.GitRepository
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "updated comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "ALTER should have been called")

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GitRepository
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")

	// Cleanup
	gitRepositoryMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GitRepository
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}
