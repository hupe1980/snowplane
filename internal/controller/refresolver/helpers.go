package refresolver

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// ConditionedClientObject combines client.Object and conditions.ConditionedObject
// so helpers can both emit events and set conditions.
type ConditionedClientObject interface {
	client.Object
	conditions.ConditionedObject
}

// HandleRefError sets ReferencesNotResolved + NotReady conditions and emits a
// warning event. It is the shared error-handling boilerplate previously
// duplicated in every adapter's resolveDatabaseRef / resolveSchemaRef.
func HandleRefError(obj ConditionedClientObject, recorder record.EventRecorder, kindLabel, sourceName string, err error) {
	msg := fmt.Sprintf("%s %s: %v", kindLabel, sourceName, err)
	conditions.SetReferencesNotResolved(obj, snowplanev1alpha1.ReasonDependencyNotReady, msg)
	conditions.SetNotReady(obj, snowplanev1alpha1.ReasonDependencyWait, msg)
	recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonDependencyNotReady, msg)
}

// ResolveDatabaseRefWithConditions resolves a database reference and, on error,
// sets the standard ReferencesNotResolved + NotReady conditions and emits a
// warning event. On success the caller is responsible for setting conditions.
func ResolveDatabaseRefWithConditions(
	ctx context.Context,
	c client.Client,
	recorder record.EventRecorder,
	obj ConditionedClientObject,
	namespace string,
	ref *snowplanev1alpha1.LocalObjectReference,
	rawName *string,
) (string, error) {
	dbFQN, err := ResolveDatabaseSource(ctx, c, namespace, ref, rawName)
	if err != nil {
		refName := SourceName(ref, rawName)
		HandleRefError(obj, recorder, "Database", refName, err)

		return "", err
	}

	return dbFQN, nil
}

// ResolveSchemaRefWithConditions resolves a schema reference and, on error,
// sets the standard ReferencesNotResolved + NotReady conditions and emits a
// warning event. On success the caller is responsible for setting conditions.
func ResolveSchemaRefWithConditions(
	ctx context.Context,
	c client.Client,
	recorder record.EventRecorder,
	obj ConditionedClientObject,
	namespace string,
	ref *snowplanev1alpha1.LocalObjectReference,
	rawName *string,
) (string, error) {
	schemaFQN, err := ResolveSchemaSource(ctx, c, namespace, ref, rawName)
	if err != nil {
		refName := SourceName(ref, rawName)
		HandleRefError(obj, recorder, "Schema", refName, err)

		return "", err
	}

	return schemaFQN, nil
}

// SetDatabaseResolvedCondition sets a ReferencesResolved condition with a
// message indicating that a single database reference was resolved.
// Used by Schema and DatabaseRole adapters.
func SetDatabaseResolvedCondition(obj conditions.ConditionedObject, ref *snowplanev1alpha1.LocalObjectReference, rawName *string, dbFQN string) {
	refName := SourceName(ref, rawName)
	conditions.SetReferencesResolved(obj, fmt.Sprintf("Database %s resolved to %s", refName, dbFQN))
}

// SetDatabaseAndSchemaResolvedCondition sets a ReferencesResolved condition with
// a message indicating that both database and schema references were resolved.
// Used by Table, View, and Stage adapters.
func SetDatabaseAndSchemaResolvedCondition(
	obj conditions.ConditionedObject,
	dbRef *snowplanev1alpha1.LocalObjectReference, dbRawName *string,
	schemaRef *snowplanev1alpha1.LocalObjectReference, schemaRawName *string,
) {
	dbName := SourceName(dbRef, dbRawName)
	schName := SourceName(schemaRef, schemaRawName)
	conditions.SetReferencesResolved(obj, fmt.Sprintf("Database %s and Schema %s resolved", dbName, schName))
}

// PreReconcileDatabaseRef resolves a database reference and handles the
// deletion-timestamp fallback. When the ref cannot be resolved during a
// delete (DeletionTimestamp is set), the cached status database name is used
// instead so that the resource can still be dropped from Snowflake. On success
// it stores the resolved FQN in the status and sets conditions. Returns the
// resolved database FQN or an error that should be returned from PreReconcile.
func PreReconcileDatabaseRef(
	ctx context.Context,
	c client.Client,
	recorder record.EventRecorder,
	obj ConditionedClientObject,
	namespace string,
	dbRef *snowplanev1alpha1.LocalObjectReference,
	dbName *string,
	cachedDBName string,
) (string, error) {
	logger := log.FromContext(ctx)

	dbFQN, err := ResolveDatabaseRefWithConditions(ctx, c, recorder, obj, namespace, dbRef, dbName)
	if err != nil {
		if !obj.GetDeletionTimestamp().IsZero() && cachedDBName != "" {
			conditions.SetReferencesResolved(obj,
				fmt.Sprintf("Using cached database name %q for deletion", cachedDBName))
			logger.Info("database reference not resolved during deletion, using cached status.databaseName",
				"databaseName", cachedDBName, "error", err)

			return cachedDBName, nil
		}

		if errors.Is(err, ErrReferenceNotFound) || errors.Is(err, ErrReferenceNotReady) {
			logger.Info("database reference not resolved, requeuing", "error", err)
		}

		return "", err
	}

	return dbFQN, nil
}

// PreReconcileSchemaRef resolves a schema reference and handles the
// deletion-timestamp fallback, analogous to PreReconcileDatabaseRef.
func PreReconcileSchemaRef(
	ctx context.Context,
	c client.Client,
	recorder record.EventRecorder,
	obj ConditionedClientObject,
	namespace string,
	schemaRef *snowplanev1alpha1.LocalObjectReference,
	schemaName *string,
	cachedSchemaName string,
) (string, error) {
	logger := log.FromContext(ctx)

	schemaFQN, err := ResolveSchemaRefWithConditions(ctx, c, recorder, obj, namespace, schemaRef, schemaName)
	if err != nil {
		if !obj.GetDeletionTimestamp().IsZero() && cachedSchemaName != "" {
			conditions.SetReferencesResolved(obj,
				fmt.Sprintf("Using cached schema name %q for deletion", cachedSchemaName))
			logger.Info("schema reference not resolved during deletion, using cached status.schemaName",
				"schemaName", cachedSchemaName, "error", err)

			return cachedSchemaName, nil
		}

		if errors.Is(err, ErrReferenceNotFound) || errors.Is(err, ErrReferenceNotReady) {
			logger.Info("schema reference not resolved, requeuing", "error", err)
		}

		return "", err
	}

	return schemaFQN, nil
}

// MapByFieldIndex returns a handler.MapFunc that maps changes on a watched
// resource (e.g. Database) to all dependent resources indexed by fieldPath.
// The listFactory must return a new, empty list instance on every call
// (e.g. func() client.ObjectList { return &v1alpha1.TableList{} }).
func MapByFieldIndex(c client.Client, listFactory func() client.ObjectList, fieldPath string, logMsg string) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		logger := log.FromContext(ctx)

		list := listFactory()
		if err := c.List(ctx, list,
			client.InNamespace(obj.GetNamespace()),
			client.MatchingFields{fieldPath: obj.GetName()},
		); err != nil {
			logger.Error(err, logMsg)
			return nil
		}

		items, err := meta.ExtractList(list)
		if err != nil {
			logger.Error(err, "extracting list items")
			return nil
		}

		requests := make([]reconcile.Request, 0, len(items))
		for _, item := range items {
			o := item.(client.Object)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: o.GetNamespace(),
					Name:      o.GetName(),
				},
			})
		}

		return requests
	}
}
