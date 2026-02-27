package roleassignment

import (
	"context"
	"fmt"
	"strings"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
)

// roleAssignmentAlterOptions implements reconciler.AlterOptions for role assignments.
// Role assignments are immutable — no ALTER is ever needed.
type roleAssignmentAlterOptions struct{}

func (roleAssignmentAlterOptions) HasChanges() bool { return false }

// roleAssignmentObserve queries Snowflake for the current state of a role assignment.
func roleAssignmentObserve(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.RoleAssignmentObservation], error) {
	raID, err := reconciler.AssertIdentifier[snowflake.RoleAssignmentIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, raID)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.RoleAssignmentObservation]{Exists: obs.Exists, Detail: obs}, nil
}

// roleAssignmentDrop revokes a role assignment given an identifier.
func roleAssignmentDrop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	raID, err := reconciler.AssertIdentifier[snowflake.RoleAssignmentIdentifier](id)
	if err != nil {
		return err
	}

	opts := snowflake.RevokeRoleOptions{
		RoleName:       raID.RoleName,
		IsDatabaseRole: raID.IsDatabaseRole,
	}

	switch strings.ToUpper(raID.GrantedTo) {
	case "ROLE":
		opts.FromRole = raID.GranteeName
	case "USER":
		opts.FromUser = raID.GranteeName
	case "DATABASE_ROLE":
		opts.FromDatabaseRole = raID.GranteeName
	case "":
		return fmt.Errorf("grantedTo is empty in identifier %s — cannot determine revoke target", raID)
	default:
		return fmt.Errorf("unsupported grantedTo type %q in identifier %s", raID.GrantedTo, raID)
	}

	return svc.RevokeRole(ctx, opts)
}

// applyRoleAssignmentShowOutput populates role assignment show output fields.
func applyRoleAssignmentShowOutput(obs *snowflake.RoleAssignmentObservation) *snowplanev1alpha1.RoleAssignmentShowOutput {
	if obs.ShowOutput == nil {
		return nil
	}

	return &snowplanev1alpha1.RoleAssignmentShowOutput{
		CreatedOn:   obs.ShowOutput.CreatedOn,
		Role:        obs.ShowOutput.Role,
		GrantedTo:   obs.ShowOutput.GrantedTo,
		GranteeName: obs.ShowOutput.GranteeName,
		GrantedBy:   obs.ShowOutput.GrantedBy,
	}
}

// detectRoleAssignmentDrift detects drift for role assignment resources.
func detectRoleAssignmentDrift(grantedTo, granteeName string, obs *snowflake.RoleAssignmentObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("GRANTED_TO", grantedTo, obs.ShowOutput.GrantedTo, true)
		d.CompareStringValueFold("GRANTEE_NAME", granteeName, obs.ShowOutput.GranteeName, true)
	}

	return d.Result()
}
