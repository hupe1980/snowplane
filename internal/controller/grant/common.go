// Package grant implements the reconciler for grant resources.
package grant

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// grantAlterOptions implements reconciler.AlterOptions for grants.
// Grants are immutable — no ALTER is ever needed. HasChanges always returns false.
type grantAlterOptions struct{}

func (grantAlterOptions) HasChanges() bool { return false }

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
		if s.SchemaName != "" {
			p.SchemaName = s.SchemaName
		} else if s.AllInDatabase != "" {
			p.AllSchemasInDB = s.AllInDatabase
		} else if s.FutureInDatabase != "" {
			p.FutureSchemasInDB = s.FutureInDatabase
		}

		return p
	}

	if so := on.SchemaObject; so != nil {
		if so.ObjectType != "" && so.ObjectName != "" {
			p.SchemaObjectType = so.ObjectType
			p.SchemaObjectName = so.ObjectName
		} else if so.All != nil {
			p.AllObjectsTypePlural = so.All.ObjectTypePlural
			p.AllObjectsInDB = so.All.InDatabase
			p.AllObjectsInSchema = so.All.InSchema
		} else if so.Future != nil {
			p.FutureObjectsTypePlural = so.Future.ObjectTypePlural
			p.FutureObjectsInDB = so.Future.InDatabase
			p.FutureObjectsInSchema = so.Future.InSchema
		}

		return p
	}

	return p
}

// resolveOnRefs resolves On-hierarchy refs in-place for the given GrantOn.
func resolveOnRefs(ctx context.Context, client sigs.Client, ns string, on *snowplanev1alpha1.GrantOn, handleErr func(kind, name string, err error) error) error {
	if s := on.Schema; s != nil {
		if ref := s.SchemaRef; ref != nil {
			fqn, err := refresolver.ResolveSchemaRef(ctx, client, ns, *ref)
			if err != nil {
				return handleErr("Schema", ref.Name, err)
			}

			on.Schema.SchemaName = fqn
			on.Schema.SchemaRef = nil
		}

		if ref := s.AllInDatabaseRef; ref != nil {
			fqn, err := refresolver.ResolveDatabaseRef(ctx, client, ns, *ref)
			if err != nil {
				return handleErr("Database", ref.Name, err)
			}

			on.Schema.AllInDatabase = fqn
			on.Schema.AllInDatabaseRef = nil
		}

		if ref := s.FutureInDatabaseRef; ref != nil {
			fqn, err := refresolver.ResolveDatabaseRef(ctx, client, ns, *ref)
			if err != nil {
				return handleErr("Database", ref.Name, err)
			}

			on.Schema.FutureInDatabase = fqn
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

				bulk.InDatabase = fqn
				bulk.InDatabaseRef = nil
			}

			if ref := bulk.InSchemaRef; ref != nil {
				fqn, err := refresolver.ResolveSchemaRef(ctx, client, ns, *ref)
				if err != nil {
					return handleErr("Schema", ref.Name, err)
				}

				bulk.InSchema = fqn
				bulk.InSchemaRef = nil
			}
		}

		if bulk := so.Future; bulk != nil {
			if ref := bulk.InDatabaseRef; ref != nil {
				fqn, err := refresolver.ResolveDatabaseRef(ctx, client, ns, *ref)
				if err != nil {
					return handleErr("Database", ref.Name, err)
				}

				bulk.InDatabase = fqn
				bulk.InDatabaseRef = nil
			}

			if ref := bulk.InSchemaRef; ref != nil {
				fqn, err := refresolver.ResolveSchemaRef(ctx, client, ns, *ref)
				if err != nil {
					return handleErr("Schema", ref.Name, err)
				}

				bulk.InSchema = fqn
				bulk.InSchemaRef = nil
			}
		}
	}

	return nil
}

// grantConditionedObject combines client.Object with the conditions interface
// so handleRefError can both set conditions and emit events.
type grantConditionedObject interface {
	sigs.Object
	conditions.ConditionedObject
}

// handleRefError emits an event and sets conditions when a reference cannot be resolved.
func handleRefError(ctx context.Context, recorder record.EventRecorder, obj grantConditionedObject, kind, name string, err error) error {
	msg := fmt.Sprintf("%s %q: %v", kind, name, err)
	conditions.SetReferencesNotResolved(obj, snowplanev1alpha1.ReasonDependencyNotReady, msg)
	conditions.SetNotReady(obj, snowplanev1alpha1.ReasonDependencyWait, msg)
	recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonDependencyNotReady, msg)

	if errors.Is(err, refresolver.ErrReferenceNotFound) || errors.Is(err, refresolver.ErrReferenceNotReady) {
		log.FromContext(ctx).Info("grant reference not resolved, requeuing", "kind", kind, "name", name, "error", err)
	}

	return fmt.Errorf("resolving %s ref %q: %w", kind, name, err)
}

// buildGrantIdentifier builds a GrantIdentifier from components.
func buildGrantIdentifier(
	on *snowplanev1alpha1.GrantOn,
	privilege string,
	toClause string,
	granteeName string,
	kind snowplanev1alpha1.GrantKind,
	isShare bool,
) snowflake.GrantIdentifier {
	params := onToParams(on)
	onClause := snowflake.BuildOnClause(params)
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
	}
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

// toToFrom converts a "TO ..." clause to a "FROM ..." clause for REVOKE.
func toToFrom(toClause string) string {
	if len(toClause) > 3 && toClause[:3] == "TO " {
		return "FROM " + toClause[3:]
	}

	return toClause
}

// caseInsensitiveEqual compares two strings case-insensitively.
func caseInsensitiveEqual(a, b string) bool {
	return strings.EqualFold(a, b)
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

// grantObserve queries Snowflake for the current state of a grant.
func grantObserve(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation, error) {
	gid, err := reconciler.AssertIdentifier[snowflake.GrantIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, gid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation{Exists: obs.Exists, Detail: obs}, nil
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
