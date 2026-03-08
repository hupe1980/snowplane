// Package grant implements the reconcilers for GrantPrivilegesToAccountRole,
// GrantPrivilegesToDatabaseRole, and GrantPrivilegesToShare resources.
package grant

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake grants.
// Unlike other resources, grants have no ALTER operation. They are either
// granted (created) or revoked (dropped).
type Service interface {
	Observe(ctx context.Context, id snowflake.GrantIdentifier) (*snowflake.GrantObservation, error)
	Grant(ctx context.Context, opts snowflake.CreateGrantOptions) error
	Revoke(ctx context.Context, opts snowflake.RevokeGrantOptions) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// grantAlterOptions implements reconciler.AlterOptions for grants.
// Grants are immutable — no ALTER is ever needed. HasChanges always returns false.
type grantAlterOptions struct{}

func (grantAlterOptions) HasChanges() bool { return false }

// ---------------------------------------------------------------------------
// Service implementation
// ---------------------------------------------------------------------------

// grantService wraps GrantClient to satisfy the Service interface.
type grantService struct {
	client *snowflake.GrantClient
}

func newGrantService(c *snowflake.GrantClient) *grantService {
	return &grantService{client: c}
}

func (s *grantService) Observe(ctx context.Context, id snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
	return s.client.Observe(ctx, id)
}

func (s *grantService) Grant(ctx context.Context, opts snowflake.CreateGrantOptions) error {
	return s.client.Grant(ctx, opts)
}

func (s *grantService) Revoke(ctx context.Context, opts snowflake.RevokeGrantOptions) error {
	return s.client.Revoke(ctx, opts)
}

// defaultServiceFactory returns the production ServiceFactory for grants.
func defaultServiceFactory() ServiceFactory {
	return reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
		return newGrantService(snowflake.NewGrantClient(exec))
	})
}

// ---------------------------------------------------------------------------
// CRUD helpers
// ---------------------------------------------------------------------------

// grantObserve queries Snowflake for the current state of a grant.
func grantObserve(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.GrantObservation], error) {
	gid, err := reconciler.AssertIdentifier[snowflake.GrantIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, gid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.GrantObservation]{Exists: obs.Exists, Detail: obs}, nil
}

// grantDrop revokes a privilege from Snowflake given an identifier.
func grantDrop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	grantID, err := reconciler.AssertIdentifier[snowflake.GrantIdentifier](id)
	if err != nil {
		return err
	}

	fromClause := toToFrom(grantID.ToClause)

	opts := snowflake.RevokeGrantOptions{
		Privilege:  grantID.Privilege,
		OnClause:   grantID.OnClause,
		FromClause: fromClause,
	}

	return svc.Revoke(ctx, opts)
}

// ---------------------------------------------------------------------------
// Identifier / observation helpers
// ---------------------------------------------------------------------------

// onToParams maps the GrantOn hierarchy to the flat OnClauseParams.
func onToParams(on *snowplanev1alpha1.GrantOn) snowflake.OnClauseParams {
	p := snowflake.OnClauseParams{}

	if on.Account {
		p.Account = true
		return p
	}

	if o := on.AccountObject; o != nil {
		p.AccountObjectType = o.ObjectType
		p.AccountObjectName = o.ObjectName
		return p
	}

	if s := on.Schema; s != nil {
		if s.SchemaName != nil {
			p.SchemaName = *s.SchemaName
		} else if s.AllInDatabase != nil {
			p.AllSchemasInDB = *s.AllInDatabase
		} else if s.FutureInDatabase != nil {
			p.FutureSchemasInDB = *s.FutureInDatabase
		}

		return p
	}

	if so := on.SchemaObject; so != nil {
		if so.ObjectType != "" && so.ObjectName != "" {
			p.SchemaObjectType = so.ObjectType
			p.SchemaObjectName = so.ObjectName
		} else if so.All != nil {
			p.AllObjectsTypePlural = so.All.ObjectTypePlural
			p.AllObjectsInDB = derefStr(so.All.InDatabase)
			p.AllObjectsInSchema = derefStr(so.All.InSchema)
		} else if so.Future != nil {
			p.FutureObjectsTypePlural = so.Future.ObjectTypePlural
			p.FutureObjectsInDB = derefStr(so.Future.InDatabase)
			p.FutureObjectsInSchema = derefStr(so.Future.InSchema)
		}

		return p
	}

	return p
}

// buildGrantIdentifier builds a GrantIdentifier from components.
func buildGrantIdentifier(
	on *snowplanev1alpha1.GrantOn,
	privilege string,
	toClause string,
	granteeName string,
	kind snowplanev1alpha1.GrantKind,
	isShare bool,
) (snowflake.GrantIdentifier, error) {
	params := onToParams(on)

	onClause, err := snowflake.BuildOnClause(params)
	if err != nil {
		return snowflake.GrantIdentifier{}, fmt.Errorf("buildGrantIdentifier: %w", err)
	}

	showTarget, future := snowflake.BuildShowGrantsTarget(params, "")

	if isShare {
		_, future = snowflake.BuildShowGrantsTarget(params, granteeName)
	}

	sfKind := snowflake.GrantKindRegular
	if future {
		sfKind = snowflake.GrantKindFuture
	} else if kind == snowplanev1alpha1.GrantKindAll {
		sfKind = snowflake.GrantKindAll
	} else if isShare {
		sfKind = snowflake.GrantKindShare
	}

	return snowflake.GrantIdentifier{
		Kind:             sfKind,
		Privilege:        privilege,
		OnClause:         onClause,
		ToClause:         toClause,
		GranteeName:      granteeName,
		ShowGrantsTarget: showTarget,
	}, nil
}

// applyGrantShowOutput populates grant show output fields from a GrantObservation.
func applyGrantShowOutput(obs *snowflake.GrantObservation) *snowplanev1alpha1.GrantShowOutput {
	if obs.ShowOutput == nil {
		return nil
	}

	return &snowplanev1alpha1.GrantShowOutput{
		CreatedOn:   obs.ShowOutput.CreatedOn,
		Privilege:   obs.ShowOutput.Privilege,
		GrantedOn:   obs.ShowOutput.GrantedOn,
		Name:        obs.ShowOutput.Name,
		GrantedTo:   obs.ShowOutput.GrantedTo,
		GranteeName: obs.ShowOutput.GranteeName,
		GrantOption: obs.ShowOutput.GrantOption,
		GrantedBy:   obs.ShowOutput.GrantedBy,
	}
}

// detectGrantDrift detects drift for grant resources.
func detectGrantDrift(privilege string, withGrantOption bool, obs *snowflake.GrantObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("PRIVILEGE", privilege, obs.ShowOutput.Privilege, true)
		d.CompareBoolValue("WITH_GRANT_OPTION", withGrantOption, obs.ShowOutput.GrantOption, true)
	}

	return d.Result()
}

// ---------------------------------------------------------------------------
// Immutability validation helpers
// ---------------------------------------------------------------------------

// validateImmutablePrivilege checks that the privilege has not changed.
func validateImmutablePrivilege(privilege string, showOutput *snowplanev1alpha1.GrantShowOutput) error {
	if showOutput.Privilege != "" &&
		!strings.EqualFold(privilege, showOutput.Privilege) {
		return fmt.Errorf("spec.privilege is immutable after creation (current: %q, desired: %q)",
			showOutput.Privilege, privilege)
	}

	return nil
}

// validateImmutableGrantOption checks that withGrantOption has not changed.
func validateImmutableGrantOption(withGrantOption bool, showOutput *snowplanev1alpha1.GrantShowOutput) error {
	if withGrantOption != showOutput.GrantOption {
		return fmt.Errorf("spec.withGrantOption is immutable after creation (current: %v, desired: %v)",
			showOutput.GrantOption, withGrantOption)
	}

	return nil
}

// warnUnknownPrivilege emits a Warning event if the privilege name is not in
// the known Snowflake privileges set. This is advisory only — the grant is
// still attempted because Snowflake may have added new privileges.
func warnUnknownPrivilege(recorder record.EventRecorder, obj runtime.Object, privilege string) {
	if !snowplanev1alpha1.IsKnownPrivilege(privilege) {
		recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonValidationFailed,
			fmt.Sprintf("privilege %q is not in the known Snowflake privileges set — check for typos", privilege))
	}
}

// ---------------------------------------------------------------------------
// Reference resolution
// ---------------------------------------------------------------------------

// resolveOnRefs resolves On-hierarchy refs in-place for the given GrantOn.
func resolveOnRefs(ctx context.Context, client sigs.Client, ns string, on *snowplanev1alpha1.GrantOn, handleErr func(kind, name string, err error) error) error {
	if s := on.Schema; s != nil {
		if ref := s.SchemaRef; ref != nil {
			fqn, err := refresolver.ResolveSchemaRef(ctx, client, ns, *ref)
			if err != nil {
				return handleErr("Schema", ref.Name, err)
			}

			on.Schema.SchemaName = &fqn
			on.Schema.SchemaRef = nil
		}

		if ref := s.AllInDatabaseRef; ref != nil {
			fqn, err := refresolver.ResolveDatabaseRef(ctx, client, ns, *ref)
			if err != nil {
				return handleErr("Database", ref.Name, err)
			}

			on.Schema.AllInDatabase = &fqn
			on.Schema.AllInDatabaseRef = nil
		}

		if ref := s.FutureInDatabaseRef; ref != nil {
			fqn, err := refresolver.ResolveDatabaseRef(ctx, client, ns, *ref)
			if err != nil {
				return handleErr("Database", ref.Name, err)
			}

			on.Schema.FutureInDatabase = &fqn
			on.Schema.FutureInDatabaseRef = nil
		}
	}

	if so := on.SchemaObject; so != nil {
		if bulk := so.All; bulk != nil {
			if ref := bulk.InDatabaseRef; ref != nil {
				fqn, err := refresolver.ResolveDatabaseRef(ctx, client, ns, *ref)
				if err != nil {
					return handleErr("Database", ref.Name, err)
				}

				bulk.InDatabase = &fqn
				bulk.InDatabaseRef = nil
			}

			if ref := bulk.InSchemaRef; ref != nil {
				fqn, err := refresolver.ResolveSchemaRef(ctx, client, ns, *ref)
				if err != nil {
					return handleErr("Schema", ref.Name, err)
				}

				bulk.InSchema = &fqn
				bulk.InSchemaRef = nil
			}
		}

		if bulk := so.Future; bulk != nil {
			if ref := bulk.InDatabaseRef; ref != nil {
				fqn, err := refresolver.ResolveDatabaseRef(ctx, client, ns, *ref)
				if err != nil {
					return handleErr("Database", ref.Name, err)
				}

				bulk.InDatabase = &fqn
				bulk.InDatabaseRef = nil
			}

			if ref := bulk.InSchemaRef; ref != nil {
				fqn, err := refresolver.ResolveSchemaRef(ctx, client, ns, *ref)
				if err != nil {
					return handleErr("Schema", ref.Name, err)
				}

				bulk.InSchema = &fqn
				bulk.InSchemaRef = nil
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// GrantPrivilegesToDatabaseRoleOn helpers
// ---------------------------------------------------------------------------

// dbRoleOnToParams maps the GrantPrivilegesToDatabaseRoleOn hierarchy to the flat OnClauseParams.
func dbRoleOnToParams(on *snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn) snowflake.OnClauseParams {
	p := snowflake.OnClauseParams{}

	if on.Database != nil {
		p.AccountObjectType = "DATABASE"
		p.AccountObjectName = *on.Database
		return p
	}

	if s := on.Schema; s != nil {
		if s.SchemaName != nil {
			p.SchemaName = *s.SchemaName
		} else if s.AllInDatabase != nil {
			p.AllSchemasInDB = *s.AllInDatabase
		} else if s.FutureInDatabase != nil {
			p.FutureSchemasInDB = *s.FutureInDatabase
		}

		return p
	}

	if so := on.SchemaObject; so != nil {
		if so.ObjectType != "" && so.ObjectName != "" {
			p.SchemaObjectType = so.ObjectType
			p.SchemaObjectName = so.ObjectName
		} else if so.All != nil {
			p.AllObjectsTypePlural = so.All.ObjectTypePlural
			p.AllObjectsInDB = derefStr(so.All.InDatabase)
			p.AllObjectsInSchema = derefStr(so.All.InSchema)
		} else if so.Future != nil {
			p.FutureObjectsTypePlural = so.Future.ObjectTypePlural
			p.FutureObjectsInDB = derefStr(so.Future.InDatabase)
			p.FutureObjectsInSchema = derefStr(so.Future.InSchema)
		}

		return p
	}

	return p
}

// buildDBRoleGrantIdentifier builds a GrantIdentifier for a GrantPrivilegesToDatabaseRole.
func buildDBRoleGrantIdentifier(
	on *snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn,
	privilege string,
	toClause string,
	granteeName string,
	kind snowplanev1alpha1.GrantKind,
) (snowflake.GrantIdentifier, error) {
	params := dbRoleOnToParams(on)

	onClause, err := snowflake.BuildOnClause(params)
	if err != nil {
		return snowflake.GrantIdentifier{}, fmt.Errorf("buildDBRoleGrantIdentifier: %w", err)
	}

	showTarget, future := snowflake.BuildShowGrantsTarget(params, "")

	sfKind := snowflake.GrantKindRegular
	if future {
		sfKind = snowflake.GrantKindFuture
	} else if kind == snowplanev1alpha1.GrantKindAll {
		sfKind = snowflake.GrantKindAll
	}

	return snowflake.GrantIdentifier{
		Kind:             sfKind,
		Privilege:        privilege,
		OnClause:         onClause,
		ToClause:         toClause,
		GranteeName:      granteeName,
		ShowGrantsTarget: showTarget,
	}, nil
}

// buildGrantPrivilegesToShareIdentifier builds a GrantIdentifier for a GrantPrivilegesToShare.
func buildGrantPrivilegesToShareIdentifier(
	on *snowplanev1alpha1.GrantPrivilegesToShareOn,
	privilege string,
	share string,
) snowflake.GrantIdentifier {
	onClause := snowflake.BuildShareOnClause(on.ObjectType(), on.ObjectName())
	toClause := snowflake.BuildToClause("", "", share)
	showTarget := "TO SHARE " + sqlbuilder.QuoteIdentifier(share)

	return snowflake.GrantIdentifier{
		Kind:             snowflake.GrantKindShare,
		Privilege:        privilege,
		OnClause:         onClause,
		ToClause:         toClause,
		GranteeName:      share,
		ShowGrantsTarget: showTarget,
	}
}

// resolveDBRoleOnRefs resolves On-hierarchy refs in-place for a GrantPrivilegesToDatabaseRoleOn.
func resolveDBRoleOnRefs(ctx context.Context, client sigs.Client, ns string, on *snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn, handleErr func(kind, name string, err error) error) error {
	if s := on.Schema; s != nil {
		if ref := s.SchemaRef; ref != nil {
			fqn, err := refresolver.ResolveSchemaRef(ctx, client, ns, *ref)
			if err != nil {
				return handleErr("Schema", ref.Name, err)
			}

			on.Schema.SchemaName = &fqn
			on.Schema.SchemaRef = nil
		}

		if ref := s.AllInDatabaseRef; ref != nil {
			fqn, err := refresolver.ResolveDatabaseRef(ctx, client, ns, *ref)
			if err != nil {
				return handleErr("Database", ref.Name, err)
			}

			on.Schema.AllInDatabase = &fqn
			on.Schema.AllInDatabaseRef = nil
		}

		if ref := s.FutureInDatabaseRef; ref != nil {
			fqn, err := refresolver.ResolveDatabaseRef(ctx, client, ns, *ref)
			if err != nil {
				return handleErr("Database", ref.Name, err)
			}

			on.Schema.FutureInDatabase = &fqn
			on.Schema.FutureInDatabaseRef = nil
		}
	}

	if so := on.SchemaObject; so != nil {
		if bulk := so.All; bulk != nil {
			if ref := bulk.InDatabaseRef; ref != nil {
				fqn, err := refresolver.ResolveDatabaseRef(ctx, client, ns, *ref)
				if err != nil {
					return handleErr("Database", ref.Name, err)
				}

				bulk.InDatabase = &fqn
				bulk.InDatabaseRef = nil
			}

			if ref := bulk.InSchemaRef; ref != nil {
				fqn, err := refresolver.ResolveSchemaRef(ctx, client, ns, *ref)
				if err != nil {
					return handleErr("Schema", ref.Name, err)
				}

				bulk.InSchema = &fqn
				bulk.InSchemaRef = nil
			}
		}

		if bulk := so.Future; bulk != nil {
			if ref := bulk.InDatabaseRef; ref != nil {
				fqn, err := refresolver.ResolveDatabaseRef(ctx, client, ns, *ref)
				if err != nil {
					return handleErr("Database", ref.Name, err)
				}

				bulk.InDatabase = &fqn
				bulk.InDatabaseRef = nil
			}

			if ref := bulk.InSchemaRef; ref != nil {
				fqn, err := refresolver.ResolveSchemaRef(ctx, client, ns, *ref)
				if err != nil {
					return handleErr("Schema", ref.Name, err)
				}

				bulk.InSchema = &fqn
				bulk.InSchemaRef = nil
			}
		}
	}

	return nil
}

// hasDBRoleOnRefs checks if the GrantPrivilegesToDatabaseRoleOn hierarchy contains any unresolved refs.
func hasDBRoleOnRefs(on *snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn) bool {
	if s := on.Schema; s != nil {
		if s.SchemaRef != nil || s.AllInDatabaseRef != nil || s.FutureInDatabaseRef != nil {
			return true
		}
	}

	if so := on.SchemaObject; so != nil {
		if bulk := so.All; bulk != nil {
			if bulk.InDatabaseRef != nil || bulk.InSchemaRef != nil {
				return true
			}
		}

		if bulk := so.Future; bulk != nil {
			if bulk.InDatabaseRef != nil || bulk.InSchemaRef != nil {
				return true
			}
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Watch / index helpers
// ---------------------------------------------------------------------------

// extractDatabaseRefsFromOn collects all Database ref names from a GrantOn, deduped.
func extractDatabaseRefsFromOn(on *snowplanev1alpha1.GrantOn) []string {
	var refs []string

	if s := on.Schema; s != nil {
		if ref := s.AllInDatabaseRef; ref != nil {
			refs = append(refs, ref.Name)
		}

		if ref := s.FutureInDatabaseRef; ref != nil {
			refs = append(refs, ref.Name)
		}
	}

	if so := on.SchemaObject; so != nil {
		if bulk := so.All; bulk != nil {
			if ref := bulk.InDatabaseRef; ref != nil {
				refs = append(refs, ref.Name)
			}
		}

		if bulk := so.Future; bulk != nil {
			if ref := bulk.InDatabaseRef; ref != nil {
				refs = append(refs, ref.Name)
			}
		}
	}

	return dedupStrings(refs)
}

// extractSchemaRefsFromOn collects all Schema ref names from a GrantOn, deduped.
func extractSchemaRefsFromOn(on *snowplanev1alpha1.GrantOn) []string {
	var refs []string

	if s := on.Schema; s != nil {
		if ref := s.SchemaRef; ref != nil {
			refs = append(refs, ref.Name)
		}
	}

	if so := on.SchemaObject; so != nil {
		if bulk := so.All; bulk != nil {
			if ref := bulk.InSchemaRef; ref != nil {
				refs = append(refs, ref.Name)
			}
		}

		if bulk := so.Future; bulk != nil {
			if ref := bulk.InSchemaRef; ref != nil {
				refs = append(refs, ref.Name)
			}
		}
	}

	return dedupStrings(refs)
}

// extractDatabaseRefsFromDBRoleOn collects all Database ref names from a GrantPrivilegesToDatabaseRoleOn, deduped.
func extractDatabaseRefsFromDBRoleOn(on *snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn) []string {
	var refs []string

	if s := on.Schema; s != nil {
		if ref := s.AllInDatabaseRef; ref != nil {
			refs = append(refs, ref.Name)
		}

		if ref := s.FutureInDatabaseRef; ref != nil {
			refs = append(refs, ref.Name)
		}
	}

	if so := on.SchemaObject; so != nil {
		if bulk := so.All; bulk != nil {
			if ref := bulk.InDatabaseRef; ref != nil {
				refs = append(refs, ref.Name)
			}
		}

		if bulk := so.Future; bulk != nil {
			if ref := bulk.InDatabaseRef; ref != nil {
				refs = append(refs, ref.Name)
			}
		}
	}

	return dedupStrings(refs)
}

// extractSchemaRefsFromDBRoleOn collects all Schema ref names from a GrantPrivilegesToDatabaseRoleOn, deduped.
func extractSchemaRefsFromDBRoleOn(on *snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn) []string {
	var refs []string

	if s := on.Schema; s != nil {
		if ref := s.SchemaRef; ref != nil {
			refs = append(refs, ref.Name)
		}
	}

	if so := on.SchemaObject; so != nil {
		if bulk := so.All; bulk != nil {
			if ref := bulk.InSchemaRef; ref != nil {
				refs = append(refs, ref.Name)
			}
		}

		if bulk := so.Future; bulk != nil {
			if ref := bulk.InSchemaRef; ref != nil {
				refs = append(refs, ref.Name)
			}
		}
	}

	return dedupStrings(refs)
}

// hasOnRefs checks if the GrantOn hierarchy contains any unresolved refs.
func hasOnRefs(on *snowplanev1alpha1.GrantOn) bool {
	if s := on.Schema; s != nil {
		if s.SchemaRef != nil || s.AllInDatabaseRef != nil || s.FutureInDatabaseRef != nil {
			return true
		}
	}

	if so := on.SchemaObject; so != nil {
		if bulk := so.All; bulk != nil {
			if bulk.InDatabaseRef != nil || bulk.InSchemaRef != nil {
				return true
			}
		}

		if bulk := so.Future; bulk != nil {
			if bulk.InDatabaseRef != nil || bulk.InSchemaRef != nil {
				return true
			}
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// String utilities
// ---------------------------------------------------------------------------

// derefStr safely dereferences a *string, returning "" if nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// toToFrom converts a "TO ..." clause to a "FROM ..." clause for REVOKE.
func toToFrom(toClause string) string {
	if len(toClause) > 3 && toClause[:3] == "TO " {
		return "FROM " + toClause[3:]
	}

	return toClause
}

// dedupStrings returns a deduplicated copy of the input slice.
func dedupStrings(ss []string) []string {
	if len(ss) <= 1 {
		return ss
	}

	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))

	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}

	return out
}
