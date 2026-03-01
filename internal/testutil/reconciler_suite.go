package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ReconcilerSetup wraps a reconciler and its fake client so test suites
// can both reconcile and inspect k8s state.
type ReconcilerSetup struct {
	Reconciler reconcile.Reconciler
	Client     client.Client
}

// ReconcileSuiteConfig configures the standard behavioral test suite
// that every managed-resource controller should pass.
//
// Usage (inside a reconciler_test.go):
//
//	func TestStandardSuite(t *testing.T) {
//	    t.Parallel()
//	    testutil.ReconcileSuiteConfig{
//	        NewReconciler: func(objs ...runtime.Object) testutil.ReconcilerSetup {
//	            r := newTestReconciler(&mockService{}, objs...)
//	            return testutil.ReconcilerSetup{Reconciler: r, Client: r.Client}
//	        },
//	        NewFixture: func(name, ns string) client.Object {
//	            return newTestRole(name, ns)
//	        },
//	        NewBlankObject: func() client.Object {
//	            return &snowplanev1alpha1.AccountRole{}
//	        },
//	        FinalizerName: finalizerName,
//	    }.Run(t)
//	}
type ReconcileSuiteConfig struct {
	// NewReconciler creates a reconciler backed by a fake k8s client
	// pre-loaded with the given runtime objects.
	NewReconciler func(objs ...runtime.Object) ReconcilerSetup

	// NewFixture returns a managed-resource CR with the given name/namespace.
	// Must have spec.providerRef.name = "default-pc".
	NewFixture func(name, ns string) client.Object

	// NewBlankObject returns a zero-value CR for client.Get calls.
	NewBlankObject func() client.Object

	// FinalizerName is the expected finalizer string.
	FinalizerName string

	// PrereqObjects returns additional runtime.Objects that must be present
	// in the fake client for ref-resolution to succeed (e.g. Database, Schema
	// CRs that the fixture references via DatabaseRef / SchemaRef).
	// May be nil for resources without cross-resource refs.
	PrereqObjects func() []runtime.Object
}

// Run executes the standard behavioral test suite. Each sub-test verifies
// a universal reconciler behavior that must hold for every managed resource.
func (c ReconcileSuiteConfig) Run(t *testing.T) {
	t.Helper()

	// prereqs returns the objects that must be seeded for cross-resource
	// refs (DatabaseRef, SchemaRef, etc.) to resolve.
	prereqs := func() []runtime.Object {
		if c.PrereqObjects != nil {
			return c.PrereqObjects()
		}

		return nil
	}

	t.Run("CRNotFound", func(t *testing.T) {
		t.Parallel()

		r := c.NewReconciler()
		result, err := r.Reconciler.Reconcile(context.Background(), ReconcileReq("nonexistent", "default"))
		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, result)
	})

	t.Run("ProviderConfigNotFound", func(t *testing.T) {
		t.Parallel()

		obj := c.NewFixture("test-obj", "default")
		objs := append([]runtime.Object{obj.(runtime.Object)}, prereqs()...)
		r := c.NewReconciler(objs...)

		_, err := r.Reconciler.Reconcile(context.Background(), ReconcileReq("test-obj", "default"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fetching ProviderConfig")
	})

	t.Run("ProviderConfigNotReady", func(t *testing.T) {
		t.Parallel()

		obj := c.NewFixture("test-obj", "default")

		pc := NewTestPC("default")
		pc.Status.Conditions = nil

		objs := append([]runtime.Object{obj.(runtime.Object), pc, NewTestSecret("default")}, prereqs()...)
		r := c.NewReconciler(objs...)

		_, err := r.Reconciler.Reconcile(context.Background(), ReconcileReq("test-obj", "default"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ProviderConfig not ready")
	})

	t.Run("AddsFinalizer", func(t *testing.T) {
		t.Parallel()

		obj := c.NewFixture("test-obj", "default")
		objs := append([]runtime.Object{obj.(runtime.Object), NewTestPC("default"), NewTestSecret("default")}, prereqs()...)
		r := c.NewReconciler(objs...)

		result, err := r.Reconciler.Reconcile(context.Background(), ReconcileReq("test-obj", "default"))
		require.NoError(t, err)
		assert.Equal(t, time.Second, result.RequeueAfter, "should requeue after adding finalizer")

		got := c.NewBlankObject()
		require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{
			Name: "test-obj", Namespace: "default",
		}, got))
		assert.Contains(t, got.GetFinalizers(), c.FinalizerName)
	})
}
