// Package stage implements the reconciler for Stage resources.
package stage

import (
	"context"

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
	finalizerName = "snowplane.hupe1980.github.io/stage"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake stages.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error)
	Create(ctx context.Context, opts snowflake.CreateStageOptions) error
	Alter(ctx context.Context, opts snowflake.AlterStageOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Stage reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Stage, Service, *snowflake.StageObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Stage, Service, *snowflake.StageObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Stage, Service, *snowflake.StageObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Stage, Service, *snowflake.StageObservation]{
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

	return snowflake.NewStageClient(sfC), cleanup, nil
}

func applyObservation(stage *snowplanev1alpha1.Stage, obs *snowflake.StageObservation) {
	if obs.ShowOutput != nil {
		stage.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		stage.Status.DatabaseName = obs.ShowOutput.DatabaseName
		stage.Status.SchemaName = obs.ShowOutput.SchemaName

		stage.Status.ShowOutput = &snowplanev1alpha1.StageShowOutput{
			CreatedOn:          obs.ShowOutput.CreatedOn,
			Name:               obs.ShowOutput.Name,
			DatabaseName:       obs.ShowOutput.DatabaseName,
			SchemaName:         obs.ShowOutput.SchemaName,
			URL:                obs.ShowOutput.URL,
			Owner:              obs.ShowOutput.Owner,
			Comment:            obs.ShowOutput.Comment,
			Type:               obs.ShowOutput.Type,
			StorageIntegration: obs.ShowOutput.StorageIntegration,
			DirectoryEnabled:   obs.ShowOutput.DirectoryEnabled,
		}
	}
}

func buildCreateOptions(stage *snowplanev1alpha1.Stage, id snowflake.SchemaObjectIdentifier) snowflake.CreateStageOptions {
	opts := snowflake.CreateStageOptions{
		Name:               id,
		URL:                stage.Spec.URL,
		StorageIntegration: stage.Spec.StorageIntegration,
		FileFormat:         stage.Spec.FileFormat,
		Comment:            stage.Spec.Comment,
	}

	if stage.Spec.Encryption != nil {
		opts.Encryption = &snowflake.StageEncryptionOptions{
			Type: stage.Spec.Encryption.Type,
		}
	}

	if stage.Spec.Directory != nil {
		opts.Directory = &snowflake.StageDirectoryCreateOptions{
			Enable:                  stage.Spec.Directory.Enable,
			AutoRefresh:             stage.Spec.Directory.AutoRefresh,
			RefreshOnCreate:         stage.Spec.Directory.RefreshOnCreate,
			NotificationIntegration: stage.Spec.Directory.NotificationIntegration,
		}
	}

	return opts
}

func buildAlterOptions(stage *snowplanev1alpha1.Stage, id snowflake.SchemaObjectIdentifier, obs *snowflake.StageObservation) snowflake.AlterStageOptions {
	opts := snowflake.AlterStageOptions{Name: id}

	if stage.Spec.Comment != nil {
		if obs.ShowOutput == nil || *stage.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = stage.Spec.Comment
		}
	}

	if obs.ShowOutput != nil {
		if stage.Spec.URL != nil && *stage.Spec.URL != obs.ShowOutput.URL {
			opts.URL = stage.Spec.URL
		}

		if stage.Spec.StorageIntegration != nil && *stage.Spec.StorageIntegration != obs.ShowOutput.StorageIntegration {
			opts.StorageIntegration = stage.Spec.StorageIntegration
		}
	}

	if stage.Spec.FileFormat != nil {
		opts.FileFormat = stage.Spec.FileFormat
	}

	if stage.Spec.Directory != nil {
		opts.Directory = &snowflake.StageDirectoryCreateOptions{
			Enable:                  stage.Spec.Directory.Enable,
			AutoRefresh:             stage.Spec.Directory.AutoRefresh,
			NotificationIntegration: stage.Spec.Directory.NotificationIntegration,
		}
	}

	opts.UnsetFields = computeUnsetFields(stage)

	return opts
}

func computeTrackedParameters(spec *snowplanev1alpha1.StageSpec) []string {
	var fields []string

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	if spec.URL != nil {
		fields = append(fields, "URL")
	}

	if spec.StorageIntegration != nil {
		fields = append(fields, "STORAGE_INTEGRATION")
	}

	if spec.FileFormat != nil {
		fields = append(fields, "FILE_FORMAT")
	}

	if spec.Directory != nil {
		fields = append(fields, "DIRECTORY")
	}

	return fields
}

// computeUnsetFields returns the Snowflake parameter names that were previously
// SET (tracked in status.TrackedParameters) but are now nil in the spec.
func computeUnsetFields(stage *snowplanev1alpha1.Stage) []string {
	if len(stage.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(stage.Status.TrackedParameters))
	for _, f := range stage.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if stage.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	if stage.Spec.URL == nil && managed["URL"] {
		unset = append(unset, "URL")
	}

	if stage.Spec.StorageIntegration == nil && managed["STORAGE_INTEGRATION"] {
		unset = append(unset, "STORAGE_INTEGRATION")
	}

	if stage.Spec.FileFormat == nil && managed["FILE_FORMAT"] {
		unset = append(unset, "FILE_FORMAT")
	}

	if stage.Spec.Directory == nil && managed["DIRECTORY"] {
		unset = append(unset, "DIRECTORY")
	}

	return unset
}

func detectDrift(stage *snowplanev1alpha1.Stage, obs *snowflake.StageObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", stage.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(stage.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(stage.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Stage type (internal/external) is immutable.
		expectedType := "INTERNAL"
		if stage.IsExternal() {
			expectedType = "EXTERNAL"
		}
		d.CompareStringValueFold("STAGE_TYPE", expectedType, obs.ShowOutput.Type, true)

		// Mutable fields.
		d.CompareString("COMMENT", stage.Spec.Comment, obs.ShowOutput.Comment, false)

		if stage.Spec.URL != nil {
			d.CompareStringValue("URL", *stage.Spec.URL, obs.ShowOutput.URL, false)
		}

		if stage.Spec.StorageIntegration != nil {
			d.CompareStringValue("STORAGE_INTEGRATION", *stage.Spec.StorageIntegration, obs.ShowOutput.StorageIntegration, false)
		}
	}

	return d.Result()
}
