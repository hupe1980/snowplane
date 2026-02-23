//go:build integration

// Package webhook integration tests exercise validating and mutating webhooks
// against a real kube-apiserver via envtest. The API server sends admission
// requests to the webhook server over TLS, mirroring production behavior.
package webhook_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/webhook"
)

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestMain(m *testing.M) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	// Bootstrap envtest with webhook support.
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "config", "webhook")},
		},
	}

	cfg, err := testEnv.Start()
	if err != nil {
		panic("failed to start envtest: " + err.Error())
	}

	scheme := testutil.TestScheme()

	// Create the controller manager with webhook server.
	webhookOpts := testEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		HealthProbeBindAddress: "0",
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Host:    webhookOpts.LocalServingHost,
			Port:    webhookOpts.LocalServingPort,
			CertDir: webhookOpts.LocalServingCertDir,
		}),
	})
	if err != nil {
		panic("failed to create manager: " + err.Error())
	}

	// Register all webhooks exactly as done in cmd/manager/main.go.
	hookServer := mgr.GetWebhookServer()

	defaultsMutator := webhook.NewDefaultsMutator(scheme)
	for _, res := range []string{
		"database", "schema", "warehouse", "accountrole",
		"databaserole", "grant", "user", "table", "view", "stage",
		"providerconfig",
	} {
		hookServer.Register(
			"/mutate-snowplane-v1alpha1-"+res,
			&ctrlwebhook.Admission{Handler: defaultsMutator},
		)
	}

	hookServer.Register("/validate-snowplane-v1alpha1-database", &ctrlwebhook.Admission{Handler: webhook.NewDatabaseValidator(scheme)})
	hookServer.Register("/validate-snowplane-v1alpha1-schema", &ctrlwebhook.Admission{Handler: webhook.NewSchemaValidator(scheme)})
	hookServer.Register("/validate-snowplane-v1alpha1-warehouse", &ctrlwebhook.Admission{Handler: webhook.NewWarehouseValidator(scheme)})
	hookServer.Register("/validate-snowplane-v1alpha1-accountrole", &ctrlwebhook.Admission{Handler: webhook.NewAccountRoleValidator(scheme)})
	hookServer.Register("/validate-snowplane-v1alpha1-databaserole", &ctrlwebhook.Admission{Handler: webhook.NewDatabaseRoleValidator(scheme)})
	hookServer.Register("/validate-snowplane-v1alpha1-grant", &ctrlwebhook.Admission{Handler: webhook.NewAccountRoleGrantValidator(scheme)})
	hookServer.Register("/validate-snowplane-v1alpha1-user", &ctrlwebhook.Admission{Handler: webhook.NewUserValidator(scheme)})
	hookServer.Register("/validate-snowplane-v1alpha1-table", &ctrlwebhook.Admission{Handler: webhook.NewTableValidator(scheme)})
	hookServer.Register("/validate-snowplane-v1alpha1-view", &ctrlwebhook.Admission{Handler: webhook.NewViewValidator(scheme)})
	hookServer.Register("/validate-snowplane-v1alpha1-stage", &ctrlwebhook.Admission{Handler: webhook.NewStageValidator(scheme)})
	hookServer.Register("/validate-snowplane-v1alpha1-providerconfig", &ctrlwebhook.Admission{Handler: webhook.NewProviderConfigValidator(scheme)})

	// Start the manager (includes webhook server).
	go func() {
		if err := mgr.Start(ctx); err != nil {
			panic("manager exited with error: " + err.Error())
		}
	}()

	// Wait for the webhook server to be ready.
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookOpts.LocalServingHost, webhookOpts.LocalServingPort)
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 10*time.Second, true, func(ctx context.Context) (bool, error) {
		conn, connErr := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
		if connErr != nil {
			return false, nil //nolint:nilerr
		}
		conn.Close()
		return true, nil
	}); err != nil {
		panic("webhook server did not start in time")
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic("failed to create k8s client: " + err.Error())
	}

	code := m.Run()

	cancel()
	if err := testEnv.Stop(); err != nil {
		_, _ = os.Stderr.WriteString("warning: failed to stop envtest: " + err.Error() + "\n")
	}

	os.Exit(code)
}

// ---------------------------------------------------------------------------
// Helper: unique name generator
// ---------------------------------------------------------------------------

var nameCounter atomic.Int64

func uniqueName(prefix string) string {
	n := nameCounter.Add(1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// ---------------------------------------------------------------------------
// Validating webhook: CREATE rejection
// ---------------------------------------------------------------------------

func TestValidating_Database_InvalidSpec_Rejected(t *testing.T) {
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("db-invalid"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name: "", // Empty name should be rejected by validation.
		},
	}

	err := k8sClient.Create(ctx, db)
	require.Error(t, err, "creating a database with empty name should fail")
	assert.Contains(t, err.Error(), "name")
}

func TestValidating_Database_ValidSpec_Accepted(t *testing.T) {
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("db-valid"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name: "TEST_DB",
		},
	}

	err := k8sClient.Create(ctx, db)
	require.NoError(t, err, "creating a valid database should succeed")
}

func TestValidating_Schema_InvalidSpec_Rejected(t *testing.T) {
	schema := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("schema-invalid"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name:        "", // Empty
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "db"},
		},
	}

	err := k8sClient.Create(ctx, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestValidating_Warehouse_InvalidSpec_Rejected(t *testing.T) {
	autoSuspend := int32(-1) // negative auto-suspend should fail
	wh := &snowplanev1alpha1.Warehouse{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("wh-invalid"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.WarehouseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name:        "INVALID_WH",
			AutoSuspend: &autoSuspend,
		},
	}

	err := k8sClient.Create(ctx, wh)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "autoSuspend")
}

func TestValidating_Grant_ValidSpec_Accepted(t *testing.T) {
	grant := &snowplanev1alpha1.AccountRoleGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("grant-valid"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.AccountRoleGrantSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Privilege: "USAGE",
			On: snowplanev1alpha1.GrantOn{
				AccountObject: &snowplanev1alpha1.GrantOnAccountObject{
					ObjectType: "DATABASE",
					ObjectName: "MY_DB",
				},
			},
			AccountRole: "ANALYST",
		},
	}

	err := k8sClient.Create(ctx, grant)
	require.NoError(t, err)
}

func TestValidating_User_InvalidEmail_Rejected(t *testing.T) {
	badEmail := "not-an-email"
	user := &snowplanev1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("user-bad-email"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.UserSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name:  "BAD_EMAIL_USER",
			Email: &badEmail,
		},
	}

	err := k8sClient.Create(ctx, user)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "email")
}

// ---------------------------------------------------------------------------
// Validating webhook: UPDATE immutability
// ---------------------------------------------------------------------------

func TestValidating_Database_ImmutableName_Rejected(t *testing.T) {
	name := uniqueName("db-imm")
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name: "ORIG_DB",
		},
	}

	err := k8sClient.Create(ctx, db)
	require.NoError(t, err)

	// Simulate that the resource was reconciled (ObservedGeneration > 0).
	err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, db)
	require.NoError(t, err)

	db.Status.ObservedGeneration = 1
	err = k8sClient.Status().Update(ctx, db)
	require.NoError(t, err)

	// Try to change the immutable name.
	err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, db)
	require.NoError(t, err)

	db.Spec.Name = "RENAMED_DB"
	err = k8sClient.Update(ctx, db)
	require.Error(t, err, "updating immutable name should fail")
	assert.Contains(t, err.Error(), "immutable")
}

func TestValidating_Database_ImmutableName_AllowedWithForceNew(t *testing.T) {
	name := uniqueName("db-forcenew")
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name: "ORIG_FN_DB",
		},
	}

	err := k8sClient.Create(ctx, db)
	require.NoError(t, err)

	// Set observed generation > 0.
	err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, db)
	require.NoError(t, err)

	db.Status.ObservedGeneration = 1
	err = k8sClient.Status().Update(ctx, db)
	require.NoError(t, err)

	// Update with force-new annotation — should be allowed.
	err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, db)
	require.NoError(t, err)

	if db.Annotations == nil {
		db.Annotations = make(map[string]string)
	}

	db.Annotations[snowplanev1alpha1.AnnotationForceNew] = "true"
	db.Spec.Name = "FORCENEW_DB"

	err = k8sClient.Update(ctx, db)
	require.NoError(t, err, "force-new annotation should bypass immutability checks")
}

func TestValidating_Schema_ImmutableDatabaseRef_Rejected(t *testing.T) {
	name := uniqueName("schema-imm")
	schema := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name:        "MY_SCHEMA",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "db-a"},
		},
	}

	err := k8sClient.Create(ctx, schema)
	require.NoError(t, err)

	// Set observed generation > 0.
	err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, schema)
	require.NoError(t, err)

	schema.Status.ObservedGeneration = 1
	err = k8sClient.Status().Update(ctx, schema)
	require.NoError(t, err)

	// Try to change immutable databaseRef.
	err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, schema)
	require.NoError(t, err)

	schema.Spec.DatabaseRef.Name = "db-b"
	err = k8sClient.Update(ctx, schema)
	require.Error(t, err, "updating immutable databaseRef should fail")
	assert.Contains(t, err.Error(), "immutable")
}

// ---------------------------------------------------------------------------
// Mutating webhook: defaults injection
// ---------------------------------------------------------------------------

func TestMutating_Database_DefaultDeletionPolicy(t *testing.T) {
	name := uniqueName("db-mutate")
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name: "MUTATED_DB",
			// DeletionPolicy not set — mutating webhook should default it.
		},
	}

	err := k8sClient.Create(ctx, db)
	require.NoError(t, err)

	// Re-read to get server-side state after mutation.
	err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, db)
	require.NoError(t, err)

	assert.Equal(t, snowplanev1alpha1.DeletionPolicyDelete, db.Spec.DeletionPolicy,
		"mutating webhook should default deletionPolicy to Delete")
}

func TestMutating_Warehouse_DefaultDeletionPolicy(t *testing.T) {
	name := uniqueName("wh-mutate")
	wh := &snowplanev1alpha1.Warehouse{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.WarehouseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name: "MUTATED_WH",
		},
	}

	err := k8sClient.Create(ctx, wh)
	require.NoError(t, err)

	err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, wh)
	require.NoError(t, err)

	assert.Equal(t, snowplanev1alpha1.DeletionPolicyDelete, wh.Spec.DeletionPolicy)
}

// ---------------------------------------------------------------------------
// AccountRole, DatabaseRole, Table, View, Stage – CREATE validation
// ---------------------------------------------------------------------------

func TestValidating_AccountRole_ValidSpec_Accepted(t *testing.T) {
	role := &snowplanev1alpha1.AccountRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("role-valid"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.AccountRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name: "DATA_ANALYST",
		},
	}

	err := k8sClient.Create(ctx, role)
	require.NoError(t, err)
}

func TestValidating_AccountRole_EmptyName_Rejected(t *testing.T) {
	role := &snowplanev1alpha1.AccountRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("role-bad"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.AccountRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name: "",
		},
	}

	err := k8sClient.Create(ctx, role)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestValidating_DatabaseRole_ValidSpec_Accepted(t *testing.T) {
	role := &snowplanev1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("dbrole-valid"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name:        "DB_READER",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "my-db"},
		},
	}

	err := k8sClient.Create(ctx, role)
	require.NoError(t, err)
}

func TestValidating_Table_ValidSpec_Accepted(t *testing.T) {
	table := &snowplanev1alpha1.Table{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("tbl-valid"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.TableSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name:        "EVENTS",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "my-db"},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: "public"},
			Columns: []snowplanev1alpha1.ColumnDefinition{
				{Name: "id", Type: "NUMBER(38,0)"},
				{Name: "name", Type: "VARCHAR(256)"},
			},
		},
	}

	err := k8sClient.Create(ctx, table)
	require.NoError(t, err)
}

func TestValidating_View_ValidSpec_Accepted(t *testing.T) {
	view := &snowplanev1alpha1.View{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("view-valid"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.ViewSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name:        "EVENTS_VIEW",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "my-db"},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: "public"},
			Statement:   "SELECT * FROM events",
		},
	}

	err := k8sClient.Create(ctx, view)
	require.NoError(t, err)
}

func TestValidating_Stage_ValidSpec_Accepted(t *testing.T) {
	stage := &snowplanev1alpha1.Stage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("stage-valid"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.StageSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name:        "MY_STAGE",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "my-db"},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: "public"},
		},
	}

	err := k8sClient.Create(ctx, stage)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// ProviderConfig webhook
// ---------------------------------------------------------------------------

func TestValidating_ProviderConfig_ValidSpec_Accepted(t *testing.T) {
	pc := &snowplanev1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueName("pc-valid"),
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.ProviderConfigSpec{
			Account:            "testaccount",
			User:               "testuser",
			AuthenticationType: snowplanev1alpha1.AuthenticationTypeUsernamePassword,
			Credentials: snowplanev1alpha1.ProviderCredentials{
				SecretRef: &snowplanev1alpha1.SecretKeyReference{
					Name:      "snowflake-creds",
					Namespace: "default",
					Key:       "password",
				},
			},
		},
	}

	err := k8sClient.Create(ctx, pc)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Edge case: DELETE should always succeed (no validation on delete)
// ---------------------------------------------------------------------------

func TestValidating_Database_Delete_Allowed(t *testing.T) {
	name := uniqueName("db-del")
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "some-pc"},
			},
			Name: "DEL_DB",
		},
	}

	err := k8sClient.Create(ctx, db)
	require.NoError(t, err)

	err = k8sClient.Delete(ctx, db)
	require.NoError(t, err, "deleting a database should always succeed through validation webhook")
}
