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
	database "github.com/hupe1980/snowplane/internal/controller/database"
	databaserolectl "github.com/hupe1980/snowplane/internal/controller/databaserole"
	dynamictablectl "github.com/hupe1980/snowplane/internal/controller/dynamictable"
	fieldexportctl "github.com/hupe1980/snowplane/internal/controller/fieldexport"
	fileformatctl "github.com/hupe1980/snowplane/internal/controller/fileformat"
	grantctl "github.com/hupe1980/snowplane/internal/controller/grant"
	grantownershipctl "github.com/hupe1980/snowplane/internal/controller/grantownership"
	maskingpolicyctl "github.com/hupe1980/snowplane/internal/controller/maskingpolicy"
	networkpolicyctl "github.com/hupe1980/snowplane/internal/controller/networkpolicy"
	networkrulectl "github.com/hupe1980/snowplane/internal/controller/networkrule"
	passwordpolicyctl "github.com/hupe1980/snowplane/internal/controller/passwordpolicy"
	pipectl "github.com/hupe1980/snowplane/internal/controller/pipe"
	providerconfig "github.com/hupe1980/snowplane/internal/controller/providerconfig"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	resourcemonitorctl "github.com/hupe1980/snowplane/internal/controller/resourcemonitor"
	rowaccesspolicyctl "github.com/hupe1980/snowplane/internal/controller/rowaccesspolicy"
	schemactl "github.com/hupe1980/snowplane/internal/controller/schema"
	securityintegrationctl "github.com/hupe1980/snowplane/internal/controller/securityintegration"
	stagectl "github.com/hupe1980/snowplane/internal/controller/stage"
	storageintegrationctl "github.com/hupe1980/snowplane/internal/controller/storageintegration"
	streamctl "github.com/hupe1980/snowplane/internal/controller/stream"
	tablectl "github.com/hupe1980/snowplane/internal/controller/table"
	tagctl "github.com/hupe1980/snowplane/internal/controller/tag"
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
	"database":            true,
	"schema":              true,
	"warehouse":           true,
	"accountrole":         true,
	"databaserole":        true,
	"accountrolegrant":    true,
	"databaserolegrant":   true,
	"sharegrant":          true,
	"user":                true,
	"table":               true,
	"view":                true,
	"stage":               true,
	"task":                true,
	"stream":              true,
	"tag":                 true,
	"networkpolicy":       true,
	"resourcemonitor":     true,
	"maskingpolicy":       true,
	"rowaccesspolicy":     true,
	"grantownership":      true,
	"fieldexport":         true,
	"storageintegration":  true,
	"fileformat":          true,
	"pipe":                true,
	"dynamictable":        true,
	"securityintegration": true,
	"passwordpolicy":      true,
	"networkrule":         true,
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

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var leaderElectionID string
	var developmentMode bool
	var maxConcurrentReconciles int
	var rateLimitQPS float64
	var rateLimitBurst int
	var requeueInterval time.Duration
	var enableAlphaResources bool
	var disableControllers string
	var watchNamespaces string
	var cbFailureThreshold int
	var cbResetTimeout time.Duration

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
		"Maximum sustained queries per second to Snowflake per provider (0 = disabled).")
	flag.IntVar(&rateLimitBurst, "rate-limit-burst", 20,
		"Maximum burst size for Snowflake API rate limiter.")
	flag.DurationVar(&requeueInterval, "requeue-interval", 5*time.Minute,
		"How often each reconciler re-observes Snowflake state for drift detection.")
	flag.BoolVar(&enableAlphaResources, "enable-alpha-resources", true,
		"Enable controllers for alpha-maturity CRDs. Set to false to skip alpha resources.")
	flag.StringVar(&disableControllers, "disable-controllers", "",
		"Comma-separated list of controller names to disable (e.g. \"accountrolegrant,stage,view\"). "+
			"Valid names: database, schema, warehouse, accountrole, databaserole, accountrolegrant, databaserolegrant, sharegrant, user, table, view, stage, task, stream, tag, networkpolicy, resourcemonitor, maskingpolicy, rowaccesspolicy, grantownership, fieldexport, storageintegration, fileformat, pipe, dynamictable, securityintegration, passwordpolicy, networkrule.")
	flag.StringVar(&watchNamespaces, "watch-namespaces", "",
		"Comma-separated list of namespaces to watch. If empty, all namespaces are watched.")
	flag.StringVar(&leaderElectionID, "leader-election-id", "snowplane-leader-election",
		"The name of the resource used for leader election.")
	flag.IntVar(&cbFailureThreshold, "circuit-breaker-threshold", 5,
		"Number of consecutive Snowflake API failures before the circuit breaker opens (per-provider).")
	flag.DurationVar(&cbResetTimeout, "circuit-breaker-reset-timeout", 60*time.Second,
		"How long the circuit breaker stays open before allowing a probe request.")

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

	// Create shared rate limiter.
	rl := ratelimit.New(ratelimit.Options{
		QPS:   rateLimitQPS,
		Burst: rateLimitBurst,
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

	kc := mgr.GetClient()

	// Set up ProviderConfig reconciler.
	if err := providerconfig.NewReconciler(
		kc,
		factory,
		sanitize.NewSafeRecorderFromEvents(mgr.GetEventRecorder("providerconfig-controller")),
		rl,
	).WithRequeueInterval(requeueInterval).SetupWithManager(mgr, maxConcurrentReconciles); err != nil {
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
		{"stream", streamctl.NewReconciler(kc, factory, controllerRec("stream"), rl)},
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
		{"securityintegration", securityintegrationctl.NewReconciler(kc, factory, controllerRec("securityintegration"), rl)},
		{"passwordpolicy", passwordpolicyctl.NewReconciler(kc, factory, controllerRec("passwordpolicy"), rl)},
		{"networkrule", networkrulectl.NewReconciler(kc, factory, controllerRec("networkrule"), rl)},
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
