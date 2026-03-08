// Package storageintegrationaws implements the reconciler for StorageIntegrationAWS resources.
package storageintegrationaws

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/storageintegrationaws"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake AWS storage integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationAWSObservation, error)
	Create(ctx context.Context, opts snowflake.CreateStorageIntegrationAWSOptions) error
	Alter(ctx context.Context, opts snowflake.AlterStorageIntegrationAWSOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new StorageIntegrationAWS reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.StorageIntegrationAWS, Service, *snowflake.StorageIntegrationAWSObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewStorageIntegrationAWSClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.StorageIntegrationAWS, Service, *snowflake.StorageIntegrationAWSObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for StorageIntegrationAWS resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.StorageIntegrationAWS, Service, *snowflake.StorageIntegrationAWSObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.StorageIntegrationAWS, Service, *snowflake.StorageIntegrationAWSObservation]{
		ResourceNameVal:  "storageintegrationaws",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.StorageIntegrationAWS { return &snowplanev1alpha1.StorageIntegrationAWS{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.StorageIntegrationAWS) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationAWSObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.StorageIntegrationAWSObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.StorageIntegrationAWS, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterStorageIntegrationAWSOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.StorageIntegrationAWS, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.StorageIntegrationAWSObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.StorageIntegrationAWS, obs *reconciler.Observation[*snowflake.StorageIntegrationAWSObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.StorageIntegrationAWS, obs *reconciler.Observation[*snowflake.StorageIntegrationAWSObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, si *snowplanev1alpha1.StorageIntegrationAWS) error {
	if reconciler.ShouldSkipImmutableValidation(si) {
		return nil
	}

	if si.Status.ShowOutput != nil {
		if si.Status.ShowOutput.Name != "" && !strings.EqualFold(si.Spec.Name, si.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", si.Status.ShowOutput.Name, si.Spec.Name)
		}
	}

	return nil
}

func applyObservation(si *snowplanev1alpha1.StorageIntegrationAWS, obs *snowflake.StorageIntegrationAWSObservation) {
	if obs.ShowOutput != nil {
		si.Status.FullyQualifiedName = obs.ShowOutput.Name
		si.Status.ShowOutput = obs.ShowOutput
	}

	// Populate DESCRIBE-derived fields for user reference.
	if obs.DescribeOutput != nil {
		si.Status.StorageAWSIAMUserARN = obs.DescribeOutput["STORAGE_AWS_IAM_USER_ARN"]
		si.Status.StorageAWSExternalID = obs.DescribeOutput["STORAGE_AWS_EXTERNAL_ID"]
	}
}

func buildCreateOptions(si *snowplanev1alpha1.StorageIntegrationAWS, id snowflake.AccountObjectIdentifier) snowflake.CreateStorageIntegrationAWSOptions {
	return snowflake.CreateStorageIntegrationAWSOptions{
		Name:                    id,
		Enabled:                 si.Spec.Enabled,
		StorageAllowedLocations: si.Spec.StorageAllowedLocations,
		StorageBlockedLocations: si.Spec.StorageBlockedLocations,
		StorageAWSRoleARN:       si.Spec.StorageAWSRoleARN,
		StorageAWSExternalID:    si.Spec.StorageAWSExternalID,
		Comment:                 si.Spec.Comment,
	}
}

func buildAlterOptions(si *snowplanev1alpha1.StorageIntegrationAWS, id snowflake.AccountObjectIdentifier, obs *snowflake.StorageIntegrationAWSObservation) snowflake.AlterStorageIntegrationAWSOptions {
	opts := snowflake.AlterStorageIntegrationAWSOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&si.Spec, si.Status.TrackedParameters)

	// Enabled.
	if si.Spec.Enabled != nil {
		if obs.ShowOutput == nil || *si.Spec.Enabled != obs.ShowOutput.Enabled {
			opts.Enabled = si.Spec.Enabled
		}
	}

	// Locations — compare sorted against observation before sending.
	specAllowed := sortedLocations(si.Spec.StorageAllowedLocations)
	obsAllowed := sortedLocations(parseLocations(obs.DescribeOutput, "STORAGE_ALLOWED_LOCATIONS"))

	if obs.DescribeOutput == nil || specAllowed != obsAllowed {
		locs := make([]string, len(si.Spec.StorageAllowedLocations))
		copy(locs, si.Spec.StorageAllowedLocations)
		opts.StorageAllowedLocations = &locs
	}

	if len(si.Spec.StorageBlockedLocations) > 0 {
		specBlocked := sortedLocations(si.Spec.StorageBlockedLocations)
		obsBlocked := sortedLocations(parseLocations(obs.DescribeOutput, "STORAGE_BLOCKED_LOCATIONS"))

		if obs.DescribeOutput == nil || specBlocked != obsBlocked {
			blocked := make([]string, len(si.Spec.StorageBlockedLocations))
			copy(blocked, si.Spec.StorageBlockedLocations)
			opts.StorageBlockedLocations = &blocked
		}
	}

	// StorageAWSRoleARN — always a required string, always send on drift.
	if obs.DescribeOutput == nil || si.Spec.StorageAWSRoleARN != obs.DescribeOutput["STORAGE_AWS_ROLE_ARN"] {
		v := si.Spec.StorageAWSRoleARN
		opts.StorageAWSRoleARN = &v
	}

	if si.Spec.StorageAWSExternalID != nil {
		if obs.DescribeOutput == nil || *si.Spec.StorageAWSExternalID != obs.DescribeOutput["STORAGE_AWS_EXTERNAL_ID"] {
			opts.StorageAWSExternalID = si.Spec.StorageAWSExternalID
		}
	}

	if si.Spec.Comment != nil {
		if obs.ShowOutput == nil || *si.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = si.Spec.Comment
		}
	}

	return opts
}

func detectDrift(si *snowplanev1alpha1.StorageIntegrationAWS, obs *snowflake.StorageIntegrationAWSObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", si.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields.
		d.CompareString("COMMENT", si.Spec.Comment, obs.ShowOutput.Comment, false)

		if si.Spec.Enabled != nil {
			obsEnabled := obs.ShowOutput.Enabled
			d.CompareBool("ENABLED", si.Spec.Enabled, &obsEnabled, false)
		}
	}

	if obs.DescribeOutput != nil {
		// Locations — compare sorted CSV.
		allowedCSV := sortedLocations(si.Spec.StorageAllowedLocations)
		d.CompareStringValue("STORAGE_ALLOWED_LOCATIONS", allowedCSV, sortedLocations(parseLocations(obs.DescribeOutput, "STORAGE_ALLOWED_LOCATIONS")), false)

		blockedCSV := sortedLocations(si.Spec.StorageBlockedLocations)
		d.CompareStringValue("STORAGE_BLOCKED_LOCATIONS", blockedCSV, sortedLocations(parseLocations(obs.DescribeOutput, "STORAGE_BLOCKED_LOCATIONS")), false)

		// Provider-specific config drift.
		d.CompareStringValue("STORAGE_AWS_ROLE_ARN", si.Spec.StorageAWSRoleARN, obs.DescribeOutput["STORAGE_AWS_ROLE_ARN"], false)

		if si.Spec.StorageAWSExternalID != nil {
			v := obs.DescribeOutput["STORAGE_AWS_EXTERNAL_ID"]
			d.CompareString("STORAGE_AWS_EXTERNAL_ID", si.Spec.StorageAWSExternalID, v, false)
		}
	}

	return d.Result()
}

func sortedLocations(locs []string) string {
	if len(locs) == 0 {
		return ""
	}

	sorted := make([]string, len(locs))
	copy(sorted, locs)

	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i]) < strings.ToLower(sorted[j])
	})

	return strings.Join(sorted, ",")
}

// parseLocations extracts a location list from DESCRIBE output by key.
func parseLocations(describeOutput map[string]string, key string) []string {
	if describeOutput == nil {
		return nil
	}

	raw, ok := describeOutput[key]
	if !ok || raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}

	return result
}
