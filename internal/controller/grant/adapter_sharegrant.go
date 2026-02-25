package grant

import (
	"context"
	"fmt"

	"k8s.io/client-go/tools/record"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
)

const (
	shareGrantFinalizer = "snowplane.hupe1980.github.io/sharegrant"
)

// shareGrantAdapter implements reconciler.ResourceAdapter for ShareGrant.
// Share grants are simpler: no On hierarchy, no refs, no WithGrantOption.
type shareGrantAdapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.ShareGrant, Service, *snowflake.GrantObservation] = (*shareGrantAdapter)(nil)

func (a *shareGrantAdapter) ResourceName() string  { return "sharegrant" }
func (a *shareGrantAdapter) FinalizerName() string { return shareGrantFinalizer }
func (a *shareGrantAdapter) NewObject() *snowplanev1alpha1.ShareGrant {
	return &snowplanev1alpha1.ShareGrant{}
}

func (a *shareGrantAdapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *shareGrantAdapter) SupportsCreateOrAlter() bool { return false }

// PreReconcile is a no-op for ShareGrant — no refs to resolve.
func (a *shareGrantAdapter) PreReconcile(_ context.Context, _ *snowplanev1alpha1.ShareGrant) error {
	return nil
}

// BuildIdentifier constructs a GrantIdentifier from the ShareGrant spec.
func (a *shareGrantAdapter) BuildIdentifier(grant *snowplanev1alpha1.ShareGrant) (reconciler.Identifier, error) {
	// Defense-in-depth: validate object type (CRD already validates).
	if err := sqlbuilder.ValidateObjectType(grant.Spec.ObjectType); err != nil {
		return nil, fmt.Errorf("shareGrantAdapter.BuildIdentifier: %w", err)
	}

	onClause := fmt.Sprintf("ON %s %s", grant.Spec.ObjectType, sqlbuilder.QuoteIdentifierParts(grant.Spec.ObjectName))
	toClause := snowflake.BuildToClause("", "", grant.Spec.Share)

	// Build show target for share grants.
	showTarget := "TO SHARE " + sqlbuilder.QuoteIdentifier(grant.Spec.Share)

	return snowflake.GrantIdentifier{
		Kind:             snowflake.GrantKindShare,
		Privilege:        grant.Spec.Privilege,
		OnClause:         onClause,
		ToClause:         toClause,
		GranteeName:      grant.Spec.Share,
		ShowGrantsTarget: showTarget,
	}, nil
}

// SetupWatches returns nil — ShareGrant has no refs to watch.
func (a *shareGrantAdapter) SetupWatches() reconciler.SetupWatchesFunc {
	return nil
}

// Observe queries Snowflake for the current state.
func (a *shareGrantAdapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.GrantObservation], error) {
	return grantObserve(ctx, svc, id)
}

// Create grants the privilege in Snowflake.
func (a *shareGrantAdapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.ShareGrant, _ reconciler.Identifier) error {
	if err := sqlbuilder.ValidateObjectType(obj.Spec.ObjectType); err != nil {
		return fmt.Errorf("invalid object type: %w", err)
	}

	onClause := fmt.Sprintf("ON %s %s", obj.Spec.ObjectType, sqlbuilder.QuoteIdentifierParts(obj.Spec.ObjectName))
	opts := snowflake.CreateGrantOptions{
		Privilege:       obj.Spec.Privilege,
		OnClause:        onClause,
		ToClause:        snowflake.BuildToClause("", "", obj.Spec.Share),
		WithGrantOption: false,
	}

	return svc.Grant(ctx, opts)
}

// Alter is a no-op for grants.
func (a *shareGrantAdapter) Alter(_ context.Context, _ Service, _ reconciler.AlterOptions) error {
	return nil
}

// Drop revokes the privilege.
func (a *shareGrantAdapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	return grantDrop(ctx, svc, id)
}

// ValidateImmutableFields checks immutability.
func (a *shareGrantAdapter) ValidateImmutableFields(_ context.Context, grant *snowplanev1alpha1.ShareGrant) error {
	if reconciler.ShouldSkipImmutableValidation(grant) {
		return nil
	}

	if grant.Status.ShowOutput != nil {
		if grant.Status.ShowOutput.Privilege != "" &&
			!caseInsensitiveEqual(grant.Spec.Privilege, grant.Status.ShowOutput.Privilege) {
			return fmt.Errorf("spec.privilege is immutable after creation (current: %q, desired: %q)",
				grant.Status.ShowOutput.Privilege, grant.Spec.Privilege)
		}
	}

	return nil
}

// BuildAlterOptions returns no-change options.
func (a *shareGrantAdapter) BuildAlterOptions(_ context.Context, _ *snowplanev1alpha1.ShareGrant, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.GrantObservation]) (reconciler.AlterOptions, error) {
	return grantAlterOptions{}, nil
}

// ApplyObservation maps the observation into the CR's status.
func (a *shareGrantAdapter) ApplyObservation(obj *snowplanev1alpha1.ShareGrant, obs *reconciler.Observation[*snowflake.GrantObservation]) {
	grantObs := obs.Detail
	if grantObs.ShowOutput != nil {
		// Object type is CRD-validated; QuoteIdentifierParts provides SQL-safe quoting.
		onClause := fmt.Sprintf("ON %s %s", obj.Spec.ObjectType, sqlbuilder.QuoteIdentifierParts(obj.Spec.ObjectName))
		toClause := snowflake.BuildToClause("", "", obj.Spec.Share)
		obj.Status.FullyQualifiedName = fmt.Sprintf("GRANT %s %s %s", grantObs.ShowOutput.Privilege, onClause, toClause)
		obj.Status.ShowOutput = applyGrantShowOutput(grantObs)
	}
}

// ComputeTrackedParameters returns nil.
func (a *shareGrantAdapter) ComputeTrackedParameters(_ *snowplanev1alpha1.ShareGrant) []string {
	return nil
}

// DetectDrift compares spec vs observation.
func (a *shareGrantAdapter) DetectDrift(obj *snowplanev1alpha1.ShareGrant, obs *reconciler.Observation[*snowflake.GrantObservation]) *drift.Result {
	detail := obs.Detail
	// Shares don't have WithGrantOption.
	return detectGrantDrift(obj.Spec.Privilege, false, detail)
}

// PostCreate is a no-op.
func (a *shareGrantAdapter) PostCreate(_ *snowplanev1alpha1.ShareGrant) {}

// PostUpdate is a no-op.
func (a *shareGrantAdapter) PostUpdate(_ *snowplanev1alpha1.ShareGrant, _ bool, _ reconciler.AlterOptions) {
}
