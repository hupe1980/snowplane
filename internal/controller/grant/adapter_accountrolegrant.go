package grant

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
	accountRoleGrantFinalizer = "snowplane.hupe1980.github.io/accountrolegrant"
)

// Field index keys for AccountRoleGrant cross-resource watches.
const (
	argIndexAccountRoleRef = ".arg.refs.accountRoleRef.name"
	argIndexSchemaRef      = ".arg.refs.schemaRef.name"
	argIndexDatabaseRef    = ".arg.refs.databaseRef.name"
)

// accountRoleGrantAdapter implements reconciler.ResourceAdapter for AccountRoleGrant.
type accountRoleGrantAdapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.AccountRoleGrant, Service, *snowflake.GrantObservation] = (*accountRoleGrantAdapter)(nil)

func (a *accountRoleGrantAdapter) ResourceName() string  { return "accountrolegrant" }
func (a *accountRoleGrantAdapter) FinalizerName() string { return accountRoleGrantFinalizer }
func (a *accountRoleGrantAdapter) NewObject() *snowplanev1alpha1.AccountRoleGrant {
	return &snowplanev1alpha1.AccountRoleGrant{}
}

func (a *accountRoleGrantAdapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *accountRoleGrantAdapter) SupportsCreateOrAlter() bool { return false }

// PreReconcile resolves optional CR references.
func (a *accountRoleGrantAdapter) PreReconcile(ctx context.Context, grant *snowplanev1alpha1.AccountRoleGrant) error {
	logger := log.FromContext(ctx)
	hasRefs := false

	// Resolve AccountRoleRef.
	if ref := grant.Spec.AccountRoleRef; ref != nil {
		hasRefs = true

		name, err := refresolver.ResolveAccountRoleRef(ctx, a.client, grant.Namespace, *ref)
		if err != nil {
			return refresolver.HandleRefError(ctx, grant, a.recorder, "AccountRole", ref.Name, err)
		}

		grant.Spec.AccountRole = name
		grant.Spec.AccountRoleRef = nil
	}

	// Resolve On refs.
	errHandler := func(kind, name string, err error) error {
		return refresolver.HandleRefError(ctx, grant, a.recorder, kind, name, err)
	}

	if err := resolveOnRefs(ctx, a.client, grant.Namespace, &grant.Spec.On, errHandler); err != nil {
		return err
	}

	if hasRefs || hasOnRefs(&grant.Spec.On) {
		conditions.SetReferencesResolved(grant, "all references resolved")
		logger.V(1).Info("accountrolegrant references resolved")
	}

	return nil
}

// BuildIdentifier constructs a GrantIdentifier from the AccountRoleGrant spec.
func (a *accountRoleGrantAdapter) BuildIdentifier(grant *snowplanev1alpha1.AccountRoleGrant) (reconciler.Identifier, error) {
	role := grant.Spec.AccountRole
	toClause := snowflake.BuildToClause(role, "", "")

	return buildGrantIdentifier(&grant.Spec.On, grant.Spec.Privilege, toClause, role, grant.Spec.ResolveKind(), false)
}

// SetupWatches configures field indexers and cross-resource watches.
func (a *accountRoleGrantAdapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		grant := &snowplanev1alpha1.AccountRoleGrant{}

		// AccountRole indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(ctx, grant, argIndexAccountRoleRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.AccountRoleGrant)
				if !ok {
					return nil
				}
				if ref := g.Spec.AccountRoleRef; ref != nil {
					return []string{ref.Name}
				}
				return nil
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", argIndexAccountRoleRef, err)
		}

		bldr.Watches(
			&snowplanev1alpha1.AccountRole{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj sigs.Object) []reconcile.Request {
				return a.listByIndex(ctx, obj, argIndexAccountRoleRef)
			}),
		)

		// Database indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(ctx, grant, argIndexDatabaseRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.AccountRoleGrant)
				if !ok {
					return nil
				}
				return extractDatabaseRefsFromOn(&g.Spec.On)
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", argIndexDatabaseRef, err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj sigs.Object) []reconcile.Request {
				return a.listByIndex(ctx, obj, argIndexDatabaseRef)
			}),
		)

		// Schema indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(ctx, grant, argIndexSchemaRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.AccountRoleGrant)
				if !ok {
					return nil
				}
				return extractSchemaRefsFromOn(&g.Spec.On)
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", argIndexSchemaRef, err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj sigs.Object) []reconcile.Request {
				return a.listByIndex(ctx, obj, argIndexSchemaRef)
			}),
		)

		return nil
	}
}

func (a *accountRoleGrantAdapter) listByIndex(ctx context.Context, obj sigs.Object, indexKey string) []reconcile.Request {
	logger := log.FromContext(ctx)

	list := &snowplanev1alpha1.AccountRoleGrantList{}
	if err := a.client.List(ctx, list,
		sigs.InNamespace(obj.GetNamespace()),
		sigs.MatchingFields{indexKey: obj.GetName()},
	); err != nil {
		logger.Error(err, "listing accountrolegrants for watch", "indexKey", indexKey)
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

// Observe queries Snowflake for the current state.
func (a *accountRoleGrantAdapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.GrantObservation], error) {
	return grantObserve(ctx, svc, id)
}

// Create grants the privilege in Snowflake.
func (a *accountRoleGrantAdapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.AccountRoleGrant, _ reconciler.Identifier) error {
	on := onToParams(&obj.Spec.On)

	onClause, err := snowflake.BuildOnClause(on)
	if err != nil {
		return fmt.Errorf("building ON clause: %w", err)
	}

	opts := snowflake.CreateGrantOptions{
		Privilege:       obj.Spec.Privilege,
		OnClause:        onClause,
		ToClause:        snowflake.BuildToClause(obj.Spec.AccountRole, "", ""),
		WithGrantOption: obj.Spec.WithGrantOption,
	}

	return svc.Grant(ctx, opts)
}

// Alter is a no-op for grants — they are immutable.
func (a *accountRoleGrantAdapter) Alter(_ context.Context, _ Service, _ reconciler.AlterOptions) error {
	return nil
}

// Drop revokes the privilege.
func (a *accountRoleGrantAdapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	return grantDrop(ctx, svc, id)
}

// ValidateImmutableFields checks immutability.
func (a *accountRoleGrantAdapter) ValidateImmutableFields(_ context.Context, grant *snowplanev1alpha1.AccountRoleGrant) error {
	if reconciler.ShouldSkipImmutableValidation(grant) {
		return nil
	}

	if grant.Status.ShowOutput != nil {
		if grant.Status.ShowOutput.Privilege != "" &&
			!caseInsensitiveEqual(grant.Spec.Privilege, grant.Status.ShowOutput.Privilege) {
			return fmt.Errorf("spec.privilege is immutable after creation (current: %q, desired: %q)",
				grant.Status.ShowOutput.Privilege, grant.Spec.Privilege)
		}

		if grant.Spec.WithGrantOption != grant.Status.ShowOutput.GrantOption {
			return fmt.Errorf("spec.withGrantOption is immutable after creation (current: %v, desired: %v)",
				grant.Status.ShowOutput.GrantOption, grant.Spec.WithGrantOption)
		}
	}

	return nil
}

// BuildAlterOptions returns no-change options since grants don't support ALTER.
func (a *accountRoleGrantAdapter) BuildAlterOptions(_ context.Context, _ *snowplanev1alpha1.AccountRoleGrant, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.GrantObservation]) (reconciler.AlterOptions, error) {
	return grantAlterOptions{}, nil
}

// ApplyObservation maps the observation into the CR's status.
func (a *accountRoleGrantAdapter) ApplyObservation(obj *snowplanev1alpha1.AccountRoleGrant, obs *reconciler.Observation[*snowflake.GrantObservation]) {
	grantObs := obs.Detail
	if grantObs.ShowOutput != nil {
		on := onToParams(&obj.Spec.On)

		onClause, err := snowflake.BuildOnClause(on)
		if err == nil {
			toClause := snowflake.BuildToClause(obj.Spec.AccountRole, "", "")
			obj.Status.FullyQualifiedName = fmt.Sprintf("GRANT %s %s %s", grantObs.ShowOutput.Privilege, onClause, toClause)
		}

		obj.Status.Kind = obj.Spec.ResolveKind()
		obj.Status.ShowOutput = applyGrantShowOutput(grantObs)
	}
}

// ComputeTrackedParameters returns nil — grants don't track parameters.
func (a *accountRoleGrantAdapter) ComputeTrackedParameters(_ *snowplanev1alpha1.AccountRoleGrant) []string {
	return nil
}

// DetectDrift compares spec vs observation.
func (a *accountRoleGrantAdapter) DetectDrift(obj *snowplanev1alpha1.AccountRoleGrant, obs *reconciler.Observation[*snowflake.GrantObservation]) *drift.Result {
	detail := obs.Detail
	return detectGrantDrift(obj.Spec.Privilege, obj.Spec.WithGrantOption, detail)
}

// PostCreate is a no-op.
func (a *accountRoleGrantAdapter) PostCreate(_ *snowplanev1alpha1.AccountRoleGrant) {}

// PostUpdate is a no-op.
func (a *accountRoleGrantAdapter) PostUpdate(_ *snowplanev1alpha1.AccountRoleGrant, _ bool, _ reconciler.AlterOptions) {
}

// hasOnRefs checks if the GrantOn hierarchy contains any unresolved refs.
func hasOnRefs(on *snowplanev1alpha1.GrantOn) bool {
	if s := on.Schema; s != nil {
		if s.SchemaRef != nil || s.AllInDatabaseRef != nil || s.FutureInDatabaseRef != nil {
			return true
		}
	}

	if so := on.SchemaObject; so != nil {
		if bulk := so.All; bulk != nil {
			if bulk.InDatabaseRef != nil || bulk.InSchemaRef != nil {
				return true
			}
		}

		if bulk := so.Future; bulk != nil {
			if bulk.InDatabaseRef != nil || bulk.InSchemaRef != nil {
				return true
			}
		}
	}

	return false
}
