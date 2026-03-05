// Package passwordpolicy implements the reconciler for PasswordPolicy resources.
package passwordpolicy

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
	finalizerName = "snowplane.hupe1980.github.io/passwordpolicy"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake password policies.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error)
	Create(ctx context.Context, opts snowflake.CreatePasswordPolicyOptions) error
	Alter(ctx context.Context, opts snowflake.AlterPasswordPolicyOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new PasswordPolicy reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewPasswordPolicyClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for PasswordPolicy resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation]{
		ResourceNameVal:  "passwordpolicy",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.PasswordPolicy { return &snowplanev1alpha1.PasswordPolicy{} },
		ServiceFactoryFn: sf,
		SupportsCoA:      true,
		BuildIdentifierFn: func(pp *snowplanev1alpha1.PasswordPolicy) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(pp.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(pp.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, pp.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.PasswordPolicyObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.PasswordPolicy, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterPasswordPolicyOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.PasswordPolicy, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.PasswordPolicyObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.PasswordPolicy, obs *reconciler.Observation[*snowflake.PasswordPolicyObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.PasswordPolicy, obs *reconciler.Observation[*snowflake.PasswordPolicyObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, pp *snowplanev1alpha1.PasswordPolicy) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, pp,
				pp.Namespace, pp.Spec.DatabaseRef, pp.Spec.DatabaseName, pp.Status.DatabaseName)
			if err != nil {
				return err
			}

			pp.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, pp,
				pp.Namespace, pp.Spec.SchemaRef, pp.Spec.SchemaName, pp.Status.SchemaName)
			if err != nil {
				return err
			}

			pp.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(pp, pp.Spec.DatabaseRef, pp.Spec.DatabaseName, pp.Spec.SchemaRef, pp.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.PasswordPolicy{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					pp, ok := o.(*snowplanev1alpha1.PasswordPolicy)
					if !ok || pp.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{pp.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.PasswordPolicy{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					pp, ok := o.(*snowplanev1alpha1.PasswordPolicy)
					if !ok || pp.Spec.SchemaRef == nil {
						return nil
					}

					return []string{pp.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.PasswordPolicyList{} }, ".spec.databaseRef.name", "listing password policies for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.PasswordPolicyList{} }, ".spec.schemaRef.name", "listing password policies for schema watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, pp *snowplanev1alpha1.PasswordPolicy) error {
	if reconciler.ShouldSkipImmutableValidation(pp) {
		return nil
	}

	if pp.Status.ShowOutput != nil {
		if pp.Status.ShowOutput.Name != "" && !strings.EqualFold(pp.Spec.Name, pp.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", pp.Status.ShowOutput.Name, pp.Spec.Name)
		}

		if pp.Status.ShowOutput.DatabaseName != "" && pp.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(pp.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, pp.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", pp.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if pp.Status.ShowOutput.SchemaName != "" && pp.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(pp.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, pp.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", pp.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func applyObservation(pp *snowplanev1alpha1.PasswordPolicy, obs *snowflake.PasswordPolicyObservation) {
	if obs.ShowOutput != nil {
		pp.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()

		pp.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		pp.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(pp *snowplanev1alpha1.PasswordPolicy, id snowflake.SchemaObjectIdentifier) snowflake.CreatePasswordPolicyOptions {
	return snowflake.CreatePasswordPolicyOptions{
		Name:                    id,
		PasswordMinLength:       pp.Spec.PasswordMinLength,
		PasswordMaxLength:       pp.Spec.PasswordMaxLength,
		PasswordMinUpperCase:    pp.Spec.PasswordMinUpperCaseChars,
		PasswordMinLowerCase:    pp.Spec.PasswordMinLowerCaseChars,
		PasswordMinNumeric:      pp.Spec.PasswordMinNumericChars,
		PasswordMinSpecial:      pp.Spec.PasswordMinSpecialChars,
		PasswordMinAgeDays:      pp.Spec.PasswordMinAgeDays,
		PasswordMaxAgeDays:      pp.Spec.PasswordMaxAgeDays,
		PasswordMaxRetries:      pp.Spec.PasswordMaxRetries,
		PasswordLockoutTimeMins: pp.Spec.PasswordLockoutTimeMins,
		PasswordHistory:         pp.Spec.PasswordHistory,
		Comment:                 pp.Spec.Comment,
	}
}

func buildAlterOptions(pp *snowplanev1alpha1.PasswordPolicy, id snowflake.SchemaObjectIdentifier, obs *snowflake.PasswordPolicyObservation) snowflake.AlterPasswordPolicyOptions {
	opts := snowflake.AlterPasswordPolicyOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&pp.Spec, pp.Status.TrackedParameters)

	// Compare each field against DESCRIBE output before including in ALTER.
	// This avoids unnecessary ALTER statements every reconciliation cycle.
	desc := describeMap(obs)

	opts.PasswordMinLength = compareDescInt32(pp.Spec.PasswordMinLength, "PASSWORD_MIN_LENGTH", desc)
	opts.PasswordMaxLength = compareDescInt32(pp.Spec.PasswordMaxLength, "PASSWORD_MAX_LENGTH", desc)
	opts.PasswordMinUpperCase = compareDescInt32(pp.Spec.PasswordMinUpperCaseChars, "PASSWORD_MIN_UPPER_CASE_CHARS", desc)
	opts.PasswordMinLowerCase = compareDescInt32(pp.Spec.PasswordMinLowerCaseChars, "PASSWORD_MIN_LOWER_CASE_CHARS", desc)
	opts.PasswordMinNumeric = compareDescInt32(pp.Spec.PasswordMinNumericChars, "PASSWORD_MIN_NUMERIC_CHARS", desc)
	opts.PasswordMinSpecial = compareDescInt32(pp.Spec.PasswordMinSpecialChars, "PASSWORD_MIN_SPECIAL_CHARS", desc)
	opts.PasswordMinAgeDays = compareDescInt32(pp.Spec.PasswordMinAgeDays, "PASSWORD_MIN_AGE_DAYS", desc)
	opts.PasswordMaxAgeDays = compareDescInt32(pp.Spec.PasswordMaxAgeDays, "PASSWORD_MAX_AGE_DAYS", desc)
	opts.PasswordMaxRetries = compareDescInt32(pp.Spec.PasswordMaxRetries, "PASSWORD_MAX_RETRIES", desc)
	opts.PasswordLockoutTimeMins = compareDescInt32(pp.Spec.PasswordLockoutTimeMins, "PASSWORD_LOCKOUT_TIME_MINS", desc)
	opts.PasswordHistory = compareDescInt32(pp.Spec.PasswordHistory, "PASSWORD_HISTORY", desc)

	if pp.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *pp.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = pp.Spec.Comment
		}
	}

	return opts
}

// describeMap safely extracts the DESCRIBE output map from an observation.
func describeMap(obs *snowflake.PasswordPolicyObservation) map[string]string {
	if obs == nil || obs.DescribeOutput == nil {
		return nil
	}

	return obs.DescribeOutput
}

// compareDescInt32 returns specVal only if it differs from the DESCRIBE output value.
// If the DESCRIBE output is missing or the values match, returns nil (no change needed).
func compareDescInt32(specVal *int32, key string, desc map[string]string) *int32 {
	if specVal == nil {
		return nil
	}

	if desc == nil {
		return specVal // No observation data — always send
	}

	descRaw, ok := desc[key]
	if !ok {
		return specVal // Key not in DESCRIBE — always send
	}

	if fmt.Sprintf("%d", *specVal) == descRaw {
		return nil // Value matches — no change needed
	}

	return specVal
}

func detectDrift(pp *snowplanev1alpha1.PasswordPolicy, obs *snowflake.PasswordPolicyObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", pp.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields.
		d.CompareString("COMMENT", pp.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	// Compare numeric parameters from DESCRIBE output.
	if obs.DescribeOutput != nil {
		compareInt32FromDescribe(d, "PASSWORD_MIN_LENGTH", pp.Spec.PasswordMinLength, obs.DescribeOutput)
		compareInt32FromDescribe(d, "PASSWORD_MAX_LENGTH", pp.Spec.PasswordMaxLength, obs.DescribeOutput)
		compareInt32FromDescribe(d, "PASSWORD_MIN_UPPER_CASE_CHARS", pp.Spec.PasswordMinUpperCaseChars, obs.DescribeOutput)
		compareInt32FromDescribe(d, "PASSWORD_MIN_LOWER_CASE_CHARS", pp.Spec.PasswordMinLowerCaseChars, obs.DescribeOutput)
		compareInt32FromDescribe(d, "PASSWORD_MIN_NUMERIC_CHARS", pp.Spec.PasswordMinNumericChars, obs.DescribeOutput)
		compareInt32FromDescribe(d, "PASSWORD_MIN_SPECIAL_CHARS", pp.Spec.PasswordMinSpecialChars, obs.DescribeOutput)
		compareInt32FromDescribe(d, "PASSWORD_MIN_AGE_DAYS", pp.Spec.PasswordMinAgeDays, obs.DescribeOutput)
		compareInt32FromDescribe(d, "PASSWORD_MAX_AGE_DAYS", pp.Spec.PasswordMaxAgeDays, obs.DescribeOutput)
		compareInt32FromDescribe(d, "PASSWORD_MAX_RETRIES", pp.Spec.PasswordMaxRetries, obs.DescribeOutput)
		compareInt32FromDescribe(d, "PASSWORD_LOCKOUT_TIME_MINS", pp.Spec.PasswordLockoutTimeMins, obs.DescribeOutput)
		compareInt32FromDescribe(d, "PASSWORD_HISTORY", pp.Spec.PasswordHistory, obs.DescribeOutput)
	}

	return d.Result()
}

// compareInt32FromDescribe compares a spec int32 pointer against a DESCRIBE output value.
func compareInt32FromDescribe(d *drift.Detector, key string, specVal *int32, descOutput map[string]string) {
	if specVal == nil {
		return
	}

	descRaw, ok := descOutput[key]
	if !ok {
		return
	}

	specStr := fmt.Sprintf("%d", *specVal)
	d.CompareStringValueFold(key, specStr, descRaw, false)
}
