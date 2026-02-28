package snowplane_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func helmTemplate(t *testing.T, setFlags ...string) string {
	t.Helper()

	root := repoRoot(t)
	chartDir := root + "/charts/snowplane"

	args := []string{
		"template", "snowplane", chartDir,
		"--show-only", "templates/networkpolicy.yaml",
	}

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

func TestNetworkPolicy_EnabledByDefault(t *testing.T) {
	out := helmTemplate(t)
	require.NotEmpty(t, out, "NetworkPolicy should render with default values")
	assert.Contains(t, out, "kind: NetworkPolicy")
	assert.Contains(t, out, "app.kubernetes.io/component: controller-manager")
}

func TestNetworkPolicy_Disabled(t *testing.T) {
	out := helmTemplate(t, "networkPolicy.enabled=false")
	assert.Empty(t, out, "NetworkPolicy should not render when disabled")
}

func TestNetworkPolicy_DefaultPorts(t *testing.T) {
	out := helmTemplate(t)
	assert.Contains(t, out, "port: 8080")
	assert.Contains(t, out, "port: 8081")
	assert.Contains(t, out, "port: 53")
	assert.Contains(t, out, "port: 443")
	assert.Contains(t, out, "port: 6443")
}

func TestNetworkPolicy_CustomPorts(t *testing.T) {
	out := helmTemplate(t,
		"metrics.containerPort=9090",
		"healthProbes.containerPort=9091",
	)
	assert.Contains(t, out, "port: 9090")
	assert.Contains(t, out, "port: 9091")
	assert.NotContains(t, out, "port: 8080")
	assert.NotContains(t, out, "port: 8081")
}

func TestNetworkPolicy_DNSRestrictedByDefault(t *testing.T) {
	out := helmTemplate(t)
	assert.Contains(t, out, "kubernetes.io/metadata.name: kube-system",
		"DNS should be restricted to kube-system by default")
}

func TestNetworkPolicy_DNSUnrestricted(t *testing.T) {
	out := helmTemplate(t, "networkPolicy.restrictDNS=false")
	assert.Contains(t, out, "port: 53")
	assert.NotContains(t, out, "kubernetes.io/metadata.name: kube-system")
}

func TestNetworkPolicy_EgressCIDRs(t *testing.T) {
	out := helmTemplate(t,
		"networkPolicy.egressCIDRs[0]=52.25.0.0/16",
		"networkPolicy.egressCIDRs[1]=10.96.0.1/32",
	)
	assert.Contains(t, out, "cidr: 52.25.0.0/16")
	assert.Contains(t, out, "cidr: 10.96.0.1/32")
	assert.Contains(t, out, "ipBlock:")
}

func TestNetworkPolicy_NoEgressCIDRs_NoIPBlock(t *testing.T) {
	out := helmTemplate(t)
	assert.NotContains(t, out, "ipBlock:")
	assert.NotContains(t, out, "cidr:")
}

func TestNetworkPolicy_MetricsNamespace(t *testing.T) {
	out := helmTemplate(t, "networkPolicy.metricsNamespace=monitoring")
	assert.Contains(t, out, "kubernetes.io/metadata.name: monitoring",
		"ingress should be restricted to the monitoring namespace")
}

func TestNetworkPolicy_NoMetricsNamespace_NoFromSelector(t *testing.T) {
	out := helmTemplate(t)
	assert.NotContains(t, out, "kubernetes.io/metadata.name: monitoring")
	count := strings.Count(out, "namespaceSelector:")
	assert.Equal(t, 1, count, "only DNS namespaceSelector expected in default mode")
}

func TestNetworkPolicy_PolicyTypes(t *testing.T) {
	out := helmTemplate(t)
	assert.Contains(t, out, "- Ingress")
	assert.Contains(t, out, "- Egress")
}

func TestNetworkPolicy_AllFeaturesCombined(t *testing.T) {
	out := helmTemplate(t,
		"networkPolicy.egressCIDRs[0]=10.0.0.0/8",
		"networkPolicy.metricsNamespace=obs",
		"networkPolicy.restrictDNS=true",
		"metrics.containerPort=9090",
	)
	assert.Contains(t, out, "kubernetes.io/metadata.name: obs")
	assert.Contains(t, out, "cidr: 10.0.0.0/8")
	assert.Contains(t, out, "kubernetes.io/metadata.name: kube-system")
	assert.Contains(t, out, "port: 9090")
}
