// Package main is the entrypoint for the Snowplane controller manager.
package main

import (
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

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/circuitbreaker"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	accountrolectl "github.com/hupe1980/snowplane/internal/controller/accountrole"
	alertctl "github.com/hupe1980/snowplane/internal/controller/alert"
	apiauthacgctl "github.com/hupe1980/snowplane/internal/controller/apiauthenticationintegrationwithauthorizationcodegrant"
	apiauthccctl "github.com/hupe1980/snowplane/internal/controller/apiauthenticationintegrationwithclientcredentials"
	apiauthjwtctl "github.com/hupe1980/snowplane/internal/controller/apiauthenticationintegrationwithjwtbearer"
	authenticationpolicyctl "github.com/hupe1980/snowplane/internal/controller/authenticationpolicy"
	database "github.com/hupe1980/snowplane/internal/controller/database"
	databaserolectl "github.com/hupe1980/snowplane/internal/controller/databaserole"
	dynamictablectl "github.com/hupe1980/snowplane/internal/controller/dynamictable"
	externaltablectl "github.com/hupe1980/snowplane/internal/controller/externaltable"
	fieldexportctl "github.com/hupe1980/snowplane/internal/controller/fieldexport"
	fileformatctl "github.com/hupe1980/snowplane/internal/controller/fileformat"
	functionjavactl "github.com/hupe1980/snowplane/internal/controller/functionjava"
	functionjavascriptctl "github.com/hupe1980/snowplane/internal/controller/functionjavascript"
	functionpythonctl "github.com/hupe1980/snowplane/internal/controller/functionpython"
	functionscalactl "github.com/hupe1980/snowplane/internal/controller/functionscala"
	functionsqlctl "github.com/hupe1980/snowplane/internal/controller/functionsql"
	grantctl "github.com/hupe1980/snowplane/internal/controller/grant"
	grantownershipctl "github.com/hupe1980/snowplane/internal/controller/grantownership"
	maskingpolicyctl "github.com/hupe1980/snowplane/internal/controller/maskingpolicy"
	maskingpolicyapplicationctl "github.com/hupe1980/snowplane/internal/controller/maskingpolicyapplication"
	materializedviewctl "github.com/hupe1980/snowplane/internal/controller/materializedview"
	networkpolicyctl "github.com/hupe1980/snowplane/internal/controller/networkpolicy"
	networkpolicyattachmentctl "github.com/hupe1980/snowplane/internal/controller/networkpolicyattachment"
	networkrulectl "github.com/hupe1980/snowplane/internal/controller/networkrule"
	notificationintegrationctl "github.com/hupe1980/snowplane/internal/controller/notificationintegration"
	passwordpolicyctl "github.com/hupe1980/snowplane/internal/controller/passwordpolicy"
	passwordpolicyattachmentctl "github.com/hupe1980/snowplane/internal/controller/passwordpolicyattachment"
	pipectl "github.com/hupe1980/snowplane/internal/controller/pipe"
	procedurejavactl "github.com/hupe1980/snowplane/internal/controller/procedurejava"
	procedurejavascriptctl "github.com/hupe1980/snowplane/internal/controller/procedurejavascript"
	procedurepythonctl "github.com/hupe1980/snowplane/internal/controller/procedurepython"
	procedurescalactl "github.com/hupe1980/snowplane/internal/controller/procedurescala"
	proceduresqlctl "github.com/hupe1980/snowplane/internal/controller/proceduresql"
	providerconfig "github.com/hupe1980/snowplane/internal/controller/providerconfig"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	resourcemonitorctl "github.com/hupe1980/snowplane/internal/controller/resourcemonitor"
	roleassignmentctl "github.com/hupe1980/snowplane/internal/controller/roleassignment"
	rowaccesspolicyctl "github.com/hupe1980/snowplane/internal/controller/rowaccesspolicy"
	schemactl "github.com/hupe1980/snowplane/internal/controller/schema"
	secretwithauthorizationcodegrantctl "github.com/hupe1980/snowplane/internal/controller/secretwithauthorizationcodegrant"
	secretwithbasicauthenticationctl "github.com/hupe1980/snowplane/internal/controller/secretwithbasicauthentication"
	secretwithclientcredentialsctl "github.com/hupe1980/snowplane/internal/controller/secretwithclientcredentials"
	secretwithgenericstringctl "github.com/hupe1980/snowplane/internal/controller/secretwithgenericstring"
	securityintegrationctl "github.com/hupe1980/snowplane/internal/controller/securityintegration"
	sequencectl "github.com/hupe1980/snowplane/internal/controller/sequence"
	stagectl "github.com/hupe1980/snowplane/internal/controller/stage"
	storageintegrationctl "github.com/hupe1980/snowplane/internal/controller/storageintegration"
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
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/utils/sanitize"

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

// validControllerNames is the set of controller names accepted by --disable-controllers.
var validControllerNames = map[string]bool{
	"alert":                            true,
	"authenticationpolicy":             true,
	"database":                         true,
	"schema":                           true,
	"warehouse":                        true,
	"accountrole":                      true,
	"databaserole":                     true,
	"accountrolegrant":                 true,
	"databaserolegrant":                true,
	"sharegrant":                       true,
	"user":                             true,
	"table":                            true,
	"view":                             true,
	"stage":                            true,
	"task":                             true,
	"streamontable":                    true,
	"streamonview":                     true,
	"streamonexternaltable":            true,
	"streamondirectorytable":           true,
	"streamondynamictable":             true,
	"tag":                              true,
	"networkpolicy":                    true,
	"resourcemonitor":                  true,
	"maskingpolicy":                    true,
	"rowaccesspolicy":                  true,
	"grantownership":                   true,
	"fieldexport":                      true,
	"storageintegration":               true,
	"fileformat":                       true,
	"pipe":                             true,
	"dynamictable":                     true,
	"notificationintegration":          true,
	"securityintegration":              true,
	"passwordpolicy":                   true,
	"networkrule":                      true,
	"accountroleassignment":            true,
	"databaseroleassignment":           true,
	"tagassociation":                   true,
	"networkpolicyattachment":          true,
	"passwordpolicyattachment":         true,
	"maskingpolicyapplication":         true,
	"sequence":                         true,
	"externaltable":                    true,
	"materializedview":                 true,
	"proceduresql":                     true,
	"procedurejavascript":              true,
	"procedurepython":                  true,
	"procedurejava":                    true,
	"procedurescala":                   true,
	"functionsql":                      true,
	"functionjavascript":               true,
	"functionpython":                   true,
	"functionjava":                     true,
	"functionscala":                    true,
	"tableconstraint":                  true,
	"secretwithclientcredentials":      true,
	"secretwithauthorizationcodegrant": true,
	"secretwithbasicauthentication":    true,
	"secretwithgenericstring":          true,
	"apiauthenticationintegrationwithclientcredentials":      true,
	"apiauthenticationintegrationwithauthorizationcodegrant": true,
	"apiauthenticationintegrationwithjwtbearer":              true,
}

// parseDisabledControllers parses a comma-separated list of controller names
// into a set for O(1) lookup. It returns an error if any name is not recognized.
func parseDisabledControllers(s string) (map[string]bool, error) {
	disabled := make(map[string]bool)

	if s == "" {
		return disabled, nil
	}

	for _, name := range strings.Split(s, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			if !validControllerNames[name] {
				valid := make([]string, 0, len(validControllerNames))
				for k := range validControllerNames {
					valid = append(valid, k)
				}
				return nil, fmt.Errorf("unknown controller name %q in --disable-controllers; valid names: %s", name, strings.Join(valid, ", "))
			}
			disabled[name] = true
		}
	}

	return disabled, nil
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
		"Comma-separated list of controller names to disable (e.g. \"accountrolegrant,stage,view\"). "+
			"Valid names: alert, database, schema, warehouse, accountrole, databaserole, accountrolegrant, databaserolegrant, sharegrant, user, table, view, stage, task, streamontable, streamonview, streamonexternaltable, streamondirectorytable, streamondynamictable, tag, networkpolicy, resourcemonitor, maskingpolicy, rowaccesspolicy, grantownership, fieldexport, storageintegration, fileformat, pipe, dynamictable, securityintegration, passwordpolicy, networkrule, accountroleassignment, databaseroleassignment.")
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

	opts := zap.Options{Development: developmentMode}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgrOpts := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       leaderElectionID,
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

	// Parse per-controller disable set.
	disabled, err := parseDisabledControllers(disableControllers)
	if err != nil {
		setupLog.Error(err, "invalid --disable-controllers flag")
		os.Exit(1)
	}

	// Parse role allowlist.
	allowedRolesSet := parseAllowedRoles(allowedRoles)

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
		{"accountrolegrant", grantctl.NewAccountRoleGrantReconciler(kc, factory, controllerRec("accountrolegrant"), rl)},
		{"databaserolegrant", grantctl.NewDatabaseRoleGrantReconciler(kc, factory, controllerRec("databaserolegrant"), rl)},
		{"sharegrant", grantctl.NewShareGrantReconciler(kc, factory, controllerRec("sharegrant"), rl)},
		{"user", userctl.NewReconciler(kc, factory, controllerRec("user"), rl)},
		{"table", tablectl.NewReconciler(kc, factory, controllerRec("table"), rl)},
		{"view", viewctl.NewReconciler(kc, factory, controllerRec("view"), rl)},
		{"stage", stagectl.NewReconciler(kc, factory, controllerRec("stage"), rl)},
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
		{"storageintegration", storageintegrationctl.NewReconciler(kc, factory, controllerRec("storageintegration"), rl)},
		{"fileformat", fileformatctl.NewReconciler(kc, factory, controllerRec("fileformat"), rl)},
		{"pipe", pipectl.NewReconciler(kc, factory, controllerRec("pipe"), rl)},
		{"dynamictable", dynamictablectl.NewReconciler(kc, factory, controllerRec("dynamictable"), rl)},
		{"notificationintegration", notificationintegrationctl.NewReconciler(kc, factory, controllerRec("notificationintegration"), rl)},
		{"securityintegration", securityintegrationctl.NewReconciler(kc, factory, controllerRec("securityintegration"), rl)},
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
