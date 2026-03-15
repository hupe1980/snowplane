// Package providerconfig implements the reconciler for ProviderConfig resources.
package providerconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/circuitbreaker"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/metrics"
	"github.com/hupe1980/snowplane/internal/provider"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
	"github.com/hupe1980/snowplane/internal/utils/finalizers"
)

const (
	requeueInterval           = 5 * time.Minute
	defaultSnowflakeOpTimeout = 60 * time.Second
	finalizerName             = "providerconfig.snowplane.hupe1980.github.io/in-use"
)

// PingFunc abstracts the Ping call for testability.
type PingFunc func(ctx context.Context, client clientfactory.SnowflakeClient) error

// Reconciler reconciles a ProviderConfig object.
type Reconciler struct {
	client             client.Client
	factory            *clientfactory.ClientFactory
	recorder           record.EventRecorder
	rateLimiter        *ratelimit.Limiter
	circuitBreaker     *circuitbreaker.Breaker
	pingFn             PingFunc
	requeueOverride    time.Duration
	snowflakeOpTimeout time.Duration   // 0 → defaultSnowflakeOpTimeout
	allowedRoles       map[string]bool // uppercase role names; nil = all allowed
}

// WithRequeueInterval overrides the default periodic-resync interval.
func (r *Reconciler) WithRequeueInterval(d time.Duration) *Reconciler {
	r.requeueOverride = d
	return r
}

func (r *Reconciler) getRequeueInterval() time.Duration {
	if r.requeueOverride > 0 {
		return r.requeueOverride
	}

	return requeueInterval
}

// WithSnowflakeOpTimeout overrides the per-operation timeout for Snowflake
// calls (e.g. Ping). Zero keeps the default (60 s).
func (r *Reconciler) WithSnowflakeOpTimeout(d time.Duration) *Reconciler {
	r.snowflakeOpTimeout = d
	return r
}

func (r *Reconciler) getSnowflakeOpTimeout() time.Duration {
	if r.snowflakeOpTimeout > 0 {
		return r.snowflakeOpTimeout
	}

	return defaultSnowflakeOpTimeout
}

// NewReconciler returns a new ProviderConfig reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter, cb *circuitbreaker.Breaker, allowedRoles map[string]bool) *Reconciler {
	return &Reconciler{
		client:         c,
		factory:        factory,
		recorder:       recorder,
		rateLimiter:    rl,
		circuitBreaker: cb,
		allowedRoles:   allowedRoles,
		pingFn: func(ctx context.Context, sfClient clientfactory.SnowflakeClient) error {
			return sfClient.Ping(ctx)
		},
	}
}

// IsRoleAllowed checks whether the given Snowflake role is permitted by the
// controller's allowlist. Comparison is case-insensitive (Snowflake unquoted
// identifiers are case-insensitive). A nil or empty allowedRoles map means
// all roles are permitted.
func (r *Reconciler) IsRoleAllowed(role string) bool {
	if len(r.allowedRoles) == 0 {
		return true
	}

	return r.allowedRoles[strings.ToUpper(role)]
}

// providerRefIndex is the virtual field path used by the field indexer for
// .spec.providerRef.name across all managed resource types.
const providerRefIndex = ".spec.providerRef.name"

// secretRefIndex is the virtual field path used by the field indexer for
// efficient Secret→ProviderConfig reverse lookups (R9-4).
const secretRefIndex = ".spec.credentials.secretRef" //nolint:gosec // G101: index key, not a credential

// secretRefKey builds the composite index key "namespace/name" for a secret.
func secretRefKey(namespace, name string) string {
	return namespace + "/" + name
}

// secretRefExtractor returns the composite "namespace/name" key for a
// ProviderConfig's credentials secret reference.
func secretRefExtractor(o client.Object) []string {
	pc, ok := o.(*snowplanev1alpha1.ProviderConfig)
	if !ok || pc.Spec.Credentials.SecretRef == nil {
		return nil
	}

	ns := pc.Spec.Credentials.SecretRef.Namespace
	if ns == "" {
		ns = pc.Namespace
	}

	return []string{secretRefKey(ns, pc.Spec.Credentials.SecretRef.Name)}
}

// managedResourceEntry pairs an Object prototype (for field-indexer
// registration in SetupWithManager) with an ObjectList factory (for
// indexed queries in isInUse). Keeping both in one struct ensures
// they can never drift out of sync.
type managedResourceEntry struct {
	proto   client.Object
	newList func() client.ObjectList
}

// managedResourceTypes returns all managed CRD types that carry a
// .spec.providerRef field. Each entry provides both the singleton
// prototype (for IndexField) and a factory for fresh ObjectList
// allocations (for client.List).
func managedResourceTypes() []managedResourceEntry {
	return []managedResourceEntry{
		{proto: &snowplanev1alpha1.Database{}, newList: func() client.ObjectList { return &snowplanev1alpha1.DatabaseList{} }},
		{proto: &snowplanev1alpha1.Schema{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SchemaList{} }},
		{proto: &snowplanev1alpha1.Warehouse{}, newList: func() client.ObjectList { return &snowplanev1alpha1.WarehouseList{} }},
		{proto: &snowplanev1alpha1.User{}, newList: func() client.ObjectList { return &snowplanev1alpha1.UserList{} }},
		{proto: &snowplanev1alpha1.AccountRole{}, newList: func() client.ObjectList { return &snowplanev1alpha1.AccountRoleList{} }},
		{proto: &snowplanev1alpha1.DatabaseRole{}, newList: func() client.ObjectList { return &snowplanev1alpha1.DatabaseRoleList{} }},
		{proto: &snowplanev1alpha1.Table{}, newList: func() client.ObjectList { return &snowplanev1alpha1.TableList{} }},
		{proto: &snowplanev1alpha1.View{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ViewList{} }},
		{proto: &snowplanev1alpha1.InternalStage{}, newList: func() client.ObjectList { return &snowplanev1alpha1.InternalStageList{} }},
		{proto: &snowplanev1alpha1.ExternalStage{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ExternalStageList{} }},
		{proto: &snowplanev1alpha1.Task{}, newList: func() client.ObjectList { return &snowplanev1alpha1.TaskList{} }},
		{proto: &snowplanev1alpha1.StreamOnTable{}, newList: func() client.ObjectList { return &snowplanev1alpha1.StreamOnTableList{} }},
		{proto: &snowplanev1alpha1.StreamOnView{}, newList: func() client.ObjectList { return &snowplanev1alpha1.StreamOnViewList{} }},
		{proto: &snowplanev1alpha1.StreamOnExternalTable{}, newList: func() client.ObjectList { return &snowplanev1alpha1.StreamOnExternalTableList{} }},
		{proto: &snowplanev1alpha1.StreamOnDirectoryTable{}, newList: func() client.ObjectList { return &snowplanev1alpha1.StreamOnDirectoryTableList{} }},
		{proto: &snowplanev1alpha1.StreamOnDynamicTable{}, newList: func() client.ObjectList { return &snowplanev1alpha1.StreamOnDynamicTableList{} }},
		{proto: &snowplanev1alpha1.Tag{}, newList: func() client.ObjectList { return &snowplanev1alpha1.TagList{} }},
		{proto: &snowplanev1alpha1.NetworkPolicy{}, newList: func() client.ObjectList { return &snowplanev1alpha1.NetworkPolicyList{} }},
		{proto: &snowplanev1alpha1.ResourceMonitor{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ResourceMonitorList{} }},
		{proto: &snowplanev1alpha1.MaskingPolicy{}, newList: func() client.ObjectList { return &snowplanev1alpha1.MaskingPolicyList{} }},
		{proto: &snowplanev1alpha1.RowAccessPolicy{}, newList: func() client.ObjectList { return &snowplanev1alpha1.RowAccessPolicyList{} }},
		{proto: &snowplanev1alpha1.GrantOwnership{}, newList: func() client.ObjectList { return &snowplanev1alpha1.GrantOwnershipList{} }},
		{proto: &snowplanev1alpha1.GrantPrivilegesToAccountRole{}, newList: func() client.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToAccountRoleList{} }},
		{proto: &snowplanev1alpha1.GrantPrivilegesToDatabaseRole{}, newList: func() client.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleList{} }},
		{proto: &snowplanev1alpha1.GrantPrivilegesToShare{}, newList: func() client.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToShareList{} }},
		{proto: &snowplanev1alpha1.StorageIntegrationAWS{}, newList: func() client.ObjectList { return &snowplanev1alpha1.StorageIntegrationAWSList{} }},
		{proto: &snowplanev1alpha1.StorageIntegrationGCS{}, newList: func() client.ObjectList { return &snowplanev1alpha1.StorageIntegrationGCSList{} }},
		{proto: &snowplanev1alpha1.StorageIntegrationAzure{}, newList: func() client.ObjectList { return &snowplanev1alpha1.StorageIntegrationAzureList{} }},
		{proto: &snowplanev1alpha1.FileFormat{}, newList: func() client.ObjectList { return &snowplanev1alpha1.FileFormatList{} }},
		{proto: &snowplanev1alpha1.Pipe{}, newList: func() client.ObjectList { return &snowplanev1alpha1.PipeList{} }},
		{proto: &snowplanev1alpha1.DynamicTable{}, newList: func() client.ObjectList { return &snowplanev1alpha1.DynamicTableList{} }},
		{proto: &snowplanev1alpha1.Alert{}, newList: func() client.ObjectList { return &snowplanev1alpha1.AlertList{} }},
		{proto: &snowplanev1alpha1.EmailNotificationIntegration{}, newList: func() client.ObjectList { return &snowplanev1alpha1.EmailNotificationIntegrationList{} }},
		{proto: &snowplanev1alpha1.QueueNotificationIntegration{}, newList: func() client.ObjectList { return &snowplanev1alpha1.QueueNotificationIntegrationList{} }},
		{proto: &snowplanev1alpha1.WebhookNotificationIntegration{}, newList: func() client.ObjectList { return &snowplanev1alpha1.WebhookNotificationIntegrationList{} }},
		{proto: &snowplanev1alpha1.SAML2Integration{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SAML2IntegrationList{} }},
		{proto: &snowplanev1alpha1.ExternalOAuthIntegration{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ExternalOAuthIntegrationList{} }},
		{proto: &snowplanev1alpha1.SCIMIntegration{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SCIMIntegrationList{} }},
		{proto: &snowplanev1alpha1.PasswordPolicy{}, newList: func() client.ObjectList { return &snowplanev1alpha1.PasswordPolicyList{} }},
		{proto: &snowplanev1alpha1.AuthenticationPolicy{}, newList: func() client.ObjectList { return &snowplanev1alpha1.AuthenticationPolicyList{} }},
		{proto: &snowplanev1alpha1.NetworkRule{}, newList: func() client.ObjectList { return &snowplanev1alpha1.NetworkRuleList{} }},
		{proto: &snowplanev1alpha1.AccountRoleAssignment{}, newList: func() client.ObjectList { return &snowplanev1alpha1.AccountRoleAssignmentList{} }},
		{proto: &snowplanev1alpha1.DatabaseRoleAssignment{}, newList: func() client.ObjectList { return &snowplanev1alpha1.DatabaseRoleAssignmentList{} }},
		{proto: &snowplanev1alpha1.TagAssociation{}, newList: func() client.ObjectList { return &snowplanev1alpha1.TagAssociationList{} }},
		{proto: &snowplanev1alpha1.NetworkPolicyAttachment{}, newList: func() client.ObjectList { return &snowplanev1alpha1.NetworkPolicyAttachmentList{} }},
		{proto: &snowplanev1alpha1.PasswordPolicyAttachment{}, newList: func() client.ObjectList { return &snowplanev1alpha1.PasswordPolicyAttachmentList{} }},
		{proto: &snowplanev1alpha1.MaskingPolicyApplication{}, newList: func() client.ObjectList { return &snowplanev1alpha1.MaskingPolicyApplicationList{} }},
		{proto: &snowplanev1alpha1.TableConstraint{}, newList: func() client.ObjectList { return &snowplanev1alpha1.TableConstraintList{} }},
		{proto: &snowplanev1alpha1.Sequence{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SequenceList{} }},
		{proto: &snowplanev1alpha1.ExternalTable{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ExternalTableList{} }},
		{proto: &snowplanev1alpha1.MaterializedView{}, newList: func() client.ObjectList { return &snowplanev1alpha1.MaterializedViewList{} }},
		{proto: &snowplanev1alpha1.ProcedureSQL{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ProcedureSQLList{} }},
		{proto: &snowplanev1alpha1.ProcedureJavascript{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ProcedureJavascriptList{} }},
		{proto: &snowplanev1alpha1.ProcedurePython{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ProcedurePythonList{} }},
		{proto: &snowplanev1alpha1.ProcedureJava{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ProcedureJavaList{} }},
		{proto: &snowplanev1alpha1.ProcedureScala{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ProcedureScalaList{} }},
		{proto: &snowplanev1alpha1.FunctionSQL{}, newList: func() client.ObjectList { return &snowplanev1alpha1.FunctionSQLList{} }},
		{proto: &snowplanev1alpha1.FunctionJavascript{}, newList: func() client.ObjectList { return &snowplanev1alpha1.FunctionJavascriptList{} }},
		{proto: &snowplanev1alpha1.FunctionPython{}, newList: func() client.ObjectList { return &snowplanev1alpha1.FunctionPythonList{} }},
		{proto: &snowplanev1alpha1.FunctionJava{}, newList: func() client.ObjectList { return &snowplanev1alpha1.FunctionJavaList{} }},
		{proto: &snowplanev1alpha1.FunctionScala{}, newList: func() client.ObjectList { return &snowplanev1alpha1.FunctionScalaList{} }},
		{proto: &snowplanev1alpha1.SecretWithClientCredentials{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SecretWithClientCredentialsList{} }},
		{proto: &snowplanev1alpha1.SecretWithAuthorizationCodeGrant{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SecretWithAuthorizationCodeGrantList{} }},
		{proto: &snowplanev1alpha1.SecretWithBasicAuthentication{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SecretWithBasicAuthenticationList{} }},
		{proto: &snowplanev1alpha1.SecretWithGenericString{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SecretWithGenericStringList{} }},
		{proto: &snowplanev1alpha1.APIAuthenticationIntegrationWithClientCredentials{}, newList: func() client.ObjectList {
			return &snowplanev1alpha1.APIAuthenticationIntegrationWithClientCredentialsList{}
		}},
		{proto: &snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant{}, newList: func() client.ObjectList {
			return &snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrantList{}
		}},
		{proto: &snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer{}, newList: func() client.ObjectList { return &snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearerList{} }},
		{proto: &snowplanev1alpha1.SQLStatement{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SQLStatementList{} }},
		{proto: &snowplanev1alpha1.ExternalVolume{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ExternalVolumeList{} }},
		{proto: &snowplanev1alpha1.CortexSearchService{}, newList: func() client.ObjectList { return &snowplanev1alpha1.CortexSearchServiceList{} }},
		{proto: &snowplanev1alpha1.GitRepository{}, newList: func() client.ObjectList { return &snowplanev1alpha1.GitRepositoryList{} }},
		{proto: &snowplanev1alpha1.Streamlit{}, newList: func() client.ObjectList { return &snowplanev1alpha1.StreamlitList{} }},
		{proto: &snowplanev1alpha1.Share{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ShareList{} }},
		{proto: &snowplanev1alpha1.ExternalFunction{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ExternalFunctionList{} }},
		{proto: &snowplanev1alpha1.ComputePool{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ComputePoolList{} }},
		{proto: &snowplanev1alpha1.ImageRepository{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ImageRepositoryList{} }},
		{proto: &snowplanev1alpha1.Service{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ServiceList{} }},
		{proto: &snowplanev1alpha1.FailoverGroup{}, newList: func() client.ObjectList { return &snowplanev1alpha1.FailoverGroupList{} }},
		{proto: &snowplanev1alpha1.APIIntegration{}, newList: func() client.ObjectList { return &snowplanev1alpha1.APIIntegrationList{} }},
		{proto: &snowplanev1alpha1.SecondaryDatabase{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SecondaryDatabaseList{} }},
		{proto: &snowplanev1alpha1.SharedDatabase{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SharedDatabaseList{} }},
		{proto: &snowplanev1alpha1.OAuthIntegrationForCustomClients{}, newList: func() client.ObjectList { return &snowplanev1alpha1.OAuthIntegrationForCustomClientsList{} }},
		{proto: &snowplanev1alpha1.OAuthIntegrationForPartnerApplications{}, newList: func() client.ObjectList { return &snowplanev1alpha1.OAuthIntegrationForPartnerApplicationsList{} }},
		{proto: &snowplanev1alpha1.PrimaryConnection{}, newList: func() client.ObjectList { return &snowplanev1alpha1.PrimaryConnectionList{} }},
		{proto: &snowplanev1alpha1.SecondaryConnection{}, newList: func() client.ObjectList { return &snowplanev1alpha1.SecondaryConnectionList{} }},
	}
}

// providerRefExtractor is a field.IndexerFunc that extracts a
// namespace-qualified "namespace/name" key from .spec.providerRef
// so that isInUse queries are scoped to the correct namespace and
// identically-named ProviderConfigs in different namespaces do not
// produce false-positive in-use results.
func providerRefExtractor(o client.Object) []string {
	mr, ok := o.(interface {
		GetProviderRef() snowplanev1alpha1.ProviderReference
	})
	if !ok {
		return nil
	}

	ref := mr.GetProviderRef()
	if ref.Name == "" {
		return nil
	}

	// Namespace-qualify: use the CR's own namespace when providerRef
	// does not specify one (the common case).
	ns := ref.Namespace
	if ns == "" {
		ns = o.GetNamespace()
	}

	return []string{ns + "/" + ref.Name}
}

// SetupWithManager sets up the reconciler with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrent int) error {
	// Register a field indexer for .spec.providerRef.name on every managed
	// resource type so that isInUse can perform efficient lookups.
	for _, entry := range managedResourceTypes() {
		if err := mgr.GetFieldIndexer().IndexField(
			context.Background(), entry.proto, providerRefIndex, providerRefExtractor,
		); err != nil {
			return fmt.Errorf("creating field indexer for %T %s: %w", entry.proto, providerRefIndex, err)
		}
	}

	// Register a field indexer for Secret→ProviderConfig reverse lookups
	// so mapSecretToProviderConfigs uses O(1) indexed queries (R9-4).
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(), &snowplanev1alpha1.ProviderConfig{}, secretRefIndex, secretRefExtractor,
	); err != nil {
		return fmt.Errorf("creating field indexer for ProviderConfig %s: %w", secretRefIndex, err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&snowplanev1alpha1.ProviderConfig{}, builder.WithPredicates(reconciler.DesiredStateChanged())).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.mapSecretToProviderConfigs),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrent}).
		Named("providerconfig").
		Complete(r)
}

// mapSecretToProviderConfigs enqueues all ProviderConfig CRs that reference
// the changed Secret, so they re-reconcile when credentials are rotated.
// Uses a field indexer for O(1) lookups instead of listing all ProviderConfigs (R9-4).
func (r *Reconciler) mapSecretToProviderConfigs(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	pcs := &snowplanev1alpha1.ProviderConfigList{}
	if err := r.client.List(ctx, pcs,
		client.MatchingFields{secretRefIndex: secretRefKey(obj.GetNamespace(), obj.GetName())},
	); err != nil {
		logger.Error(err, "listing ProviderConfigs for secret watch")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(pcs.Items))

	for i := range pcs.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: pcs.Items[i].Namespace,
				Name:      pcs.Items[i].Name,
			},
		})
	}

	return requests
}

// Reconcile performs a single reconciliation loop for a ProviderConfig.
//
// Required RBAC:
//   - providerconfigs: get, list, watch, create, update, patch, delete
//   - providerconfigs/status: get, update, patch
//   - secrets: get, list, watch
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	start := time.Now()

	defer func() {
		metrics.ReconcileDuration.With(prometheus.Labels{"controller": "providerconfig"}).Observe(time.Since(start).Seconds())

		reconcileResult := "success"
		if retErr != nil {
			reconcileResult = "error"
		}

		metrics.RecordReconcile("providerconfig", reconcileResult)
	}()

	logger := log.FromContext(ctx)

	// Fetch the ProviderConfig.
	pc := &snowplanev1alpha1.ProviderConfig{}
	if err := r.client.Get(ctx, req.NamespacedName, pc); err != nil {
		if apierrors.IsNotFound(err) {
			// CR deleted — evict any cached client and clean up metrics.
			// Use namespace-qualified key to avoid cross-namespace collisions.
			cacheKey := provider.ProviderCacheKey(req.Namespace, req.Name)
			r.factory.Evict(cacheKey)
			r.rateLimiter.Evict(cacheKey)
			r.circuitBreaker.Evict(cacheKey)
			metrics.DeleteProviderConfigHealthy(cacheKey)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("reconciling ProviderConfig", "name", pc.Name)

	// Namespace-qualified cache key for all subsystems.
	cacheKey := provider.ProviderCacheKey(pc.Namespace, pc.Name)

	// Handle deletion with in-use guard.
	if !pc.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, pc, cacheKey)
	}

	// Ensure finalizer is present.
	if !finalizers.Has(pc, finalizerName) {
		patchBase := pc.DeepCopy()
		finalizers.Add(pc, finalizerName)

		if err := r.client.Patch(ctx, pc, client.MergeFrom(patchBase)); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding finalizer: %w", err)
		}

	}

	// Enforce role allowlist: reject roles that are not in the operator's
	// configured --allowed-roles set (M-4: ProviderConfig role escalation guard).
	if pc.Spec.Role != "" && !r.IsRoleAllowed(pc.Spec.Role) {
		msg := fmt.Sprintf("role %q is not in the allowed roles list", pc.Spec.Role)
		conditions.SetNotReady(pc, snowplanev1alpha1.ReasonRoleNotAllowed, msg)
		conditions.SetNotSynced(pc, snowplanev1alpha1.ReasonRoleNotAllowed, msg)
		r.recorder.Event(pc, corev1.EventTypeWarning, snowplanev1alpha1.ReasonRoleNotAllowed, msg)

		metrics.SetProviderConfigHealthy(cacheKey, pc.Spec.Account, false)

		r.bestEffortPatchStatus(ctx, pc)

		// Return nil to avoid exponential backoff — the role won't change
		// until the user updates the ProviderConfig spec or the operator
		// restarts with a different --allowed-roles value. Periodic resync
		// will re-evaluate.
		return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
	}

	// Resolve credentials: either from a Secret or from WIF (no Secret).
	var secret *corev1.Secret

	if pc.Spec.AuthenticationType == snowplanev1alpha1.AuthenticationTypeWorkloadIdentity {
		// WorkloadIdentity: the gosnowflake driver reads the token file natively.
		secret = nil
	} else if pc.Spec.Credentials.SecretRef != nil {
		secret = &corev1.Secret{}

		secretNS := pc.Spec.Credentials.SecretRef.Namespace
		if secretNS == "" {
			secretNS = pc.Namespace
		}

		secretRef := types.NamespacedName{
			Namespace: secretNS,
			Name:      pc.Spec.Credentials.SecretRef.Name,
		}

		if err := r.client.Get(ctx, secretRef, secret); err != nil {
			msg := fmt.Sprintf("credentials secret %q not found: %v", secretRef, err)
			conditions.SetNotReady(pc, snowplanev1alpha1.ReasonSecretNotFound, msg)
			conditions.SetNotSynced(pc, snowplanev1alpha1.ReasonSecretNotFound, msg)
			r.recorder.Event(pc, corev1.EventTypeWarning, snowplanev1alpha1.ReasonSecretNotFound, msg)

			r.factory.Evict(cacheKey)

			r.bestEffortPatchStatus(ctx, pc)

			return ctrl.Result{}, err
		}
	} else {
		msg := "spec.credentials.secretRef is required"
		conditions.SetNotReady(pc, snowplanev1alpha1.ReasonCredentialsError, msg)
		conditions.SetNotSynced(pc, snowplanev1alpha1.ReasonCredentialsError, msg)
		r.recorder.Event(pc, corev1.EventTypeWarning, snowplanev1alpha1.ReasonCredentialsError, msg)

		r.factory.Evict(cacheKey)

		r.bestEffortPatchStatus(ctx, pc)

		return ctrl.Result{}, errors.New(msg)
	}

	// Build the Snowflake client config.
	cfg, err := provider.BuildSnowflakeConfig(pc, secret)
	if err != nil {
		conditions.SetNotReady(pc, snowplanev1alpha1.ReasonInvalidConfig, err.Error())
		conditions.SetNotSynced(pc, snowplanev1alpha1.ReasonInvalidConfig, err.Error())
		r.recorder.Event(pc, corev1.EventTypeWarning, snowplanev1alpha1.ReasonInvalidConfig, err.Error())

		r.factory.Evict(cacheKey)

		metrics.SetProviderConfigHealthy(cacheKey, pc.Spec.Account, false)

		r.bestEffortPatchStatus(ctx, pc)

		return ctrl.Result{}, err
	}

	// Compute a hash of the config to detect changes.
	hash := provider.ComputeHash(cfg)

	// Detect credential rotation: if the factory has a cached client with a
	// different hash, credentials have changed since the last reconciliation.
	credentialsRotated := r.factory.HasStaleHash(cacheKey, hash)

	// Get or create a cached Snowflake client.
	sfClient, err := r.factory.GetOrCreate(ctx, cacheKey, hash, cfg)
	if err != nil {
		conditions.SetNotReady(pc, snowplanev1alpha1.ReasonClientFailed, err.Error())
		conditions.SetNotSynced(pc, snowplanev1alpha1.ReasonClientFailed, err.Error())
		r.recorder.Event(pc, corev1.EventTypeWarning, snowplanev1alpha1.ReasonClientFailed, err.Error())

		metrics.SetProviderConfigHealthy(cacheKey, pc.Spec.Account, false)

		r.bestEffortPatchStatus(ctx, pc)

		return ctrl.Result{}, err
	}

	// Verify connectivity with timeout.
	pingCtx, pingCancel := context.WithTimeout(ctx, r.getSnowflakeOpTimeout())
	defer pingCancel()

	if err := metrics.ObserveSnowflakeOp("providerconfig", "ping", func() error {
		return r.pingFn(pingCtx, sfClient)
	}); err != nil {
		msg := fmt.Sprintf("failed to ping Snowflake: %v", err)
		conditions.SetNotReady(pc, snowplanev1alpha1.ReasonPingFailed, msg)
		conditions.SetNotSynced(pc, snowplanev1alpha1.ReasonPingFailed, msg)
		r.recorder.Event(pc, corev1.EventTypeWarning, snowplanev1alpha1.ReasonPingFailed, msg)

		r.factory.Evict(cacheKey)

		metrics.SetProviderConfigHealthy(cacheKey, pc.Spec.Account, false)

		r.bestEffortPatchStatus(ctx, pc)

		return ctrl.Result{}, err
	}

	// Success.
	conditions.SetReady(pc, "Snowflake connection verified")
	conditions.SetSynced(pc, "Reconciliation complete")
	pc.Status.ObservedGeneration = pc.Generation

	metrics.SetProviderConfigHealthy(cacheKey, pc.Spec.Account, true)

	if credentialsRotated {
		r.recorder.Event(pc, corev1.EventTypeNormal, snowplanev1alpha1.ReasonCredentialsRotated, "Credentials rotated, reconnecting")
	}

	r.recorder.Event(pc, corev1.EventTypeNormal, snowplanev1alpha1.ReasonAvailable, "Snowflake connection verified")

	if err := r.patchStatus(ctx, pc); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
}

// patchStatus uses Server-Side Apply (SSA) to update the status subresource.
// SSA eliminates the need for ResourceVersion-based conflict detection —
// the server resolves ownership via managedFields instead.
func (r *Reconciler) patchStatus(ctx context.Context, pc *snowplanev1alpha1.ProviderConfig) error {
	pc.SetGroupVersionKind(snowplanev1alpha1.GroupVersion.WithKind("ProviderConfig"))

	// SSA patch objects must not contain managedFields.
	pc.SetManagedFields(nil)

	return r.client.SubResource("status").Patch(ctx, pc, client.Apply, //nolint:staticcheck // client.Apply removal requires generated ApplyConfiguration types
		client.FieldOwner(reconciler.StatusFieldOwner),
		client.ForceOwnership,
	)
}

// bestEffortPatchStatus patches status and logs a warning on failure.
// Used in error paths where the primary error must be returned.
func (r *Reconciler) bestEffortPatchStatus(ctx context.Context, pc *snowplanev1alpha1.ProviderConfig) {
	if err := r.patchStatus(ctx, pc); err != nil {
		log.FromContext(ctx).Error(err, "best-effort status patch failed")
	}
}

// reconcileDelete handles ProviderConfig deletion with an in-use guard.
// If any managed resource still references this ProviderConfig, the finalizer
// is not removed and a warning event is emitted.
func (r *Reconciler) reconcileDelete(ctx context.Context, pc *snowplanev1alpha1.ProviderConfig, cacheKey string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !finalizers.Has(pc, finalizerName) {
		// No finalizer — nothing to do.
		r.factory.Evict(cacheKey)
		r.rateLimiter.Evict(cacheKey)
		r.circuitBreaker.Evict(cacheKey)
		return ctrl.Result{}, nil
	}

	// Check if any managed resources still reference this ProviderConfig.
	inUse, err := r.isInUse(ctx, pc.Namespace, pc.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("checking in-use references: %w", err)
	}

	if inUse {
		msg := fmt.Sprintf("ProviderConfig %q is still referenced by managed resources; cannot delete", pc.Name)
		logger.Info(msg)
		r.recorder.Event(pc, corev1.EventTypeWarning, snowplanev1alpha1.ReasonInUse, msg)
		conditions.SetNotReady(pc, snowplanev1alpha1.ReasonInUse, msg)
		r.bestEffortPatchStatus(ctx, pc)

		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Safe to delete — remove finalizer.
	patchBase := pc.DeepCopy()
	finalizers.Remove(pc, finalizerName)
	if err := r.client.Patch(ctx, pc, client.MergeFrom(patchBase)); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing finalizer: %w", err)
	}

	r.factory.Evict(cacheKey)
	r.rateLimiter.Evict(cacheKey)
	r.circuitBreaker.Evict(cacheKey)
	metrics.DeleteProviderConfigHealthy(cacheKey)
	logger.Info("ProviderConfig deleted, client evicted", "name", pc.Name)

	return ctrl.Result{}, nil
}

// isInUse checks if any managed resource in the cluster references the given
// ProviderConfig by namespace/name, using the field indexer for O(1) lookups per type.
func (r *Reconciler) isInUse(ctx context.Context, namespace, providerName string) (bool, error) {
	key := namespace + "/" + providerName

	for _, entry := range managedResourceTypes() {
		list := entry.newList()
		if err := r.client.List(ctx, list,
			client.MatchingFields{providerRefIndex: key},
			client.Limit(1),
		); err != nil {
			return false, err
		}

		if listLen(list) > 0 {
			return true, nil
		}
	}

	return false, nil
}

// listLen returns the number of items in an ObjectList using the generic
// meta.ExtractList helper.  This avoids a type switch that silently returns 0
// when new resource types are added without updating the switch.
func listLen(list client.ObjectList) int {
	items, err := meta.ExtractList(list)
	if err != nil {
		return 0
	}

	return len(items)
}
