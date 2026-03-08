// Package externalvolume implements the reconciler for ExternalVolume resources.
package externalvolume

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
	finalizerName = "snowplane.hupe1980.github.io/externalvolume"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake external volumes.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ExternalVolumeObservation, error)
	Create(ctx context.Context, opts snowflake.CreateExternalVolumeOptions) error
	Alter(ctx context.Context, opts snowflake.AlterExternalVolumeOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new ExternalVolume reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.ExternalVolume, Service, *snowflake.ExternalVolumeObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewExternalVolumeClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.ExternalVolume, Service, *snowflake.ExternalVolumeObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for ExternalVolume resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.ExternalVolume, Service, *snowflake.ExternalVolumeObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.ExternalVolume, Service, *snowflake.ExternalVolumeObservation]{
		ResourceNameVal:  "externalvolume",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.ExternalVolume { return &snowplanev1alpha1.ExternalVolume{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.ExternalVolume) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.ExternalVolumeObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.ExternalVolumeObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.ExternalVolume, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterExternalVolumeOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.ExternalVolume, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.ExternalVolumeObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.ExternalVolume, obs *reconciler.Observation[*snowflake.ExternalVolumeObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.ExternalVolume, obs *reconciler.Observation[*snowflake.ExternalVolumeObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, ev *snowplanev1alpha1.ExternalVolume) error {
	if reconciler.ShouldSkipImmutableValidation(ev) {
		return nil
	}

	if ev.Status.ShowOutput != nil {
		if ev.Status.ShowOutput.Name != "" && !strings.EqualFold(ev.Spec.Name, ev.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", ev.Status.ShowOutput.Name, ev.Spec.Name)
		}
	}

	return nil
}

func applyObservation(ev *snowplanev1alpha1.ExternalVolume, obs *snowflake.ExternalVolumeObservation) {
	if obs.ShowOutput != nil {
		ev.Status.FullyQualifiedName = obs.ShowOutput.Name
		ev.Status.ShowOutput = obs.ShowOutput
	}

	if len(obs.StorageLocationNames) > 0 {
		ev.Status.StorageLocationNames = obs.StorageLocationNames
	}
}

func buildCreateOptions(ev *snowplanev1alpha1.ExternalVolume, id snowflake.AccountObjectIdentifier) snowflake.CreateExternalVolumeOptions {
	return snowflake.CreateExternalVolumeOptions{
		Name:             id,
		StorageLocations: convertStorageLocations(ev.Spec.StorageLocations),
		AllowWrites:      ev.Spec.AllowWrites,
		Comment:          ev.Spec.Comment,
	}
}

func buildAlterOptions(ev *snowplanev1alpha1.ExternalVolume, id snowflake.AccountObjectIdentifier, obs *snowflake.ExternalVolumeObservation) snowflake.AlterExternalVolumeOptions {
	opts := snowflake.AlterExternalVolumeOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&ev.Spec, ev.Status.TrackedParameters)

	// AllowWrites drift detection.
	if ev.Spec.AllowWrites != nil {
		if obs.ShowOutput == nil || *ev.Spec.AllowWrites != obs.ShowOutput.AllowWrites {
			opts.AllowWrites = ev.Spec.AllowWrites
		}
	}

	// Comment drift detection.
	if ev.Spec.Comment != nil {
		if obs.ShowOutput == nil || *ev.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = ev.Spec.Comment
		}
	}

	// Storage location reconciliation: ADD new, REMOVE gone.
	specNames := storageLocationNameSet(ev.Spec.StorageLocations)
	obsNames := stringSet(obs.StorageLocationNames)

	// Locations to ADD: in spec but not in observed.
	for _, loc := range ev.Spec.StorageLocations {
		if _, exists := obsNames[strings.ToUpper(loc.Name)]; !exists {
			opts.AddLocations = append(opts.AddLocations, convertStorageLocation(loc))
		}
	}

	// Locations to REMOVE: in observed but not in spec.
	for _, obsName := range obs.StorageLocationNames {
		if _, exists := specNames[strings.ToUpper(obsName)]; !exists {
			opts.RemoveLocationNames = append(opts.RemoveLocationNames, obsName)
		}
	}

	return opts
}

func detectDrift(ev *snowplanev1alpha1.ExternalVolume, obs *snowflake.ExternalVolumeObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", ev.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields.
		d.CompareString("COMMENT", ev.Spec.Comment, obs.ShowOutput.Comment, false)

		if ev.Spec.AllowWrites != nil {
			obsAllowWrites := obs.ShowOutput.AllowWrites
			d.CompareBool("ALLOW_WRITES", ev.Spec.AllowWrites, &obsAllowWrites, false)
		}
	}

	// Detect storage location drift by comparing sorted name lists.
	if len(obs.StorageLocationNames) > 0 {
		specNames := sortedLocationNames(ev.Spec.StorageLocations)
		obsNames := sortedStrings(obs.StorageLocationNames)
		d.CompareStringValue("STORAGE_LOCATIONS", specNames, obsNames, false)
	}

	return d.Result()
}

// convertStorageLocations converts CRD storage locations to client options.
func convertStorageLocations(locs []snowplanev1alpha1.ExternalVolumeStorageLocation) []snowflake.ExternalVolumeStorageLocationOption {
	result := make([]snowflake.ExternalVolumeStorageLocationOption, len(locs))
	for i, loc := range locs {
		result[i] = convertStorageLocation(loc)
	}

	return result
}

// convertStorageLocation converts a single CRD storage location to a client option.
func convertStorageLocation(loc snowplanev1alpha1.ExternalVolumeStorageLocation) snowflake.ExternalVolumeStorageLocationOption {
	return snowflake.ExternalVolumeStorageLocationOption{
		Name:                 loc.Name,
		StorageProvider:      loc.StorageProvider,
		StorageBaseURL:       loc.StorageBaseURL,
		StorageAWSRoleARN:    loc.StorageAWSRoleARN,
		StorageAWSExternalID: loc.StorageAWSExternalID,
		EncryptionType:       loc.EncryptionType,
		EncryptionKMSKeyID:   loc.EncryptionKMSKeyID,
		AzureTenantID:        loc.AzureTenantID,
	}
}

// storageLocationNameSet builds an uppercase name set from spec locations.
func storageLocationNameSet(locs []snowplanev1alpha1.ExternalVolumeStorageLocation) map[string]struct{} {
	s := make(map[string]struct{}, len(locs))
	for _, loc := range locs {
		s[strings.ToUpper(loc.Name)] = struct{}{}
	}

	return s
}

// stringSet builds an uppercase name set from a string slice.
func stringSet(names []string) map[string]struct{} {
	s := make(map[string]struct{}, len(names))
	for _, n := range names {
		s[strings.ToUpper(n)] = struct{}{}
	}

	return s
}

// sortedLocationNames returns a comma-joined sorted list of spec location names.
func sortedLocationNames(locs []snowplanev1alpha1.ExternalVolumeStorageLocation) string {
	names := make([]string, len(locs))
	for i, loc := range locs {
		names[i] = strings.ToUpper(loc.Name)
	}

	sort.Strings(names)

	return strings.Join(names, ",")
}

// sortedStrings returns a comma-joined sorted uppercase list.
func sortedStrings(input []string) string {
	sorted := make([]string, len(input))
	for i, s := range input {
		sorted[i] = strings.ToUpper(s)
	}

	sort.Strings(sorted)

	return strings.Join(sorted, ",")
}
