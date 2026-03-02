package grantownership

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
)

const (
	indexAccountRoleRef  = ".arg.refs.accountRoleRef.name"
	indexDatabaseRoleRef = ".arg.refs.databaseRoleRef.name"
)

// adapter implements reconciler.ResourceAdapter for GrantOwnership.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "grantownership" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.GrantOwnership {
	return &snowplanev1alpha1.GrantOwnership{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, g *snowplanev1alpha1.GrantOwnership) error {
	// Resolve AccountRoleRef.
	if ref := g.Spec.AccountRoleRef; ref != nil {
		name, err := refresolver.ResolveAccountRoleRef(ctx, a.client, g.Namespace, *ref)
		if err != nil {
			a.recorder.Eventf(g, corev1.EventTypeWarning, snowplanev1alpha1.ReasonRefResolutionFailed,
				"AccountRole ref %q resolution failed: %v", ref.Name, err)
			return err
		}

		g.Spec.AccountRole = &name
		g.Spec.AccountRoleRef = nil
	}

	// Resolve DatabaseRoleRef.
	if ref := g.Spec.DatabaseRoleRef; ref != nil {
		name, err := refresolver.ResolveDatabaseRoleRef(ctx, a.client, g.Namespace, *ref)
		if err != nil {
			a.recorder.Eventf(g, corev1.EventTypeWarning, snowplanev1alpha1.ReasonRefResolutionFailed,
				"DatabaseRole ref %q resolution failed: %v", ref.Name, err)
			return err
		}

		g.Spec.DatabaseRole = &name
		g.Spec.DatabaseRoleRef = nil
	}

	return nil
}

func (a *adapter) BuildIdentifier(g *snowplanev1alpha1.GrantOwnership) (reconciler.Identifier, error) {
	// Defense-in-depth: validate object type (CRD already validates).
	if err := sqlbuilder.ValidateObjectType(g.Spec.ObjectType); err != nil {
		return nil, fmt.Errorf("grantownership.BuildIdentifier: %w", err)
	}

	return snowflake.GrantOwnershipIdentifier{
		ObjectType:  g.Spec.ObjectType,
		ObjectName:  g.Spec.ObjectName, // pre-quoted from CRD; validated during Create
		GranteeName: resolveGranteeName(g),
	}, nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(ctx, &snowplanev1alpha1.GrantOwnership{}, indexAccountRoleRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.GrantOwnership)
				if !ok || g.Spec.AccountRoleRef == nil {
					return nil
				}

				return []string{g.Spec.AccountRoleRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", indexAccountRoleRef, err)
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &snowplanev1alpha1.GrantOwnership{}, indexDatabaseRoleRef,
			func(o sigs.Object) []string {
				g, ok := o.(*snowplanev1alpha1.GrantOwnership)
				if !ok || g.Spec.DatabaseRoleRef == nil {
					return nil
				}

				return []string{g.Spec.DatabaseRoleRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for %s: %w", indexDatabaseRoleRef, err)
		}

		bldr.Watches(
			&snowplanev1alpha1.AccountRole{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client,
				func() sigs.ObjectList { return &snowplanev1alpha1.GrantOwnershipList{} },
				indexAccountRoleRef, "listing grant ownerships for account role watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.DatabaseRole{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client,
				func() sigs.ObjectList { return &snowplanev1alpha1.GrantOwnershipList{} },
				indexDatabaseRoleRef, "listing grant ownerships for database role watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.GrantOwnershipObservation], error) {
	oid, err := reconciler.AssertIdentifier[snowflake.GrantOwnershipIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, oid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.GrantOwnershipObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.GrantOwnership, _ reconciler.Identifier) error {
	toRole := buildToRole(obj)

	var currentGrantsBehavior string
	if obj.Spec.CurrentGrantsBehavior != nil {
		currentGrantsBehavior = string(*obj.Spec.CurrentGrantsBehavior)
	}

	opts := snowflake.CreateGrantOwnershipOptions{
		ObjectType:            obj.Spec.ObjectType,
		ObjectName:            obj.Spec.ObjectName,
		ToRole:                toRole,
		CurrentGrantsBehavior: currentGrantsBehavior,
	}

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(_ context.Context, _ Service, _ reconciler.AlterOptions) error {
	// Ownership transfers are immutable — all fields are validated as immutable.
	return nil
}

func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	oid, err := reconciler.AssertIdentifier[snowflake.GrantOwnershipIdentifier](id)
	if err != nil {
		return err
	}

	// Drop is a no-op — ownership cannot be revoked, only transferred.
	return svc.Drop(ctx, oid)
}

func (a *adapter) ValidateImmutableFields(_ context.Context, g *snowplanev1alpha1.GrantOwnership) error {
	if reconciler.ShouldSkipImmutableValidation(g) {
		return nil
	}

	if g.Status.ShowOutput != nil {
		granteeName := resolveGranteeName(g)
		if g.Status.ShowOutput.GranteeName != "" && granteeName != "" {
			if g.Status.ShowOutput.GranteeName != granteeName {
				return fmt.Errorf("ownership target is immutable after creation (current: %q, desired: %q)",
					g.Status.ShowOutput.GranteeName, granteeName)
			}
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, _ *snowplanev1alpha1.GrantOwnership, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.GrantOwnershipObservation]) (reconciler.AlterOptions, error) {
	// Return no-change alter options — all fields are immutable.
	return &ownershipAlterOptions{}, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.GrantOwnership, obs *reconciler.Observation[*snowflake.GrantOwnershipObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(_ *snowplanev1alpha1.GrantOwnership) []string {
	return nil
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.GrantOwnership, obs *reconciler.Observation[*snowflake.GrantOwnershipObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.GrantOwnership, Service, *snowflake.GrantOwnershipObservation] = (*adapter)(nil)
