// Package storageintegration implements the reconciler for StorageIntegration resources.
package storageintegration

import (
	"context"
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
)

const (
	finalizerName = "snowplane.hupe1980.github.io/storageintegration"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake storage integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error)
	Create(ctx context.Context, opts snowflake.CreateStorageIntegrationOptions) error
	Alter(ctx context.Context, opts snowflake.AlterStorageIntegrationOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new StorageIntegration reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.StorageIntegration, Service, *snowflake.StorageIntegrationObservation] {
	a := &adapter{newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.StorageIntegration, Service, *snowflake.StorageIntegrationObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.StorageIntegration, Service, *snowflake.StorageIntegrationObservation] {
	a := &adapter{newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.StorageIntegration, Service, *snowflake.StorageIntegrationObservation]{
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

	return snowflake.NewStorageIntegrationClient(sfC), cleanup, nil
}

func applyObservation(si *snowplanev1alpha1.StorageIntegration, obs *snowflake.StorageIntegrationObservation) {
	if obs.ShowOutput != nil {
		si.Status.FullyQualifiedName = obs.ShowOutput.Name

		si.Status.ShowOutput = &snowplanev1alpha1.StorageIntegrationShowOutput{
			CreatedOn: obs.ShowOutput.CreatedOn,
			Name:      obs.ShowOutput.Name,
			Type:      obs.ShowOutput.Type,
			Category:  obs.ShowOutput.Category,
			Enabled:   obs.ShowOutput.Enabled,
			Comment:   obs.ShowOutput.Comment,
		}
	}

	// Populate DESCRIBE-derived fields for user reference.
	if obs.DescribeOutput != nil {
		si.Status.StorageAWSIAMUserARN = obs.DescribeOutput["STORAGE_AWS_IAM_USER_ARN"]
		si.Status.StorageAWSExternalID = obs.DescribeOutput["STORAGE_AWS_EXTERNAL_ID"]
	}
}

func buildCreateOptions(si *snowplanev1alpha1.StorageIntegration, id snowflake.AccountObjectIdentifier) snowflake.CreateStorageIntegrationOptions {
	return snowflake.CreateStorageIntegrationOptions{
		Name:                    id,
		Type:                    string(si.Spec.Type),
		Enabled:                 si.Spec.Enabled,
		StorageProvider:         si.Spec.StorageProvider,
		StorageAllowedLocations: si.Spec.StorageAllowedLocations,
		StorageBlockedLocations: si.Spec.StorageBlockedLocations,
		StorageAWSRoleARN:       si.Spec.StorageAWSRoleARN,
		StorageAWSExternalID:    si.Spec.StorageAWSExternalID,
		AzureTenantID:           si.Spec.AzureTenantID,
		Comment:                 si.Spec.Comment,
	}
}

func buildAlterOptions(si *snowplanev1alpha1.StorageIntegration, id snowflake.AccountObjectIdentifier, obs *snowflake.StorageIntegrationObservation) snowflake.AlterStorageIntegrationOptions {
	opts := snowflake.AlterStorageIntegrationOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(si)

	// Enabled — always send if set.
	if si.Spec.Enabled != nil {
		if obs.ShowOutput == nil || *si.Spec.Enabled != obs.ShowOutput.Enabled {
			opts.Enabled = si.Spec.Enabled
		}
	}

	// Locations — compare sorted against observation before sending.
	specAllowed := sortedLocations(si.Spec.StorageAllowedLocations)
	obsAllowed := sortedLocations(parseLocations(obs, "STORAGE_ALLOWED_LOCATIONS"))

	if obs.DescribeOutput == nil || specAllowed != obsAllowed {
		locs := make([]string, len(si.Spec.StorageAllowedLocations))
		copy(locs, si.Spec.StorageAllowedLocations)
		opts.StorageAllowedLocations = &locs
	}

	if len(si.Spec.StorageBlockedLocations) > 0 {
		specBlocked := sortedLocations(si.Spec.StorageBlockedLocations)
		obsBlocked := sortedLocations(parseLocations(obs, "STORAGE_BLOCKED_LOCATIONS"))

		if obs.DescribeOutput == nil || specBlocked != obsBlocked {
			blocked := make([]string, len(si.Spec.StorageBlockedLocations))
			copy(blocked, si.Spec.StorageBlockedLocations)
			opts.StorageBlockedLocations = &blocked
		}
	}

	if si.Spec.StorageAWSRoleARN != nil {
		if obs.DescribeOutput == nil || *si.Spec.StorageAWSRoleARN != obs.DescribeOutput["STORAGE_AWS_ROLE_ARN"] {
			opts.StorageAWSRoleARN = si.Spec.StorageAWSRoleARN
		}
	}

	if si.Spec.StorageAWSExternalID != nil {
		if obs.DescribeOutput == nil || *si.Spec.StorageAWSExternalID != obs.DescribeOutput["STORAGE_AWS_EXTERNAL_ID"] {
			opts.StorageAWSExternalID = si.Spec.StorageAWSExternalID
		}
	}

	if si.Spec.AzureTenantID != nil {
		if obs.DescribeOutput == nil || *si.Spec.AzureTenantID != obs.DescribeOutput["AZURE_TENANT_ID"] {
			opts.AzureTenantID = si.Spec.AzureTenantID
		}
	}

	if si.Spec.Comment != nil {
		if obs.ShowOutput == nil || *si.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = si.Spec.Comment
		}
	}

	return opts
}

func computeUnsetFields(si *snowplanev1alpha1.StorageIntegration) []string {
	if len(si.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(si.Status.TrackedParameters))
	for _, f := range si.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if si.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	if len(si.Spec.StorageBlockedLocations) == 0 && managed["STORAGE_BLOCKED_LOCATIONS"] {
		unset = append(unset, "STORAGE_BLOCKED_LOCATIONS")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.StorageIntegrationSpec) []string {
	var fields []string

	if spec.Enabled != nil {
		fields = append(fields, "ENABLED")
	}

	if len(spec.StorageAllowedLocations) > 0 {
		fields = append(fields, "STORAGE_ALLOWED_LOCATIONS")
	}

	if len(spec.StorageBlockedLocations) > 0 {
		fields = append(fields, "STORAGE_BLOCKED_LOCATIONS")
	}

	if spec.StorageAWSRoleARN != nil {
		fields = append(fields, "STORAGE_AWS_ROLE_ARN")
	}

	if spec.StorageAWSExternalID != nil {
		fields = append(fields, "STORAGE_AWS_EXTERNAL_ID")
	}

	if spec.AzureTenantID != nil {
		fields = append(fields, "AZURE_TENANT_ID")
	}

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	return fields
}

func detectDrift(si *snowplanev1alpha1.StorageIntegration, obs *snowflake.StorageIntegrationObservation) *drift.Result {
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
		d.CompareStringValue("STORAGE_ALLOWED_LOCATIONS", allowedCSV, sortedLocations(parseLocations(obs, "STORAGE_ALLOWED_LOCATIONS")), false)

		// Blocked locations — compare sorted CSV.
		blockedCSV := sortedLocations(si.Spec.StorageBlockedLocations)
		d.CompareStringValue("STORAGE_BLOCKED_LOCATIONS", blockedCSV, sortedLocations(parseLocations(obs, "STORAGE_BLOCKED_LOCATIONS")), false)

		// Provider-specific config drift.
		if si.Spec.StorageAWSRoleARN != nil {
			d.CompareString("STORAGE_AWS_ROLE_ARN", si.Spec.StorageAWSRoleARN, obs.DescribeOutput["STORAGE_AWS_ROLE_ARN"], false)
		}

		if si.Spec.StorageAWSExternalID != nil {
			d.CompareString("STORAGE_AWS_EXTERNAL_ID", si.Spec.StorageAWSExternalID, obs.DescribeOutput["STORAGE_AWS_EXTERNAL_ID"], false)
		}

		if si.Spec.AzureTenantID != nil {
			d.CompareString("AZURE_TENANT_ID", si.Spec.AzureTenantID, obs.DescribeOutput["AZURE_TENANT_ID"], false)
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
func parseLocations(obs *snowflake.StorageIntegrationObservation, key string) []string {
	if obs.DescribeOutput == nil {
		return nil
	}

	raw, ok := obs.DescribeOutput[key]
	if !ok || raw == "" {
		return nil
	}

	// Snowflake returns locations as a comma-separated string.
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
