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
	accountRoleAssignmentFinalizer = "snowplane.hupe1980.github.io/accountroleassignment"
)

// Field index keys for AccountRoleAssignment cross-resource watches.
const (
	araIndexRoleRef   = ".ara.refs.roleRef.name"
	araIndexToRoleRef = ".ara.refs.toRoleRef.name"
	araIndexToUserRef = ".ara.refs.toUserRef.name"
)

// accountRoleAssignmentAdapter implements reconciler.ResourceAdapter for AccountRoleAssignment.
type accountRoleAssignmentAdapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.AccountRoleAssignment, Service, *snowflake.RoleAssignmentObservation] = (*accountRoleAssignmentAdapter)(nil)

func (a *accountRoleAssignmentAdapter) ResourceName() string { return "accountroleassignment" }
func (a *accountRoleAssignmentAdapter) FinalizerName() string {
	return accountRoleAssignmentFinalizer
}

func (a *accountRoleAssignmentAdapter) NewObject() *snowplanev1alpha1.AccountRoleAssignment {
	return &snowplanev1alpha1.AccountRoleAssignment{}
}

func (a *accountRoleAssignmentAdapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

// PreReconcile resolves optional CR references.
func (a *accountRoleAssignmentAdapter) PreReconcile(ctx context.Context, obj *snowplanev1alpha1.AccountRoleAssignment) error {
	logger := log.FromContext(ctx)
	hasRefs := false

	// Resolve RoleRef → RoleName.
	if ref := obj.Spec.RoleRef; ref != nil {
		hasRefs = true

		name, err := refresolver.ResolveAccountRoleRef(ctx, a.client, obj.Namespace, *ref)
		if err != nil {
			return refresolver.HandleRefError(ctx, obj, a.recorder, "AccountRole", ref.Name, err)
		}

		obj.Spec.RoleName = &name
		obj.Spec.RoleRef = nil
	}

	// Resolve ToRoleRef → ToRole.
	if ref := obj.Spec.ToRoleRef; ref != nil {
		hasRefs = true

		name, err := refresolver.ResolveAccountRoleRef(ctx, a.client, obj.Namespace, *ref)
		if err != nil {
			return refresolver.HandleRefError(ctx, obj, a.recorder, "AccountRole", ref.Name, err)
		}

		obj.Spec.ToRole = &name
		obj.Spec.ToRoleRef = nil
	}

	// Resolve ToUserRef → ToUser.
	if ref := obj.Spec.ToUserRef; ref != nil {
		hasRefs = true

		name, err := refresolver.ResolveUserRef(ctx, a.client, obj.Namespace, *ref)
		if err != nil {
			return refresolver.HandleRefError(ctx, obj, a.recorder, "User", ref.Name, err)
		}

		obj.Spec.ToUser = &name
		obj.Spec.ToUserRef = nil
	}

	if hasRefs {
		conditions.SetReferencesResolved(obj, "all references resolved")
		logger.V(1).Info("accountroleassignment references resolved")
	}

	return nil
}

// BuildIdentifier constructs a RoleAssignmentIdentifier from the spec.
func (a *accountRoleAssignmentAdapter) BuildIdentifier(obj *snowplanev1alpha1.AccountRoleAssignment) (reconciler.Identifier, error) {
	grantedTo, granteeName := resolveAccountTarget(obj)
	if grantedTo == "" || granteeName == "" {
		return nil, fmt.Errorf("exactly one of toRole, toRoleRef, toUser, or toUserRef must be set")
	}

	return snowflake.RoleAssignmentIdentifier{
		RoleName:       derefStr(obj.Spec.RoleName),
		IsDatabaseRole: false,
		GrantedTo:      grantedTo,
		GranteeName:    granteeName,
	}, nil
}

// SetupWatches configures field indexers and cross-resource watches.
func (a *accountRoleAssignmentAdapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		obj := &snowplanev1alpha1.AccountRoleAssignment{}

		// RoleRef indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(ctx, obj, araIndexRoleRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.AccountRoleAssignment)
				if !ok {
					return nil
				}
				if ref := g.Spec.RoleRef; ref != nil {
					return []string{ref.Name}
				}
				return nil
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", araIndexRoleRef, err)
		}

		// ToRoleRef indexer (must be registered before the combined AccountRole watch).
		if err := mgr.GetFieldIndexer().IndexField(ctx, obj, araIndexToRoleRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.AccountRoleAssignment)
				if !ok {
					return nil
				}
				if ref := g.Spec.ToRoleRef; ref != nil {
					return []string{ref.Name}
				}
				return nil
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", araIndexToRoleRef, err)
		}

		// Single AccountRole watch that queries BOTH RoleRef and ToRoleRef indexes.
		bldr.Watches(
			&snowplanev1alpha1.AccountRole{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj sigs.Object) []reconcile.Request {
				return a.listByIndexes(ctx, obj, araIndexRoleRef, araIndexToRoleRef)
			}),
		)

		// ToUserRef indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(ctx, obj, araIndexToUserRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.AccountRoleAssignment)
				if !ok {
					return nil
				}
				if ref := g.Spec.ToUserRef; ref != nil {
					return []string{ref.Name}
				}
				return nil
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", araIndexToUserRef, err)
		}

		bldr.Watches(
			&snowplanev1alpha1.User{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj sigs.Object) []reconcile.Request {
				return a.listByIndex(ctx, obj, araIndexToUserRef)
			}),
		)

		return nil
	}
}

func (a *accountRoleAssignmentAdapter) listByIndex(ctx context.Context, obj sigs.Object, indexKey string) []reconcile.Request {
	logger := log.FromContext(ctx)

	list := &snowplanev1alpha1.AccountRoleAssignmentList{}
	if err := a.client.List(ctx, list,
		sigs.InNamespace(obj.GetNamespace()),
		sigs.MatchingFields{indexKey: obj.GetName()},
	); err != nil {
		logger.Error(err, "listing accountroleassignments for watch", "indexKey", indexKey)
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
func (a *accountRoleAssignmentAdapter) listByIndexes(ctx context.Context, obj sigs.Object, indexKeys ...string) []reconcile.Request {
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
func (a *accountRoleAssignmentAdapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.RoleAssignmentObservation], error) {
	return roleAssignmentObserve(ctx, svc, id)
}

// Create grants the role in Snowflake.
func (a *accountRoleAssignmentAdapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.AccountRoleAssignment, _ reconciler.Identifier) error {
	opts := snowflake.GrantRoleOptions{
		RoleName:       derefStr(obj.Spec.RoleName),
		IsDatabaseRole: false,
		ToRole:         derefStr(obj.Spec.ToRole),
		ToUser:         derefStr(obj.Spec.ToUser),
	}

	return svc.GrantRole(ctx, opts)
}

// Alter is a no-op for role assignments — they are immutable.
func (a *accountRoleAssignmentAdapter) Alter(_ context.Context, _ Service, _ reconciler.AlterOptions) error {
	return nil
}

// Drop revokes the role assignment.
func (a *accountRoleAssignmentAdapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	return roleAssignmentDrop(ctx, svc, id)
}

// ValidateImmutableFields checks immutability.
func (a *accountRoleAssignmentAdapter) ValidateImmutableFields(_ context.Context, obj *snowplanev1alpha1.AccountRoleAssignment) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	// Role assignments are fully immutable — CEL rules enforce this on all fields.
	// No additional runtime validation needed beyond what CEL provides.
	return nil
}

// BuildAlterOptions returns no-change options since role assignments don't support ALTER.
func (a *accountRoleAssignmentAdapter) BuildAlterOptions(_ context.Context, _ *snowplanev1alpha1.AccountRoleAssignment, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.RoleAssignmentObservation]) (reconciler.AlterOptions, error) {
	return roleAssignmentAlterOptions{}, nil
}

// ApplyObservation maps the observation into the CR's status.
func (a *accountRoleAssignmentAdapter) ApplyObservation(obj *snowplanev1alpha1.AccountRoleAssignment, obs *reconciler.Observation[*snowflake.RoleAssignmentObservation]) {
	raObs := obs.Detail
	if raObs.ShowOutput != nil {
		grantedTo, granteeName := resolveAccountTarget(obj)
		id := snowflake.RoleAssignmentIdentifier{
			RoleName:       derefStr(obj.Spec.RoleName),
			IsDatabaseRole: false,
			GrantedTo:      grantedTo,
			GranteeName:    granteeName,
		}
		obj.Status.FullyQualifiedName = id.FullyQualifiedName()
		obj.Status.ShowOutput = applyRoleAssignmentShowOutput(raObs)
	}
}

// ComputeTrackedParameters returns nil — role assignments don't track parameters.
func (a *accountRoleAssignmentAdapter) ComputeTrackedParameters(_ *snowplanev1alpha1.AccountRoleAssignment) []string {
	return nil
}

// DetectDrift compares spec vs observation.
func (a *accountRoleAssignmentAdapter) DetectDrift(obj *snowplanev1alpha1.AccountRoleAssignment, obs *reconciler.Observation[*snowflake.RoleAssignmentObservation]) *drift.Result {
	grantedTo, granteeName := resolveAccountTarget(obj)
	return detectRoleAssignmentDrift(grantedTo, granteeName, obs.Detail)
}

// resolveAccountTarget determines the grantedTo category and grantee name from the spec.
func resolveAccountTarget(obj *snowplanev1alpha1.AccountRoleAssignment) (grantedTo, granteeName string) {
	if obj.Spec.ToRole != nil {
		return "ROLE", *obj.Spec.ToRole
	}

	if obj.Spec.ToUser != nil {
		return "USER", *obj.Spec.ToUser
	}

	return "", ""
}
