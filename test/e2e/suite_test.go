//go:build e2e

// Package e2e contains end-to-end tests that deploy the Snowplane operator
// into a k3s testcontainer and test against a real Snowflake account.
//
// Prerequisites:
//
//	docker, helm (for chart deployment)
//	source .env  (SNOWFLAKE_* variables)
//	go test -tags e2e -v -timeout 15m -count=1 ./test/e2e/
package e2e

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/snowflakedb/gosnowflake"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/k3s"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultTimeout  = 3 * time.Minute
	defaultInterval = 2 * time.Second
	testNamespace   = "snowplane-system"
	providerName    = "default"
	sfPrefix        = "SNOWPLANE_E2E_"

	// k3s image to use for the testcontainer.
	k3sImage = "rancher/k3s:v1.31.6-k3s1"
	// Docker image name for the operator built during setup.
	operatorImage = "snowplane:e2e"
)

var (
	k8sClient     kubernetes.Interface
	dynamicClient dynamic.Interface
	sfDB          *sql.DB

	sfAccount    string
	sfUser       string
	sfPassword   string
	sfPrivateKey string
	sfRole       string
	sfWarehouse  string
)

var (
	gvrDatabase = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "databases",
	}
	gvrSchema = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "schemas",
	}
	gvrWarehouse = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "warehouses",
	}
	gvrUser = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "users",
	}
	gvrAccountRole = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "accountroles",
	}
	gvrDatabaseRole = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "databaseroles",
	}
	gvrTable = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "tables",
	}
	gvrView = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "views",
	}
	gvrStage = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "stages",
	}
	gvrFieldExport = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "fieldexports",
	}
	gvrGrantPrivilegesToAccountRole = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "grantprivilegestoaccountroles",
	}
	gvrGrantPrivilegesToDatabaseRole = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "grantprivilegestodatabaseroles",
	}
	gvrGrantPrivilegesToShare = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "grantprivilegestoshares",
	}
	gvrNetworkPolicy = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "networkpolicies",
	}
	gvrMaskingPolicy = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "maskingpolicies",
	}
	gvrPasswordPolicy = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "passwordpolicies",
	}
	gvrTag = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "tags",
	}
	gvrTask = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "tasks",
	}
	gvrStreamOnTable = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "streamontables",
	}
	gvrAPIIntegration = schema.GroupVersionResource{
		Group: "snowplane.hupe1980.github.io", Version: "v1alpha1", Resource: "apiintegrations",
	}
)

func TestMain(m *testing.M) {
	sfAccount = requireEnv("SNOWFLAKE_ACCOUNT")
	sfUser = requireEnv("SNOWFLAKE_USER")
	sfPassword = os.Getenv("SNOWFLAKE_PASSWORD")
	sfPrivateKey = os.Getenv("SNOWFLAKE_PRIVATE_KEY")
	sfRole = envOrDefault("SNOWFLAKE_ROLE", "SYSADMIN")
	sfWarehouse = os.Getenv("SNOWFLAKE_WAREHOUSE") // optional; many DDL ops work without one

	if sfPassword == "" && sfPrivateKey == "" {
		fmt.Fprintf(os.Stderr, "either SNOWFLAKE_PASSWORD or SNOWFLAKE_PRIVATE_KEY must be set\n")
		os.Exit(1)
	}

	sfConfig := &gosnowflake.Config{
		Account:   sfAccount,
		User:      sfUser,
		Role:      sfRole,
		Warehouse: sfWarehouse, // may be "" — fine for most DDL
	}

	switch {
	case sfPrivateKey != "":
		key, err := parsePrivateKeyPEM(sfPrivateKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse private key: %v\n", err)
			os.Exit(1)
		}
		sfConfig.Authenticator = gosnowflake.AuthTypeJwt
		sfConfig.PrivateKey = key
	case sfPassword != "":
		sfConfig.Password = sfPassword
	}

	dsn, err := gosnowflake.DSN(sfConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build DSN: %v\n", err)
		os.Exit(1)
	}

	sfDB, err = sql.Open("snowflake", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open Snowflake connection: %v\n", err)
		os.Exit(1)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer pingCancel()

	if err := sfDB.PingContext(pingCtx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to ping Snowflake: %v\n", err)
		os.Exit(1)
	}

	// — Locate repo root (for Dockerfile and Helm chart) —
	repoRoot := findRepoRoot()

	// — Build the operator Docker image —
	fmt.Println("==> Building operator Docker image")

	// Detect the host architecture so the binary matches the k3s container.
	arch := runtime.GOARCH
	if err := runCmd(repoRoot, "docker", "build",
		"--build-arg", "TARGETARCH="+arch,
		"--no-cache",
		"-t", operatorImage, "."); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build Docker image: %v\n", err)
		os.Exit(1)
	}

	// — Start k3s testcontainer —
	fmt.Println("==> Starting k3s testcontainer")
	ctx := context.Background()

	k3sContainer, err := k3s.Run(ctx, k3sImage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start k3s container: %v\n", err)
		os.Exit(1)
	}

	defer func() {
		fmt.Println("==> Terminating k3s testcontainer")
		if err := k3sContainer.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to terminate k3s container: %v\n", err)
		}
	}()

	// — Load the operator image into k3s —
	fmt.Println("==> Loading operator image into k3s")
	if err := k3sContainer.LoadImages(ctx, operatorImage); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load image into k3s: %v\n", err)
		os.Exit(1)
	}

	// — Extract kubeconfig and write to temp file (for helm + k8s clients) —
	kubeConfigBytes, err := k3sContainer.GetKubeConfig(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get kubeconfig from k3s: %v\n", err)
		os.Exit(1)
	}

	kubeconfigFile, err := os.CreateTemp("", "snowplane-e2e-kubeconfig-*.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp kubeconfig: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(kubeconfigFile.Name())

	if _, err := kubeconfigFile.Write(kubeConfigBytes); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write kubeconfig: %v\n", err)
		os.Exit(1)
	}
	if err := kubeconfigFile.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to close kubeconfig file: %v\n", err)
		os.Exit(1)
	}

	kubeconfigPath := kubeconfigFile.Name()

	// — Build k8s clients from kubeconfig —
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build kubeconfig: %v\n", err)
		os.Exit(1)
	}

	k8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create kubernetes client: %v\n", err)
		os.Exit(1)
	}

	dynamicClient, err = dynamic.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create dynamic client: %v\n", err)
		os.Exit(1)
	}

	// — Wait for k3s to be fully ready —
	fmt.Println("==> Waiting for k3s cluster to be ready")
	if err := waitForClusterReady(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "k3s cluster not ready: %v\n", err)
		os.Exit(1)
	}

	// — Install CRDs via kubectl apply —
	fmt.Println("==> Installing CRDs")
	crdDir := filepath.Join(repoRoot, "config", "crd", "bases")
	if err := runCmd(repoRoot, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "--server-side", "--force-conflicts", "-f", crdDir); err != nil {
		fmt.Fprintf(os.Stderr, "failed to install CRDs: %v\n", err)
		os.Exit(1)
	}

	// — Deploy operator via Helm —
	fmt.Println("==> Deploying operator via Helm")
	chartDir := filepath.Join(repoRoot, "charts", "snowplane")
	if err := runCmd(repoRoot, "helm", "upgrade", "--install", "snowplane", chartDir,
		"--kubeconfig", kubeconfigPath,
		"--namespace", testNamespace, "--create-namespace",
		"--skip-crds",
		"--set", "image.repository=snowplane",
		"--set", "image.tag=e2e",
		"--set", "image.pullPolicy=Never",
		"--set", "leaderElection.enabled=false",
		"--set", "controller.requeueInterval=10s",
		"--set", "controller.enableAlphaResources=true",
		"--set", "rbac.secrets.write=true",
		"--set", "rbac.configMaps.write=true",
		"--set", "networkPolicy.enabled=false",
		"--wait", "--timeout", "120s",
	); err != nil {
		fmt.Fprintf(os.Stderr, "failed to deploy Helm chart: %v\n", err)
		os.Exit(1)
	}

	// — Wait for operator deployment to be ready —
	fmt.Println("==> Waiting for operator deployment")
	if err := runCmd(repoRoot, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", testNamespace, "rollout", "status", "deployment/snowplane", "--timeout=120s",
	); err != nil {
		fmt.Fprintf(os.Stderr, "operator deployment not ready: %v\n", err)
		os.Exit(1)
	}

	// — Create Snowflake credentials Secret + ProviderConfig —
	fmt.Println("==> Creating Snowflake credentials and ProviderConfig")
	if err := setupSnowflakeCredentials(ctx, kubeconfigPath, repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup credentials: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("==> Setup complete, running E2E tests")
	code := m.Run()

	// Dump controller logs for debugging on failure.
	if code != 0 {
		fmt.Println("==> Dumping controller pod logs (test failure)")
		if err := runCmd(repoRoot, "kubectl", "--kubeconfig", kubeconfigPath,
			"-n", testNamespace, "logs", "-l", "app.kubernetes.io/name=snowplane", "--tail=200"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to dump controller logs: %v\n", err)
		}
	}

	cleanupK8sCRs()
	cleanupSnowflake()

	if err := sfDB.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to close Snowflake connection: %v\n", err)
	}
	os.Exit(code)
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "required environment variable %s is not set\n", key)
		os.Exit(1)
	}
	return v
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ---------------------------------------------------------------------------
// k3s testcontainer helpers
// ---------------------------------------------------------------------------

// findRepoRoot walks up from the test directory to find the project root
// containing go.mod.
func findRepoRoot() string {
	dir, err := filepath.Abs(filepath.Join(".", "..", ".."))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve repo root: %v\n", err)
		os.Exit(1)
	}

	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		fmt.Fprintf(os.Stderr, "repo root %s does not contain go.mod: %v\n", dir, err)
		os.Exit(1)
	}

	return dir
}

// runCmd executes a command with stdout/stderr forwarded to the test process.
func runCmd(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("    + %s %s\n", name, strings.Join(args, " "))

	return cmd.Run()
}

// waitForClusterReady polls until at least one k3s node reports Ready.
func waitForClusterReady(ctx context.Context) error {
	deadline := time.After(2 * time.Minute)
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("timed out waiting for k3s nodes to be ready")
		case <-tick.C:
			nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			if err != nil {
				continue
			}

			for i := range nodes.Items {
				for _, cond := range nodes.Items[i].Status.Conditions {
					if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
						return nil
					}
				}
			}
		}
	}
}

// setupSnowflakeCredentials creates the Secret and ProviderConfig CR that the
// operator needs to connect to Snowflake.
func setupSnowflakeCredentials(ctx context.Context, kubeconfigPath, repoRoot string) error {
	// Determine authentication type.
	var authType, secretKey, secretValue string

	switch {
	case sfPrivateKey != "":
		authType = "KeyPair"
		secretKey = "privateKey"
		secretValue = sfPrivateKey
	default:
		authType = "UsernamePassword"
		secretKey = "password"
		secretValue = sfPassword
	}

	// Delete any existing secret (ignore errors).
	_ = runCmd(repoRoot, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", testNamespace, "delete", "secret", "snowflake-credentials", "--ignore-not-found")

	// L-5: Write credentials to a temporary file and use --from-file to avoid
	// leaking secrets in process listings (/proc/*/cmdline) and shell history.
	// This also correctly handles multiline PEM private keys.
	credFile, err := os.CreateTemp("", "snowplane-cred-*")
	if err != nil {
		return fmt.Errorf("creating temporary credential file: %w", err)
	}
	defer os.Remove(credFile.Name())

	if _, err := credFile.WriteString(secretValue); err != nil {
		credFile.Close()
		return fmt.Errorf("writing credential to temp file: %w", err)
	}

	if err := credFile.Close(); err != nil {
		return fmt.Errorf("closing temp credential file: %w", err)
	}

	// Create the credentials secret from file (credentials never appear in argv).
	if err := runCmd(repoRoot, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", testNamespace, "create", "secret", "generic", "snowflake-credentials",
		fmt.Sprintf("--from-file=%s=%s", secretKey, credFile.Name()),
	); err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	// Build ProviderConfig YAML.
	warehouseLine := ""
	if sfWarehouse != "" {
		warehouseLine = fmt.Sprintf("  warehouse: %q\n", sfWarehouse)
	}

	providerYAML := fmt.Sprintf(`apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
  namespace: %s
spec:
  account: %q
  user: %q
  role: %q
%s  authenticationType: %s
  credentials:
    secretRef:
      name: snowflake-credentials
      key: %s
`, testNamespace, sfAccount, sfUser, sfRole, warehouseLine, authType, secretKey)

	// Apply via kubectl (pipe YAML through stdin).
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(providerYAML)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("    + kubectl apply ProviderConfig (%s)\n", authType)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply ProviderConfig: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Snowflake SQL helpers
// ---------------------------------------------------------------------------

func sfExec(t *testing.T, query string) {
	t.Helper()
	execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := sfDB.ExecContext(execCtx, query)
	require.NoError(t, err, "Snowflake exec failed: %s", query)
}

func sfExists(t *testing.T, resourceType, name string) bool {
	t.Helper()
	qCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	query := fmt.Sprintf("SHOW %s LIKE '%s'", resourceType, sqlbuilder.EscapeLikePattern(name))
	rows, err := sfDB.QueryContext(qCtx, query)
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next()
}

func sfExistsInDB(t *testing.T, resourceType, dbName, name string) bool {
	t.Helper()
	qCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	query := fmt.Sprintf("SHOW %s LIKE '%s' IN DATABASE %s", resourceType, sqlbuilder.EscapeLikePattern(name), sqlbuilder.QuoteIdentifier(dbName))
	rows, err := sfDB.QueryContext(qCtx, query)
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next()
}

func sfExistsInSchema(t *testing.T, resourceType, dbName, schemaName, name string) bool {
	t.Helper()
	qCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	query := fmt.Sprintf("SHOW %s LIKE '%s' IN SCHEMA %s.%s", resourceType, sqlbuilder.EscapeLikePattern(name), sqlbuilder.QuoteIdentifier(dbName), sqlbuilder.QuoteIdentifier(schemaName))
	rows, err := sfDB.QueryContext(qCtx, query)
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next()
}

func sfGetComment(t *testing.T, resourceType, name string) string {
	t.Helper()
	qCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	query := fmt.Sprintf("SHOW %s LIKE '%s'", resourceType, sqlbuilder.EscapeLikePattern(name))
	rows, err := sfDB.QueryContext(qCtx, query)
	if err != nil {
		return ""
	}
	defer rows.Close()
	if !rows.Next() {
		return ""
	}
	cols, _ := rows.Columns()
	values := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return ""
	}
	for i, col := range cols {
		if strings.EqualFold(col, "comment") {
			if values[i] == nil {
				return ""
			}
			return fmt.Sprintf("%v", values[i])
		}
	}
	return ""
}

func sfGetSchemaComment(t *testing.T, dbName, schemaName string) string {
	t.Helper()
	qCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	query := fmt.Sprintf("SHOW SCHEMAS LIKE '%s' IN DATABASE %s", sqlbuilder.EscapeLikePattern(schemaName), sqlbuilder.QuoteIdentifier(dbName))
	rows, err := sfDB.QueryContext(qCtx, query)
	if err != nil {
		return ""
	}
	defer rows.Close()
	if !rows.Next() {
		return ""
	}
	cols, _ := rows.Columns()
	values := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return ""
	}
	for i, col := range cols {
		if strings.EqualFold(col, "comment") {
			if values[i] == nil {
				return ""
			}
			return fmt.Sprintf("%v", values[i])
		}
	}
	return ""
}

func sfDrop(t *testing.T, resourceType, name string) {
	t.Helper()
	dropCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = sfDB.ExecContext(dropCtx, fmt.Sprintf("DROP %s IF EXISTS %s", resourceType, sqlbuilder.QuoteIdentifier(name)))
}

func sfDropInSchema(t *testing.T, resourceType, dbName, schemaName, name string) {
	t.Helper()
	dropCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = sfDB.ExecContext(dropCtx, fmt.Sprintf("DROP %s IF EXISTS %s.%s.%s", resourceType, sqlbuilder.QuoteIdentifier(dbName), sqlbuilder.QuoteIdentifier(schemaName), sqlbuilder.QuoteIdentifier(name)))
}

// cleanupK8sCRs deletes all Snowplane CRs from the test namespace.
// This prevents orphaned CRs from prior test runs from continuing to
// reconcile (and generating Snowflake queries) after the tests finish.
func cleanupK8sCRs() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Delete in dependency order: children first, then parents.
	gvrs := []schema.GroupVersionResource{
		gvrFieldExport,
		gvrGrantPrivilegesToAccountRole,
		gvrGrantPrivilegesToDatabaseRole,
		gvrGrantPrivilegesToShare,
		gvrStreamOnTable,
		gvrTask,
		gvrMaskingPolicy,
		gvrPasswordPolicy,
		gvrTag,
		gvrNetworkPolicy,
		gvrAPIIntegration,
		gvrStage,
		gvrView,
		gvrTable,
		gvrSchema,
		gvrDatabaseRole,
		gvrAccountRole,
		gvrUser,
		gvrWarehouse,
		gvrDatabase,
	}

	for _, gvr := range gvrs {
		list, err := dynamicClient.Resource(gvr).Namespace(testNamespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to list %s: %v\n", gvr.Resource, err)
			continue
		}

		for i := range list.Items {
			name := list.Items[i].GetName()
			// Remove finalizers so deletion isn't blocked by a controller
			// that may be shutting down or already gone.
			item := &list.Items[i]
			if len(item.GetFinalizers()) > 0 {
				item.SetFinalizers(nil)
				_, _ = dynamicClient.Resource(gvr).Namespace(testNamespace).Update(ctx, item, metav1.UpdateOptions{})
			}

			_ = dynamicClient.Resource(gvr).Namespace(testNamespace).Delete(ctx, name, metav1.DeleteOptions{})
		}
	}
}

func cleanupSnowflake() {
	cleanCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	resourceTypes := []string{"STREAMS", "TASKS", "MASKING POLICIES", "PASSWORD POLICIES", "TAGS", "VIEWS", "TABLES", "STAGES", "SCHEMAS", "NETWORK POLICIES", "INTEGRATIONS", "DATABASES", "WAREHOUSES", "USERS", "ROLES"}
	for _, rt := range resourceTypes {
		rows, err := sfDB.QueryContext(cleanCtx, fmt.Sprintf("SHOW %s LIKE '%s%%'", rt, sqlbuilder.EscapeLikePattern(sfPrefix)))
		if err != nil {
			continue
		}
		cols, _ := rows.Columns()
		nameIdx := -1
		for i, c := range cols {
			if strings.EqualFold(c, "name") {
				nameIdx = i
				break
			}
		}
		if nameIdx < 0 {
			rows.Close()
			continue
		}
		var names []string
		for rows.Next() {
			vals := make([]interface{}, len(cols))
			valPtrs := make([]interface{}, len(cols))
			for i := range vals {
				valPtrs[i] = &vals[i]
			}
			if err := rows.Scan(valPtrs...); err == nil {
				if vals[nameIdx] != nil {
					names = append(names, fmt.Sprintf("%v", vals[nameIdx]))
				}
			}
		}
		rows.Close()
		singular := strings.TrimSuffix(rt, "S")
		if rt == "DATABASES" {
			singular = "DATABASE"
		}
		for _, n := range names {
			_, _ = sfDB.ExecContext(cleanCtx, fmt.Sprintf("DROP %s IF EXISTS \"%s\" CASCADE", singular, n))
		}
	}
}

// ---------------------------------------------------------------------------
// Kubernetes helpers
// ---------------------------------------------------------------------------

func createCR(t *testing.T, gvr schema.GroupVersionResource, obj *unstructured.Unstructured) func() {
	t.Helper()
	ns := obj.GetNamespace()
	if ns == "" {
		ns = testNamespace
		obj.SetNamespace(ns)
	}
	created, err := dynamicClient.Resource(gvr).Namespace(ns).Create(context.Background(), obj, metav1.CreateOptions{})
	require.NoError(t, err, "failed to create %s/%s", gvr.Resource, obj.GetName())
	return func() {
		_ = dynamicClient.Resource(gvr).Namespace(ns).Delete(context.Background(), created.GetName(), metav1.DeleteOptions{})
	}
}

func getCR(t *testing.T, gvr schema.GroupVersionResource, name string) *unstructured.Unstructured {
	t.Helper()
	obj, err := dynamicClient.Resource(gvr).Namespace(testNamespace).Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err, "failed to get %s/%s", gvr.Resource, name)
	return obj
}

func updateCR(t *testing.T, gvr schema.GroupVersionResource, name string, mutate func(*unstructured.Unstructured)) {
	t.Helper()
	obj := getCR(t, gvr, name)
	mutate(obj)
	_, err := dynamicClient.Resource(gvr).Namespace(testNamespace).Update(context.Background(), obj, metav1.UpdateOptions{})
	require.NoError(t, err, "failed to update %s/%s", gvr.Resource, name)
}

func deleteCR(t *testing.T, gvr schema.GroupVersionResource, name string) {
	t.Helper()
	err := dynamicClient.Resource(gvr).Namespace(testNamespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return
	}
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, getErr := dynamicClient.Resource(gvr).Namespace(testNamespace).Get(context.Background(), name, metav1.GetOptions{})
		return apierrors.IsNotFound(getErr)
	}, defaultTimeout, defaultInterval, "CR %s/%s was not deleted in time", gvr.Resource, name)
}

func waitForReady(t *testing.T, gvr schema.GroupVersionResource, name string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return isReady(gvr, name)
	}, defaultTimeout, defaultInterval, "%s/%s did not become Ready\n%s", gvr.Resource, name, dumpConditions(gvr, name))
}

// dumpConditions returns a formatted string of all status conditions for diagnostics.
func dumpConditions(gvr schema.GroupVersionResource, name string) string {
	obj, err := dynamicClient.Resource(gvr).Namespace(testNamespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Sprintf("  (could not fetch CR: %v)", err)
	}
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found || len(conditions) == 0 {
		return "  (no status conditions found)"
	}
	var lines []string
	for _, c := range conditions {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("  type=%v status=%v reason=%v message=%v", cm["type"], cm["status"], cm["reason"], cm["message"]))
	}
	return strings.Join(lines, "\n")
}

func isReady(gvr schema.GroupVersionResource, name string) bool {
	obj, err := dynamicClient.Resource(gvr).Namespace(testNamespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return false
	}
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, c := range conditions {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cm["type"] == "Ready" && cm["status"] == "True" {
			return true
		}
	}
	return false
}

func getStatusField(t *testing.T, gvr schema.GroupVersionResource, name string, fields ...string) interface{} {
	t.Helper()
	obj := getCR(t, gvr, name)
	path := append([]string{"status"}, fields...)
	val, found, err := unstructured.NestedFieldNoCopy(obj.Object, path...)
	require.NoError(t, err)
	require.True(t, found, "status field %v not found in %s/%s", fields, gvr.Resource, name)
	return val
}

func getConditionReason(t *testing.T, gvr schema.GroupVersionResource, name, condType string) string {
	t.Helper()
	obj := getCR(t, gvr, name)
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return ""
	}
	for _, c := range conditions {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cm["type"] == condType {
			if r, ok := cm["reason"].(string); ok {
				return r
			}
		}
	}
	return ""
}

func getConfigMap(t *testing.T, ns, name string) *corev1.ConfigMap {
	t.Helper()
	cm, err := k8sClient.CoreV1().ConfigMaps(ns).Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	return cm
}

func getSecret(t *testing.T, ns, name string) *corev1.Secret {
	t.Helper()
	s, err := k8sClient.CoreV1().Secrets(ns).Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	return s
}

func waitForConfigMapKey(t *testing.T, ns, name, key string) string {
	t.Helper()
	var val string
	require.Eventually(t, func() bool {
		cm, err := k8sClient.CoreV1().ConfigMaps(ns).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		v, ok := cm.Data[key]
		if ok {
			val = v
		}
		return ok
	}, defaultTimeout, defaultInterval, "ConfigMap %s/%s did not get key %s", ns, name, key)
	return val
}

func waitForCRDeleted(t *testing.T, gvr schema.GroupVersionResource, name string) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := dynamicClient.Resource(gvr).Namespace(testNamespace).Get(context.Background(), name, metav1.GetOptions{})
		return apierrors.IsNotFound(err)
	}, defaultTimeout, defaultInterval, "%s/%s still exists", gvr.Resource, name)
}

func getNSName(name string) types.NamespacedName {
	return types.NamespacedName{Namespace: testNamespace, Name: name}
}

// ---------------------------------------------------------------------------
// CR builders
// ---------------------------------------------------------------------------

func newDatabaseCR(name, sfName, comment string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "Database",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":    sfName,
				"comment": comment,
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newSchemaCR(name, sfName, dbRefName, comment string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "Schema",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":    sfName,
				"comment": comment,
				"databaseRef": map[string]interface{}{
					"name": dbRefName,
				},
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newWarehouseCR(name, sfName, comment string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "Warehouse",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":          sfName,
				"warehouseSize": "XSMALL",
				"autoSuspend":   int64(60),
				"autoResume":    true,
				"comment":       comment,
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newUserCR(name, sfName, comment string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "User",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":    sfName,
				"comment": comment,
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newAccountRoleCR(name, sfName, comment string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "AccountRole",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":    sfName,
				"comment": comment,
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newDatabaseRoleCR(name, sfName, dbRefName, comment string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "DatabaseRole",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":    sfName,
				"comment": comment,
				"databaseRef": map[string]interface{}{
					"name": dbRefName,
				},
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newTableCR(name, sfName, dbRefName, schemaRefName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "Table",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name": sfName,
				"databaseRef": map[string]interface{}{
					"name": dbRefName,
				},
				"schemaRef": map[string]interface{}{
					"name": schemaRefName,
				},
				"columns": []interface{}{
					map[string]interface{}{
						"name": "ID",
						"type": "NUMBER(38,0)",
					},
					map[string]interface{}{
						"name":    "NAME",
						"type":    "VARCHAR(100)",
						"comment": "e2e test column",
					},
				},
				"comment": "e2e test table",
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newViewCR(name, sfName, dbRefName, schemaRefName, statement string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "View",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":      sfName,
				"statement": statement,
				"databaseRef": map[string]interface{}{
					"name": dbRefName,
				},
				"schemaRef": map[string]interface{}{
					"name": schemaRefName,
				},
				"comment": "e2e test view",
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newStageCR(name, sfName, dbRefName, schemaRefName, comment string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "Stage",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":    sfName,
				"comment": comment,
				"databaseRef": map[string]interface{}{
					"name": dbRefName,
				},
				"schemaRef": map[string]interface{}{
					"name": schemaRefName,
				},
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newFieldExportCR(name, sourceKind, sourceName, path, targetKind, targetName, targetKey string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "FieldExport",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"from": map[string]interface{}{
					"resource": map[string]interface{}{
						"kind": sourceKind,
						"name": sourceName,
					},
					"path": path,
				},
				"to": map[string]interface{}{
					"kind": targetKind,
					"name": targetName,
					"key":  targetKey,
				},
			},
		},
	}
}

func newGrantPrivilegesToAccountRoleCR(name, privilege, accountRole string, on map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "GrantPrivilegesToAccountRole",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"privilege":   privilege,
				"accountRole": accountRole,
				"on":          on,
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newGrantPrivilegesToDatabaseRoleCR(name, privilege, databaseRole string, on map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "GrantPrivilegesToDatabaseRole",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"privilege":    privilege,
				"databaseRole": databaseRole,
				"on":           on,
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newNetworkPolicyCR(name, sfName string, allowedIPs []interface{}, comment string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":          sfName,
				"allowedIPList": allowedIPs,
				"comment":       comment,
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newPasswordPolicyCR(name, sfName, dbRefName, schemaRefName string, minLen int64, comment string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "PasswordPolicy",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":              sfName,
				"passwordMinLength": minLen,
				"comment":           comment,
				"databaseRef": map[string]interface{}{
					"name": dbRefName,
				},
				"schemaRef": map[string]interface{}{
					"name": schemaRefName,
				},
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newMaskingPolicyCR(name, sfName, dbRefName, schemaRefName, body string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "MaskingPolicy",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name": sfName,
				"signature": []interface{}{
					map[string]interface{}{"name": "val", "type": "VARCHAR"},
				},
				"body": body,
				"databaseRef": map[string]interface{}{
					"name": dbRefName,
				},
				"schemaRef": map[string]interface{}{
					"name": schemaRefName,
				},
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newTagCR(name, sfName, dbRefName, schemaRefName, comment string, allowedValues []interface{}) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"name":    sfName,
		"comment": comment,
		"databaseRef": map[string]interface{}{
			"name": dbRefName,
		},
		"schemaRef": map[string]interface{}{
			"name": schemaRefName,
		},
		"providerRef": map[string]interface{}{
			"name": providerName,
		},
	}
	if len(allowedValues) > 0 {
		spec["allowedValues"] = allowedValues
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "Tag",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": spec,
		},
	}
}

func newTaskCR(name, sfName, dbRefName, schemaRefName, warehouseName, sqlStatement string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "Task",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":          sfName,
				"sqlStatement":  sqlStatement,
				"comment":       "e2e test task",
				"schedule":      "60 MINUTE",
				"warehouseName": warehouseName,
				"suspend":       true,
				"databaseRef": map[string]interface{}{
					"name": dbRefName,
				},
				"schemaRef": map[string]interface{}{
					"name": schemaRefName,
				},
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newStreamOnTableCR(name, sfName, dbRefName, schemaRefName, table string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "StreamOnTable",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"name":      sfName,
				"tableName": table,
				"comment":   "e2e test stream on table",
				"databaseRef": map[string]interface{}{
					"name": dbRefName,
				},
				"schemaRef": map[string]interface{}{
					"name": schemaRefName,
				},
				"providerRef": map[string]interface{}{
					"name": providerName,
				},
			},
		},
	}
}

func newAPIIntegrationCR(name, sfName string, allowedPrefixes []interface{}, comment string) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"name":               sfName,
		"apiProvider":        "git_https_api",
		"apiAllowedPrefixes": allowedPrefixes,
		"enabled":            true,
		"providerRef": map[string]interface{}{
			"name": providerName,
		},
	}

	if comment != "" {
		spec["comment"] = comment
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
			"kind":       "APIIntegration",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": spec,
		},
	}
}

// sfGrantExists checks whether a specific privilege grant exists on a role.
func sfGrantExists(t *testing.T, roleName, privilege, grantedOn, objectName string) bool {
	t.Helper()
	qCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	query := fmt.Sprintf("SHOW GRANTS TO ROLE %s", sqlbuilder.QuoteIdentifier(roleName))
	rows, err := sfDB.QueryContext(qCtx, query)
	if err != nil {
		return false
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		colMap := make(map[string]string, len(cols))
		for i, c := range cols {
			if vals[i] != nil {
				colMap[strings.ToLower(c)] = fmt.Sprintf("%v", vals[i])
			}
		}
		if strings.EqualFold(colMap["privilege"], privilege) &&
			strings.EqualFold(colMap["granted_on"], grantedOn) &&
			strings.Contains(strings.ToUpper(colMap["name"]), strings.ToUpper(objectName)) {
			return true
		}
	}
	return false
}

func withAnnotation(obj *unstructured.Unstructured, key, value string) *unstructured.Unstructured {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
	return obj
}

func withDeletionPolicy(obj *unstructured.Unstructured, policy string) *unstructured.Unstructured {
	_ = unstructured.SetNestedField(obj.Object, policy, "spec", "deletionPolicy")
	return obj
}

func uniqueName(base string) string {
	return fmt.Sprintf("%s%s_%d", sfPrefix, strings.ToUpper(base), time.Now().UnixNano()%100000)
}

func k8sName(sfName string) string {
	return strings.ToLower(strings.ReplaceAll(sfName, "_", "-"))
}

// parsePrivateKeyPEM decodes a PEM-encoded RSA private key (PKCS#8 or PKCS#1).
func parsePrivateKeyPEM(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS8 key is not RSA")
		}
		return rsaKey, nil
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing private key (tried PKCS8 and PKCS1): %w", err)
	}
	return key, nil
}
