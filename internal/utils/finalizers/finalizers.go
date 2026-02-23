// Package finalizers provides helpers for managing Kubernetes finalizers.
package finalizers

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Has reports whether the object has the named finalizer.
func Has(obj client.Object, name string) bool {
	return controllerutil.ContainsFinalizer(obj, name)
}

// Add adds the named finalizer to the object if not already present.
// Returns true if the finalizer was added.
func Add(obj client.Object, name string) bool {
	if Has(obj, name) {
		return false
	}

	controllerutil.AddFinalizer(obj, name)

	return true
}

// Remove removes the named finalizer from the object if present.
// Returns true if the finalizer was removed.
func Remove(obj client.Object, name string) bool {
	if !Has(obj, name) {
		return false
	}

	controllerutil.RemoveFinalizer(obj, name)

	return true
}
