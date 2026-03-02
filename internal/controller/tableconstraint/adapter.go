package tableconstraint

import (
	"context"
	"strings"

	"k8s.io/client-go/tools/record"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
)

// adapter implements reconciler.ResourceAdapter for TableConstraint.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.TableConstraint, Service, *snowflake.TableConstraintObservation] = (*adapter)(nil)

func (a *adapter) ResourceName() string  { return "tableconstraint" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.TableConstraint {
	return &snowplanev1alpha1.TableConstraint{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

// BuildIdentifier constructs a TableConstraintIdentifier from the spec.
func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.TableConstraint) (reconciler.Identifier, error) {
	return snowflake.TableConstraintIdentifier{
		ConstraintName: obj.Spec.Name,
		TableName:      obj.Spec.TableName,
	}, nil
}

// Observe queries Snowflake for the current state of the constraint.
func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.TableConstraintObservation], error) {
	tcID, err := reconciler.AssertIdentifier[snowflake.TableConstraintIdentifier](id)
	if err != nil {
		return nil, err
	}

	// We need the constraint type to know which SHOW command to use.
	// Get it from the object in context. The reconciler calls Observe
	// after loading the object, so we can rely on the spec.
	// For Observe, we use the constraint type embedded in the object.
	// Since the generic reconciler provides the identifier but not the
	// object to Observe, we store the constraint type on the identifier
	// via our BuildIdentifier. But Identifier is an interface — we
	// reconstruct it here. Use all three SHOW commands as fallback.
	obs, err := a.observeConstraint(ctx, svc, tcID)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.TableConstraintObservation]{
		Exists: obs.Exists,
		Detail: obs,
	}, nil
}

// observeConstraint tries all three constraint type SHOW commands to find the constraint.
func (a *adapter) observeConstraint(ctx context.Context, svc Service, id snowflake.TableConstraintIdentifier) (*snowflake.TableConstraintObservation, error) {
	// Try all three types — we don't know which SHOW command will find it
	// since the identifier doesn't carry the type. In practice the
	// reconciler always rebuilds the identifier from the current object,
	// so the constraint type should match. But for robustness we try all.
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

// Create adds the constraint to the table.
func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.TableConstraint, _ reconciler.Identifier) error {
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
}

// Alter updates the constraint's mutable properties.
func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	alterOpts, err := reconciler.AssertAlterOptions[*tableConstraintAlterOptions](opts)
	if err != nil {
		return err
	}

	return svc.AlterConstraint(ctx, alterOpts.inner)
}

// Drop removes the constraint from the table.
func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	tcID, err := reconciler.AssertIdentifier[snowflake.TableConstraintIdentifier](id)
	if err != nil {
		return err
	}

	return svc.DropConstraint(ctx, tcID)
}

// ValidateImmutableFields checks immutability of identity fields.
func (a *adapter) ValidateImmutableFields(_ context.Context, _ *snowplanev1alpha1.TableConstraint) error {
	// Identity fields are protected by CEL XValidation rules.
	return nil
}

// tableConstraintAlterOptions implements reconciler.AlterOptions.
type tableConstraintAlterOptions struct {
	inner snowflake.AlterConstraintOptions
}

func (o *tableConstraintAlterOptions) HasChanges() bool {
	return o.inner.HasChanges()
}

// BuildAlterOptions builds alter options by comparing spec against observation.
func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.TableConstraint, id reconciler.Identifier, _ *reconciler.Observation[*snowflake.TableConstraintObservation]) (reconciler.AlterOptions, error) {
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
}

// ApplyObservation maps the observation into the CR's status.
func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.TableConstraint, obs *reconciler.Observation[*snowflake.TableConstraintObservation]) {
	detail := obs.Detail
	if detail != nil && detail.Exists {
		obj.Status.FullyQualifiedName = snowflake.TableConstraintIdentifier{
			ConstraintName: obj.Spec.Name,
			TableName:      obj.Spec.TableName,
		}.FullyQualifiedName()

		obj.Status.ConstraintName = detail.ConstraintName
		obj.Status.ConstraintType = detail.ConstraintType
	}
}

// ComputeTrackedParameters returns nil — table constraints don't track parameters.
func (a *adapter) ComputeTrackedParameters(_ *snowplanev1alpha1.TableConstraint) []string {
	return nil
}

// DetectDrift compares the spec against the observed value.
func (a *adapter) DetectDrift(obj *snowplanev1alpha1.TableConstraint, obs *reconciler.Observation[*snowflake.TableConstraintObservation]) *drift.Result {
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
}
