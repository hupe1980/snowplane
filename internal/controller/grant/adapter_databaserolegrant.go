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
	databaseRoleGrantFinalizer = "snowplane.hupe1980.github.io/databaserolegrant"
)

// Field index keys for DatabaseRoleGrant cross-resource watches.
const (
	drgIndexDatabaseRoleRef = ".drg.refs.databaseRoleRef.name"
	drgIndexSchemaRef       = ".drg.refs.schemaRef.name"
	drgIndexDatabaseRef     = ".drg.refs.databaseRef.name"
)

// databaseRoleGrantAdapter implements reconciler.ResourceAdapter for DatabaseRoleGrant.
type databaseRoleGrantAdapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.DatabaseRoleGrant, Service, *snowflake.GrantObservation] = (*databaseRoleGrantAdapter)(nil)

func (a *databaseRoleGrantAdapter) ResourceName() string  { return "databaserolegrant" }
func (a *databaseRoleGrantAdapter) FinalizerName() string { return databaseRoleGrantFinalizer }
func (a *databaseRoleGrantAdapter) NewObject() *snowplanev1alpha1.DatabaseRoleGrant {
	return &snowplanev1alpha1.DatabaseRoleGrant{}
}

func (a *databaseRoleGrantAdapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *databaseRoleGrantAdapter) SupportsCreateOrAlter() bool { return false }

// PreReconcile resolves optional CR references.
func (a *databaseRoleGrantAdapter) PreReconcile(ctx context.Context, grant *snowplanev1alpha1.DatabaseRoleGrant) error {
	logger := log.FromContext(ctx)
	hasRefs := false

	// Resolve DatabaseRoleRef.
	if ref := grant.Spec.DatabaseRoleRef; ref != nil {
		hasRefs = true

		fqn, err := refresolver.ResolveDatabaseRoleRef(ctx, a.client, grant.Namespace, *ref)
		if err != nil {
			return refresolver.HandleRefError(ctx, grant, a.recorder, "DatabaseRole", ref.Name, err)
		}

		grant.Spec.DatabaseRole = fqn
		grant.Spec.DatabaseRoleRef = nil
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
		logger.V(1).Info("databaserolegrant references resolved")
	}

	return nil
}

// BuildIdentifier constructs a GrantIdentifier from the DatabaseRoleGrant spec.
func (a *databaseRoleGrantAdapter) BuildIdentifier(grant *snowplanev1alpha1.DatabaseRoleGrant) (reconciler.Identifier, error) {
	role := grant.Spec.DatabaseRole
	toClause := snowflake.BuildToClause("", role, "")

	return buildGrantIdentifier(&grant.Spec.On, grant.Spec.Privilege, toClause, role, grant.Spec.ResolveKind(), false)
}

// SetupWatches configures field indexers and cross-resource watches.
func (a *databaseRoleGrantAdapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		grant := &snowplanev1alpha1.DatabaseRoleGrant{}

		// DatabaseRole indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(ctx, grant, drgIndexDatabaseRoleRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.DatabaseRoleGrant)
				if !ok {
					return nil
				}
				if ref := g.Spec.DatabaseRoleRef; ref != nil {
					return []string{ref.Name}
				}
				return nil
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", drgIndexDatabaseRoleRef, err)
		}

		bldr.Watches(
			&snowplanev1alpha1.DatabaseRole{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj sigs.Object) []reconcile.Request {
				return a.listByIndex(ctx, obj, drgIndexDatabaseRoleRef)
			}),
		)

		// Database indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(ctx, grant, drgIndexDatabaseRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.DatabaseRoleGrant)
				if !ok {
					return nil
				}
				return extractDatabaseRefsFromOn(&g.Spec.On)
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", drgIndexDatabaseRef, err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj sigs.Object) []reconcile.Request {
				return a.listByIndex(ctx, obj, drgIndexDatabaseRef)
			}),
		)

		// Schema indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(ctx, grant, drgIndexSchemaRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.DatabaseRoleGrant)
				if !ok {
					return nil
				}
				return extractSchemaRefsFromOn(&g.Spec.On)
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", drgIndexSchemaRef, err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj sigs.Object) []reconcile.Request {
				return a.listByIndex(ctx, obj, drgIndexSchemaRef)
			}),
		)

		return nil
	}
}

func (a *databaseRoleGrantAdapter) listByIndex(ctx context.Context, obj sigs.Object, indexKey string) []reconcile.Request {
	logger := log.FromContext(ctx)

	list := &snowplanev1alpha1.DatabaseRoleGrantList{}
	if err := a.client.List(ctx, list,
		sigs.InNamespace(obj.GetNamespace()),
		sigs.MatchingFields{indexKey: obj.GetName()},
	); err != nil {
		logger.Error(err, "listing databaserolegrants for watch", "indexKey", indexKey)
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
func (a *databaseRoleGrantAdapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.GrantObservation], error) {
	return grantObserve(ctx, svc, id)
}

// Create grants the privilege in Snowflake.
func (a *databaseRoleGrantAdapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.DatabaseRoleGrant, _ reconciler.Identifier) error {
	on := onToParams(&obj.Spec.On)

	onClause, err := snowflake.BuildOnClause(on)
	if err != nil {
		return fmt.Errorf("building ON clause: %w", err)
	}

	opts := snowflake.CreateGrantOptions{
		Privilege:       obj.Spec.Privilege,
		OnClause:        onClause,
		ToClause:        snowflake.BuildToClause("", obj.Spec.DatabaseRole, ""),
		WithGrantOption: obj.Spec.WithGrantOption,
	}

	return svc.Grant(ctx, opts)
}

// Alter is a no-op for grants.
func (a *databaseRoleGrantAdapter) Alter(_ context.Context, _ Service, _ reconciler.AlterOptions) error {
	return nil
}

// Drop revokes the privilege.
func (a *databaseRoleGrantAdapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	return grantDrop(ctx, svc, id)
}

// ValidateImmutableFields checks immutability.
func (a *databaseRoleGrantAdapter) ValidateImmutableFields(_ context.Context, grant *snowplanev1alpha1.DatabaseRoleGrant) error {
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

// BuildAlterOptions returns no-change options.
func (a *databaseRoleGrantAdapter) BuildAlterOptions(_ context.Context, _ *snowplanev1alpha1.DatabaseRoleGrant, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.GrantObservation]) (reconciler.AlterOptions, error) {
	return grantAlterOptions{}, nil
}

// ApplyObservation maps the observation into the CR's status.
func (a *databaseRoleGrantAdapter) ApplyObservation(obj *snowplanev1alpha1.DatabaseRoleGrant, obs *reconciler.Observation[*snowflake.GrantObservation]) {
	grantObs := obs.Detail
	if grantObs.ShowOutput != nil {
		on := onToParams(&obj.Spec.On)

		onClause, err := snowflake.BuildOnClause(on)
		if err == nil {
			toClause := snowflake.BuildToClause("", obj.Spec.DatabaseRole, "")
			obj.Status.FullyQualifiedName = fmt.Sprintf("GRANT %s %s %s", grantObs.ShowOutput.Privilege, onClause, toClause)
		}

		obj.Status.Kind = obj.Spec.ResolveKind()
		obj.Status.ShowOutput = applyGrantShowOutput(grantObs)
	}
}

// ComputeTrackedParameters returns nil.
func (a *databaseRoleGrantAdapter) ComputeTrackedParameters(_ *snowplanev1alpha1.DatabaseRoleGrant) []string {
	return nil
}

// DetectDrift compares spec vs observation.
func (a *databaseRoleGrantAdapter) DetectDrift(obj *snowplanev1alpha1.DatabaseRoleGrant, obs *reconciler.Observation[*snowflake.GrantObservation]) *drift.Result {
	detail := obs.Detail
	return detectGrantDrift(obj.Spec.Privilege, obj.Spec.WithGrantOption, detail)
}

// PostCreate is a no-op.
func (a *databaseRoleGrantAdapter) PostCreate(_ *snowplanev1alpha1.DatabaseRoleGrant) {}

// PostUpdate is a no-op.
func (a *databaseRoleGrantAdapter) PostUpdate(_ *snowplanev1alpha1.DatabaseRoleGrant, _ bool, _ reconciler.AlterOptions) {
}
