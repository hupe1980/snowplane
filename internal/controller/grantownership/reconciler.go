// Package grantownership implements the reconciler for GrantOwnership resources.
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
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
)

const (
	finalizerName        = "snowplane.hupe1980.github.io/grantownership"
	indexAccountRoleRef  = ".arg.refs.accountRoleRef.name"
	indexDatabaseRoleRef = ".arg.refs.databaseRoleRef.name"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake ownership transfers.
type Service interface {
	Observe(ctx context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error)
	Create(ctx context.Context, opts snowflake.CreateGrantOwnershipOptions) error
	Drop(ctx context.Context, id snowflake.GrantOwnershipIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new GrantOwnership reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantOwnership, Service, *snowflake.GrantOwnershipObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewGrantOwnershipClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantOwnership, Service, *snowflake.GrantOwnershipObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for GrantOwnership resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.GrantOwnership, Service, *snowflake.GrantOwnershipObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.GrantOwnership, Service, *snowflake.GrantOwnershipObservation]{
		ResourceNameVal:  "grantownership",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.GrantOwnership { return &snowplanev1alpha1.GrantOwnership{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(g *snowplanev1alpha1.GrantOwnership) (reconciler.Identifier, error) {
			if err := sqlbuilder.ValidateObjectType(g.Spec.ObjectType); err != nil {
				return nil, fmt.Errorf("grantownership.BuildIdentifier: %w", err)
			}

			return snowflake.GrantOwnershipIdentifier{
				ObjectType:  g.Spec.ObjectType,
				ObjectName:  g.Spec.ObjectName,
				GranteeName: resolveGranteeName(g),
			}, nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.GrantOwnershipObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.GrantOwnership, _ snowflake.GrantOwnershipIdentifier) error {
			toRole := buildToRole(obj)

			var currentGrantsBehavior string
			if obj.Spec.CurrentGrantsBehavior != nil {
				currentGrantsBehavior = string(*obj.Spec.CurrentGrantsBehavior)
			}

			return svc.Create(ctx, snowflake.CreateGrantOwnershipOptions{
				ObjectType:            obj.Spec.ObjectType,
				ObjectName:            obj.Spec.ObjectName,
				ToRole:                toRole,
				CurrentGrantsBehavior: currentGrantsBehavior,
			})
		}),
		AlterFn: func(_ context.Context, _ Service, _ reconciler.AlterOptions) error { return nil },
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.GrantOwnershipIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.GrantOwnership, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.GrantOwnershipObservation]) (reconciler.AlterOptions, error) {
			return &ownershipAlterOptions{}, nil
		},
		ApplyObservationFn: func(obj *snowplanev1alpha1.GrantOwnership, obs *reconciler.Observation[*snowflake.GrantOwnershipObservation]) {
			applyObservation(obj, obs.Detail)
		},
		TrackedParamsFn: func(_ *snowplanev1alpha1.GrantOwnership) []string { return nil },
		DetectDriftFn: func(obj *snowplanev1alpha1.GrantOwnership, obs *reconciler.Observation[*snowflake.GrantOwnershipObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		PreReconcileFn: func(ctx context.Context, g *snowplanev1alpha1.GrantOwnership) error {
			// Resolve AccountRoleRef.
			if ref := g.Spec.AccountRoleRef; ref != nil {
				name, err := refresolver.ResolveAccountRoleRef(ctx, c, g.Namespace, *ref)
				if err != nil {
					recorder.Eventf(g, corev1.EventTypeWarning, snowplanev1alpha1.ReasonRefResolutionFailed,
						"AccountRole ref %q resolution failed: %v", ref.Name, err)
					return err
				}

				g.Spec.AccountRole = &name
				g.Spec.AccountRoleRef = nil
			}

			// Resolve DatabaseRoleRef.
			if ref := g.Spec.DatabaseRoleRef; ref != nil {
				name, err := refresolver.ResolveDatabaseRoleRef(ctx, c, g.Namespace, *ref)
				if err != nil {
					recorder.Eventf(g, corev1.EventTypeWarning, snowplanev1alpha1.ReasonRefResolutionFailed,
						"DatabaseRole ref %q resolution failed: %v", ref.Name, err)
					return err
				}

				g.Spec.DatabaseRole = &name
				g.Spec.DatabaseRoleRef = nil
			}

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c,
					func() sigs.ObjectList { return &snowplanev1alpha1.GrantOwnershipList{} },
					indexAccountRoleRef, "listing grant ownerships for account role watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.DatabaseRole{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c,
					func() sigs.ObjectList { return &snowplanev1alpha1.GrantOwnershipList{} },
					indexDatabaseRoleRef, "listing grant ownerships for database role watch")),
			)

			return nil
		},
	}
}

// ownershipAlterOptions always reports no changes (ownership is immutable).
type ownershipAlterOptions struct{}

func (o *ownershipAlterOptions) HasChanges() bool { return false }

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, g *snowplanev1alpha1.GrantOwnership) error {
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

func applyObservation(g *snowplanev1alpha1.GrantOwnership, obs *snowflake.GrantOwnershipObservation) {
	g.Status.RoleName = resolveGranteeName(g)

	if obs.ShowOutput != nil {
		g.Status.ShowOutput = obs.ShowOutput
	}
}

// resolveGranteeName extracts the resolved grantee name from the spec.
func resolveGranteeName(g *snowplanev1alpha1.GrantOwnership) string {
	if g.Spec.AccountRole != nil {
		return *g.Spec.AccountRole
	}

	if g.Spec.DatabaseRole != nil {
		return *g.Spec.DatabaseRole
	}

	return ""
}

// buildToRole constructs the TO clause value (e.g. "ROLE MY_ROLE").
func buildToRole(g *snowplanev1alpha1.GrantOwnership) string {
	accountRole := ""
	if g.Spec.AccountRole != nil {
		accountRole = *g.Spec.AccountRole
	}

	databaseRole := ""
	if g.Spec.DatabaseRole != nil {
		databaseRole = *g.Spec.DatabaseRole
	}

	return snowflake.BuildToClause(accountRole, databaseRole, "")
}

func detectDrift(g *snowplanev1alpha1.GrantOwnership, obs *snowflake.GrantOwnershipObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		granteeName := resolveGranteeName(g)

		// Ownership target is immutable.
		d.CompareStringValueFold("GRANTEE", granteeName, obs.ShowOutput.GranteeName, true)
	}

	return d.Result()
}
