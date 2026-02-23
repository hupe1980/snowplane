// Package tag implements the reconciler for Tag resources.
package tag

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
	finalizerName = "snowplane.hupe1980.github.io/tag"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake tags.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error)
	Create(ctx context.Context, opts snowflake.CreateTagOptions) error
	Alter(ctx context.Context, opts snowflake.AlterTagOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Tag reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Tag, Service] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Tag, Service]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Tag, Service] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Tag, Service]{
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

	return snowflake.NewTagClient(sfC), cleanup, nil
}

func applyObservation(tag *snowplanev1alpha1.Tag, obs *snowflake.TagObservation) {
	if obs.ShowOutput != nil {
		tag.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		tag.Status.DatabaseName = obs.ShowOutput.DatabaseName
		tag.Status.SchemaName = obs.ShowOutput.SchemaName

		tag.Status.ShowOutput = &snowplanev1alpha1.TagShowOutput{
			CreatedOn:     obs.ShowOutput.CreatedOn,
			Name:          obs.ShowOutput.Name,
			DatabaseName:  obs.ShowOutput.DatabaseName,
			SchemaName:    obs.ShowOutput.SchemaName,
			Owner:         obs.ShowOutput.Owner,
			Comment:       obs.ShowOutput.Comment,
			AllowedValues: obs.ShowOutput.AllowedValues,
		}
	}
}

func buildCreateOptions(tag *snowplanev1alpha1.Tag, id snowflake.SchemaObjectIdentifier) snowflake.CreateTagOptions {
	return snowflake.CreateTagOptions{
		Name:          id,
		AllowedValues: tag.Spec.AllowedValues,
		Comment:       tag.Spec.Comment,
	}
}

func buildAlterOptions(tag *snowplanev1alpha1.Tag, id snowflake.SchemaObjectIdentifier, obs *snowflake.TagObservation) snowflake.AlterTagOptions {
	opts := snowflake.AlterTagOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(tag)

	// Compare allowed values.
	if len(tag.Spec.AllowedValues) > 0 {
		desiredSorted := make([]string, len(tag.Spec.AllowedValues))
		copy(desiredSorted, tag.Spec.AllowedValues)
		sort.Strings(desiredSorted)

		observedCSV := ""
		if obs.ShowOutput != nil {
			observedCSV = obs.ShowOutput.AllowedValues
		}

		if strings.Join(desiredSorted, ",") != observedCSV {
			av := make([]string, len(tag.Spec.AllowedValues))
			copy(av, tag.Spec.AllowedValues)
			opts.AllowedValues = &av
		}
	} else if obs.ShowOutput != nil && obs.ShowOutput.AllowedValues != "" {
		// Spec has no allowed values but Snowflake has them → unset.
		empty := []string{}
		opts.AllowedValues = &empty
	}

	// Compare comment.
	if tag.Spec.Comment != nil {
		if obs.ShowOutput == nil || *tag.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = tag.Spec.Comment
		}
	}

	return opts
}

func computeUnsetFields(tag *snowplanev1alpha1.Tag) []string {
	if len(tag.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(tag.Status.TrackedParameters))
	for _, f := range tag.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if tag.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.TagSpec) []string {
	var fields []string

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	if len(spec.AllowedValues) > 0 {
		fields = append(fields, "ALLOWED_VALUES")
	}

	return fields
}

func detectDrift(tag *snowplanev1alpha1.Tag, obs *snowflake.TagObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", tag.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(tag.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(tag.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Mutable fields.
		d.CompareString("COMMENT", tag.Spec.Comment, obs.ShowOutput.Comment, false)

		// Allowed values comparison: sort desired and compare to observed CSV.
		desiredCSV := ""
		if len(tag.Spec.AllowedValues) > 0 {
			sorted := make([]string, len(tag.Spec.AllowedValues))
			copy(sorted, tag.Spec.AllowedValues)
			sort.Strings(sorted)

			desiredCSV = strings.Join(sorted, ",")
		}

		d.CompareStringValue("ALLOWED_VALUES", desiredCSV, obs.ShowOutput.AllowedValues, false)
	}

	return d.Result()
}
