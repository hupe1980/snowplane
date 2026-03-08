// Package main is the entrypoint for the Snowplane controller manager.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	// Automatically set GOMAXPROCS to match the container's CPU quota.
	// Without this, Go defaults to the host CPU count, causing excessive
	// goroutine scheduling overhead in containerized deployments.
	_ "go.uber.org/automaxprocs"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/circuitbreaker"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	accountrolectl "github.com/hupe1980/snowplane/internal/controller/accountrole"
	alertctl "github.com/hupe1980/snowplane/internal/controller/alert"
	apiauthacgctl "github.com/hupe1980/snowplane/internal/controller/apiauthenticationintegrationwithauthorizationcodegrant"
	apiauthccctl "github.com/hupe1980/snowplane/internal/controller/apiauthenticationintegrationwithclientcredentials"
	apiauthjwtctl "github.com/hupe1980/snowplane/internal/controller/apiauthenticationintegrationwithjwtbearer"
	apiintegrationctl "github.com/hupe1980/snowplane/internal/controller/apiintegration"
	authenticationpolicyctl "github.com/hupe1980/snowplane/internal/controller/authenticationpolicy"

	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	computepoolctl "github.com/hupe1980/snowplane/internal/controller/computepool"
	cortexsearchservicectl "github.com/hupe1980/snowplane/internal/controller/cortexsearchservice"
	database "github.com/hupe1980/snowplane/internal/controller/database"
	databaserolectl "github.com/hupe1980/snowplane/internal/controller/databaserole"
	dynamictablectl "github.com/hupe1980/snowplane/internal/controller/dynamictable"
	emailnotificationintegrationctl "github.com/hupe1980/snowplane/internal/controller/emailnotificationintegration"
	externalfunctionctl "github.com/hupe1980/snowplane/internal/controller/externalfunction"
	externaloauthintegrationctl "github.com/hupe1980/snowplane/internal/controller/externaloauthintegration"
	externalstagectl "github.com/hupe1980/snowplane/internal/controller/externalstage"
	externaltablectl "github.com/hupe1980/snowplane/internal/controller/externaltable"
	externalvolumectl "github.com/hupe1980/snowplane/internal/controller/externalvolume"
	failovergroupctl "github.com/hupe1980/snowplane/internal/controller/failovergroup"
	fieldexportctl "github.com/hupe1980/snowplane/internal/controller/fieldexport"
	fileformatctl "github.com/hupe1980/snowplane/internal/controller/fileformat"
	functionjavactl "github.com/hupe1980/snowplane/internal/controller/functionjava"
	functionjavascriptctl "github.com/hupe1980/snowplane/internal/controller/functionjavascript"
	functionpythonctl "github.com/hupe1980/snowplane/internal/controller/functionpython"
	functionscalactl "github.com/hupe1980/snowplane/internal/controller/functionscala"
	functionsqlctl "github.com/hupe1980/snowplane/internal/controller/functionsql"
	gitrepositoryctl "github.com/hupe1980/snowplane/internal/controller/gitrepository"
	grantctl "github.com/hupe1980/snowplane/internal/controller/grant"
	grantownershipctl "github.com/hupe1980/snowplane/internal/controller/grantownership"
	imagerepositoryctl "github.com/hupe1980/snowplane/internal/controller/imagerepository"
	internalstagectl "github.com/hupe1980/snowplane/internal/controller/internalstage"
	maskingpolicyctl "github.com/hupe1980/snowplane/internal/controller/maskingpolicy"
	maskingpolicyapplicationctl "github.com/hupe1980/snowplane/internal/controller/maskingpolicyapplication"
	materializedviewctl "github.com/hupe1980/snowplane/internal/controller/materializedview"
	networkpolicyctl "github.com/hupe1980/snowplane/internal/controller/networkpolicy"
	networkpolicyattachmentctl "github.com/hupe1980/snowplane/internal/controller/networkpolicyattachment"
	networkrulectl "github.com/hupe1980/snowplane/internal/controller/networkrule"
	passwordpolicyctl "github.com/hupe1980/snowplane/internal/controller/passwordpolicy"
	passwordpolicyattachmentctl "github.com/hupe1980/snowplane/internal/controller/passwordpolicyattachment"
	pipectl "github.com/hupe1980/snowplane/internal/controller/pipe"
	procedurejavactl "github.com/hupe1980/snowplane/internal/controller/procedurejava"
	procedurejavascriptctl "github.com/hupe1980/snowplane/internal/controller/procedurejavascript"
	procedurepythonctl "github.com/hupe1980/snowplane/internal/controller/procedurepython"
	procedurescalactl "github.com/hupe1980/snowplane/internal/controller/procedurescala"
	proceduresqlctl "github.com/hupe1980/snowplane/internal/controller/proceduresql"
	providerconfig "github.com/hupe1980/snowplane/internal/controller/providerconfig"
	queuenotificationintegrationctl "github.com/hupe1980/snowplane/internal/controller/queuenotificationintegration"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	resourcemonitorctl "github.com/hupe1980/snowplane/internal/controller/resourcemonitor"
	roleassignmentctl "github.com/hupe1980/snowplane/internal/controller/roleassignment"
	rowaccesspolicyctl "github.com/hupe1980/snowplane/internal/controller/rowaccesspolicy"
	saml2integrationctl "github.com/hupe1980/snowplane/internal/controller/saml2integration"
	schemactl "github.com/hupe1980/snowplane/internal/controller/schema"
	scimintegrationctl "github.com/hupe1980/snowplane/internal/controller/scimintegration"
	secondarydatabasectl "github.com/hupe1980/snowplane/internal/controller/secondarydatabase"
	secretwithauthorizationcodegrantctl "github.com/hupe1980/snowplane/internal/controller/secretwithauthorizationcodegrant"
	secretwithbasicauthenticationctl "github.com/hupe1980/snowplane/internal/controller/secretwithbasicauthentication"
	secretwithclientcredentialsctl "github.com/hupe1980/snowplane/internal/controller/secretwithclientcredentials"
	secretwithgenericstringctl "github.com/hupe1980/snowplane/internal/controller/secretwithgenericstring"
	sequencectl "github.com/hupe1980/snowplane/internal/controller/sequence"
	servicectl "github.com/hupe1980/snowplane/internal/controller/service"
	sharectl "github.com/hupe1980/snowplane/internal/controller/share"
	shareddatabasectl "github.com/hupe1980/snowplane/internal/controller/shareddatabase"
	sqlstatementctl "github.com/hupe1980/snowplane/internal/controller/sqlstatement"
	storageintegrationawsctl "github.com/hupe1980/snowplane/internal/controller/storageintegrationaws"
	storageintegrationazurectl "github.com/hupe1980/snowplane/internal/controller/storageintegrationazure"
	storageintegrationgcsctl "github.com/hupe1980/snowplane/internal/controller/storageintegrationgcs"
	streamlitctl "github.com/hupe1980/snowplane/internal/controller/streamlit"
	streamondirectorytablectl "github.com/hupe1980/snowplane/internal/controller/streamondirectorytable"
	streamondynamictablectl "github.com/hupe1980/snowplane/internal/controller/streamondynamictable"
	streamonexternaltablectl "github.com/hupe1980/snowplane/internal/controller/streamonexternaltable"
	streamontablectl "github.com/hupe1980/snowplane/internal/controller/streamontable"
	streamonviewctl "github.com/hupe1980/snowplane/internal/controller/streamonview"
	tablectl "github.com/hupe1980/snowplane/internal/controller/table"
	tableconstraintctl "github.com/hupe1980/snowplane/internal/controller/tableconstraint"
	tagctl "github.com/hupe1980/snowplane/internal/controller/tag"
	tagassociationctl "github.com/hupe1980/snowplane/internal/controller/tagassociation"
	taskctl "github.com/hupe1980/snowplane/internal/controller/task"
	userctl "github.com/hupe1980/snowplane/internal/controller/user"
	viewctl "github.com/hupe1980/snowplane/internal/controller/view"
	warehousectl "github.com/hupe1980/snowplane/internal/controller/warehouse"
	webhooknotificationintegrationctl "github.com/hupe1980/snowplane/internal/controller/webhooknotificationintegration"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/sharding"
	"github.com/hupe1980/snowplane/internal/tracing"
	"github.com/hupe1980/snowplane/internal/utils/sanitize"
	"github.com/hupe1980/snowplane/internal/webhook"

	// Register Prometheus metrics in controller-runtime's registry.
	_ "github.com/hupe1980/snowplane/internal/metrics"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(snowplanev1alpha1.AddToScheme(scheme))
}

// parseDisabledControllers parses a comma-separated list of controller names
// into a set for O(1) lookup. Validation is deferred to
// validateDisabledControllers which checks against the registration table.
func parseDisabledControllers(s string) map[string]bool {
	disabled := make(map[string]bool)

	if s == "" {
		return disabled
	}

	for _, name := range strings.Split(s, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			disabled[name] = true
		}
	}

	return disabled
}

// validateDisabledControllers checks that every name in disabled is a known
// controller name from the registration table. This is auto-derived — no
// manual map to keep in sync.
func validateDisabledControllers(disabled map[string]bool, controllerNames []string) error {
	valid := make(map[string]bool, len(controllerNames))
	for _, n := range controllerNames {
		valid[n] = true
	}

	for name := range disabled {
		if !valid[name] {
			return fmt.Errorf("unknown controller name %q in --disable-controllers; valid names: %s",
				name, strings.Join(controllerNames, ", "))
		}
	}

	return nil
}

// parseAllowedRoles parses a comma-separated list of Snowflake role names into
// an uppercase set for case-insensitive O(1) lookup. Returns nil if the input
// is empty (all roles allowed).
func parseAllowedRoles(s string) map[string]bool {
	if s == "" {
		return nil
	}

	roles := make(map[string]bool)

	for _, role := range strings.Split(s, ",") {
		role = strings.TrimSpace(role)
		if role != "" {
			roles[strings.ToUpper(role)] = true
		}
	}

	if len(roles) == 0 {
		return nil
	}

	return roles
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var leaderElectionID string
	var developmentMode bool
	var maxConcurrentReconciles int
	var rateLimitQPS float64
	var rateLimitBurst int
	var accountRateLimitQPS float64
	var accountRateLimitBurst int
	var requeueInterval time.Duration
	var enableAlphaResources bool
	var disableControllers string
	var watchNamespaces string
	var cbFailureThreshold int
	var cbResetTimeout time.Duration
	var allowedRoles string
	var snowflakeOpTimeout time.Duration
	var enableWebhook bool
	var webhookPort int
	var webhookCertDir string
	var enableSQLStatement bool
	var sqlStatementDenylist string
	var enableTracing bool
	var otelEndpoint string
	var otelSamplingRatio float64
	var otelInsecure bool
	var shardID int
	var shardCount int

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&developmentMode, "development", false,
		"Enable development mode logging (human-readable, debug level).")
	flag.IntVar(&maxConcurrentReconciles, "max-concurrent-reconciles", 3,
		"The maximum number of concurrent reconciles per controller.")
	flag.Float64Var(&rateLimitQPS, "rate-limit-qps", 10,
		"Maximum sustained queries per second to Snowflake per controller per provider (0 = disabled).")
	flag.IntVar(&rateLimitBurst, "rate-limit-burst", 20,
		"Maximum burst size for the per-controller Snowflake API rate limiter.")
	flag.Float64Var(&accountRateLimitQPS, "account-rate-limit-qps", 50,
		"Maximum aggregate queries per second to Snowflake per account (provider). "+
			"Caps total QPS across all controllers for a given Snowflake account (0 = disabled).")
	flag.IntVar(&accountRateLimitBurst, "account-rate-limit-burst", 100,
		"Maximum burst size for the per-account aggregate Snowflake API rate limiter.")
	flag.DurationVar(&requeueInterval, "requeue-interval", 5*time.Minute,
		"How often each reconciler re-observes Snowflake state for drift detection.")
	flag.BoolVar(&enableAlphaResources, "enable-alpha-resources", true,
		"Enable controllers for alpha-maturity CRDs. Set to false to skip alpha resources.")
	flag.StringVar(&disableControllers, "disable-controllers", "",
		"Comma-separated list of controller names to disable (e.g. \"grantprivilegestoaccountrole,internalstage,view\"). "+
			"Pass an invalid name to see the full list of valid controller names.")
	flag.StringVar(&watchNamespaces, "watch-namespaces", "",
		"Comma-separated list of namespaces to watch. If empty, all namespaces are watched.")
	flag.StringVar(&leaderElectionID, "leader-election-id", "snowplane-leader-election",
		"The name of the resource used for leader election.")
	flag.IntVar(&cbFailureThreshold, "circuit-breaker-threshold", 5,
		"Number of consecutive Snowflake API failures before the circuit breaker opens (per-provider).")
	flag.DurationVar(&cbResetTimeout, "circuit-breaker-reset-timeout", 60*time.Second,
		"How long the circuit breaker stays open before allowing a probe request.")
	flag.StringVar(&allowedRoles, "allowed-roles", "",
		"Comma-separated allowlist of Snowflake roles permitted in ProviderConfig (case-insensitive). "+
			"If empty, all roles are allowed. Example: \"SYSADMIN,USERADMIN,DATA_ENGINEER\".")
	flag.DurationVar(&snowflakeOpTimeout, "snowflake-op-timeout", 60*time.Second,
		"Per-operation timeout for Snowflake CRUD calls (Observe, Create, Alter, Drop).")
	flag.BoolVar(&enableSQLStatement, "enable-sql-statement", false,
		"Enable the SQLStatement escape-hatch controller. "+
			"This allows executing arbitrary SQL against Snowflake — use with caution.")
	flag.StringVar(&sqlStatementDenylist, "sql-statement-denylist", "",
		"Comma-separated list of SQL keyword patterns to block in SQLStatement execute/revert SQL. "+
			"Example: \"DROP DATABASE,ALTER USER,DROP SCHEMA\". Empty means no denylist (all statements allowed).")
	flag.BoolVar(&enableWebhook, "enable-webhook", false,
		"Enable the ownership-conflict validating admission webhook.")
	flag.IntVar(&webhookPort, "webhook-port", 9443,
		"Port for the webhook server (only used when --enable-webhook is set).")
	flag.StringVar(&webhookCertDir, "webhook-cert-dir", "/tmp/k8s-webhook-server/serving-certs",
		"Directory containing TLS cert and key for the webhook server.")
	flag.BoolVar(&enableTracing, "enable-tracing", false,
		"Enable OpenTelemetry distributed tracing export.")
	flag.StringVar(&otelEndpoint, "otel-endpoint", "localhost:4317",
		"gRPC endpoint for the OpenTelemetry Collector (host:port).")
	flag.Float64Var(&otelSamplingRatio, "otel-sampling-ratio", 1.0,
		"Trace sampling ratio (0.0–1.0). 1.0 = sample everything, 0.1 = 10%%.")
	flag.BoolVar(&otelInsecure, "otel-insecure", true,
		"Use insecure (plaintext) gRPC connection to the OTel Collector.")
	flag.IntVar(&shardID, "shard-id", 0,
		"Zero-based shard index for this manager instance (used with --shard-count).")
	flag.IntVar(&shardCount, "shard-count", 1,
		"Total number of manager shards. 1 = no sharding (single instance).")

	opts := zap.Options{Development: developmentMode}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Set up OpenTelemetry tracing.
	tracingProvider, err := tracing.Setup(context.Background(), tracing.Config{
		Enabled:       enableTracing,
		Endpoint:      otelEndpoint,
		SamplingRatio: otelSamplingRatio,
		Insecure:      otelInsecure,
	})
	if err != nil {
		setupLog.Error(err, "unable to set up OpenTelemetry tracing")
		os.Exit(1)
	}
	defer tracingProvider.Shutdown()

	if enableTracing {
		setupLog.Info("OpenTelemetry tracing enabled", "endpoint", otelEndpoint, "samplingRatio", otelSamplingRatio)
	}

	mgrOpts := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       leaderElectionID,
	}

	// Configure the webhook server when the ownership webhook is enabled.
	if enableWebhook {
		mgrOpts.WebhookServer = ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port:    webhookPort,
			CertDir: webhookCertDir,
		})
	}

	// Restrict the cache to specific namespaces if --watch-namespaces is set.
	if watchNamespaces != "" {
		nsMap := make(map[string]cache.Config)
		for _, ns := range strings.Split(watchNamespaces, ",") {
			ns = strings.TrimSpace(ns)
			if ns != "" {
				nsMap[ns] = cache.Config{}
			}
		}

		if len(nsMap) > 0 {
			mgrOpts.Cache = cache.Options{
				DefaultNamespaces: nsMap,
			}

			setupLog.Info("restricting watch to namespaces", "namespaces", watchNamespaces)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// Create shared client factory.
	factory := clientfactory.NewClientFactory().
		WithStartupGrace(30 * time.Second) // L-2: grace period before readiness probe checks Snowflake connectivity
	defer factory.Close()

	// Create shared rate limiter with hierarchical per-controller + per-account budgets.
	rl := ratelimit.New(ratelimit.Options{
		QPS:          rateLimitQPS,
		Burst:        rateLimitBurst,
		AccountQPS:   accountRateLimitQPS,
		AccountBurst: accountRateLimitBurst,
	})

	// Create shared circuit breaker for provider failure isolation.
	cb := circuitbreaker.New(circuitbreaker.Options{
		FailureThreshold: cbFailureThreshold,
		ResetTimeout:     cbResetTimeout,
	})

	// Parse per-controller disable set (validated after registration table is built).
	disabled := parseDisabledControllers(disableControllers)

	// Parse role allowlist.
	allowedRolesSet := parseAllowedRoles(allowedRoles)

	// Configure sharding predicate (no-op when shardCount <= 1).
	shardOpts := sharding.Options{ShardID: shardID, ShardCount: shardCount}
	var shardPred predicate.Predicate
	if shardOpts.Enabled() {
		shardPred = sharding.NewPredicate(shardOpts)
		setupLog.Info("controller sharding enabled", "shardID", shardID, "shardCount", shardCount)
	}

	kc := mgr.GetClient()

	// Set up ProviderConfig reconciler.
	if err := providerconfig.NewReconciler(
		kc,
		factory,
		sanitize.NewSafeRecorderFromEvents(mgr.GetEventRecorder("providerconfig-controller")),
		rl,
		cb,
		allowedRolesSet,
	).WithRequeueInterval(requeueInterval).WithSnowflakeOpTimeout(snowflakeOpTimeout).SetupWithManager(mgr, maxConcurrentReconciles); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProviderConfig")
		os.Exit(1)
	}

	// controllerRec creates a safe event recorder for a controller.
	controllerRec := func(name string) record.EventRecorder {
		return sanitize.NewSafeRecorderFromEvents(mgr.GetEventRecorder(name + "-controller"))
	}

	// Shared controller configuration. Per-controller Disabled override
	// is applied in the registration loop below.
	cfg := reconciler.SetupConfig{
		Manager:                 mgr,
		CircuitBreaker:          cb,
		RequeueInterval:         requeueInterval,
		SnowflakeOpTimeout:      snowflakeOpTimeout,
		Maturity:                "alpha",
		AlphaEnabled:            enableAlphaResources,
		MaxConcurrentReconciles: maxConcurrentReconciles,
		ShardPredicate:          shardPred,
	}

	// Declarative controller registration table.
	// Each entry is a name paired with a pre-constructed reconciler.  The
	// Setup method on GenericReconciler applies all shared config in one call,
	// eliminating repetitive With*() chains.
	controllers := []struct {
		name string
		ctrl reconciler.Registerable
	}{
		{"alert", alertctl.NewReconciler(kc, factory, controllerRec("alert"), rl)},
		{"authenticationpolicy", authenticationpolicyctl.NewReconciler(kc, factory, controllerRec("authenticationpolicy"), rl)},
		{"database", database.NewReconciler(kc, factory, controllerRec("database"), rl)},
		{"schema", schemactl.NewReconciler(kc, factory, controllerRec("schema"), rl)},
		{"warehouse", warehousectl.NewReconciler(kc, factory, controllerRec("warehouse"), rl)},
		{"accountrole", accountrolectl.NewReconciler(kc, factory, controllerRec("accountrole"), rl)},
		{"databaserole", databaserolectl.NewReconciler(kc, factory, controllerRec("databaserole"), rl)},
		{"grantprivilegestoaccountrole", grantctl.NewGrantPrivilegesToAccountRoleReconciler(kc, factory, controllerRec("grantprivilegestoaccountrole"), rl)},
		{"grantprivilegestodatabaserole", grantctl.NewGrantPrivilegesToDatabaseRoleReconciler(kc, factory, controllerRec("grantprivilegestodatabaserole"), rl)},
		{"grantprivilegestoshare", grantctl.NewGrantPrivilegesToShareReconciler(kc, factory, controllerRec("grantprivilegestoshare"), rl)},
		{"user", userctl.NewReconciler(kc, factory, controllerRec("user"), rl)},
		{"table", tablectl.NewReconciler(kc, factory, controllerRec("table"), rl)},
		{"view", viewctl.NewReconciler(kc, factory, controllerRec("view"), rl)},
		{"externalstage", externalstagectl.NewReconciler(kc, factory, controllerRec("externalstage"), rl)},
		{"internalstage", internalstagectl.NewReconciler(kc, factory, controllerRec("internalstage"), rl)},
		{"task", taskctl.NewReconciler(kc, factory, controllerRec("task"), rl)},
		{"streamontable", streamontablectl.NewReconciler(kc, factory, controllerRec("streamontable"), rl)},
		{"streamonview", streamonviewctl.NewReconciler(kc, factory, controllerRec("streamonview"), rl)},
		{"streamonexternaltable", streamonexternaltablectl.NewReconciler(kc, factory, controllerRec("streamonexternaltable"), rl)},
		{"streamondirectorytable", streamondirectorytablectl.NewReconciler(kc, factory, controllerRec("streamondirectorytable"), rl)},
		{"streamondynamictable", streamondynamictablectl.NewReconciler(kc, factory, controllerRec("streamondynamictable"), rl)},
		{"tag", tagctl.NewReconciler(kc, factory, controllerRec("tag"), rl)},
		{"networkpolicy", networkpolicyctl.NewReconciler(kc, factory, controllerRec("networkpolicy"), rl)},
		{"resourcemonitor", resourcemonitorctl.NewReconciler(kc, factory, controllerRec("resourcemonitor"), rl)},
		{"maskingpolicy", maskingpolicyctl.NewReconciler(kc, factory, controllerRec("maskingpolicy"), rl)},
		{"rowaccesspolicy", rowaccesspolicyctl.NewReconciler(kc, factory, controllerRec("rowaccesspolicy"), rl)},
		{"grantownership", grantownershipctl.NewReconciler(kc, factory, controllerRec("grantownership"), rl)},
		{"externalvolume", externalvolumectl.NewReconciler(kc, factory, controllerRec("externalvolume"), rl)},
		{"cortexsearchservice", cortexsearchservicectl.NewReconciler(kc, factory, controllerRec("cortexsearchservice"), rl)},
		{"storageintegrationaws", storageintegrationawsctl.NewReconciler(kc, factory, controllerRec("storageintegrationaws"), rl)},
		{"storageintegrationazure", storageintegrationazurectl.NewReconciler(kc, factory, controllerRec("storageintegrationazure"), rl)},
		{"storageintegrationgcs", storageintegrationgcsctl.NewReconciler(kc, factory, controllerRec("storageintegrationgcs"), rl)},
		{"fileformat", fileformatctl.NewReconciler(kc, factory, controllerRec("fileformat"), rl)},
		{"pipe", pipectl.NewReconciler(kc, factory, controllerRec("pipe"), rl)},
		{"dynamictable", dynamictablectl.NewReconciler(kc, factory, controllerRec("dynamictable"), rl)},
		{"emailnotificationintegration", emailnotificationintegrationctl.NewReconciler(kc, factory, controllerRec("emailnotificationintegration"), rl)},
		{"queuenotificationintegration", queuenotificationintegrationctl.NewReconciler(kc, factory, controllerRec("queuenotificationintegration"), rl)},
		{"webhooknotificationintegration", webhooknotificationintegrationctl.NewReconciler(kc, factory, controllerRec("webhooknotificationintegration"), rl)},
		{"saml2integration", saml2integrationctl.NewReconciler(kc, factory, controllerRec("saml2integration"), rl)},
		{"scimintegration", scimintegrationctl.NewReconciler(kc, factory, controllerRec("scimintegration"), rl)},
		{"externaloauthintegration", externaloauthintegrationctl.NewReconciler(kc, factory, controllerRec("externaloauthintegration"), rl)},
		{"failovergroup", failovergroupctl.NewReconciler(kc, factory, controllerRec("failovergroup"), rl)},
		{"apiintegration", apiintegrationctl.NewReconciler(kc, factory, controllerRec("apiintegration"), rl)},
		{"secondarydatabase", secondarydatabasectl.NewReconciler(kc, factory, controllerRec("secondarydatabase"), rl)},
		{"shareddatabase", shareddatabasectl.NewReconciler(kc, factory, controllerRec("shareddatabase"), rl)},
		{"passwordpolicy", passwordpolicyctl.NewReconciler(kc, factory, controllerRec("passwordpolicy"), rl)},
		{"networkrule", networkrulectl.NewReconciler(kc, factory, controllerRec("networkrule"), rl)},
		{"accountroleassignment", roleassignmentctl.NewAccountRoleAssignmentReconciler(kc, factory, controllerRec("accountroleassignment"), rl)},
		{"databaseroleassignment", roleassignmentctl.NewDatabaseRoleAssignmentReconciler(kc, factory, controllerRec("databaseroleassignment"), rl)},
		{"tagassociation", tagassociationctl.NewReconciler(kc, factory, controllerRec("tagassociation"), rl)},
		{"networkpolicyattachment", networkpolicyattachmentctl.NewReconciler(kc, factory, controllerRec("networkpolicyattachment"), rl)},
		{"passwordpolicyattachment", passwordpolicyattachmentctl.NewReconciler(kc, factory, controllerRec("passwordpolicyattachment"), rl)},
		{"maskingpolicyapplication", maskingpolicyapplicationctl.NewReconciler(kc, factory, controllerRec("maskingpolicyapplication"), rl)},
		{"sequence", sequencectl.NewReconciler(kc, factory, controllerRec("sequence"), rl)},
		{"externaltable", externaltablectl.NewReconciler(kc, factory, controllerRec("externaltable"), rl)},
		{"materializedview", materializedviewctl.NewReconciler(kc, factory, controllerRec("materializedview"), rl)},
		{"proceduresql", proceduresqlctl.NewReconciler(kc, factory, controllerRec("proceduresql"), rl)},
		{"procedurejavascript", procedurejavascriptctl.NewReconciler(kc, factory, controllerRec("procedurejavascript"), rl)},
		{"procedurepython", procedurepythonctl.NewReconciler(kc, factory, controllerRec("procedurepython"), rl)},
		{"procedurejava", procedurejavactl.NewReconciler(kc, factory, controllerRec("procedurejava"), rl)},
		{"procedurescala", procedurescalactl.NewReconciler(kc, factory, controllerRec("procedurescala"), rl)},
		{"functionsql", functionsqlctl.NewReconciler(kc, factory, controllerRec("functionsql"), rl)},
		{"functionjavascript", functionjavascriptctl.NewReconciler(kc, factory, controllerRec("functionjavascript"), rl)},
		{"functionpython", functionpythonctl.NewReconciler(kc, factory, controllerRec("functionpython"), rl)},
		{"functionjava", functionjavactl.NewReconciler(kc, factory, controllerRec("functionjava"), rl)},
		{"functionscala", functionscalactl.NewReconciler(kc, factory, controllerRec("functionscala"), rl)},
		{"tableconstraint", tableconstraintctl.NewReconciler(kc, factory, controllerRec("tableconstraint"), rl)},
		{"secretwithclientcredentials", secretwithclientcredentialsctl.NewReconciler(kc, factory, controllerRec("secretwithclientcredentials"), rl)},
		{"secretwithauthorizationcodegrant", secretwithauthorizationcodegrantctl.NewReconciler(kc, factory, controllerRec("secretwithauthorizationcodegrant"), rl)},
		{"secretwithbasicauthentication", secretwithbasicauthenticationctl.NewReconciler(kc, factory, controllerRec("secretwithbasicauthentication"), rl)},
		{"secretwithgenericstring", secretwithgenericstringctl.NewReconciler(kc, factory, controllerRec("secretwithgenericstring"), rl)},
		{"apiauthenticationintegrationwithclientcredentials", apiauthccctl.NewReconciler(kc, factory, controllerRec("apiauthenticationintegrationwithclientcredentials"), rl)},
		{"apiauthenticationintegrationwithauthorizationcodegrant", apiauthacgctl.NewReconciler(kc, factory, controllerRec("apiauthenticationintegrationwithauthorizationcodegrant"), rl)},
		{"apiauthenticationintegrationwithjwtbearer", apiauthjwtctl.NewReconciler(kc, factory, controllerRec("apiauthenticationintegrationwithjwtbearer"), rl)},
		{"share", sharectl.NewReconciler(kc, factory, controllerRec("share"), rl)},
		{"externalfunction", externalfunctionctl.NewReconciler(kc, factory, controllerRec("externalfunction"), rl)},
		{"computepool", computepoolctl.NewReconciler(kc, factory, controllerRec("computepool"), rl)},
		{"service", servicectl.NewReconciler(kc, factory, controllerRec("service"), rl)},
		{"imagerepository", imagerepositoryctl.NewReconciler(kc, factory, controllerRec("imagerepository"), rl)},
		{"gitrepository", gitrepositoryctl.NewReconciler(kc, factory, controllerRec("gitrepository"), rl)},
		{"streamlit", streamlitctl.NewReconciler(kc, factory, controllerRec("streamlit"), rl)},
	}

	// Validate --disable-controllers against the registration table (single
	// source of truth — no hand-maintained map to keep in sync).
	controllerNames := make([]string, 0, len(controllers)+1)
	for _, c := range controllers {
		controllerNames = append(controllerNames, c.name)
	}

	controllerNames = append(controllerNames, "fieldexport")  // registered standalone below
	controllerNames = append(controllerNames, "sqlstatement") // registered standalone below

	if err := validateDisabledControllers(disabled, controllerNames); err != nil {
		setupLog.Error(err, "invalid --disable-controllers flag")
		os.Exit(1)
	}

	for _, entry := range controllers {
		entryCfg := cfg
		entryCfg.Disabled = disabled[entry.name]

		if err := entry.ctrl.Setup(entryCfg); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", entry.name)
			os.Exit(1)
		}
	}

	// Set up FieldExport reconciler (standalone — does not use GenericReconciler).
	if !disabled["fieldexport"] && enableAlphaResources {
		if err := fieldexportctl.NewReconciler(
			kc,
			sanitize.NewSafeRecorderFromEvents(mgr.GetEventRecorder("fieldexport-controller")),
		).SetupWithManager(mgr, maxConcurrentReconciles); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "FieldExport")
			os.Exit(1)
		}
	}

	// Set up SQLStatement reconciler (standalone — gated behind --enable-sql-statement
	// due to the inherent security risks of executing arbitrary SQL).
	if !disabled["sqlstatement"] && enableSQLStatement {
		sqlstmtCfg := cfg
		sqlstmtCfg.Disabled = false // already gated by enableSQLStatement flag

		denylist, err := sqlstatementctl.ParseStatementDenylist(sqlStatementDenylist)
		if err != nil {
			setupLog.Error(err, "invalid --sql-statement-denylist flag")
			os.Exit(1)
		}

		if !denylist.IsEmpty() {
			setupLog.Info("SQLStatement denylist configured", "patterns", denylist.Len())
		}

		if err := sqlstatementctl.NewReconciler(kc, factory, controllerRec("sqlstatement"), rl, denylist).Setup(sqlstmtCfg); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "SQLStatement")
			os.Exit(1)
		}

		setupLog.Info("SQLStatement escape-hatch controller enabled")
	}

	// Register the ownership-conflict validating webhook.
	if enableWebhook {
		mgr.GetWebhookServer().Register("/validate-ownership",
			&admission.Webhook{Handler: &webhook.OwnershipValidator{Client: kc}},
		)
		setupLog.Info("ownership webhook registered", "port", webhookPort)
	}

	// Health checks.
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", factory.CheckHealth); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
