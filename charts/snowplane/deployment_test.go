package snowplane_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helmTemplateShow renders a single Helm template file and returns the YAML output.
func helmTemplateShow(t *testing.T, templateFile string, setFlags ...string) string {
	t.Helper()

	root := repoRoot(t)
	chartDir := root + "/charts/snowplane"

	args := make([]string, 0, 5+2*len(setFlags))
	args = append(args,
		"template", "snowplane", chartDir,
		"--show-only", "templates/"+templateFile,
	)

	for _, f := range setFlags {
		args = append(args, "--set", f)
	}

	cmd := exec.Command("helm", args...)

	out, err := cmd.CombinedOutput()
	output := string(out)

	if err != nil && strings.Contains(output, "could not find template") {
		return ""
	}

	require.NoError(t, err, "helm template failed: %s", output)

	return output
}

// --------------------------------------------------------------------------
// Deployment tests
// --------------------------------------------------------------------------

func TestDeployment_DefaultArgs(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml")
	require.Contains(t, out, "kind: Deployment")

	// Controller flags with defaults
	assert.Contains(t, out, "--max-concurrent-reconciles=3")
	assert.Contains(t, out, "--requeue-interval=5m")
	assert.Contains(t, out, "--snowflake-op-timeout=60s")
	assert.Contains(t, out, "--rate-limit-qps=10")
	assert.Contains(t, out, "--rate-limit-burst=20")
	assert.Contains(t, out, "--account-rate-limit-qps=50")
	assert.Contains(t, out, "--account-rate-limit-burst=100")
	assert.Contains(t, out, "--circuit-breaker-threshold=5")
	assert.Contains(t, out, "--circuit-breaker-reset-timeout=60s")
	assert.Contains(t, out, "--zap-log-level=info")
}

func TestDeployment_CustomControllerFlags(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml",
		"controller.maxConcurrentReconciles=10",
		"controller.requeueInterval=10m",
		"controller.snowflakeOpTimeout=120s",
		"controller.allowedRoles=SYSADMIN\\,DBA",
		"controller.disableControllers=stage\\,view",
	)
	assert.Contains(t, out, "--max-concurrent-reconciles=10")
	assert.Contains(t, out, "--requeue-interval=10m")
	assert.Contains(t, out, "--snowflake-op-timeout=120s")
	assert.Contains(t, out, "--allowed-roles=SYSADMIN,DBA")
	assert.Contains(t, out, "--disable-controllers=stage,view")
}

func TestDeployment_RateLimitFlags(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml",
		"rateLimit.qps=25",
		"rateLimit.burst=50",
		"rateLimit.accountQps=100",
		"rateLimit.accountBurst=200",
	)
	assert.Contains(t, out, "--rate-limit-qps=25")
	assert.Contains(t, out, "--rate-limit-burst=50")
	assert.Contains(t, out, "--account-rate-limit-qps=100")
	assert.Contains(t, out, "--account-rate-limit-burst=200")
}

func TestDeployment_LeaderElectionEnabled(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml")
	assert.Contains(t, out, "--leader-elect")
	assert.Contains(t, out, "--leader-election-id=")
}

func TestDeployment_LeaderElectionDisabled(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml", "leaderElection.enabled=false")
	assert.NotContains(t, out, "--leader-elect")
}

func TestDeployment_AlphaResourcesDisabled(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml", "controller.enableAlphaResources=false")
	assert.Contains(t, out, "--enable-alpha-resources=false")
}

func TestDeployment_DevelopmentLogging(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml", "logging.development=true")
	assert.Contains(t, out, "--development")

	outDefault := helmTemplateShow(t, "deployment.yaml")
	assert.NotContains(t, outDefault, "--development")
}

func TestDeployment_WatchNamespaces(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml", "watchNamespaces=default\\,team-a")
	assert.Contains(t, out, "--watch-namespaces=default,team-a")

	outDefault := helmTemplateShow(t, "deployment.yaml")
	assert.NotContains(t, outDefault, "--watch-namespaces")
}

func TestDeployment_Ports(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml")
	assert.Contains(t, out, "containerPort: 8080")
	assert.Contains(t, out, "containerPort: 8081")
	assert.Contains(t, out, "name: metrics")
	assert.Contains(t, out, "name: health")
}

func TestDeployment_CustomPorts(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml",
		"metrics.containerPort=9090",
		"healthProbes.containerPort=9091",
	)
	assert.Contains(t, out, "containerPort: 9090")
	assert.Contains(t, out, "containerPort: 9091")
}

func TestDeployment_HealthProbes(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml")
	assert.Contains(t, out, "/healthz")
	assert.Contains(t, out, "/readyz")
	assert.Contains(t, out, "livenessProbe:")
	assert.Contains(t, out, "readinessProbe:")
	assert.Contains(t, out, "startupProbe:")
}

func TestDeployment_SecurityContext(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml")
	assert.Contains(t, out, "runAsNonRoot: true")
	assert.Contains(t, out, "readOnlyRootFilesystem: true")
	assert.Contains(t, out, "allowPrivilegeEscalation: false")
}

func TestDeployment_Replicas(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml", "replicaCount=3")
	assert.Contains(t, out, "replicas: 3")
}

func TestDeployment_PriorityClassName(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml", "priorityClassName=system-cluster-critical")
	assert.Contains(t, out, "priorityClassName:")
	assert.Contains(t, out, "system-cluster-critical")
}

func TestDeployment_WorkloadIdentity(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "deployment.yaml", "workloadIdentity.enabled=true")
	assert.Contains(t, out, "snowflake-token")
	assert.Contains(t, out, "serviceAccountToken")
	assert.Contains(t, out, "volumeMounts:")
	assert.Contains(t, out, "volumes:")

	outDefault := helmTemplateShow(t, "deployment.yaml")
	assert.NotContains(t, outDefault, "snowflake-token")
}

// --------------------------------------------------------------------------
// ServiceAccount tests
// --------------------------------------------------------------------------

func TestServiceAccount_CreatedByDefault(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "serviceaccount.yaml")
	require.NotEmpty(t, out)
	assert.Contains(t, out, "kind: ServiceAccount")
}

func TestServiceAccount_Disabled(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "serviceaccount.yaml", "serviceAccount.create=false")
	assert.Empty(t, out)
}

func TestServiceAccount_CustomAnnotations(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "serviceaccount.yaml",
		"serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn=arn:aws:iam::123:role/test",
	)
	assert.Contains(t, out, "eks.amazonaws.com/role-arn")
}

// --------------------------------------------------------------------------
// Service (metrics) tests
// --------------------------------------------------------------------------

func TestServiceMetrics_EnabledByDefault(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "service-metrics.yaml")
	require.NotEmpty(t, out)
	assert.Contains(t, out, "kind: Service")
	assert.Contains(t, out, "port: 8080")
	assert.Contains(t, out, "component: metrics")
}

func TestServiceMetrics_Disabled(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "service-metrics.yaml", "metrics.service.enabled=false")
	assert.Empty(t, out)
}

func TestServiceMetrics_CustomPort(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "service-metrics.yaml", "metrics.service.port=9090")
	assert.Contains(t, out, "port: 9090")
}

// --------------------------------------------------------------------------
// PodDisruptionBudget tests
// --------------------------------------------------------------------------

func TestPDB_EnabledByDefault(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "pdb.yaml")
	require.NotEmpty(t, out)
	assert.Contains(t, out, "kind: PodDisruptionBudget")
	assert.Contains(t, out, "maxUnavailable: 1")
}

func TestPDB_Disabled(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "pdb.yaml", "podDisruptionBudget.enabled=false")
	assert.Empty(t, out)
}

// --------------------------------------------------------------------------
// ServiceMonitor tests
// --------------------------------------------------------------------------

func TestServiceMonitor_DisabledByDefault(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "servicemonitor.yaml")
	assert.Empty(t, out)
}

func TestServiceMonitor_Enabled(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "servicemonitor.yaml", "metrics.serviceMonitor.enabled=true")
	require.NotEmpty(t, out)
	assert.Contains(t, out, "kind: ServiceMonitor")
	assert.Contains(t, out, "interval: 30s")
	assert.Contains(t, out, "path: /metrics")
}

func TestServiceMonitor_CustomInterval(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "servicemonitor.yaml",
		"metrics.serviceMonitor.enabled=true",
		"metrics.serviceMonitor.interval=60s",
	)
	assert.Contains(t, out, "interval: 60s")
}

func TestServiceMonitor_AdditionalLabels(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "servicemonitor.yaml",
		"metrics.serviceMonitor.enabled=true",
		"metrics.serviceMonitor.additionalLabels.release=prometheus",
	)
	assert.Contains(t, out, "release: prometheus")
}

// --------------------------------------------------------------------------
// Grafana dashboard tests
// --------------------------------------------------------------------------

func TestGrafanaDashboard_DisabledByDefault(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "grafana-dashboard.yaml")
	assert.Empty(t, out, "grafana dashboard should not render when disabled (default)")
}

func TestGrafanaDashboard_EnabledRendersConfigMap(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "grafana-dashboard.yaml",
		"grafana.dashboard.enabled=true",
	)
	assert.Contains(t, out, "kind: ConfigMap")
	assert.Contains(t, out, "grafana-dashboard")
	assert.Contains(t, out, "grafana_dashboard: \"1\"")
	assert.Contains(t, out, "snowplane-dashboard.json")
}

func TestGrafanaDashboard_CustomNamespace(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "grafana-dashboard.yaml",
		"grafana.dashboard.enabled=true",
		"grafana.dashboard.namespace=monitoring",
	)
	assert.Contains(t, out, "namespace: monitoring")
}

func TestGrafanaDashboard_CustomLabels(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "grafana-dashboard.yaml",
		"grafana.dashboard.enabled=true",
		"grafana.dashboard.labels.team=platform",
	)
	assert.Contains(t, out, "team: platform")
}

func TestGrafanaDashboard_CustomAnnotations(t *testing.T) {
	t.Parallel()

	out := helmTemplateShow(t, "grafana-dashboard.yaml",
		"grafana.dashboard.enabled=true",
		"grafana.dashboard.annotations.source=snowplane",
	)
	assert.Contains(t, out, "source: snowplane")
}
