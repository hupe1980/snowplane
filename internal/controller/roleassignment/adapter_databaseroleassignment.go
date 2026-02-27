package roleassignment

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

const (
	databaseRoleAssignmentFinalizer = "snowplane.hupe1980.github.io/databaseroleassignment"
)

// Field index keys for DatabaseRoleAssignment cross-resource watches.
const (
	draIndexDatabaseRoleRef   = ".dra.refs.databaseRoleRef.name"
	draIndexToRoleRef         = ".dra.refs.toRoleRef.name"
	draIndexToDatabaseRoleRef = ".dra.refs.toDatabaseRoleRef.name"
)

// databaseRoleAssignmentAdapter implements reconciler.ResourceAdapter for DatabaseRoleAssignment.
type databaseRoleAssignmentAdapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.DatabaseRoleAssignment, Service, *snowflake.RoleAssignmentObservation] = (*databaseRoleAssignmentAdapter)(nil)

func (a *databaseRoleAssignmentAdapter) ResourceName() string { return "databaseroleassignment" }
func (a *databaseRoleAssignmentAdapter) FinalizerName() string {
	return databaseRoleAssignmentFinalizer
}

func (a *databaseRoleAssignmentAdapter) NewObject() *snowplanev1alpha1.DatabaseRoleAssignment {
	return &snowplanev1alpha1.DatabaseRoleAssignment{}
}

func (a *databaseRoleAssignmentAdapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

// PreReconcile resolves optional CR references.
func (a *databaseRoleAssignmentAdapter) PreReconcile(ctx context.Context, obj *snowplanev1alpha1.DatabaseRoleAssignment) error {
	logger := log.FromContext(ctx)
	hasRefs := false

	// Resolve DatabaseRoleRef → DatabaseRoleName.
	if ref := obj.Spec.DatabaseRoleRef; ref != nil {
		hasRefs = true

		fqn, err := refresolver.ResolveDatabaseRoleRef(ctx, a.client, obj.Namespace, *ref)
		if err != nil {
			return refresolver.HandleRefError(ctx, obj, a.recorder, "DatabaseRole", ref.Name, err)
		}

		obj.Spec.DatabaseRoleName = fqn
		obj.Spec.DatabaseRoleRef = nil
	}

	// Resolve ToRoleRef → ToRole.
	if ref := obj.Spec.ToRoleRef; ref != nil {
		hasRefs = true

		name, err := refresolver.ResolveAccountRoleRef(ctx, a.client, obj.Namespace, *ref)
		if err != nil {
			return refresolver.HandleRefError(ctx, obj, a.recorder, "AccountRole", ref.Name, err)
		}

		obj.Spec.ToRole = name
		obj.Spec.ToRoleRef = nil
	}

	// Resolve ToDatabaseRoleRef → ToDatabaseRole.
	if ref := obj.Spec.ToDatabaseRoleRef; ref != nil {
		hasRefs = true

		fqn, err := refresolver.ResolveDatabaseRoleRef(ctx, a.client, obj.Namespace, *ref)
		if err != nil {
			return refresolver.HandleRefError(ctx, obj, a.recorder, "DatabaseRole", ref.Name, err)
		}

		obj.Spec.ToDatabaseRole = fqn
		obj.Spec.ToDatabaseRoleRef = nil
	}

	if hasRefs {
		conditions.SetReferencesResolved(obj, "all references resolved")
		logger.V(1).Info("databaseroleassignment references resolved")
	}

	return nil
}

// BuildIdentifier constructs a RoleAssignmentIdentifier from the spec.
func (a *databaseRoleAssignmentAdapter) BuildIdentifier(obj *snowplanev1alpha1.DatabaseRoleAssignment) (reconciler.Identifier, error) {
	grantedTo, granteeName := resolveDatabaseTarget(obj)
	if grantedTo == "" || granteeName == "" {
		return nil, fmt.Errorf("exactly one of toRole, toRoleRef, toDatabaseRole, or toDatabaseRoleRef must be set")
	}

	return snowflake.RoleAssignmentIdentifier{
		RoleName:       obj.Spec.DatabaseRoleName,
		IsDatabaseRole: true,
		GrantedTo:      grantedTo,
		GranteeName:    granteeName,
	}, nil
}

// SetupWatches configures field indexers and cross-resource watches.
func (a *databaseRoleAssignmentAdapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		obj := &snowplanev1alpha1.DatabaseRoleAssignment{}

		// DatabaseRoleRef indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(ctx, obj, draIndexDatabaseRoleRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.DatabaseRoleAssignment)
				if !ok {
					return nil
				}
				if ref := g.Spec.DatabaseRoleRef; ref != nil {
					return []string{ref.Name}
				}
				return nil
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", draIndexDatabaseRoleRef, err)
		}

		// ToDatabaseRoleRef indexer (must be registered before the combined DatabaseRole watch).
		if err := mgr.GetFieldIndexer().IndexField(ctx, obj, draIndexToDatabaseRoleRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.DatabaseRoleAssignment)
				if !ok {
					return nil
				}
				if ref := g.Spec.ToDatabaseRoleRef; ref != nil {
					return []string{ref.Name}
				}
				return nil
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", draIndexToDatabaseRoleRef, err)
		}

		// Single DatabaseRole watch that queries BOTH DatabaseRoleRef and ToDatabaseRoleRef indexes.
		bldr.Watches(
			&snowplanev1alpha1.DatabaseRole{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj sigs.Object) []reconcile.Request {
				return a.listByIndexes(ctx, obj, draIndexDatabaseRoleRef, draIndexToDatabaseRoleRef)
			}),
		)

		// ToRoleRef indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(ctx, obj, draIndexToRoleRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.DatabaseRoleAssignment)
				if !ok {
					return nil
				}
				if ref := g.Spec.ToRoleRef; ref != nil {
					return []string{ref.Name}
				}
				return nil
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", draIndexToRoleRef, err)
		}

		bldr.Watches(
			&snowplanev1alpha1.AccountRole{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj sigs.Object) []reconcile.Request {
				return a.listByIndex(ctx, obj, draIndexToRoleRef)
			}),
		)

		return nil
	}
}

func (a *databaseRoleAssignmentAdapter) listByIndex(ctx context.Context, obj sigs.Object, indexKey string) []reconcile.Request {
	logger := log.FromContext(ctx)

	list := &snowplanev1alpha1.DatabaseRoleAssignmentList{}
	if err := a.client.List(ctx, list,
		sigs.InNamespace(obj.GetNamespace()),
		sigs.MatchingFields{indexKey: obj.GetName()},
	); err != nil {
		logger.Error(err, "listing databaseroleassignments for watch", "indexKey", indexKey)
		return nil
	}

	requests := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: list.Items[i].Namespace,
				Name:      list.Items[i].Name,
			},
		})
	}

	return requests
}

// listByIndexes queries multiple field indexes for the same object and merges
// the results, deduplicating by NamespacedName.
func (a *databaseRoleAssignmentAdapter) listByIndexes(ctx context.Context, obj sigs.Object, indexKeys ...string) []reconcile.Request {
	seen := make(map[types.NamespacedName]struct{})
	var merged []reconcile.Request

	for _, key := range indexKeys {
		for _, req := range a.listByIndex(ctx, obj, key) {
			if _, ok := seen[req.NamespacedName]; !ok {
				seen[req.NamespacedName] = struct{}{}
				merged = append(merged, req)
			}
		}
	}

	return merged
}

// Observe queries Snowflake for the current state.
func (a *databaseRoleAssignmentAdapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.RoleAssignmentObservation], error) {
	return roleAssignmentObserve(ctx, svc, id)
}

// Create grants the database role in Snowflake.
func (a *databaseRoleAssignmentAdapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.DatabaseRoleAssignment, _ reconciler.Identifier) error {
	opts := snowflake.GrantRoleOptions{
		RoleName:       obj.Spec.DatabaseRoleName,
		IsDatabaseRole: true,
		ToRole:         obj.Spec.ToRole,
		ToDatabaseRole: obj.Spec.ToDatabaseRole,
	}

	return svc.GrantRole(ctx, opts)
}

// Alter is a no-op for role assignments — they are immutable.
func (a *databaseRoleAssignmentAdapter) Alter(_ context.Context, _ Service, _ reconciler.AlterOptions) error {
	return nil
}

// Drop revokes the database role assignment.
func (a *databaseRoleAssignmentAdapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	return roleAssignmentDrop(ctx, svc, id)
}

// ValidateImmutableFields checks immutability.
func (a *databaseRoleAssignmentAdapter) ValidateImmutableFields(_ context.Context, obj *snowplanev1alpha1.DatabaseRoleAssignment) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	// Database role assignments are fully immutable — CEL rules enforce this on all fields.
	return nil
}

// BuildAlterOptions returns no-change options.
func (a *databaseRoleAssignmentAdapter) BuildAlterOptions(_ context.Context, _ *snowplanev1alpha1.DatabaseRoleAssignment, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.RoleAssignmentObservation]) (reconciler.AlterOptions, error) {
	return roleAssignmentAlterOptions{}, nil
}

// ApplyObservation maps the observation into the CR's status.
func (a *databaseRoleAssignmentAdapter) ApplyObservation(obj *snowplanev1alpha1.DatabaseRoleAssignment, obs *reconciler.Observation[*snowflake.RoleAssignmentObservation]) {
	raObs := obs.Detail
	if raObs.ShowOutput != nil {
		grantedTo, granteeName := resolveDatabaseTarget(obj)
		id := snowflake.RoleAssignmentIdentifier{
			RoleName:       obj.Spec.DatabaseRoleName,
			IsDatabaseRole: true,
			GrantedTo:      grantedTo,
			GranteeName:    granteeName,
		}
		obj.Status.FullyQualifiedName = id.FullyQualifiedName()
		obj.Status.ShowOutput = applyRoleAssignmentShowOutput(raObs)
	}
}

// ComputeTrackedParameters returns nil.
func (a *databaseRoleAssignmentAdapter) ComputeTrackedParameters(_ *snowplanev1alpha1.DatabaseRoleAssignment) []string {
	return nil
}

// DetectDrift compares spec vs observation.
func (a *databaseRoleAssignmentAdapter) DetectDrift(obj *snowplanev1alpha1.DatabaseRoleAssignment, obs *reconciler.Observation[*snowflake.RoleAssignmentObservation]) *drift.Result {
	grantedTo, granteeName := resolveDatabaseTarget(obj)
	return detectRoleAssignmentDrift(grantedTo, granteeName, obs.Detail)
}

// resolveDatabaseTarget determines the grantedTo category and grantee name from the spec.
func resolveDatabaseTarget(obj *snowplanev1alpha1.DatabaseRoleAssignment) (grantedTo, granteeName string) {
	if obj.Spec.ToRole != "" {
		return "ROLE", obj.Spec.ToRole
	}

	if obj.Spec.ToDatabaseRole != "" {
		return "DATABASE_ROLE", obj.Spec.ToDatabaseRole
	}

	return "", ""
}
