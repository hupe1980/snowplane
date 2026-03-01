// Package refresolver provides a centralized reference resolver for cross-resource
// dependency management in Snowplane controllers.
package refresolver

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// ErrReferenceNotFound indicates the referenced resource does not exist.
var ErrReferenceNotFound = fmt.Errorf("referenced resource not found")

// ErrReferenceNotReady indicates the referenced resource exists but is not Ready.
var ErrReferenceNotReady = fmt.Errorf("referenced resource is not ready")

// ErrNeitherRefNorNameSet indicates neither a reference nor a raw name was provided.
var ErrNeitherRefNorNameSet = fmt.Errorf("neither ref nor name is set")

// ReferableObject is a Kubernetes object that exposes conditions and a
// fully qualified Snowflake name in its status.
type ReferableObject interface {
	client.Object
	conditions.ConditionedObject
	GetFullyQualifiedName() string
}

// ResolveLocalRef looks up a managed resource by name in the given namespace,
// checks that it is Ready, and returns its fullyQualifiedName from status.
//
// The factory function creates an empty instance of the correct Go type.
func ResolveLocalRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	factory func() ReferableObject,
) (string, error) {
	obj := factory()

	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	if err := c.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("%w: %s %q in namespace %q", ErrReferenceNotFound, obj.GetObjectKind().GroupVersionKind().Kind, name, namespace)
		}

		return "", fmt.Errorf("fetching reference %q: %w", name, err)
	}

	if !conditions.IsTrue(obj, snowplanev1alpha1.TypeReady) {
		return "", fmt.Errorf("%w: %s %q in namespace %q", ErrReferenceNotReady, obj.GetObjectKind().GroupVersionKind().Kind, name, namespace)
	}

	fqn := obj.GetFullyQualifiedName()
	if fqn == "" {
		return "", fmt.Errorf("%w: %s %q has empty fullyQualifiedName", ErrReferenceNotReady, obj.GetObjectKind().GroupVersionKind().Kind, name)
	}

	return fqn, nil
}

// ResolveDatabaseRef resolves a LocalObjectReference to a Database CR, returning
// the Snowflake fully qualified database name.
func ResolveDatabaseRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	ref snowplanev1alpha1.LocalObjectReference,
) (string, error) {
	return ResolveLocalRef(ctx, c, namespace, ref.Name, func() ReferableObject {
		db := &snowplanev1alpha1.Database{}
		db.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   snowplanev1alpha1.GroupVersion.Group,
			Version: snowplanev1alpha1.GroupVersion.Version,
			Kind:    "Database",
		})

		return db
	})
}

// ResolveSchemaRef resolves a LocalObjectReference to a Schema CR, returning
// the Snowflake fully qualified schema name (e.g. "DB"."SCHEMA").
func ResolveSchemaRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	ref snowplanev1alpha1.LocalObjectReference,
) (string, error) {
	return ResolveLocalRef(ctx, c, namespace, ref.Name, func() ReferableObject {
		s := &snowplanev1alpha1.Schema{}
		s.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   snowplanev1alpha1.GroupVersion.Group,
			Version: snowplanev1alpha1.GroupVersion.Version,
			Kind:    "Schema",
		})

		return s
	})
}

// ResolveAccountRoleRef resolves a LocalObjectReference to an AccountRole CR,
// returning the Snowflake role name from the CR's fullyQualifiedName.
func ResolveAccountRoleRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	ref snowplanev1alpha1.LocalObjectReference,
) (string, error) {
	return ResolveLocalRef(ctx, c, namespace, ref.Name, func() ReferableObject {
		r := &snowplanev1alpha1.AccountRole{}
		r.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   snowplanev1alpha1.GroupVersion.Group,
			Version: snowplanev1alpha1.GroupVersion.Version,
			Kind:    "AccountRole",
		})

		return r
	})
}

// ResolveDatabaseRoleRef resolves a LocalObjectReference to a DatabaseRole CR,
// returning the Snowflake fully qualified database role name.
func ResolveDatabaseRoleRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	ref snowplanev1alpha1.LocalObjectReference,
) (string, error) {
	return ResolveLocalRef(ctx, c, namespace, ref.Name, func() ReferableObject {
		r := &snowplanev1alpha1.DatabaseRole{}
		r.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   snowplanev1alpha1.GroupVersion.Group,
			Version: snowplanev1alpha1.GroupVersion.Version,
			Kind:    "DatabaseRole",
		})

		return r
	})
}

// ResolveUserRef resolves a LocalObjectReference to a User CR,
// returning the Snowflake user name from the CR's fullyQualifiedName.
func ResolveUserRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	ref snowplanev1alpha1.LocalObjectReference,
) (string, error) {
	return ResolveLocalRef(ctx, c, namespace, ref.Name, func() ReferableObject {
		u := &snowplanev1alpha1.User{}
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   snowplanev1alpha1.GroupVersion.Group,
			Version: snowplanev1alpha1.GroupVersion.Version,
			Kind:    "User",
		})

		return u
	})
}

// ResolveSecretKeyRef reads the value at the specified key from a Kubernetes Secret.
func ResolveSecretKeyRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	ref snowplanev1alpha1.SecretKeyReference,
) (string, error) {
	secretNS := ref.Namespace
	if secretNS == "" {
		secretNS = namespace
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{
		Namespace: secretNS,
		Name:      ref.Name,
	}

	if err := c.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("%w: Secret %q in namespace %q", ErrReferenceNotFound, ref.Name, secretNS)
		}

		return "", fmt.Errorf("fetching Secret %q: %w", ref.Name, err)
	}

	data, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("%w: Secret %q does not contain key %q", ErrReferenceNotFound, ref.Name, ref.Key)
	}

	return string(data), nil
}

// ResolveDatabaseSource resolves the database identifier from either a DatabaseRef (CR
// reference) or a raw DatabaseName string. Exactly one must be non-nil.
// When databaseName is used, it is passed through as a simple Snowflake identifier.
func ResolveDatabaseSource(
	ctx context.Context,
	c client.Client,
	namespace string,
	ref *snowplanev1alpha1.LocalObjectReference,
	rawName *string,
) (string, error) {
	if ref != nil {
		return ResolveDatabaseRef(ctx, c, namespace, *ref)
	}

	if rawName != nil && *rawName != "" {
		return *rawName, nil
	}

	return "", ErrNeitherRefNorNameSet
}

// ResolveSchemaSource resolves the schema identifier from either a SchemaRef (CR
// reference) or a raw SchemaName string. Exactly one must be non-nil.
// When schemaName is used, it should be a simple identifier (e.g. "PUBLIC").
// The controller constructs the FQN from databaseName + schemaName + name.
func ResolveSchemaSource(
	ctx context.Context,
	c client.Client,
	namespace string,
	ref *snowplanev1alpha1.LocalObjectReference,
	rawName *string,
) (string, error) {
	if ref != nil {
		return ResolveSchemaRef(ctx, c, namespace, *ref)
	}

	if rawName != nil && *rawName != "" {
		return *rawName, nil
	}

	return "", ErrNeitherRefNorNameSet
}

// SourceName returns a human-readable display name for log/event messages
// describing whether a reference or an inline name is used. It works for
// both database and schema references (R9-5).
func SourceName(ref *snowplanev1alpha1.LocalObjectReference, name *string) string {
	if ref != nil {
		return fmt.Sprintf("%q (ref)", ref.Name)
	}

	if name != nil {
		return fmt.Sprintf("%q (inline)", *name)
	}

	return "<unset>"
}
