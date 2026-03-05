package grant

import (
	"context"
	"fmt"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	grantPrivilegesToAccountRoleFinalizer  = "snowplane.hupe1980.github.io/grantprivilegestoaccountrole"
	grantPrivilegesToDatabaseRoleFinalizer = "snowplane.hupe1980.github.io/grantprivilegestodatabaserole"
	grantPrivilegesToShareFinalizer        = "snowplane.hupe1980.github.io/grantprivilegestoshare"
)

// Field index keys for GrantPrivilegesToAccountRole cross-resource watches.
const (
	argIndexAccountRoleRef = ".arg.refs.accountRoleRef.name"
	argIndexSchemaRef      = ".arg.refs.schemaRef.name"
	argIndexDatabaseRef    = ".arg.refs.databaseRef.name"
)

// Field index keys for GrantPrivilegesToDatabaseRole cross-resource watches.
const (
	drgIndexDatabaseRoleRef = ".drg.refs.databaseRoleRef.name"
	drgIndexSchemaRef       = ".drg.refs.schemaRef.name"
	drgIndexDatabaseRef     = ".drg.refs.databaseRef.name"
)

// =========================================================================
// GrantPrivilegesToAccountRole
// =========================================================================

// NewGrantPrivilegesToAccountRoleReconciler returns a new reconciler for GrantPrivilegesToAccountRole.
func NewGrantPrivilegesToAccountRoleReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToAccountRole, Service, *snowflake.GrantObservation] {
	return NewGrantPrivilegesToAccountRoleReconcilerWithServiceFactory(c, factory, recorder, rl, defaultServiceFactory())
}

// NewGrantPrivilegesToAccountRoleReconcilerWithServiceFactory lets callers inject a custom
// ServiceFactory (for tests).
func NewGrantPrivilegesToAccountRoleReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToAccountRole, Service, *snowflake.GrantObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl,
		newGrantPrivilegesToAccountRoleAdapter(c, recorder, sf))
}

func newGrantPrivilegesToAccountRoleAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.GrantPrivilegesToAccountRole, Service, *snowflake.GrantObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.GrantPrivilegesToAccountRole, Service, *snowflake.GrantObservation]{
		ResourceNameVal:  "grantprivilegestoaccountrole",
		FinalizerNameVal: grantPrivilegesToAccountRoleFinalizer,
		NewObjectFn: func() *snowplanev1alpha1.GrantPrivilegesToAccountRole {
			return &snowplanev1alpha1.GrantPrivilegesToAccountRole{}
		},
		ServiceFactoryFn: sf,

		BuildIdentifierFn: func(g *snowplanev1alpha1.GrantPrivilegesToAccountRole) (reconciler.Identifier, error) {
			role := derefStr(g.Spec.AccountRole)
			toClause := snowflake.BuildToClause(role, "", "")
			return buildGrantIdentifier(&g.Spec.On, g.Spec.ResolvedPrivilege(), toClause, role, g.Spec.ResolveKind(), false)
		},

		ObserveFn: func(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.GrantObservation], error) {
			return grantObserve(ctx, svc, id)
		},

		CreateFn: func(ctx context.Context, svc Service, obj *snowplanev1alpha1.GrantPrivilegesToAccountRole, _ reconciler.Identifier) error {
			on := onToParams(&obj.Spec.On)

			onClause, err := snowflake.BuildOnClause(on)
			if err != nil {
				return fmt.Errorf("building ON clause: %w", err)
			}

			return svc.Grant(ctx, snowflake.CreateGrantOptions{
				Privilege:       obj.Spec.ResolvedPrivilege(),
				OnClause:        onClause,
				ToClause:        snowflake.BuildToClause(derefStr(obj.Spec.AccountRole), "", ""),
				WithGrantOption: obj.Spec.WithGrantOption,
			})
		},

		AlterFn: func(_ context.Context, _ Service, _ reconciler.AlterOptions) error { return nil },

		DropFn: func(ctx context.Context, svc Service, id reconciler.Identifier) error {
			return grantDrop(ctx, svc, id)
		},

		ValidateImmutableFn: func(_ context.Context, g *snowplanev1alpha1.GrantPrivilegesToAccountRole) error {
			if reconciler.ShouldSkipImmutableValidation(g) {
				return nil
			}

			if g.Status.ShowOutput != nil {
				if err := validateImmutablePrivilege(g.Spec.ResolvedPrivilege(), g.Status.ShowOutput); err != nil {
					return err
				}

				return validateImmutableGrantOption(g.Spec.WithGrantOption, g.Status.ShowOutput)
			}

			return nil
		},

		BuildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.GrantPrivilegesToAccountRole, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.GrantObservation]) (reconciler.AlterOptions, error) {
			return grantAlterOptions{}, nil
		},

		ApplyObservationFn: func(obj *snowplanev1alpha1.GrantPrivilegesToAccountRole, obs *reconciler.Observation[*snowflake.GrantObservation]) {
			grantObs := obs.Detail
			if grantObs.ShowOutput != nil {
				on := onToParams(&obj.Spec.On)

				onClause, err := snowflake.BuildOnClause(on)
				if err == nil {
					toClause := snowflake.BuildToClause(derefStr(obj.Spec.AccountRole), "", "")
					obj.Status.FullyQualifiedName = fmt.Sprintf("GRANT %s %s %s", grantObs.ShowOutput.Privilege, onClause, toClause)
				}

				obj.Status.Kind = obj.Spec.ResolveKind()
				obj.Status.ShowOutput = applyGrantShowOutput(grantObs)
			}
		},

		TrackedParamsFn: func(_ *snowplanev1alpha1.GrantPrivilegesToAccountRole) []string { return nil },

		DetectDriftFn: func(obj *snowplanev1alpha1.GrantPrivilegesToAccountRole, obs *reconciler.Observation[*snowflake.GrantObservation]) *drift.Result {
			return detectGrantDrift(obj.Spec.ResolvedPrivilege(), obj.Spec.WithGrantOption, obs.Detail)
		},

		PreReconcileFn: func(ctx context.Context, g *snowplanev1alpha1.GrantPrivilegesToAccountRole) error {
			logger := log.FromContext(ctx)
			hasRefs := false

			if ref := g.Spec.AccountRoleRef; ref != nil {
				hasRefs = true

				name, err := refresolver.ResolveAccountRoleRef(ctx, c, g.Namespace, *ref)
				if err != nil {
					return refresolver.HandleRefError(ctx, g, recorder, "AccountRole", ref.Name, err)
				}

				g.Spec.AccountRole = &name
				g.Spec.AccountRoleRef = nil
			}

			errHandler := func(kind, name string, err error) error {
				return refresolver.HandleRefError(ctx, g, recorder, kind, name, err)
			}

			if err := resolveOnRefs(ctx, c, g.Namespace, &g.Spec.On, errHandler); err != nil {
				return err
			}

			if hasRefs || hasOnRefs(&g.Spec.On) {
				conditions.SetReferencesResolved(g, "all references resolved")
				logger.V(1).Info("grantprivilegestoaccountrole references resolved")
			}

			return nil
		},

		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			grant := &snowplanev1alpha1.GrantPrivilegesToAccountRole{}

			// AccountRole indexer + watch.
			if err := mgr.GetFieldIndexer().IndexField(ctx, grant, argIndexAccountRoleRef,
				func(o sigs.Object) []string {
					g, ok := o.(*snowplanev1alpha1.GrantPrivilegesToAccountRole)
					if !ok || g.Spec.AccountRoleRef == nil {
						return nil
					}
					return []string{g.Spec.AccountRoleRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for %s: %w", argIndexAccountRoleRef, err)
			}

			bldr.Watches(
				&snowplanev1alpha1.AccountRole{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c,
					func() sigs.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToAccountRoleList{} },
					argIndexAccountRoleRef, "listing account role grants for account role watch")),
			)

			// Database indexer + watch.
			if err := mgr.GetFieldIndexer().IndexField(ctx, grant, argIndexDatabaseRef,
				func(o sigs.Object) []string {
					g, ok := o.(*snowplanev1alpha1.GrantPrivilegesToAccountRole)
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c,
					func() sigs.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToAccountRoleList{} },
					argIndexDatabaseRef, "listing account role grants for database watch")),
			)

			// Schema indexer + watch.
			if err := mgr.GetFieldIndexer().IndexField(ctx, grant, argIndexSchemaRef,
				func(o sigs.Object) []string {
					g, ok := o.(*snowplanev1alpha1.GrantPrivilegesToAccountRole)
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c,
					func() sigs.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToAccountRoleList{} },
					argIndexSchemaRef, "listing account role grants for schema watch")),
			)

			return nil
		},
	}
}

// =========================================================================
// GrantPrivilegesToDatabaseRole
// =========================================================================

// NewGrantPrivilegesToDatabaseRoleReconciler returns a new reconciler for GrantPrivilegesToDatabaseRole.
func NewGrantPrivilegesToDatabaseRoleReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToDatabaseRole, Service, *snowflake.GrantObservation] {
	return NewGrantPrivilegesToDatabaseRoleReconcilerWithServiceFactory(c, factory, recorder, rl, defaultServiceFactory())
}

// NewGrantPrivilegesToDatabaseRoleReconcilerWithServiceFactory lets callers inject a custom
// ServiceFactory (for tests).
func NewGrantPrivilegesToDatabaseRoleReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToDatabaseRole, Service, *snowflake.GrantObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl,
		newGrantPrivilegesToDatabaseRoleAdapter(c, recorder, sf))
}

func newGrantPrivilegesToDatabaseRoleAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.GrantPrivilegesToDatabaseRole, Service, *snowflake.GrantObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.GrantPrivilegesToDatabaseRole, Service, *snowflake.GrantObservation]{
		ResourceNameVal:  "grantprivilegestodatabaserole",
		FinalizerNameVal: grantPrivilegesToDatabaseRoleFinalizer,
		NewObjectFn: func() *snowplanev1alpha1.GrantPrivilegesToDatabaseRole {
			return &snowplanev1alpha1.GrantPrivilegesToDatabaseRole{}
		},
		ServiceFactoryFn: sf,

		BuildIdentifierFn: func(g *snowplanev1alpha1.GrantPrivilegesToDatabaseRole) (reconciler.Identifier, error) {
			role := derefStr(g.Spec.DatabaseRole)
			toClause := snowflake.BuildToClause("", role, "")
			return buildDBRoleGrantIdentifier(&g.Spec.On, g.Spec.ResolvedPrivilege(), toClause, role, g.Spec.ResolveKind())
		},

		ObserveFn: func(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.GrantObservation], error) {
			return grantObserve(ctx, svc, id)
		},

		CreateFn: func(ctx context.Context, svc Service, obj *snowplanev1alpha1.GrantPrivilegesToDatabaseRole, _ reconciler.Identifier) error {
			on := dbRoleOnToParams(&obj.Spec.On)

			onClause, err := snowflake.BuildOnClause(on)
			if err != nil {
				return fmt.Errorf("building ON clause: %w", err)
			}

			return svc.Grant(ctx, snowflake.CreateGrantOptions{
				Privilege:       obj.Spec.ResolvedPrivilege(),
				OnClause:        onClause,
				ToClause:        snowflake.BuildToClause("", derefStr(obj.Spec.DatabaseRole), ""),
				WithGrantOption: obj.Spec.WithGrantOption,
			})
		},

		AlterFn: func(_ context.Context, _ Service, _ reconciler.AlterOptions) error { return nil },

		DropFn: func(ctx context.Context, svc Service, id reconciler.Identifier) error {
			return grantDrop(ctx, svc, id)
		},

		ValidateImmutableFn: func(_ context.Context, g *snowplanev1alpha1.GrantPrivilegesToDatabaseRole) error {
			if reconciler.ShouldSkipImmutableValidation(g) {
				return nil
			}

			if g.Status.ShowOutput != nil {
				if err := validateImmutablePrivilege(g.Spec.ResolvedPrivilege(), g.Status.ShowOutput); err != nil {
					return err
				}

				return validateImmutableGrantOption(g.Spec.WithGrantOption, g.Status.ShowOutput)
			}

			return nil
		},

		BuildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.GrantPrivilegesToDatabaseRole, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.GrantObservation]) (reconciler.AlterOptions, error) {
			return grantAlterOptions{}, nil
		},

		ApplyObservationFn: func(obj *snowplanev1alpha1.GrantPrivilegesToDatabaseRole, obs *reconciler.Observation[*snowflake.GrantObservation]) {
			grantObs := obs.Detail
			if grantObs.ShowOutput != nil {
				on := dbRoleOnToParams(&obj.Spec.On)

				onClause, err := snowflake.BuildOnClause(on)
				if err == nil {
					toClause := snowflake.BuildToClause("", derefStr(obj.Spec.DatabaseRole), "")
					obj.Status.FullyQualifiedName = fmt.Sprintf("GRANT %s %s %s", grantObs.ShowOutput.Privilege, onClause, toClause)
				}

				obj.Status.Kind = obj.Spec.ResolveKind()
				obj.Status.ShowOutput = applyGrantShowOutput(grantObs)
			}
		},

		TrackedParamsFn: func(_ *snowplanev1alpha1.GrantPrivilegesToDatabaseRole) []string { return nil },

		DetectDriftFn: func(obj *snowplanev1alpha1.GrantPrivilegesToDatabaseRole, obs *reconciler.Observation[*snowflake.GrantObservation]) *drift.Result {
			return detectGrantDrift(obj.Spec.ResolvedPrivilege(), obj.Spec.WithGrantOption, obs.Detail)
		},

		PreReconcileFn: func(ctx context.Context, g *snowplanev1alpha1.GrantPrivilegesToDatabaseRole) error {
			logger := log.FromContext(ctx)
			hasRefs := false

			if ref := g.Spec.DatabaseRoleRef; ref != nil {
				hasRefs = true

				fqn, err := refresolver.ResolveDatabaseRoleRef(ctx, c, g.Namespace, *ref)
				if err != nil {
					return refresolver.HandleRefError(ctx, g, recorder, "DatabaseRole", ref.Name, err)
				}

				g.Spec.DatabaseRole = &fqn
				g.Spec.DatabaseRoleRef = nil
			}

			errHandler := func(kind, name string, err error) error {
				return refresolver.HandleRefError(ctx, g, recorder, kind, name, err)
			}

			if err := resolveDBRoleOnRefs(ctx, c, g.Namespace, &g.Spec.On, errHandler); err != nil {
				return err
			}

			if hasRefs || hasDBRoleOnRefs(&g.Spec.On) {
				conditions.SetReferencesResolved(g, "all references resolved")
				logger.V(1).Info("grantprivilegestodatabaserole references resolved")
			}

			return nil
		},

		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			grant := &snowplanev1alpha1.GrantPrivilegesToDatabaseRole{}

			// DatabaseRole indexer + watch.
			if err := mgr.GetFieldIndexer().IndexField(ctx, grant, drgIndexDatabaseRoleRef,
				func(o sigs.Object) []string {
					g, ok := o.(*snowplanev1alpha1.GrantPrivilegesToDatabaseRole)
					if !ok || g.Spec.DatabaseRoleRef == nil {
						return nil
					}
					return []string{g.Spec.DatabaseRoleRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for %s: %w", drgIndexDatabaseRoleRef, err)
			}

			bldr.Watches(
				&snowplanev1alpha1.DatabaseRole{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c,
					func() sigs.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleList{} },
					drgIndexDatabaseRoleRef, "listing database role grants for database role watch")),
			)

			// Database indexer + watch.
			if err := mgr.GetFieldIndexer().IndexField(ctx, grant, drgIndexDatabaseRef,
				func(o sigs.Object) []string {
					g, ok := o.(*snowplanev1alpha1.GrantPrivilegesToDatabaseRole)
					if !ok {
						return nil
					}
					return extractDatabaseRefsFromDBRoleOn(&g.Spec.On)
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for %s: %w", drgIndexDatabaseRef, err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c,
					func() sigs.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleList{} },
					drgIndexDatabaseRef, "listing database role grants for database watch")),
			)

			// Schema indexer + watch.
			if err := mgr.GetFieldIndexer().IndexField(ctx, grant, drgIndexSchemaRef,
				func(o sigs.Object) []string {
					g, ok := o.(*snowplanev1alpha1.GrantPrivilegesToDatabaseRole)
					if !ok {
						return nil
					}
					return extractSchemaRefsFromDBRoleOn(&g.Spec.On)
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for %s: %w", drgIndexSchemaRef, err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c,
					func() sigs.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleList{} },
					drgIndexSchemaRef, "listing database role grants for schema watch")),
			)

			return nil
		},
	}
}

// =========================================================================
// GrantPrivilegesToShare
// =========================================================================

// NewGrantPrivilegesToShareReconciler returns a new reconciler for GrantPrivilegesToShare.
func NewGrantPrivilegesToShareReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToShare, Service, *snowflake.GrantObservation] {
	return NewGrantPrivilegesToShareReconcilerWithServiceFactory(c, factory, recorder, rl, defaultServiceFactory())
}

// NewGrantPrivilegesToShareReconcilerWithServiceFactory lets callers inject a custom
// ServiceFactory (for tests).
func NewGrantPrivilegesToShareReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToShare, Service, *snowflake.GrantObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl,
		newGrantPrivilegesToShareAdapter(c, recorder, sf))
}

func newGrantPrivilegesToShareAdapter(_ sigs.Client, _ record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.GrantPrivilegesToShare, Service, *snowflake.GrantObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.GrantPrivilegesToShare, Service, *snowflake.GrantObservation]{
		ResourceNameVal:  "grantprivilegestoshare",
		FinalizerNameVal: grantPrivilegesToShareFinalizer,
		NewObjectFn:      func() *snowplanev1alpha1.GrantPrivilegesToShare { return &snowplanev1alpha1.GrantPrivilegesToShare{} },
		ServiceFactoryFn: sf,

		BuildIdentifierFn: func(g *snowplanev1alpha1.GrantPrivilegesToShare) (reconciler.Identifier, error) {
			return buildGrantPrivilegesToShareIdentifier(&g.Spec.On, g.Spec.Privilege, g.Spec.Share), nil
		},

		ObserveFn: func(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.GrantObservation], error) {
			return grantObserve(ctx, svc, id)
		},

		CreateFn: func(ctx context.Context, svc Service, obj *snowplanev1alpha1.GrantPrivilegesToShare, _ reconciler.Identifier) error {
			onClause := snowflake.BuildShareOnClause(obj.Spec.On.ObjectType(), obj.Spec.On.ObjectName())

			return svc.Grant(ctx, snowflake.CreateGrantOptions{
				Privilege:       obj.Spec.Privilege,
				OnClause:        onClause,
				ToClause:        snowflake.BuildToClause("", "", obj.Spec.Share),
				WithGrantOption: false,
			})
		},

		AlterFn: func(_ context.Context, _ Service, _ reconciler.AlterOptions) error { return nil },

		DropFn: func(ctx context.Context, svc Service, id reconciler.Identifier) error {
			return grantDrop(ctx, svc, id)
		},

		ValidateImmutableFn: func(_ context.Context, g *snowplanev1alpha1.GrantPrivilegesToShare) error {
			if reconciler.ShouldSkipImmutableValidation(g) {
				return nil
			}

			if g.Status.ShowOutput != nil {
				return validateImmutablePrivilege(g.Spec.Privilege, g.Status.ShowOutput)
			}

			return nil
		},

		BuildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.GrantPrivilegesToShare, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.GrantObservation]) (reconciler.AlterOptions, error) {
			return grantAlterOptions{}, nil
		},

		ApplyObservationFn: func(obj *snowplanev1alpha1.GrantPrivilegesToShare, obs *reconciler.Observation[*snowflake.GrantObservation]) {
			grantObs := obs.Detail
			if grantObs.ShowOutput != nil {
				onClause := snowflake.BuildShareOnClause(obj.Spec.On.ObjectType(), obj.Spec.On.ObjectName())
				toClause := snowflake.BuildToClause("", "", obj.Spec.Share)
				obj.Status.FullyQualifiedName = fmt.Sprintf("GRANT %s %s %s", grantObs.ShowOutput.Privilege, onClause, toClause)
				obj.Status.ShowOutput = applyGrantShowOutput(grantObs)
			}
		},

		TrackedParamsFn: func(_ *snowplanev1alpha1.GrantPrivilegesToShare) []string { return nil },

		DetectDriftFn: func(obj *snowplanev1alpha1.GrantPrivilegesToShare, obs *reconciler.Observation[*snowflake.GrantObservation]) *drift.Result {
			return detectGrantDrift(obj.Spec.Privilege, false, obs.Detail)
		},
	}
}
