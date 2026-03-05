// Package view implements the reconciler for View resources.
package view

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/view"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake views.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error)
	Create(ctx context.Context, opts snowflake.CreateViewOptions) error
	Alter(ctx context.Context, opts snowflake.AlterViewOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new View reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewViewClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for View resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation]{
		ResourceNameVal:  "view",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.View { return &snowplanev1alpha1.View{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(view *snowplanev1alpha1.View) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(view.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(view.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, view.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.ViewObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.View, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterViewOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.View, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.ViewObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.View, obs *reconciler.Observation[*snowflake.ViewObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.View, obs *reconciler.Observation[*snowflake.ViewObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		SupportsCoA:      true,
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, view *snowplanev1alpha1.View) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, view,
				view.Namespace, view.Spec.DatabaseRef, view.Spec.DatabaseName, view.Status.DatabaseName)
			if err != nil {
				return err
			}

			view.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, view,
				view.Namespace, view.Spec.SchemaRef, view.Spec.SchemaName, view.Status.SchemaName)
			if err != nil {
				return err
			}

			view.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(view, view.Spec.DatabaseRef, view.Spec.DatabaseName, view.Spec.SchemaRef, view.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.View{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					v, ok := o.(*snowplanev1alpha1.View)
					if !ok || v.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{v.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.View{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					v, ok := o.(*snowplanev1alpha1.View)
					if !ok || v.Spec.SchemaRef == nil {
						return nil
					}

					return []string{v.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.ViewList{} }, ".spec.databaseRef.name", "listing views for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.ViewList{} }, ".spec.schemaRef.name", "listing views for schema watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, view *snowplanev1alpha1.View) error {
	if reconciler.ShouldSkipImmutableValidation(view) {
		return nil
	}

	if view.Status.ShowOutput != nil {
		if view.Status.ShowOutput.Name != "" && !strings.EqualFold(view.Spec.Name, view.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", view.Status.ShowOutput.Name, view.Spec.Name)
		}

		if view.Status.ShowOutput.DatabaseName != "" && view.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(view.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, view.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", view.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if view.Status.ShowOutput.SchemaName != "" && view.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(view.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, view.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", view.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

	}

	return nil
}

func applyObservation(view *snowplanev1alpha1.View, obs *snowflake.ViewObservation) {
	if obs.ShowOutput != nil {
		view.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		view.Status.DatabaseName = obs.ShowOutput.DatabaseName
		view.Status.SchemaName = obs.ShowOutput.SchemaName

		view.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(view *snowplanev1alpha1.View, id snowflake.SchemaObjectIdentifier) snowflake.CreateViewOptions {
	return snowflake.CreateViewOptions{
		Name:           id,
		Statement:      view.Spec.Statement,
		Secure:         view.Spec.Secure,
		Comment:        view.Spec.Comment,
		ChangeTracking: view.Spec.ChangeTracking,
	}
}

func buildAlterOptions(view *snowplanev1alpha1.View, id snowflake.SchemaObjectIdentifier, obs *snowflake.ViewObservation) snowflake.AlterViewOptions {
	opts := snowflake.AlterViewOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&view.Spec, view.Status.TrackedParameters)

	// Detect statement change → requires CREATE OR REPLACE VIEW (R9-1).
	if obs.ShowOutput != nil && view.Spec.Statement != obs.ShowOutput.Text {
		opts.ReplaceStatement = &snowflake.ReplaceViewStatement{
			Statement:      view.Spec.Statement,
			Secure:         view.Spec.Secure,
			Comment:        view.Spec.Comment,
			ChangeTracking: view.Spec.ChangeTracking,
		}

		// When replacing, all fields are carried by the CREATE OR REPLACE
		// statement; no additional ALTER is needed.
		return opts
	}

	if view.Spec.Comment != nil {
		if obs.ShowOutput == nil || *view.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = view.Spec.Comment
		}
	}

	if obs.ShowOutput != nil {
		// Secure toggle: compare bool values.
		desiredSecure := view.Spec.Secure
		observedSecure := obs.ShowOutput.IsSecure

		if desiredSecure != observedSecure {
			opts.Secure = &desiredSecure
		}

		// Change tracking: compare bool values.
		if view.Spec.ChangeTracking != nil {
			if *view.Spec.ChangeTracking != obs.ShowOutput.ChangeTracking {
				opts.ChangeTracking = view.Spec.ChangeTracking
			}
		}
	}

	return opts
}

func detectDrift(view *snowplanev1alpha1.View, obs *snowflake.ViewObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", view.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(view.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(view.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Mutable fields.
		d.CompareStringValue("STATEMENT", view.Spec.Statement, obs.ShowOutput.Text, false)
		d.CompareString("COMMENT", view.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareBoolValue("IS_SECURE", view.Spec.Secure, obs.ShowOutput.IsSecure, false)

		if view.Spec.ChangeTracking != nil {
			d.CompareBoolValue("CHANGE_TRACKING", *view.Spec.ChangeTracking, obs.ShowOutput.ChangeTracking, false)
		}
	}

	return d.Result()
}
