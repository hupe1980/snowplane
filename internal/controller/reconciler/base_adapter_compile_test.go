package reconciler

// Compile-time interface assertions for BaseAdapter.
// These use a dummy concrete type to verify that BaseAdapter always satisfies
// every interface the reconciler may type-assert against.

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// Use Database as a representative ManagedResource.
type (
	testT = *snowplanev1alpha1.Database
	testS = any
	testD = any
)

// Core interface.
var _ ResourceAdapter[testT, testS, testD] = (*BaseAdapter[testT, testS, testD])(nil)

// Optional interfaces — always satisfied with nil-safe defaults.
var _ PreReconciler[testT] = (*BaseAdapter[testT, testS, testD])(nil)
var _ WatchConfigurer = (*BaseAdapter[testT, testS, testD])(nil)
var _ PostCreateHook[testT] = (*BaseAdapter[testT, testS, testD])(nil)
var _ PostUpdateHook[testT] = (*BaseAdapter[testT, testS, testD])(nil)
var _ CreateOrAlterSupporter = (*BaseAdapter[testT, testS, testD])(nil)
var _ CascadeDropSupporter = (*BaseAdapter[testT, testS, testD])(nil)
var _ CascadeDropper[testT, testS] = (*BaseAdapter[testT, testS, testD])(nil)
var _ LateInitializer[testT, testD] = (*BaseAdapter[testT, testS, testD])(nil)
var _ PreFlightChecker[testT] = (*BaseAdapter[testT, testS, testD])(nil)
