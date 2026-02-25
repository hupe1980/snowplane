// Package passwordpolicy implements the reconciler for PasswordPolicy resources.
package passwordpolicy

import (
	"context"
	"fmt"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
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
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// defaultServiceFactory is the production ServiceFactory.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewPasswordPolicyClient(sfC), cleanup, nil
}

func applyObservation(pp *snowplanev1alpha1.PasswordPolicy, obs *snowflake.PasswordPolicyObservation) {
	if obs.ShowOutput != nil {
		pp.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()

		pp.Status.ShowOutput = &snowplanev1alpha1.PasswordPolicyShowOutput{
			CreatedOn:    obs.ShowOutput.CreatedOn,
			Name:         obs.ShowOutput.Name,
			DatabaseName: obs.ShowOutput.DatabaseName,
			SchemaName:   obs.ShowOutput.SchemaName,
			Owner:        obs.ShowOutput.Owner,
			Comment:      obs.ShowOutput.Comment,
		}
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

func buildAlterOptions(pp *snowplanev1alpha1.PasswordPolicy, id snowflake.SchemaObjectIdentifier, _ *snowflake.PasswordPolicyObservation) snowflake.AlterPasswordPolicyOptions {
	opts := snowflake.AlterPasswordPolicyOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(pp)

	opts.PasswordMinLength = pp.Spec.PasswordMinLength
	opts.PasswordMaxLength = pp.Spec.PasswordMaxLength
	opts.PasswordMinUpperCase = pp.Spec.PasswordMinUpperCaseChars
	opts.PasswordMinLowerCase = pp.Spec.PasswordMinLowerCaseChars
	opts.PasswordMinNumeric = pp.Spec.PasswordMinNumericChars
	opts.PasswordMinSpecial = pp.Spec.PasswordMinSpecialChars
	opts.PasswordMinAgeDays = pp.Spec.PasswordMinAgeDays
	opts.PasswordMaxAgeDays = pp.Spec.PasswordMaxAgeDays
	opts.PasswordMaxRetries = pp.Spec.PasswordMaxRetries
	opts.PasswordLockoutTimeMins = pp.Spec.PasswordLockoutTimeMins
	opts.PasswordHistory = pp.Spec.PasswordHistory
	opts.Comment = pp.Spec.Comment

	return opts
}

func computeUnsetFields(pp *snowplanev1alpha1.PasswordPolicy) []string {
	if len(pp.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(pp.Status.TrackedParameters))
	for _, f := range pp.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if pp.Spec.PasswordMinLength == nil && managed["PASSWORD_MIN_LENGTH"] {
		unset = append(unset, "PASSWORD_MIN_LENGTH")
	}
	if pp.Spec.PasswordMaxLength == nil && managed["PASSWORD_MAX_LENGTH"] {
		unset = append(unset, "PASSWORD_MAX_LENGTH")
	}
	if pp.Spec.PasswordMinUpperCaseChars == nil && managed["PASSWORD_MIN_UPPER_CASE_CHARS"] {
		unset = append(unset, "PASSWORD_MIN_UPPER_CASE_CHARS")
	}
	if pp.Spec.PasswordMinLowerCaseChars == nil && managed["PASSWORD_MIN_LOWER_CASE_CHARS"] {
		unset = append(unset, "PASSWORD_MIN_LOWER_CASE_CHARS")
	}
	if pp.Spec.PasswordMinNumericChars == nil && managed["PASSWORD_MIN_NUMERIC_CHARS"] {
		unset = append(unset, "PASSWORD_MIN_NUMERIC_CHARS")
	}
	if pp.Spec.PasswordMinSpecialChars == nil && managed["PASSWORD_MIN_SPECIAL_CHARS"] {
		unset = append(unset, "PASSWORD_MIN_SPECIAL_CHARS")
	}
	if pp.Spec.PasswordMinAgeDays == nil && managed["PASSWORD_MIN_AGE_DAYS"] {
		unset = append(unset, "PASSWORD_MIN_AGE_DAYS")
	}
	if pp.Spec.PasswordMaxAgeDays == nil && managed["PASSWORD_MAX_AGE_DAYS"] {
		unset = append(unset, "PASSWORD_MAX_AGE_DAYS")
	}
	if pp.Spec.PasswordMaxRetries == nil && managed["PASSWORD_MAX_RETRIES"] {
		unset = append(unset, "PASSWORD_MAX_RETRIES")
	}
	if pp.Spec.PasswordLockoutTimeMins == nil && managed["PASSWORD_LOCKOUT_TIME_MINS"] {
		unset = append(unset, "PASSWORD_LOCKOUT_TIME_MINS")
	}
	if pp.Spec.PasswordHistory == nil && managed["PASSWORD_HISTORY"] {
		unset = append(unset, "PASSWORD_HISTORY")
	}
	if pp.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.PasswordPolicySpec) []string {
	var fields []string

	if spec.PasswordMinLength != nil {
		fields = append(fields, "PASSWORD_MIN_LENGTH")
	}
	if spec.PasswordMaxLength != nil {
		fields = append(fields, "PASSWORD_MAX_LENGTH")
	}
	if spec.PasswordMinUpperCaseChars != nil {
		fields = append(fields, "PASSWORD_MIN_UPPER_CASE_CHARS")
	}
	if spec.PasswordMinLowerCaseChars != nil {
		fields = append(fields, "PASSWORD_MIN_LOWER_CASE_CHARS")
	}
	if spec.PasswordMinNumericChars != nil {
		fields = append(fields, "PASSWORD_MIN_NUMERIC_CHARS")
	}
	if spec.PasswordMinSpecialChars != nil {
		fields = append(fields, "PASSWORD_MIN_SPECIAL_CHARS")
	}
	if spec.PasswordMinAgeDays != nil {
		fields = append(fields, "PASSWORD_MIN_AGE_DAYS")
	}
	if spec.PasswordMaxAgeDays != nil {
		fields = append(fields, "PASSWORD_MAX_AGE_DAYS")
	}
	if spec.PasswordMaxRetries != nil {
		fields = append(fields, "PASSWORD_MAX_RETRIES")
	}
	if spec.PasswordLockoutTimeMins != nil {
		fields = append(fields, "PASSWORD_LOCKOUT_TIME_MINS")
	}
	if spec.PasswordHistory != nil {
		fields = append(fields, "PASSWORD_HISTORY")
	}
	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	return fields
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
