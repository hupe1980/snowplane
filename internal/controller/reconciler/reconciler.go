package reconciler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/circuitbreaker"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/metrics"
	"github.com/hupe1980/snowplane/internal/provider"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/sfretry"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
	"github.com/hupe1980/snowplane/internal/utils/finalizers"
)

const (
	// DefaultRequeueInterval is the default periodic-resync interval.
	DefaultRequeueInterval = 5 * time.Minute

	// DefaultSnowflakeOpTimeout is the default per-operation timeout for
	// Snowflake CRUD calls (Observe, Create, Alter, Drop).
	DefaultSnowflakeOpTimeout = 60 * time.Second

	// DefaultReconcileTimeout is the maximum duration for a single Reconcile
	// call. This prevents a blocked reconcile goroutine from occupying a
	// worker indefinitely. The controller-runtime default is 10 minutes;
	// we tighten this to 5 minutes for faster failure detection.
	DefaultReconcileTimeout = 5 * time.Minute

	// StatusFieldOwner is the SSA field manager for status patches.
	// Using a dedicated field owner ensures the controller has exclusive
	// ownership of .status fields and eliminates conflict-based retries.
	StatusFieldOwner = "snowplane-controller"
)

// GenericReconciler implements the shared Observe-Diff-Apply state machine.
// Resource-specific behaviour is delegated to the ResourceAdapter.
//
// Type parameters:
//   - T: the CRD type (e.g. *snowplanev1alpha1.Database)
//   - S: the Snowflake CRUD service interface (e.g. database.Service)
//   - D: the resource-specific observation detail type (e.g. *snowflake.DatabaseObservation)
type GenericReconciler[T ManagedResource, S any, D any] struct {
	Client         client.Client
	Factory        *clientfactory.ClientFactory
	Recorder       record.EventRecorder
	RateLimiter    *ratelimit.Limiter
	CircuitBreaker *circuitbreaker.Breaker
	Adapter        ResourceAdapter[T, S, D]
	GVK            schema.GroupVersionKind // set during SetupWithManager or manually in tests

	requeueOverride      time.Duration
	snowflakeOpTimeout   time.Duration       // 0 → DefaultSnowflakeOpTimeout
	reconcileTimeout     time.Duration       // 0 → DefaultReconcileTimeout
	maturity             string              // alpha, beta, stable (default: alpha)
	enableAlphaResources bool                // gate: register alpha controllers only when true
	disabled             bool                // explicit disable via --disable-controllers
	shardPredicate       predicate.Predicate // optional hash-based shard filter
}

// WithRequeueInterval overrides the default periodic-resync interval.
func (r *GenericReconciler[T, S, D]) WithRequeueInterval(d time.Duration) *GenericReconciler[T, S, D] {
	r.requeueOverride = d
	return r
}

// WithMaturity sets the maturity classification for this controller.
// Valid values are "alpha", "beta", and "stable". Defaults to "alpha".
func (r *GenericReconciler[T, S, D]) WithMaturity(m string) *GenericReconciler[T, S, D] {
	r.maturity = m
	return r
}

// WithAlphaEnabled controls whether alpha-maturity controllers are registered.
// When false, SetupWithManager will skip alpha controllers.
func (r *GenericReconciler[T, S, D]) WithAlphaEnabled(enabled bool) *GenericReconciler[T, S, D] {
	r.enableAlphaResources = enabled
	return r
}

// WithCircuitBreaker configures a per-provider circuit breaker for this
// controller. When set, the reconciler rejects calls to providers that have
// exceeded the failure threshold and records success/failure after each
// reconciliation.
func (r *GenericReconciler[T, S, D]) WithCircuitBreaker(cb *circuitbreaker.Breaker) *GenericReconciler[T, S, D] {
	r.CircuitBreaker = cb
	return r
}

// WithDisabled explicitly disables this controller. When true, SetupWithManager
// will skip registration regardless of maturity settings. This is used by the
// --disable-controllers flag for fine-grained per-controller control.
func (r *GenericReconciler[T, S, D]) WithDisabled(disabled bool) *GenericReconciler[T, S, D] {
	r.disabled = disabled
	return r
}

func (r *GenericReconciler[T, S, D]) getMaturity() string {
	if r.maturity != "" {
		return r.maturity
	}

	return snowplanev1alpha1.MaturityAlpha
}

// supportsCreateOrAlter returns true when the adapter implements
// CreateOrAlterSupporter and reports support, false otherwise.
func (r *GenericReconciler[T, S, D]) supportsCreateOrAlter() bool {
	if s, ok := r.Adapter.(CreateOrAlterSupporter); ok {
		return s.SupportsCreateOrAlter()
	}

	return false
}

// invokePostCreate calls the PostCreateHook if the adapter implements it.
func (r *GenericReconciler[T, S, D]) invokePostCreate(obj T) {
	if h, ok := r.Adapter.(PostCreateHook[T]); ok {
		h.PostCreate(obj)
	}
}

// invokePostUpdate calls the PostUpdateHook if the adapter implements it.
func (r *GenericReconciler[T, S, D]) invokePostUpdate(obj T, altered bool, alterOpts AlterOptions) {
	if h, ok := r.Adapter.(PostUpdateHook[T]); ok {
		h.PostUpdate(obj, altered, alterOpts)
	}
}

func (r *GenericReconciler[T, S, D]) getRequeueInterval() time.Duration {
	if r.requeueOverride > 0 {
		return r.requeueOverride
	}

	return DefaultRequeueInterval
}

func (r *GenericReconciler[T, S, D]) getSnowflakeOpTimeout() time.Duration {
	if r.snowflakeOpTimeout > 0 {
		return r.snowflakeOpTimeout
	}

	return DefaultSnowflakeOpTimeout
}

// WithSnowflakeOpTimeout overrides the per-operation timeout for Snowflake
// CRUD calls (Observe, Create, Alter, Drop).
func (r *GenericReconciler[T, S, D]) WithSnowflakeOpTimeout(d time.Duration) *GenericReconciler[T, S, D] {
	r.snowflakeOpTimeout = d
	return r
}

// WithReconcileTimeout overrides the overall reconcile timeout. This is the
// maximum time a single Reconcile call may take before the context is cancelled.
func (r *GenericReconciler[T, S, D]) WithReconcileTimeout(d time.Duration) *GenericReconciler[T, S, D] {
	r.reconcileTimeout = d
	return r
}

func (r *GenericReconciler[T, S, D]) getReconcileTimeout() time.Duration {
	if r.reconcileTimeout > 0 {
		return r.reconcileTimeout
	}

	return DefaultReconcileTimeout
}

// SetupWithManager registers the controller with the manager.
// Controllers that are explicitly disabled or alpha-maturity controllers with
// alpha not enabled are skipped.
func (r *GenericReconciler[T, S, D]) SetupWithManager(mgr ctrl.Manager, maxConcurrent int) error {
	if r.disabled {
		log.Log.Info("skipping disabled controller (--disable-controllers)",
			"controller", r.Adapter.ResourceName())
		return nil
	}

	maturity := r.getMaturity()
	if maturity == snowplanev1alpha1.MaturityAlpha && !r.enableAlphaResources {
		log.Log.Info("skipping alpha controller (--enable-alpha-resources not set)",
			"controller", r.Adapter.ResourceName(), "maturity", maturity)
		return nil
	}

	// Resolve GVK once from the scheme for SSA status patches.
	if mgr == nil {
		return fmt.Errorf("manager must not be nil for %s", r.Adapter.ResourceName())
	}

	var err error

	r.GVK, err = apiutil.GVKForObject(r.Adapter.NewObject(), mgr.GetScheme())
	if err != nil {
		return fmt.Errorf("resolving GVK for %s: %w", r.Adapter.ResourceName(), err)
	}

	bldr := ctrl.NewControllerManagedBy(mgr).
		For(r.Adapter.NewObject(), builder.WithPredicates(DesiredStateChanged())).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrent}).
		Named(r.Adapter.ResourceName())

	// Apply sharding predicate when running in multi-shard mode.
	if r.shardPredicate != nil {
		bldr = bldr.WithEventFilter(r.shardPredicate)
	}

	// Let the adapter add resource-specific watches (e.g., Schema -> Database).
	if wc, ok := r.Adapter.(WatchConfigurer); ok {
		if fn := wc.SetupWatches(); fn != nil {
			setupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := fn(setupCtx, mgr, bldr); err != nil {
				return fmt.Errorf("setting up watches for %s: %w", r.Adapter.ResourceName(), err)
			}
		}
	}

	return bldr.Complete(r)
}

// Reconcile implements the shared reconciliation state machine.
func (r *GenericReconciler[T, S, D]) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	// M6: Overall reconcile timeout guard — prevents a blocked reconcile
	// goroutine from occupying a worker indefinitely.
	ctx, reconcileCancel := context.WithTimeout(ctx, r.getReconcileTimeout())
	defer reconcileCancel()

	resName := r.Adapter.ResourceName()
	start := time.Now()

	// OpenTelemetry: wrap the entire reconcile in a span.
	ctx, span := otel.Tracer("snowplane").Start(ctx, "Reconcile/"+resName,
		trace.WithAttributes(
			attribute.String("snowplane.resource.type", resName),
			attribute.String("k8s.namespace", req.Namespace),
			attribute.String("k8s.name", req.Name),
		),
	)
	defer func() {
		if retErr != nil {
			span.RecordError(retErr)
			span.SetStatus(codes.Error, retErr.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}()

	var providerName string       // populated after provider resolution, used in defer
	var snowflakeOpAttempted bool // true once an actual Snowflake I/O call is made

	obj := r.Adapter.NewObject()

	defer func() {
		metrics.ReconcileDuration.With(prometheus.Labels{"controller": resName}).Observe(time.Since(start).Seconds())

		reconcileResult := "success"
		if retErr != nil {
			reconcileResult = "error"
		} else if conditions.IsTerminal(obj) {
			reconcileResult = "terminal"
		} else if obj.GetPaused() {
			reconcileResult = "paused"
		}

		metrics.RecordReconcile(resName, reconcileResult)

		// Only update circuit breaker when a Snowflake operation was actually
		// attempted.  Validation-only or adoption-only paths should not
		// inflate the success count and dilute the breaker's accuracy.
		if r.CircuitBreaker != nil && providerName != "" && snowflakeOpAttempted {
			if retErr != nil {
				r.CircuitBreaker.RecordFailure(providerName)
			} else {
				r.CircuitBreaker.RecordSuccess(providerName)
			}
		}
	}()

	logger := log.FromContext(ctx)

	if err := r.Client.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	// Snapshot for status patching (MergeFrom patch).
	statusBase := obj.DeepCopyObject().(T)

	// M-2: Paused resources skip all Snowflake operations.
	if obj.GetPaused() {
		logger.Info("reconciliation paused", "name", obj.GetName(), resName, obj.GetSpecName())
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonReconcilePaused,
			"Reconciliation is paused via spec.paused — no Snowflake operations will be performed")
		r.Recorder.Event(obj, corev1.EventTypeNormal, snowplanev1alpha1.ReasonReconcilePaused,
			fmt.Sprintf("%s %q reconciliation paused", resName, obj.GetSpecName()))
		r.bestEffortPatchStatus(ctx, obj)

		return ctrl.Result{}, nil
	}

	logger.Info("reconciling "+resName, "name", obj.GetName(), resName, obj.GetSpecName())

	// L-2: Warn on ambiguous boolean annotation values (e.g. "True" instead of "true").
	for _, w := range snowplanev1alpha1.AmbiguousBoolAnnotations(obj.GetAnnotations()) {
		r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonValidationFailed, w)
	}

	// H3: Inject AllowedRefNamespaces from ProviderConfig into context so that
	// ref-resolution functions automatically enforce cross-namespace restrictions
	// during PreReconcile without requiring adapter signature changes.
	ctx = r.injectAllowedRefNamespaces(ctx, obj)

	// Resource-specific pre-reconcile hook (e.g. Schema resolves databaseRef).
	if pr, ok := r.Adapter.(PreReconciler[T]); ok {
		if err := pr.PreReconcile(ctx, obj); err != nil {
			logger.Info("pre-reconcile failed, will retry", "error", err)
			conditions.SetNotReady(obj, snowplanev1alpha1.ReasonDependencyNotReady, fmt.Sprintf("pre-reconcile failed: %v", err))
			conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonDependencyNotReady, err.Error())
			r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonDependencyNotReady,
				fmt.Sprintf("Pre-reconcile failed for %s %q: %v", resName, obj.GetSpecName(), err))
			r.bestEffortPatchStatus(ctx, obj)
			// Return the error to controller-runtime for exponential backoff
			// instead of a fixed 10s requeue interval.
			return ctrl.Result{}, err
		}
	}

	// Resolve Snowflake client via ProviderConfig.
	resolved, err := provider.ResolveClient(ctx, r.Client, r.Factory, obj, obj.GetProviderRef(), obj.GetNamespace(), r.RateLimiter, r.CircuitBreaker, resName)
	if err != nil {
		if !obj.GetDeletionTimestamp().IsZero() {
			logger.Info("cannot resolve provider during deletion, removing finalizer to unblock", "error", err)

			// H-1: Emit a warning event and set conditions so operators are
			// alerted that the Snowflake resource may still exist.
			orphanMsg := fmt.Sprintf(
				"Snowflake %s %q may still exist — provider resolution failed during deletion: %v. "+
					"Manual cleanup may be required.",
				resName, obj.GetSpecName(), err)
			r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonOrphanedResource, orphanMsg)
			conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonOrphanedResource, orphanMsg)
			r.bestEffortPatchStatus(ctx, obj)
			metrics.RecordOrphanedResource(resName)

			if finalizers.Remove(obj, r.Adapter.FinalizerName()) {
				if updateErr := r.Client.Update(ctx, obj); updateErr != nil {
					return ctrl.Result{}, fmt.Errorf("removing finalizer during provider resolution failure: %w", updateErr)
				}
			}

			return ctrl.Result{}, nil
		}

		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonReconcileError,
			fmt.Sprintf("provider resolution failed: %v", err))
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonReconcileError,
			fmt.Sprintf("provider resolution failed: %v", err))
		r.bestEffortPatchStatus(ctx, obj)

		return ctrl.Result{}, err
	}

	sfClient := resolved.Client
	providerName = resolved.CacheKey

	// L-3: Enrich logger with provider metadata for multi-account log correlation.
	logger = logger.WithValues("provider", resolved.Name, "account", resolved.Account)

	// F7: Enforce AllowedDatabases / AllowedSchemas restrictions from the ProviderConfig.
	// Only runs for resources that implement ScopedResource (schema-scoped CRDs).
	if scoped, ok := any(obj).(ScopedResource); ok {
		if dbName := scoped.GetScopeDatabaseName(); dbName != "" {
			if !provider.IsDatabaseAllowed(resolved, dbName) {
				msg := fmt.Sprintf("database %q is not allowed by ProviderConfig %q", dbName, resolved.Name)
				conditions.SetNotReady(obj, snowplanev1alpha1.ReasonDatabaseNotAllowed, msg)
				conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonDatabaseNotAllowed, msg)
				r.bestEffortPatchStatus(ctx, obj)

				return ctrl.Result{}, fmt.Errorf("database not allowed: %s", msg)
			}

			if schemaName := scoped.GetScopeSchemaName(); schemaName != "" {
				if !provider.IsSchemaAllowed(resolved, dbName, schemaName) {
					msg := fmt.Sprintf("schema %q.%q is not allowed by ProviderConfig %q", dbName, schemaName, resolved.Name)
					conditions.SetNotReady(obj, snowplanev1alpha1.ReasonSchemaNotAllowed, msg)
					conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonSchemaNotAllowed, msg)
					r.bestEffortPatchStatus(ctx, obj)

					return ctrl.Result{}, fmt.Errorf("schema not allowed: %s", msg)
				}
			}
		}
	}

	// Resolve use role.
	useRole := ""
	if or := obj.GetUseRole(); or != nil {
		useRole = *or
	}

	svc, cleanup, err := r.Adapter.ServiceFromClient(ctx, sfClient, useRole)
	if err != nil {
		terminal := conditions.SetConditionFromError(obj, err)
		r.bestEffortPatchStatus(ctx, obj)

		if terminal {
			resName := r.Adapter.ResourceName()
			r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonTerminalError,
				fmt.Sprintf("Terminal error setting up service client for %s %q: %v", resName, obj.GetSpecName(), err))

			return ctrl.Result{}, nil // Terminal — do not requeue; conditions already set.
		}

		return ctrl.Result{}, err
	}

	if cleanup != nil {
		defer cleanup(ctx)
	}

	// F8: Pre-flight existence check for raw databaseName/schemaName.
	// Runs after ServiceFromClient so a Snowflake client is available.
	// For any ScopedResource (38 types), the reconciler automatically
	// checks database/schema existence when raw names are used (ref-based
	// resolution already validates existence via CR readiness).
	// Adapters can also implement PreFlightChecker for custom checks.
	if err := r.runPreFlightChecks(ctx, logger, sfClient, obj); err != nil {
		metrics.RecordPreflightFailure(resName, snowplanev1alpha1.ReasonDependencyNotReady)
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonDependencyNotReady,
			fmt.Sprintf("pre-flight check failed: %v", err))
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonDependencyNotReady, err.Error())
		r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonDependencyNotReady,
			fmt.Sprintf("Pre-flight check failed for %s %q: %v", resName, obj.GetSpecName(), err))
		r.bestEffortPatchStatus(ctx, obj)

		return ctrl.Result{}, err
	}

	id, err := r.Adapter.BuildIdentifier(obj)
	if err != nil {
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonTerminalError, err.Error())
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonTerminalError, err.Error())
		r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonTerminalError,
			fmt.Sprintf("Failed to build identifier for %s %q: %s", resName, obj.GetSpecName(), err.Error()))
		r.bestEffortPatchStatus(ctx, obj)

		return ctrl.Result{}, nil // Terminal — BuildIdentifier failure is deterministic.
	}

	// Handle deletion.
	if !obj.GetDeletionTimestamp().IsZero() {
		// ObserveOnly: remove finalizer without dropping the Snowflake resource.
		if obj.GetManagementPolicies().IsObserveOnly() {
			logger.Info("observe-only mode: removing finalizer without dropping Snowflake resource", resName, obj.GetSpecName())
			r.Recorder.Event(obj, corev1.EventTypeNormal, snowplanev1alpha1.ReasonObserveOnly,
				fmt.Sprintf("Observe-only: %s %q finalizer removed without DROP", resName, obj.GetSpecName()))

			if finalizers.Remove(obj, r.Adapter.FinalizerName()) {
				if updateErr := r.Client.Update(ctx, obj); updateErr != nil {
					return ctrl.Result{}, fmt.Errorf("removing finalizer in observe-only delete: %w", updateErr)
				}
			}

			return ctrl.Result{}, nil
		}

		// Mark Snowflake I/O as attempted so the deferred circuit breaker
		// update fires — reconcileDelete calls Drop() which is real I/O.
		snowflakeOpAttempted = true

		return r.reconcileDelete(ctx, obj, svc, id)
	}

	// Ensure finalizer — use PATCH to avoid conflict with concurrent spec
	// changes.
	if finalizers.Add(obj, r.Adapter.FinalizerName()) {
		patch := client.MergeFrom(statusBase.DeepCopyObject().(T))
		if err := r.Client.Patch(ctx, obj, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding finalizer: %w", err)
		}

		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Defense-in-depth: validate spec beyond what CEL rules enforce.
	if err := obj.ValidateSpec(); err != nil {
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonValidationFailed, err.Error())
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonValidationFailed, err.Error())
		r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonValidationFailed, err.Error())
		r.bestEffortPatchStatus(ctx, obj)

		return ctrl.Result{}, nil // Terminal - do not requeue.
	}

	// Check immutable field violations.
	if err := r.Adapter.ValidateImmutableFields(ctx, obj); err != nil {
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonImmutableField, err.Error())
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonImmutableField, err.Error())
		r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonImmutableField, err.Error())
		r.bestEffortPatchStatus(ctx, obj)

		return ctrl.Result{}, nil // Terminal — do not requeue.
	}

	// Audit trail: emit a warning event when force-new is active.
	if snowplanev1alpha1.IsForceNew(obj.GetAnnotations()) {
		logger.Info("force-new annotation active, immutable field validation bypassed", "resource", resName)
		r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonForceNewActive,
			fmt.Sprintf("force-new annotation active on %s %q — immutable field validation bypassed, resource may be deleted and recreated", resName, obj.GetSpecName()))
	}

	// Observe current Snowflake state.
	opCtx, cancel := context.WithTimeout(ctx, r.getSnowflakeOpTimeout())
	defer cancel()

	var obs *Observation[D]

	logger.V(1).Info("observing current Snowflake state", "resource", resName, "name", obj.GetSpecName())

	snowflakeOpAttempted = true

	if err := metrics.ObserveSnowflakeOp(resName, "observe", func() error {
		return sfretry.Do(opCtx, sfretry.DefaultOptions(), func() error {
			var e error
			obs, e = r.Adapter.Observe(opCtx, svc, id)
			return e
		})
	}); err != nil {
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonReconcileError, fmt.Sprintf("failed to observe %s: %v", resName, err))
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonReconcileError, err.Error())
		r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonReconcileError, fmt.Sprintf("Failed to observe %s %q: %v", resName, obj.GetSpecName(), err))
		r.bestEffortPatchStatus(ctx, obj)

		return ctrl.Result{}, err
	}

	// Guard against nil observation — adapters must return a non-nil Observation
	// even when the resource doesn't exist (Exists=false).
	if obs == nil {
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonReconcileError, fmt.Sprintf("observe returned nil for %s", resName))
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonReconcileError, "observe returned nil observation")
		r.bestEffortPatchStatus(ctx, obj)

		return ctrl.Result{}, fmt.Errorf("adapter %s.Observe returned nil observation", resName)
	}

	// M-1: ObserveOnly — read Snowflake state and populate status but never
	// issue CREATE, ALTER, or DROP statements.
	if obj.GetManagementPolicies().IsObserveOnly() {
		return r.reconcileObserveOnly(ctx, obj, resName, obs)
	}

	if !obs.Exists {
		logger.V(1).Info("resource does not exist in Snowflake, will create", "resource", resName)
		return r.reconcileCreate(ctx, obj, statusBase, svc, id)
	}

	// First reconciliation with existing Snowflake resource: adoption check.
	// Indicated by ObservedGeneration == 0 (never successfully reconciled).
	// However, if we set the creation-initiated annotation ourselves,
	// this is a post-crash continuation — skip adoption and finish setup.
	if obj.GetObservedGeneration() == 0 {
		if hasCreationInitiated(obj) {
			logger.Info("post-crash continuation: resource was created by this controller", "resource", resName)
			return r.reconcilePostCrashCreate(ctx, obj, statusBase, obs)
		}

		return r.reconcileAdoptOrReject(ctx, obj, statusBase, obs)
	}

	return r.reconcileUpdate(ctx, obj, svc, id, obs)
}

// runPreFlightChecks verifies that prerequisite Snowflake resources exist
// before the main reconciliation begins. It supports two mechanisms:
//
//  1. Automatic checks: If the managed resource implements ScopedResource,
//     the reconciler automatically verifies database/schema existence when
//     raw databaseName/schemaName strings are used (ref-based resolution
//     already validates existence via CR readiness in PreReconcile).
//
//  2. Adapter-specific checks: If the adapter implements PreFlightChecker,
//     its PreFlightCheck method is called after the automatic checks.
//     This is for non-standard pre-flight requirements only.
//
// Definitive "not found" errors (ErrDatabaseNotFound, ErrSchemaNotFound)
// cause a hard failure. Non-definitive errors (connection timeouts, etc.)
// are logged and skipped — the subsequent service creation will surface
// the same connectivity issues with proper error handling.
func (r *GenericReconciler[T, S, D]) runPreFlightChecks(ctx context.Context, logger logr.Logger, sfClient clientfactory.SnowflakeClient, obj T) error {
	// Automatic pre-flight for all ScopedResource types (38 types).
	if scoped, ok := any(obj).(ScopedResource); ok {
		if err := refresolver.PreFlightCheckDatabaseExists(ctx, sfClient, scoped.GetSpecDatabaseRef(), scoped.GetSpecDatabaseName()); err != nil {
			if errors.Is(err, refresolver.ErrDatabaseNotFound) {
				logger.Info("pre-flight database check failed", "error", err)
				return err
			}

			logger.V(1).Info("pre-flight database check skipped (non-definitive error)", "error", err)
		}

		if scoped.GetSpecSchemaName() != nil {
			dbName := ""
			if n := scoped.GetSpecDatabaseName(); n != nil {
				dbName = *n
			}

			if err := refresolver.PreFlightCheckSchemaExists(ctx, sfClient, dbName, scoped.GetSpecSchemaRef(), scoped.GetSpecSchemaName()); err != nil {
				if errors.Is(err, refresolver.ErrSchemaNotFound) {
					logger.Info("pre-flight schema check failed", "error", err)
					return err
				}

				logger.V(1).Info("pre-flight schema check skipped (non-definitive error)", "error", err)
			}
		}
	}

	// Adapter-specific pre-flight checks (non-standard requirements).
	if pfc, ok := r.Adapter.(PreFlightChecker[T]); ok {
		if err := pfc.PreFlightCheck(ctx, sfClient, obj); err != nil {
			logger.Info("adapter pre-flight check failed", "error", err)
			return err
		}
	}

	return nil
}

func (r *GenericReconciler[T, S, D]) reconcileCreate(ctx context.Context, obj T, statusBase T, svc S, id Identifier) (ctrl.Result, error) {
	resName := r.Adapter.ResourceName()
	logger := log.FromContext(ctx)

	ctx, span := otel.Tracer("snowplane").Start(ctx, "Create/"+resName)
	defer span.End()

	logger.Info("creating "+resName+" in Snowflake", resName, obj.GetSpecName())

	// Mark creation-initiated before issuing the Snowflake CREATE so that
	// a post-crash reconcile can distinguish "I created this" from
	// "someone else created this".
	if !hasCreationInitiated(obj) {
		setCreationInitiated(obj)

		// Stamp ownership label and check for same-cluster conflicts
		// before issuing the Snowflake CREATE.
		fqn := id.FullyQualifiedName()
		hash := ComputeExternalNameHash(fqn)

		if conflict, err := r.checkOwnershipConflict(ctx, obj, hash); err != nil {
			return ctrl.Result{}, fmt.Errorf("checking ownership conflict during create: %w", err)
		} else if conflict {
			return ctrl.Result{}, nil // Terminal — do not requeue; ConflictDetected condition is set.
		}

		setExternalNameLabel(obj, hash)

		// PATCH a copy rather than obj so that in-memory status mutations
		// from PreReconcile (e.g. Schema.Status.DatabaseName) are preserved.
		// The server response strips status from non-status patches.
		patchTarget := obj.DeepCopyObject().(T)
		if err := r.Client.Patch(ctx, patchTarget, client.MergeFrom(statusBase.DeepCopyObject().(T))); err != nil {
			return ctrl.Result{}, fmt.Errorf("setting creation-initiated annotation: %w", err)
		}

		// Carry forward the server-assigned ResourceVersion.
		obj.SetResourceVersion(patchTarget.GetResourceVersion())

		// Re-snap statusBase so future status patches use the correct base.
		statusBase = patchTarget.DeepCopyObject().(T)
	}

	// Fresh timeout ensures the CREATE gets the full budget, independent of
	// how much time the preceding Observe consumed.
	opCtx, cancel := context.WithTimeout(ctx, r.getSnowflakeOpTimeout())
	defer cancel()

	logger.V(1).Info("executing Snowflake CREATE", "resource", resName, "name", obj.GetSpecName())

	useCoA := r.supportsCreateOrAlter() && r.isCreateOrAlter(obj)

	if err := r.executeSnowflakeOp(ctx, opCtx, obj, "create", "create", func() error {
		return r.Adapter.Create(opCtx, svc, obj, id)
	}); err != nil {
		// Graceful fallback: if CREATE OR ALTER is not supported for this
		// resource type on the current Snowflake account, disable
		// createOrAlter in the spec and retry with plain CREATE ... IF NOT EXISTS.
		if useCoA && isCreateOrAlterUnsupported(err) {
			logger.Info("CREATE OR ALTER not supported for new resource, falling back to CREATE IF NOT EXISTS", "resource", resName, "name", obj.GetSpecName())

			r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonCreateOrAlterFallback,
				fmt.Sprintf("CREATE OR ALTER not supported for %s %q, falling back to CREATE IF NOT EXISTS", resName, obj.GetSpecName()))

			// Disable createOrAlter in-memory for the retry.
			// Persist the spec change so subsequent reconciles skip
			// the unsupported CREATE OR ALTER attempt.
			patchBase := obj.DeepCopyObject().(T) // copy BEFORE mutation

			f := false
			obj.SetCreateOrAlter(&f)

			if err := r.Client.Patch(ctx, obj, client.MergeFrom(patchBase)); err != nil {
				logger.Error(err, "failed to persist createOrAlter=false fallback")
			}

			if err := r.executeSnowflakeOp(ctx, opCtx, obj, "create", "create", func() error {
				return r.Adapter.Create(opCtx, svc, obj, id)
			}); err != nil {
				if snowflake.IsTerminalError(err) {
					return ctrl.Result{}, nil // Terminal — do not requeue; conditions already set.
				}

				return ctrl.Result{}, err
			}
		} else {
			if snowflake.IsTerminalError(err) {
				return ctrl.Result{}, nil // Terminal — do not requeue; conditions already set.
			}

			return ctrl.Result{}, err
		}
	}

	// Post-create observation.
	var postObs *Observation[D]

	if err := sfretry.Do(opCtx, sfretry.DefaultOptions(), func() error {
		var e error
		postObs, e = r.Adapter.Observe(opCtx, svc, id)
		return e
	}); err != nil || postObs == nil || !postObs.Exists {
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonCreating, resName+" created but not yet observable")
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonCreating, "awaiting post-create verification")

		// Use bestEffortPatchStatus so that a patch error doesn't defeat
		// the explicit fast requeue.
		r.bestEffortPatchStatus(ctx, obj)

		// Return the error (if any) alongside RequeueAfter so that
		// controller-runtime applies exponential backoff instead of a
		// fixed 5-second polling loop.
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("post-create observe: %w", err)
		}

		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	r.Adapter.ApplyObservation(obj, postObs)
	conditions.ClearDriftDetected(obj)
	conditions.SetReady(obj, resName+" created successfully")
	conditions.SetSynced(obj, "Reconciliation complete")
	obj.SetObservedGeneration(obj.GetGeneration())

	if err := r.finalizeSpec(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}

	// Resource-specific post-create hook (e.g. User password hash tracking).
	r.invokePostCreate(obj)

	r.Recorder.Event(obj, corev1.EventTypeNormal, snowplanev1alpha1.ReasonCreating, fmt.Sprintf("%s %q created", resName, obj.GetSpecName()))

	// Persist annotation changes (clear creation-initiated) via a metadata
	// patch first, then status. This ordering ensures the ResourceVersion
	// stays consistent across both patches and prevents the metadata patch
	// from overwriting status fields set by SSA Apply.
	clearCreationInitiated(obj)

	patchTarget := obj.DeepCopyObject().(T)
	if err := r.Client.Patch(ctx, patchTarget, client.MergeFrom(statusBase.DeepCopyObject().(T))); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing creation-initiated annotation: %w", err)
	}

	obj.SetResourceVersion(patchTarget.GetResourceVersion())

	if err := r.patchStatus(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
}

func (r *GenericReconciler[T, S, D]) reconcileUpdate(ctx context.Context, obj T, svc S, id Identifier, obs *Observation[D]) (ctrl.Result, error) {
	resName := r.Adapter.ResourceName()
	logger := log.FromContext(ctx)

	ctx, span := otel.Tracer("snowplane").Start(ctx, "Update/"+resName)
	defer span.End()

	// Fresh timeout ensures ALTER gets the full budget, independent of
	// how much time the preceding Observe consumed.
	opCtx, cancel := context.WithTimeout(ctx, r.getSnowflakeOpTimeout())
	defer cancel()

	r.Adapter.ApplyObservation(obj, obs)

	// Always compute diff - enables drift correction even without spec changes.
	alterOpts, err := r.Adapter.BuildAlterOptions(ctx, obj, id, obs)
	if err != nil {
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonTerminalError, err.Error())
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonTerminalError, err.Error())
		r.bestEffortPatchStatus(ctx, obj)

		return ctrl.Result{}, nil // Terminal — do not requeue; conditions already set.
	}

	altered := false

	if alterOpts.HasChanges() {
		currentHash, hashErr := obj.ComputeSpecHash()
		if hashErr != nil {
			conditions.SetNotReady(obj, snowplanev1alpha1.ReasonTerminalError, hashErr.Error())
			conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonTerminalError, hashErr.Error())
			r.bestEffortPatchStatus(ctx, obj)

			return ctrl.Result{}, nil // Terminal — do not requeue; hash computation is deterministic.
		}

		isDrift := obj.GetLastAppliedSpecHash() != "" && obj.GetLastAppliedSpecHash() == currentHash

		if isDrift {
			// Only compute field-level diffs when changes are from external drift.
			driftResult := r.Adapter.DetectDrift(obj, obs)
			logger.Info("drift detected", resName, obj.GetSpecName(), "summary", driftResult.Summary())
			conditions.SetDriftDetected(obj, driftResult.SafeSummary())
			metrics.RecordDriftDetected(resName)
			r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonDriftDetected, fmt.Sprintf("Drift detected on %s %q: %s", resName, obj.GetSpecName(), driftResult.SafeSummary()))

			// Immutable field violations cannot be fixed via ALTER.
			// Emit a distinct event and skip correction when only immutable fields drifted.
			if driftResult.HasImmutableViolation {
				r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonImmutableField,
					fmt.Sprintf("Immutable field(s) changed externally on %s %q — manual intervention required: %s",
						resName, obj.GetSpecName(), driftResult.ImmutableSummary()))

				if !driftResult.HasDrift {
					// Only immutable drift — ALTER would be a no-op, skip it.
					logger.Info("immutable-only drift, skipping correction", resName, obj.GetSpecName())
					conditions.SetReady(obj, resName+" is ready (immutable drift detected, manual intervention required)")
					conditions.SetSynced(obj, "Immutable field drift detected — cannot auto-correct")
					obj.SetObservedGeneration(obj.GetGeneration())

					if err := r.finalizeSpec(ctx, obj); err != nil {
						return ctrl.Result{}, err
					}

					if err := r.patchStatus(ctx, obj); err != nil {
						return ctrl.Result{}, err
					}

					return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
				}

				// Mixed drift: immutable + mutable — proceed with ALTER for mutable fields only.
				logger.Info("mixed drift (immutable + mutable), correcting mutable fields only", resName, obj.GetSpecName())
			}
		} else {
			logger.Info("spec changed, applying diff", resName, obj.GetSpecName())
		}

		// Detect-only drift policy: skip the alter and requeue.
		detectOnly := obj.GetManagementPolicies().IsDetectOnly()
		if isDrift && detectOnly {
			logger.Info("detect-only drift policy, skipping correction", resName, obj.GetSpecName())

			conditions.SetReady(obj, resName+" is ready (drift detected, detect-only policy)")
			conditions.SetSynced(obj, "Drift detected but not corrected (detect-only)")
			obj.SetObservedGeneration(obj.GetGeneration())

			if err := r.finalizeSpec(ctx, obj); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.patchStatus(ctx, obj); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
		}

		// Use CREATE OR ALTER when enabled and the adapter supports it,
		// otherwise fall through to the standard ALTER path.
		useCoA := r.supportsCreateOrAlter() && r.isCreateOrAlter(obj)

		if useCoA {
			logger.Info("using CREATE OR ALTER (Snowflake preview feature)", "resource", resName, "name", obj.GetSpecName(), "isDrift", isDrift)

			if err := r.executeSnowflakeOp(ctx, opCtx, obj, "create_or_alter", "CREATE OR ALTER", func() error {
				return r.Adapter.Create(opCtx, svc, obj, id)
			}); err != nil {
				// Graceful fallback: if CREATE OR ALTER is not supported,
				// fall back to the standard ALTER path.
				if isCreateOrAlterUnsupported(err) {
					logger.Info("CREATE OR ALTER not supported, falling back to ALTER", "resource", resName, "name", obj.GetSpecName())
					r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonCreateOrAlterFallback,
						fmt.Sprintf("CREATE OR ALTER not supported for %s %q, falling back to ALTER", resName, obj.GetSpecName()))

					if err := r.executeSnowflakeOp(ctx, opCtx, obj, "alter", "alter", func() error {
						return r.Adapter.Alter(opCtx, svc, alterOpts)
					}); err != nil {
						if snowflake.IsTerminalError(err) {
							return ctrl.Result{}, nil // Terminal — do not requeue; conditions already set.
						}

						return ctrl.Result{}, err
					}
				} else {
					if snowflake.IsTerminalError(err) {
						return ctrl.Result{}, nil // Terminal — do not requeue; conditions already set.
					}

					return ctrl.Result{}, err
				}
			}
		} else {
			// Warn when createOrAlter is explicitly enabled on an unsupported resource type.
			if !r.supportsCreateOrAlter() && obj.GetManagementPolicies().CreateOrAlter != nil && *obj.GetManagementPolicies().CreateOrAlter {
				logger.Info("spec.managementPolicies.createOrAlter ignored: not supported for resource type", "resource", resName)
				r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonUnsupportedAnnotation,
					fmt.Sprintf("spec.managementPolicies.createOrAlter is not supported for %s, using ALTER", resName))
			}

			logger.V(1).Info("executing Snowflake ALTER", "resource", resName, "name", obj.GetSpecName(), "isDrift", isDrift)

			if err := r.executeSnowflakeOp(ctx, opCtx, obj, "alter", "alter", func() error {
				return r.Adapter.Alter(opCtx, svc, alterOpts)
			}); err != nil {
				if snowflake.IsTerminalError(err) {
					return ctrl.Result{}, nil // Terminal — do not requeue; conditions already set.
				}

				return ctrl.Result{}, err
			}
		}

		altered = true

		// Emit event after successful alter.
		if isDrift {
			r.Recorder.Event(obj, corev1.EventTypeNormal, snowplanev1alpha1.ReasonDriftCorrected, fmt.Sprintf("Drift corrected on %s %q", resName, obj.GetSpecName()))
		} else {
			r.Recorder.Event(obj, corev1.EventTypeNormal, snowplanev1alpha1.ReasonReconcileSuccess, fmt.Sprintf("%s %q updated", resName, obj.GetSpecName()))
		}

		// Re-observe after successful alter.
		var reObs *Observation[D]

		if err := sfretry.Do(opCtx, sfretry.DefaultOptions(), func() error {
			var e error
			reObs, e = r.Adapter.Observe(opCtx, svc, id)
			return e
		}); err == nil && reObs != nil && reObs.Exists {
			r.Adapter.ApplyObservation(obj, reObs)
		}
	}

	conditions.ClearDriftDetected(obj)

	conditions.SetReady(obj, resName+" is ready")
	conditions.SetSynced(obj, "Reconciliation complete")
	obj.SetObservedGeneration(obj.GetGeneration())

	if err := r.finalizeSpec(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}

	// Resource-specific post-update hook — pass alterOpts so adapters can
	// read per-reconciliation values (e.g. password hash) without storing
	// mutable state on the shared adapter struct.
	r.invokePostUpdate(obj, altered, alterOpts)

	if err := r.patchStatus(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
}

// reconcilePostCrashCreate handles the case where CREATE succeeded in Snowflake
// but the controller crashed before status was committed. The creation-initiated
// annotation proves we created this resource, so we skip the adoption path and
// finish the normal post-create setup.
func (r *GenericReconciler[T, S, D]) reconcilePostCrashCreate(ctx context.Context, obj T, statusBase T, obs *Observation[D]) (ctrl.Result, error) {
	resName := r.Adapter.ResourceName()
	logger := log.FromContext(ctx)
	logger.Info("completing post-crash create setup", resName, obj.GetSpecName())

	r.Adapter.ApplyObservation(obj, obs)
	conditions.ClearDriftDetected(obj)

	// Late-initialize spec from observed state for post-crash recovery:
	// Snowflake may have applied defaults (e.g. warehouse size, data retention)
	// during CREATE that the user didn't specify. Capture them now.
	r.checkLateInit(obj, obs, logger, resName)

	conditions.SetReady(obj, resName+" created successfully (recovered)")
	conditions.SetSynced(obj, "Reconciliation complete")
	obj.SetObservedGeneration(obj.GetGeneration())

	if err := r.finalizeSpec(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}

	r.invokePostCreate(obj)

	r.Recorder.Event(obj, corev1.EventTypeNormal, snowplanev1alpha1.ReasonCreating,
		fmt.Sprintf("%s %q create recovered after restart", resName, obj.GetSpecName()))

	// Persist annotation changes (late-initialized, clear creation-initiated)
	// via a metadata patch first, then status. This ordering ensures the
	// ResourceVersion stays consistent across both patches.
	clearCreationInitiated(obj)

	patchTarget := obj.DeepCopyObject().(T)
	if err := r.Client.Patch(ctx, patchTarget, client.MergeFrom(statusBase.DeepCopyObject().(T))); err != nil {
		return ctrl.Result{}, fmt.Errorf("patching annotations after post-crash recovery: %w", err)
	}

	obj.SetResourceVersion(patchTarget.GetResourceVersion())

	if err := r.patchStatus(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
}

// reconcileAdoptOrReject handles the first reconciliation when the Snowflake
// resource already exists. Without adoptionPolicy=adopt, this is a Terminal
// error. With adopt, the reconciler takes over management.
func (r *GenericReconciler[T, S, D]) reconcileAdoptOrReject(ctx context.Context, obj T, statusBase T, obs *Observation[D]) (ctrl.Result, error) {
	resName := r.Adapter.ResourceName()
	logger := log.FromContext(ctx)

	policy := getAdoptionPolicy(obj)

	if policy != snowplanev1alpha1.AdoptionPolicyTypeAdopt {
		// Resource exists but adoption is not requested → Terminal error.
		msg := fmt.Sprintf("%s %q already exists in Snowflake; set spec.managementPolicies.adoptionPolicy to %q to adopt",
			resName, obj.GetSpecName(),
			snowplanev1alpha1.AdoptionPolicyTypeAdopt)
		logger.Info("resource already exists, adoption not requested", resName, obj.GetSpecName())
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonResourceExists, msg)
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonResourceExists, msg)
		r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonResourceExists, msg)
		metrics.RecordAdoptionRejected(resName)
		r.bestEffortPatchStatus(ctx, obj)

		return ctrl.Result{}, nil // Terminal — do not requeue.
	}

	// Adoption flow: take over management of the existing resource.
	logger.Info("adopting existing "+resName, resName, obj.GetSpecName())

	// Ownership conflict detection: before adopting, check whether
	// another CR in this cluster already manages the same Snowflake resource.
	id, err := r.Adapter.BuildIdentifier(obj)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("building identifier for adoption: %w", err)
	}

	fqn := id.FullyQualifiedName()
	hash := ComputeExternalNameHash(fqn)

	if conflict, err := r.checkOwnershipConflict(ctx, obj, hash); err != nil {
		return ctrl.Result{}, fmt.Errorf("checking ownership conflict during adoption: %w", err)
	} else if conflict {
		return ctrl.Result{}, nil // Terminal — do not requeue; ConflictDetected condition is set.
	}

	// Stamp the ownership label so future adoption attempts by other CRs
	// will detect the conflict.
	setExternalNameLabel(obj, hash)

	r.Adapter.ApplyObservation(obj, obs)
	conditions.ClearDriftDetected(obj)

	// Late-initialize spec from observed state: fill nil spec fields with
	// values from the live Snowflake resource. This makes the adopted CR's
	// spec a complete representation of the managed state, following the
	// Crossplane late-initialization pattern.
	r.checkLateInit(obj, obs, logger, resName)

	conditions.SetReady(obj, resName+" adopted successfully")
	conditions.SetSynced(obj, "Reconciliation complete")
	obj.SetObservedGeneration(obj.GetGeneration())

	if err := r.finalizeSpec(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}

	r.invokePostCreate(obj)

	r.Recorder.Event(obj, corev1.EventTypeNormal, snowplanev1alpha1.ReasonAdopted, fmt.Sprintf("%s %q adopted from existing Snowflake resource", resName, obj.GetSpecName()))
	metrics.RecordAdoption(resName)

	// Persist annotation changes (late-initialized) via a metadata patch first.
	// Use a copy so the in-memory status mutations are preserved for patchStatus.
	patchTarget := obj.DeepCopyObject().(T)
	if err := r.Client.Patch(ctx, patchTarget, client.MergeFrom(statusBase.DeepCopyObject().(T))); err != nil {
		return ctrl.Result{}, fmt.Errorf("patching annotations after adoption: %w", err)
	}

	obj.SetResourceVersion(patchTarget.GetResourceVersion())

	if err := r.patchStatus(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
}

// getAdoptionPolicy returns the adoption policy for the object.
// Reads from spec.managementPolicies.adoptionPolicy.
// Defaults to "fail-if-exists" when not set.
func getAdoptionPolicy[T ManagedResource](obj T) snowplanev1alpha1.AdoptionPolicy {
	if p := obj.GetManagementPolicies().AdoptionPolicy; p != "" {
		return p
	}

	return snowplanev1alpha1.AdoptionPolicyTypeFailIfExists
}

func (r *GenericReconciler[T, S, D]) reconcileDelete(ctx context.Context, obj T, svc S, id Identifier) (ctrl.Result, error) {
	resName := r.Adapter.ResourceName()
	logger := log.FromContext(ctx)

	ctx, span := otel.Tracer("snowplane").Start(ctx, "Delete/"+resName)
	defer span.End()

	if !finalizers.Has(obj, r.Adapter.FinalizerName()) {
		return ctrl.Result{}, nil
	}

	switch obj.GetDeletionPolicy() {
	case snowplanev1alpha1.DeletionPolicyOrphan:
		logger.Info("orphaning "+resName+" in Snowflake", resName, obj.GetSpecName())
		r.Recorder.Event(obj, corev1.EventTypeNormal, snowplanev1alpha1.ReasonDeleting, fmt.Sprintf("%s %q orphaned", resName, obj.GetSpecName()))
	case snowplanev1alpha1.DeletionPolicyDelete:
		// Escape hatch: if a DROP is permanently blocked (e.g., insufficient
		// privileges), the user can annotate the resource with
		// abandon-on-delete=true to remove the finalizer without dropping.
		if snowplanev1alpha1.IsAbandonOnDelete(obj.GetAnnotations()) {
			abandonMsg := fmt.Sprintf(
				"Snowflake %s %q abandoned per %s annotation — resource may still exist in Snowflake and require manual cleanup",
				resName, obj.GetSpecName(), snowplanev1alpha1.AnnotationAbandonOnDelete)
			logger.Info("abandoning "+resName+" in Snowflake per annotation", resName, obj.GetSpecName())
			r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonOrphanedResource, abandonMsg)
			conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonOrphanedResource, abandonMsg)
			r.bestEffortPatchStatus(ctx, obj)
			metrics.RecordOrphanedResource(resName)
		} else {
			// Default to Delete for safety — unknown values must not silently orphan.
			logger.Info("dropping "+resName+" from Snowflake", resName, obj.GetSpecName())

			logger.V(1).Info("executing Snowflake DROP", "resource", resName, "name", obj.GetSpecName())

			opCtx, cancel := context.WithTimeout(ctx, r.getSnowflakeOpTimeout())
			defer cancel()

			// Determine whether to issue a cascading DROP.
			cascade := snowplanev1alpha1.IsForceDestroy(obj.GetAnnotations())

			if err := metrics.ObserveSnowflakeOp(resName, "drop", func() error {
				return sfretry.Do(opCtx, sfretry.DefaultOptions(), func() error {
					if cascade {
						// Determine cascade support. CascadeDropSupporter
						// (implemented by BaseAdapter) gives an explicit answer.
						// Non-BaseAdapter adapters that implement CascadeDropper
						// are assumed to support cascade.
						supportsCascade := false
						if cds, ok := any(r.Adapter).(CascadeDropSupporter); ok {
							supportsCascade = cds.SupportsCascadeDrop()
						} else if _, ok := any(r.Adapter).(CascadeDropper[T, S]); ok {
							supportsCascade = true
						}

						if supportsCascade {
							cd := any(r.Adapter).(CascadeDropper[T, S]) // guaranteed by SupportsCascadeDrop
							logger.Info("cascade DROP requested via force-destroy annotation",
								"resource", resName, "name", obj.GetSpecName())
							r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonDeleting,
								fmt.Sprintf("CASCADE DROP %s %q — all child Snowflake objects will be destroyed", resName, obj.GetSpecName()))
							return cd.DropCascade(opCtx, svc, id)
						}

						logger.Info("force-destroy annotation set but resource does not support CASCADE, falling back to standard DROP",
							"resource", resName, "name", obj.GetSpecName())
					}

					return r.Adapter.Drop(opCtx, svc, id)
				})
			}); err != nil {
				if !snowflake.IsObjectNotFound(err) && !snowflake.IsObjectNotExistOrNotAuthorized(err) {
					if snowflake.IsTerminalError(err) {
						conditions.SetNotReady(obj, snowplanev1alpha1.ReasonDeleteBlocked, fmt.Sprintf("terminal error dropping %s: %v", resName, err))
						conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonDeleteBlocked, err.Error())
						r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonDeleteBlocked,
							fmt.Sprintf("Terminal error dropping %s %q: %v — resolve the issue or set annotation %s=true to abandon",
								resName, obj.GetSpecName(), err, snowplanev1alpha1.AnnotationAbandonOnDelete))
					} else {
						conditions.SetNotReady(obj, snowplanev1alpha1.ReasonDeleting, fmt.Sprintf("failed to drop %s: %v", resName, err))
						conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonReconcileError, err.Error())
						r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonReconcileError,
							fmt.Sprintf("Failed to drop %s %q: %v", resName, obj.GetSpecName(), err))
					}

					r.bestEffortPatchStatus(ctx, obj)

					if snowflake.IsTerminalError(err) {
						// Terminal — do not requeue. The periodic resync will
						// retry in case the underlying issue is fixed (e.g.,
						// privileges granted). The user can also set the
						// abandon-on-delete annotation to force finalizer removal.
						return ctrl.Result{}, nil
					}

					return ctrl.Result{}, err
				}
			}

			r.Recorder.Event(obj, corev1.EventTypeNormal, snowplanev1alpha1.ReasonDeleting, fmt.Sprintf("%s %q dropped", resName, obj.GetSpecName()))
		}
	}

	// Take a snapshot before removing the finalizer so that MergeFrom
	// produces a diff containing the removal.
	patchBase := obj.DeepCopyObject().(T)

	finalizers.Remove(obj, r.Adapter.FinalizerName())

	// Use PATCH for finalizer removal to avoid conflict with concurrent
	// spec changes.
	if err := r.Client.Patch(ctx, obj, client.MergeFrom(patchBase)); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing finalizer: %w", err)
	}

	return ctrl.Result{}, nil
}

// reconcileObserveOnly handles the observe-only management policy. It reads
// the Snowflake resource state and populates the CR status without issuing
// any CREATE, ALTER, or DROP statements.
func (r *GenericReconciler[T, S, D]) reconcileObserveOnly(ctx context.Context, obj T, resName string, obs *Observation[D]) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !obs.Exists {
		logger.Info("observe-only: resource does not exist in Snowflake", "resource", resName, "name", obj.GetSpecName())
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonObserveOnly,
			fmt.Sprintf("%s %q does not exist in Snowflake (observe-only — no CREATE will be issued)", resName, obj.GetSpecName()))
		conditions.SetSynced(obj, "Observe-only mode active — resource not found")
		r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonObserveOnly,
			fmt.Sprintf("Observe-only: %s %q not found in Snowflake", resName, obj.GetSpecName()))
		r.bestEffortPatchStatus(ctx, obj)

		return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
	}

	logger.Info("observe-only: resource exists, populating status", "resource", resName, "name", obj.GetSpecName())

	r.Adapter.ApplyObservation(obj, obs)
	conditions.SetReady(obj, fmt.Sprintf("%s is observable (observe-only — no mutations)", resName))
	conditions.SetSynced(obj, "Observe-only mode active — no CREATE/ALTER/DROP")
	obj.SetObservedGeneration(obj.GetGeneration())

	if err := r.patchStatus(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
}

// patchStatus uses Server-Side Apply (SSA) to update the status subresource.
// SSA eliminates the need for ResourceVersion-based conflict detection and
// retry loops — the server resolves ownership via managedFields instead.
// ForceOwnership ensures the controller takes exclusive ownership of .status
// fields even if another field manager previously owned them.
func (r *GenericReconciler[T, S, D]) patchStatus(ctx context.Context, obj T) error {
	// SSA requires TypeMeta (apiVersion + kind) in the patch payload.
	// controller-runtime's Get() strips TypeMeta from typed objects,
	// so we must set it explicitly from the GVK resolved at setup time.
	obj.GetObjectKind().SetGroupVersionKind(r.GVK)

	// SSA patch objects must not contain managedFields — the server
	// populates them automatically based on the field manager. Objects
	// obtained via Get() include managedFields, which causes the API
	// server to reject the patch with "metadata.managedFields must be nil".
	obj.SetManagedFields(nil)

	return r.Client.SubResource("status").Patch(ctx, obj, client.Apply, //nolint:staticcheck // client.Apply removal requires generated ApplyConfiguration types
		client.FieldOwner(StatusFieldOwner),
		client.ForceOwnership,
	)
}

// bestEffortPatchStatus patches status and logs a warning on failure.
// Used in error paths where the primary error must be returned.
func (r *GenericReconciler[T, S, D]) bestEffortPatchStatus(ctx context.Context, obj T) {
	if err := r.patchStatus(ctx, obj); err != nil {
		log.FromContext(ctx).Error(err, "best-effort status patch failed")
	}
}

// injectAllowedRefNamespaces performs a lightweight ProviderConfig lookup to
// extract AllowedRefNamespaces and inject it into the context. This runs
// before PreReconcile so that all ref-resolution functions in adapters
// automatically enforce the restriction without adapter signature changes (H3).
// On lookup failure (e.g., ProviderConfig not found yet), returns unchanged ctx
// — the full ResolveClient later will surface the error with proper conditions.
func (r *GenericReconciler[T, S, D]) injectAllowedRefNamespaces(ctx context.Context, obj T) context.Context {
	providerRef := obj.GetProviderRef()
	pcNamespace := obj.GetNamespace()

	if providerRef.Namespace != "" {
		pcNamespace = providerRef.Namespace
	}

	pc := &snowplanev1alpha1.ProviderConfig{}
	pcKey := types.NamespacedName{Namespace: pcNamespace, Name: providerRef.Name}

	if err := r.Client.Get(ctx, pcKey, pc); err != nil {
		// ProviderConfig may not exist yet — the full ResolveClient will
		// surface this error later with proper conditions.
		return ctx
	}

	if len(pc.Spec.AllowedRefNamespaces) > 0 {
		return refresolver.WithAllowedRefNamespaces(ctx, pc.Spec.AllowedRefNamespaces)
	}

	return ctx
}

// hasCreationInitiated checks whether the creation-initiated annotation is set.
func hasCreationInitiated[T ManagedResource](obj T) bool {
	if ann := obj.GetAnnotations(); ann != nil {
		return ann[snowplanev1alpha1.AnnotationCreationInitiated] == "true"
	}

	return false
}

// setCreationInitiated sets the creation-initiated annotation on the object.
func setCreationInitiated[T ManagedResource](obj T) {
	ann := obj.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}

	ann[snowplanev1alpha1.AnnotationCreationInitiated] = "true"
	obj.SetAnnotations(ann)
}

// clearCreationInitiated removes the creation-initiated annotation.
func clearCreationInitiated[T ManagedResource](obj T) {
	ann := obj.GetAnnotations()
	if ann == nil {
		return
	}

	delete(ann, snowplanev1alpha1.AnnotationCreationInitiated)
	obj.SetAnnotations(ann)
}

// setLateInitializedAnnotation sets the late-initialized annotation on the
// object, replacing the former LateInitialized condition.
func setLateInitializedAnnotation[T ManagedResource](obj T) {
	ann := obj.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}

	ann[snowplanev1alpha1.AnnotationLateInitialized] = "true"
	obj.SetAnnotations(ann)
}

// executeSnowflakeOp runs a Snowflake operation with retries, metrics, and
// standard error handling. On error it sets NotReady/NotSynced conditions,
// emits events, and attempts a best-effort status patch. The opName is used
// for metrics (e.g. "create", "alter"), and opVerb for human-readable event
// messages (e.g. "creating", "altering", "CREATE OR ALTER").
func (r *GenericReconciler[T, S, D]) executeSnowflakeOp(
	ctx context.Context,
	opCtx context.Context,
	obj T,
	opName, opVerb string,
	opFn func() error,
) error {
	resName := r.Adapter.ResourceName()

	if err := metrics.ObserveSnowflakeOp(resName, opName, func() error {
		return sfretry.Do(opCtx, sfretry.DefaultOptions(), opFn)
	}); err != nil {
		// L7: Record Snowflake error code for observability.
		if code, ok := snowflake.ExtractErrorCode(err); ok {
			provRef := obj.GetProviderRef()
			prov := provRef.Name
			if ns := provRef.Namespace; ns != "" {
				prov = ns + "/" + prov
			} else {
				prov = obj.GetNamespace() + "/" + prov
			}

			metrics.RecordSnowflakeErrorCode(prov, code)
		}

		if snowflake.IsTerminalError(err) {
			conditions.SetNotReady(obj, snowplanev1alpha1.ReasonTerminalError, err.Error())
			r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonTerminalError,
				fmt.Sprintf("Terminal error %s %s %q: %v", opVerb, resName, obj.GetSpecName(), err))
		} else {
			conditions.SetNotReady(obj, snowplanev1alpha1.ReasonReconcileError, err.Error())
			r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonReconcileError,
				fmt.Sprintf("Failed to %s %s %q: %v", opVerb, resName, obj.GetSpecName(), err))
		}

		// L-11: Use matching reason for Synced condition.
		if snowflake.IsTerminalError(err) {
			conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonTerminalError, err.Error())
		} else {
			conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonReconcileError, err.Error())
		}

		r.bestEffortPatchStatus(ctx, obj)

		return err
	}

	return nil
}

// finalizeSpec computes the spec hash, stores it on the object, and updates the
// tracked-parameters list. On error it sets terminal conditions and attempts a
// best-effort status patch. This consolidates the repeated hash-computation +
// error-handling pattern used after every successful create/update/adopt.
func (r *GenericReconciler[T, S, D]) finalizeSpec(ctx context.Context, obj T) error {
	hash, err := obj.ComputeSpecHash()
	if err != nil {
		err = fmt.Errorf("computing spec hash: %w", err)
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonTerminalError, err.Error())
		conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonTerminalError, err.Error())
		r.bestEffortPatchStatus(ctx, obj)

		return err
	}

	obj.SetLastAppliedSpecHash(hash)
	obj.SetTrackedParametersList(r.Adapter.ComputeTrackedParameters(obj))
	obj.SetLastReconcileTime(&metav1.Time{Time: time.Now()})

	return nil
}

// checkLateInit performs optional late-initialization of spec fields from
// observed state. Only adapters implementing LateInitializer[T, D] are
// affected. The late-initialized annotation is set only when fields were
// actually modified.
func (r *GenericReconciler[T, S, D]) checkLateInit(obj T, obs *Observation[D], logger logr.Logger, resName string) {
	// The any() wrapper is required because Go generics do not allow direct
	// type assertion from one parameterised interface to another.
	if li, ok := any(r.Adapter).(LateInitializer[T, D]); ok {
		modified := li.LateInitialize(obj, obs)
		metrics.RecordLateInit(r.Adapter.ResourceName(), modified)

		if modified {
			setLateInitializedAnnotation(obj)
			logger.Info("late-initialized spec fields from observed state", resName, obj.GetSpecName())
		}
	}
}

// isCreateOrAlter returns true when CREATE OR ALTER should be used.
// Reads from spec.managementPolicies.createOrAlter (defaults to true).
func (r *GenericReconciler[T, S, D]) isCreateOrAlter(obj T) bool {
	return obj.GetManagementPolicies().IsCreateOrAlter()
}

// isCreateOrAlterUnsupported checks whether an error indicates that the
// CREATE OR ALTER syntax is not supported by the Snowflake account.
// Delegates to the shared snowflake.IsCreateOrAlterUnsupported helper
// which prefers structured error code matching (code 2032), then falls
// back to targeted string matching for "UNSUPPORTED" and "UNEXPECTED 'OR'"
// (intentionally excludes generic "SYNTAX ERROR" — see FINDINGS H4).
func isCreateOrAlterUnsupported(err error) bool {
	return snowflake.IsCreateOrAlterUnsupported(err)
}
