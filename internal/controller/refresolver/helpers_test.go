package refresolver

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// ---------------------------------------------------------------------------
// Tests: HandleRefError
// ---------------------------------------------------------------------------

func TestHandleRefError_SetsConditionsAndEmitsEvent(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "my-db", Namespace: "default"},
	}
	recorder := record.NewFakeRecorder(10)

	HandleRefError(db, recorder, "Database", `"my-ref" (ref)`, fmt.Errorf("not found"))

	// ReferencesResolved condition should be False.
	refCond := conditions.Get(db, snowplanev1alpha1.TypeReferencesResolved)
	require.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionFalse, refCond.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonDependencyNotReady, refCond.Reason)
	assert.Contains(t, refCond.Message, "Database")
	assert.Contains(t, refCond.Message, "not found")

	// Ready condition should be False.
	readyCond := conditions.Get(db, snowplanev1alpha1.TypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)

	// An event should have been recorded.
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "DependencyNotReady")
		assert.Contains(t, event, "not found")
	default:
		t.Fatal("expected a warning event to be recorded")
	}
}

// ---------------------------------------------------------------------------
// Tests: ResolveDatabaseRefWithConditions
// ---------------------------------------------------------------------------

func TestResolveDatabaseRefWithConditions_Success(t *testing.T) {
	t.Parallel()

	db := readyDB("my-db", "default", `"ANALYTICS"`)
	c := fake.NewClientBuilder().WithScheme(testScheme()).
		WithRuntimeObjects(db).
		WithStatusSubresource(&snowplanev1alpha1.Database{}).
		Build()
	recorder := record.NewFakeRecorder(10)

	target := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "caller", Namespace: "default"},
	}
	ref := &snowplanev1alpha1.LocalObjectReference{Name: "my-db"}

	fqn, err := ResolveDatabaseRefWithConditions(context.Background(), c, recorder, target, "default", ref, nil)
	require.NoError(t, err)
	assert.Equal(t, `"ANALYTICS"`, fqn)

	// No conditions should be set on success (caller is responsible).
	refCond := conditions.Get(target, snowplanev1alpha1.TypeReferencesResolved)
	assert.Nil(t, refCond)
}

func TestResolveDatabaseRefWithConditions_Error(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	recorder := record.NewFakeRecorder(10)

	target := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "caller", Namespace: "default"},
	}
	ref := &snowplanev1alpha1.LocalObjectReference{Name: "missing"}

	_, err := ResolveDatabaseRefWithConditions(context.Background(), c, recorder, target, "default", ref, nil)
	require.Error(t, err)

	// ReferencesResolved condition should be False.
	refCond := conditions.Get(target, snowplanev1alpha1.TypeReferencesResolved)
	require.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionFalse, refCond.Status)

	// Ready condition should be False.
	readyCond := conditions.Get(target, snowplanev1alpha1.TypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
}

// ---------------------------------------------------------------------------
// Tests: ResolveSchemaRefWithConditions
// ---------------------------------------------------------------------------

func readySchema(name, namespace, fqn string) *snowplanev1alpha1.Schema {
	s := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: snowplanev1alpha1.SchemaSpec{
			Name:         "PUBLIC",
			DatabaseName: strPtr("ANALYTICS"),
		},
		Status: snowplanev1alpha1.SchemaStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{FullyQualifiedName: fqn},
		},
	}
	conditions.SetReady(s, "ok")

	return s
}

func strPtr(s string) *string { return &s }

func TestResolveSchemaRefWithConditions_Success(t *testing.T) {
	t.Parallel()

	schema := readySchema("my-schema", "default", `"ANALYTICS"."PUBLIC"`)
	c := fake.NewClientBuilder().WithScheme(testScheme()).
		WithRuntimeObjects(schema).
		WithStatusSubresource(&snowplanev1alpha1.Schema{}).
		Build()
	recorder := record.NewFakeRecorder(10)

	target := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "caller", Namespace: "default"},
	}
	ref := &snowplanev1alpha1.LocalObjectReference{Name: "my-schema"}

	fqn, err := ResolveSchemaRefWithConditions(context.Background(), c, recorder, target, "default", ref, nil)
	require.NoError(t, err)
	assert.Equal(t, `"ANALYTICS"."PUBLIC"`, fqn)

	// No conditions set on success.
	refCond := conditions.Get(target, snowplanev1alpha1.TypeReferencesResolved)
	assert.Nil(t, refCond)
}

func TestResolveSchemaRefWithConditions_Error(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	recorder := record.NewFakeRecorder(10)

	target := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "caller", Namespace: "default"},
	}
	ref := &snowplanev1alpha1.LocalObjectReference{Name: "missing"}

	_, err := ResolveSchemaRefWithConditions(context.Background(), c, recorder, target, "default", ref, nil)
	require.Error(t, err)

	// ReferencesResolved condition should be False.
	refCond := conditions.Get(target, snowplanev1alpha1.TypeReferencesResolved)
	require.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionFalse, refCond.Status)
}

// ---------------------------------------------------------------------------
// Tests: SetDatabaseResolvedCondition
// ---------------------------------------------------------------------------

func TestSetDatabaseResolvedCondition(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "my-db", Namespace: "default"},
	}
	ref := &snowplanev1alpha1.LocalObjectReference{Name: "my-ref"}
	SetDatabaseResolvedCondition(db, ref, nil, `"ANALYTICS"`)

	refCond := conditions.Get(db, snowplanev1alpha1.TypeReferencesResolved)
	require.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionTrue, refCond.Status)
	assert.Contains(t, refCond.Message, `Database "my-ref" (ref) resolved to "ANALYTICS"`)
}

// ---------------------------------------------------------------------------
// Tests: SetDatabaseAndSchemaResolvedCondition
// ---------------------------------------------------------------------------

func TestSetDatabaseAndSchemaResolvedCondition(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "my-db", Namespace: "default"},
	}
	dbRef := &snowplanev1alpha1.LocalObjectReference{Name: "db-ref"}
	schemaName := "PUBLIC"
	SetDatabaseAndSchemaResolvedCondition(db, dbRef, nil, nil, &schemaName)

	refCond := conditions.Get(db, snowplanev1alpha1.TypeReferencesResolved)
	require.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionTrue, refCond.Status)
	assert.Contains(t, refCond.Message, `Database "db-ref" (ref)`)
	assert.Contains(t, refCond.Message, `Schema "PUBLIC" (inline)`)
}

// ---------------------------------------------------------------------------
// Tests: MapByFieldIndex
// ---------------------------------------------------------------------------

func TestMapByFieldIndex_NoMatches(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	mapFn := MapByFieldIndex(c,
		func() client.ObjectList { return &snowplanev1alpha1.DatabaseList{} },
		".spec.databaseRef.name",
		"listing dependents",
	)

	// Trigger with a Database object; no items indexed → empty result.
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "my-db", Namespace: "default"},
	}
	requests := mapFn(context.Background(), db)
	assert.Empty(t, requests)
}
