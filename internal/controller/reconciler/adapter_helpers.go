package reconciler

import (
	"context"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/ratelimit"
)

// ---------------------------------------------------------------------------
// CRUD helper factories — create closures that handle the repetitive
// AssertIdentifier / AssertAlterOptions / Observation wrapping boilerplate.
// Type inference allows callers to omit explicit type parameters.
// ---------------------------------------------------------------------------

// MakeObserve creates an Observe closure that asserts the identifier type,
// calls the concrete observe function, and wraps the result in an Observation.
func MakeObserve[S any, D any, ID Identifier](
	observeFn func(context.Context, S, ID) (D, error),
	exists func(D) bool,
) func(context.Context, S, Identifier) (*Observation[D], error) {
	return func(ctx context.Context, svc S, id Identifier) (*Observation[D], error) {
		sfID, err := AssertIdentifier[ID](id)
		if err != nil {
			return nil, err
		}

		detail, err := observeFn(ctx, svc, sfID)
		if err != nil {
			return nil, err
		}

		return &Observation[D]{Exists: exists(detail), Detail: detail}, nil
	}
}

// MakeCreate creates a Create closure that asserts the identifier type
// and calls the concrete create function.
func MakeCreate[T ManagedResource, S any, ID Identifier](
	createFn func(context.Context, S, T, ID) error,
) func(context.Context, S, T, Identifier) error {
	return func(ctx context.Context, svc S, obj T, id Identifier) error {
		sfID, err := AssertIdentifier[ID](id)
		if err != nil {
			return err
		}

		return createFn(ctx, svc, obj, sfID)
	}
}

// MakeAlter creates an Alter closure that asserts the concrete alter options
// type and calls the concrete alter function. A must be the pointer type
// (e.g. *AlterDatabaseOptions) that satisfies AlterOptions.
func MakeAlter[S any, A AlterOptions](
	alterFn func(context.Context, S, A) error,
) func(context.Context, S, AlterOptions) error {
	return func(ctx context.Context, svc S, opts AlterOptions) error {
		ao, err := AssertAlterOptions[A](opts)
		if err != nil {
			return err
		}

		return alterFn(ctx, svc, ao)
	}
}

// MakeDrop creates a Drop closure that asserts the identifier type
// and calls the concrete drop function.
func MakeDrop[S any, ID Identifier](
	dropFn func(context.Context, S, ID) error,
) func(context.Context, S, Identifier) error {
	return func(ctx context.Context, svc S, id Identifier) error {
		sfID, err := AssertIdentifier[ID](id)
		if err != nil {
			return err
		}

		return dropFn(ctx, svc, sfID)
	}
}

// MakeBuildAlterOpts creates a BuildAlterOptions closure that asserts the
// identifier type and passes the observation detail to the build function.
func MakeBuildAlterOpts[T ManagedResource, D any, ID Identifier](
	buildFn func(context.Context, T, ID, *Observation[D]) (AlterOptions, error),
) func(context.Context, T, Identifier, *Observation[D]) (AlterOptions, error) {
	return func(ctx context.Context, obj T, id Identifier, obs *Observation[D]) (AlterOptions, error) {
		sfID, err := AssertIdentifier[ID](id)
		if err != nil {
			return nil, err
		}

		return buildFn(ctx, obj, sfID, obs)
	}
}

// ---------------------------------------------------------------------------
// Service factory helper
// ---------------------------------------------------------------------------

// MakeServiceFactory creates a ServiceFactory closure using the standard
// WithUseRole + newClient pattern common to all adapters.
func MakeServiceFactory[S any](
	newClient func(snowflake.SQLExecutor) S,
) func(context.Context, clientfactory.SnowflakeClient, string) (S, func(context.Context), error) {
	return func(ctx context.Context, sfClient clientfactory.SnowflakeClient, useRole string) (S, func(context.Context), error) {
		sfC, cleanup, err := WithUseRole(ctx, sfClient, useRole)
		if err != nil {
			var zero S
			return zero, nil, err
		}

		return newClient(sfC), cleanup, nil
	}
}

// ---------------------------------------------------------------------------
// GenericReconciler constructor helper
// ---------------------------------------------------------------------------

// NewGenericReconciler constructs a GenericReconciler with the given adapter.
// This eliminates the repeated struct literal construction in each adapter
// package's NewReconciler function.
func NewGenericReconciler[T ManagedResource, S any, D any](
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	adapter ResourceAdapter[T, S, D],
) *GenericReconciler[T, S, D] {
	return &GenericReconciler[T, S, D]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     adapter,
	}
}
