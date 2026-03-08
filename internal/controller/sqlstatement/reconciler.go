// Package sqlstatement implements the reconciler for SQLStatement resources.
//
// SQLStatement is a non-standard resource: instead of mapping to a single
// Snowflake DDL object, it executes user-provided SQL verbatim. The reconciler
// adapts this to the generic framework by packing the observe/revert/execute
// SQL into the identifier, which the Observe/Drop closures read back:
//
//   - Observe: runs spec.observe SQL and checks observeExpect expectations.
//   - Create: runs spec.execute SQL and records the execute hash.
//   - Alter: no-op (execute changes are detected as immutable field violations,
//     triggering force-new semantics — the user deletes and recreates the CR).
//   - Drop: runs spec.revert SQL (when set).
//
// The controller is gated behind --enable-sql-statement due to the inherent
// risks of arbitrary SQL execution.
package sqlstatement

import (
	"context"
	"fmt"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	sqlstmtclient "github.com/hupe1980/snowplane/internal/clients/snowflake/sqlstatement"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/metrics"
	"github.com/hupe1980/snowplane/internal/ratelimit"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/sqlstatement"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs.
type Service interface {
	Execute(ctx context.Context, sql string) error
	Revert(ctx context.Context, sql string) error
	Observe(ctx context.Context, observeSQL string, expectations []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error)
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// sqlStatementIdentifier implements reconciler.Identifier for SQLStatement.
// It carries the observe/revert/execute SQL so the Observe/Drop closures
// can access them without needing the CRD object. This is the standard
// pattern for non-standard resources where the identifier needs to carry
// context beyond a simple Snowflake name.
//
// The executeHash field is used to detect whether the execute SQL has
// already run (non-empty hash = already executed). When no observe SQL is
// configured, this allows ObserveFn to report "exists" after the first
// successful execution, preventing the infinite re-execute loop that
// would otherwise occur.
type sqlStatementIdentifier struct {
	name         string
	namespace    string // needed for audit metrics
	observeSQL   string
	expectations []sqlstmtclient.Expectation
	revertSQL    string
	executeHash  string // from status — non-empty after first successful execute
}

func (id sqlStatementIdentifier) FullyQualifiedName() string { return id.name }
func (id sqlStatementIdentifier) String() string             { return id.name }

// noopAlterOptions implements reconciler.AlterOptions for SQLStatement.
// SQLStatement does not support ALTER — execute changes trigger force-new.
type noopAlterOptions struct{}

func (o *noopAlterOptions) HasChanges() bool { return false }

// NewReconciler returns a new SQLStatement reconciler backed by the generic framework.
// The denylist parameter (may be nil) blocks SQL statements matching configured patterns
// before they are executed against Snowflake (H1 hardening).
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter, denylist *StatementDenylist) *reconciler.GenericReconciler[*snowplanev1alpha1.SQLStatement, Service, *sqlstmtclient.Observation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl, denylist,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return sqlstmtclient.NewClient(exec)
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
	denylist *StatementDenylist,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.SQLStatement, Service, *sqlstmtclient.Observation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf, denylist))
}

// newAdapter creates the BaseAdapter for SQLStatement resources.
func newAdapter(sf ServiceFactory, denylist *StatementDenylist) *reconciler.BaseAdapter[*snowplanev1alpha1.SQLStatement, Service, *sqlstmtclient.Observation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.SQLStatement, Service, *sqlstmtclient.Observation]{
		ResourceNameVal:  "sqlstatement",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.SQLStatement { return &snowplanev1alpha1.SQLStatement{} },
		ServiceFactoryFn: sf,

		// BuildIdentifier packs the observe/revert SQL into the identifier so
		// the Observe and Drop closures can access them without needing the
		// CRD object directly.
		BuildIdentifierFn: func(obj *snowplanev1alpha1.SQLStatement) (reconciler.Identifier, error) {
			var observeSQL string
			if obj.Spec.Observe != nil {
				observeSQL = *obj.Spec.Observe
			}

			var revertSQL string
			if obj.Spec.Revert != nil {
				revertSQL = *obj.Spec.Revert
			}

			return sqlStatementIdentifier{
				name:         obj.Name,
				namespace:    obj.Namespace,
				observeSQL:   observeSQL,
				expectations: specExpectationsToClient(obj.Spec.ObserveExpect),
				revertSQL:    revertSQL,
				executeHash:  obj.Status.ExecuteHash,
			}, nil
		},

		// Observe runs the user's observe SQL with expectations.
		ObserveFn: func(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*sqlstmtclient.Observation], error) {
			sid := id.(sqlStatementIdentifier)

			if sid.observeSQL == "" {
				// No observe query — cannot determine existence via SQL.
				// Use the executeHash from status to decide:
				// - Empty hash → never executed → Exists: false → enters create path.
				// - Non-empty hash → already executed → Exists: true → enters update path.
				//
				// Without this check, the reconciler enters reconcileCreate on every
				// loop (ObserveFn would always return Exists:false), re-executing the
				// SQL every ~5 seconds — an infinite re-execute loop that corrupts
				// state for non-idempotent statements (INSERT, GRANT ON ALL, etc.).
				alreadyExecuted := sid.executeHash != ""

				return &reconciler.Observation[*sqlstmtclient.Observation]{
					Exists: alreadyExecuted,
					Detail: &sqlstmtclient.Observation{
						Exists:  alreadyExecuted,
						Matched: alreadyExecuted,
					},
				}, nil
			}

			obs, err := svc.Observe(ctx, sid.observeSQL, sid.expectations)
			if err != nil {
				return nil, err
			}

			return &reconciler.Observation[*sqlstmtclient.Observation]{
				Exists: obs.Exists,
				Detail: obs,
			}, nil
		},

		// Create runs the execute SQL and records the hash.
		CreateFn: func(ctx context.Context, svc Service, obj *snowplanev1alpha1.SQLStatement, _ reconciler.Identifier) error {
			// H1: Check execute SQL against statement denylist.
			if err := denylist.Check(obj.Spec.Execute); err != nil {
				metrics.RecordSQLStatementDenied(obj.Namespace, "execute")
				return snowflake.NewTerminalError(err)
			}

			if err := svc.Execute(ctx, obj.Spec.Execute); err != nil {
				return err
			}

			// H1: Audit trail for arbitrary SQL execution.
			metrics.RecordSQLStatementExecution(obj.Namespace, "execute")

			// Record the execute hash so we can detect changes.
			obj.Status.ExecuteHash = sqlstmtclient.HashSQL(obj.Spec.Execute)

			return nil
		},

		// Alter is a no-op — execute changes are caught by immutable validation.
		AlterFn: func(_ context.Context, _ Service, _ reconciler.AlterOptions) error {
			return nil
		},

		// Drop runs the revert SQL when set.
		DropFn: func(ctx context.Context, svc Service, id reconciler.Identifier) error {
			sid := id.(sqlStatementIdentifier)
			if sid.revertSQL == "" {
				// No revert SQL — nothing to clean up in Snowflake.
				return nil
			}

			// H1: Check revert SQL against statement denylist.
			if err := denylist.Check(sid.revertSQL); err != nil {
				metrics.RecordSQLStatementDenied(sid.namespace, "revert")
				return snowflake.NewTerminalError(err)
			}

			if err := svc.Revert(ctx, sid.revertSQL); err != nil {
				return err
			}

			// H1: Audit trail for revert SQL execution.
			metrics.RecordSQLStatementExecution(sid.namespace, "revert")

			return nil
		},

		ValidateImmutableFn: validateImmutableFields,

		BuildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.SQLStatement, _ reconciler.Identifier, _ *reconciler.Observation[*sqlstmtclient.Observation]) (reconciler.AlterOptions, error) {
			return &noopAlterOptions{}, nil
		},

		ApplyObservationFn: func(obj *snowplanev1alpha1.SQLStatement, obs *reconciler.Observation[*sqlstmtclient.Observation]) {
			if obs.Detail != nil {
				obj.Status.ObserveResult = &snowplanev1alpha1.SQLStatementObserveResult{
					RowCount: obs.Detail.RowCount,
					Matched:  obs.Detail.Matched,
				}

				obj.Status.FullyQualifiedName = obj.Name
			}
		},

		DetectDriftFn: func(obj *snowplanev1alpha1.SQLStatement, obs *reconciler.Observation[*sqlstmtclient.Observation]) *drift.Result {
			d := drift.New()

			if obs.Detail != nil && len(obj.Spec.ObserveExpect) > 0 {
				d.CompareBoolValue("OBSERVE_EXPECT", true, obs.Detail.Matched, false)
			}

			return d.Result()
		},

		TrackedParamsFn: func(_ *snowplanev1alpha1.SQLStatement) []string { return nil },
		SupportsCoA:     false,
	}
}

// validateImmutableFields checks that execute SQL hasn't changed since
// the resource was created. Changes require the force-new annotation.
func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.SQLStatement) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ExecuteHash != "" {
		currentHash := sqlstmtclient.HashSQL(obj.Spec.Execute)
		if currentHash != obj.Status.ExecuteHash {
			return fmt.Errorf("spec.execute is immutable after execution (use the snowplane.hupe1980.github.io/force-new annotation to delete and recreate)")
		}
	}

	return nil
}

// specExpectationsToClient converts API expectations to client expectations.
func specExpectationsToClient(exps []snowplanev1alpha1.SQLStatementExpectation) []sqlstmtclient.Expectation {
	out := make([]sqlstmtclient.Expectation, len(exps))
	for i, exp := range exps {
		out[i] = sqlstmtclient.Expectation{
			Column: exp.Column,
			Value:  exp.Value,
		}
	}

	return out
}
