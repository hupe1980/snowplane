// Package tableconstraint implements the reconciler for TableConstraint resources.
package tableconstraint

import (
	"context"
	"strings"

	"k8s.io/client-go/tools/record"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/tableconstraint"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake table constraints.
type Service interface {
	Observe(ctx context.Context, id snowflake.TableConstraintIdentifier, constraintType string) (*snowflake.TableConstraintObservation, error)
	AddConstraint(ctx context.Context, opts snowflake.AddConstraintOptions) error
	AlterConstraint(ctx context.Context, opts snowflake.AlterConstraintOptions) error
	DropConstraint(ctx context.Context, id snowflake.TableConstraintIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new TableConstraint reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.TableConstraint, Service, *snowflake.TableConstraintObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return newTableConstraintService(snowflake.NewTableConstraintClient(exec))
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.TableConstraint, Service, *snowflake.TableConstraintObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// tableConstraintAlterOptions implements reconciler.AlterOptions.
type tableConstraintAlterOptions struct {
	inner snowflake.AlterConstraintOptions
}

func (o *tableConstraintAlterOptions) HasChanges() bool {
	return o.inner.HasChanges()
}

// newAdapter creates the BaseAdapter for TableConstraint resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.TableConstraint, Service, *snowflake.TableConstraintObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.TableConstraint, Service, *snowflake.TableConstraintObservation]{
		ResourceNameVal:  "tableconstraint",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.TableConstraint { return &snowplanev1alpha1.TableConstraint{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.TableConstraint) (reconciler.Identifier, error) {
			return snowflake.TableConstraintIdentifier{
				ConstraintName: obj.Spec.Name,
				TableName:      obj.Spec.TableName,
			}, nil
		},
		ObserveFn: func(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.TableConstraintObservation], error) {
			tcID, err := reconciler.AssertIdentifier[snowflake.TableConstraintIdentifier](id)
			if err != nil {
				return nil, err
			}

			obs, err := observeConstraint(ctx, svc, tcID)
			if err != nil {
				return nil, err
			}

			return &reconciler.Observation[*snowflake.TableConstraintObservation]{
				Exists: obs.Exists,
				Detail: obs,
			}, nil
		},
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.TableConstraint, _ snowflake.TableConstraintIdentifier) error {
			opts := snowflake.AddConstraintOptions{
				ConstraintName: obj.Spec.Name,
				ConstraintType: string(obj.Spec.Type),
				TableName:      obj.Spec.TableName,
				Columns:        obj.Spec.Columns,
				Comment:        obj.Spec.Comment,
			}

			if fk := obj.Spec.ForeignKeyProperties; fk != nil {
				opts.ReferencesTableName = fk.ReferencesTableName
				opts.ReferencesColumns = fk.ReferencesColumns
				opts.Match = fk.Match
				opts.OnUpdate = fk.OnUpdate
				opts.OnDelete = fk.OnDelete
			}

			if props := obj.Spec.Properties; props != nil {
				opts.Enforced = props.Enforced
				opts.Deferrable = props.Deferrable
				opts.Initially = props.Initially
				opts.Rely = props.Rely
				opts.ShouldValidate = props.Validate
			}

			return svc.AddConstraint(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *tableConstraintAlterOptions) error {
			return svc.AlterConstraint(ctx, opts.inner)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.TableConstraintIdentifier) error {
			return svc.DropConstraint(ctx, id)
		}),
		ValidateImmutableFn: func(_ context.Context, _ *snowplanev1alpha1.TableConstraint) error {
			// Identity fields are protected by CEL XValidation rules.
			return nil
		},
		BuildAlterOptsFn: func(_ context.Context, obj *snowplanev1alpha1.TableConstraint, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.TableConstraintObservation]) (reconciler.AlterOptions, error) {
			opts := snowflake.AlterConstraintOptions{
				ConstraintName: obj.Spec.Name,
				TableName:      obj.Spec.TableName,
			}

			if props := obj.Spec.Properties; props != nil {
				opts.Enforced = props.Enforced
				opts.Rely = props.Rely
				opts.ShouldValidate = props.Validate
			}

			opts.Comment = obj.Spec.Comment

			return &tableConstraintAlterOptions{inner: opts}, nil
		},
		ApplyObservationFn: func(obj *snowplanev1alpha1.TableConstraint, obs *reconciler.Observation[*snowflake.TableConstraintObservation]) {
			detail := obs.Detail
			if detail != nil && detail.Exists {
				obj.Status.FullyQualifiedName = snowflake.TableConstraintIdentifier{
					ConstraintName: obj.Spec.Name,
					TableName:      obj.Spec.TableName,
				}.FullyQualifiedName()

				obj.Status.ConstraintName = detail.ConstraintName
				obj.Status.ConstraintType = detail.ConstraintType
			}
		},
		TrackedParamsFn: func(_ *snowplanev1alpha1.TableConstraint) []string { return nil },
		DetectDriftFn: func(obj *snowplanev1alpha1.TableConstraint, obs *reconciler.Observation[*snowflake.TableConstraintObservation]) *drift.Result {
			d := drift.New()

			detail := obs.Detail
			if detail != nil && detail.Exists {
				d.CompareStringValueFold("CONSTRAINT_NAME", obj.Spec.Name, detail.ConstraintName, true)
				d.CompareStringValueFold("CONSTRAINT_TYPE", string(obj.Spec.Type), detail.ConstraintType, true)

				if len(detail.Columns) > 0 {
					specCols := strings.ToUpper(strings.Join(obj.Spec.Columns, ","))
					obsCols := strings.ToUpper(strings.Join(detail.Columns, ","))
					d.CompareStringValue("COLUMNS", specCols, obsCols, true)
				}
			}

			return d.Result()
		},
	}
}

// observeConstraint tries all three constraint type SHOW commands to find the constraint.
func observeConstraint(ctx context.Context, svc Service, id snowflake.TableConstraintIdentifier) (*snowflake.TableConstraintObservation, error) {
	for _, ct := range []string{"PRIMARY KEY", "UNIQUE", "FOREIGN KEY"} {
		obs, err := svc.Observe(ctx, id, ct)
		if err != nil {
			return nil, err
		}

		if obs.Exists {
			return obs, nil
		}
	}

	return &snowflake.TableConstraintObservation{Exists: false}, nil
}

// tableConstraintService wraps TableConstraintClient to satisfy the Service interface.
type tableConstraintService struct {
	client *snowflake.TableConstraintClient
}

func newTableConstraintService(c *snowflake.TableConstraintClient) *tableConstraintService {
	return &tableConstraintService{client: c}
}

func (s *tableConstraintService) Observe(ctx context.Context, id snowflake.TableConstraintIdentifier, constraintType string) (*snowflake.TableConstraintObservation, error) {
	return s.client.Observe(ctx, id, constraintType)
}

func (s *tableConstraintService) AddConstraint(ctx context.Context, opts snowflake.AddConstraintOptions) error {
	return s.client.AddConstraint(ctx, opts)
}

func (s *tableConstraintService) AlterConstraint(ctx context.Context, opts snowflake.AlterConstraintOptions) error {
	return s.client.AlterConstraint(ctx, opts)
}

func (s *tableConstraintService) DropConstraint(ctx context.Context, id snowflake.TableConstraintIdentifier) error {
	return s.client.DropConstraint(ctx, id)
}
