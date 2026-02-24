//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	accountrolectl "github.com/hupe1980/snowplane/internal/controller/accountrole"
	database "github.com/hupe1980/snowplane/internal/controller/database"
	databaserolectl "github.com/hupe1980/snowplane/internal/controller/databaserole"
	fieldexportctl "github.com/hupe1980/snowplane/internal/controller/fieldexport"
	grantctl "github.com/hupe1980/snowplane/internal/controller/grant"
	schemactl "github.com/hupe1980/snowplane/internal/controller/schema"
	stagectl "github.com/hupe1980/snowplane/internal/controller/stage"
	tablectl "github.com/hupe1980/snowplane/internal/controller/table"
	userctl "github.com/hupe1980/snowplane/internal/controller/user"
	viewctl "github.com/hupe1980/snowplane/internal/controller/view"
	warehousectl "github.com/hupe1980/snowplane/internal/controller/warehouse"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// Package-level state shared across all integration tests.
var (
	k8sClient           client.Client
	testEnv             *envtest.Environment
	ctx                 context.Context
	cancel              context.CancelFunc
	dbMockSvc           *mockDatabaseService
	schemaMockSvc       *mockSchemaService
	tableMockSvc        *mockTableService
	viewMockSvc         *mockViewService
	stageMockSvc        *mockStageService
	warehouseMockSvc    *mockWarehouseService
	userMockSvc         *mockUserService
	accountRoleMockSvc  *mockAccountRoleService
	databaseRoleMockSvc *mockDatabaseRoleService
	grantMockSvc        *mockGrantService
)

const (
	defaultTimeout  = 30 * time.Second
	defaultInterval = 250 * time.Millisecond
	neverDuration   = 3 * time.Second
	testNamespace   = "default"
)

func TestMain(m *testing.M) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	// Bootstrap envtest — starts a real kube-apiserver + etcd.
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		panic("failed to start envtest: " + err.Error())
	}

	// Register Snowplane CRDs in the scheme.
	scheme := testutil.TestScheme()

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic("failed to create k8s client: " + err.Error())
	}

	// Create ProviderConfig and credential Secret that all tests reference.
	pc := testutil.NewTestPC(testNamespace)
	if err := k8sClient.Create(ctx, pc); err != nil {
		panic("failed to create test ProviderConfig: " + err.Error())
	}

	// envtest doesn't run controllers — manually set Ready condition on PC.
	pc.Status.Conditions = testutil.NewTestPC(testNamespace).Status.Conditions
	if err := k8sClient.Status().Update(ctx, pc); err != nil {
		panic("failed to update ProviderConfig status: " + err.Error())
	}

	secret := testutil.NewTestSecret(testNamespace)
	if err := k8sClient.Create(ctx, secret); err != nil {
		panic("failed to create test secret: " + err.Error())
	}

	// Create the controller manager with mock Snowflake services.
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			// Bind to :0 to avoid port conflicts in CI.
			BindAddress: "0",
		},
		// Disable health probes in tests.
		HealthProbeBindAddress: "0",
	})
	if err != nil {
		panic("failed to create manager: " + err.Error())
	}

	// Build a ClientFactory that returns our mock client.
	factory := clientfactory.NewTestClientFactoryWithFn(
		func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
			return &mockSnowflakeClient{}, nil
		},
	)

	rl := ratelimit.New(ratelimit.Options{QPS: 0}) // No rate limiting in tests.
	recorder := record.NewFakeRecorder(200)

	// Drain the event recorder channel continuously to prevent blocking.
	// FakeRecorder.writeEvent() does a blocking channel send; if the buffer
	// fills up (200 events), any controller trying to record an event blocks
	// forever, freezing all subsequent reconciliations.
	go func() {
		for range recorder.Events {
		}
	}()

	// --- Database controller ---
	dbMockSvc = &mockDatabaseService{}

	dbServiceFactory := func(_ context.Context, _ database.SnowflakeClient, _ string) (database.Service, func(context.Context), error) {
		return dbMockSvc, func(context.Context) {}, nil
	}

	dbReconciler := database.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, dbServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true) // Fast requeue for tests.

	if err := dbReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup database controller: " + err.Error())
	}

	// --- Schema controller ---
	schemaMockSvc = &mockSchemaService{}

	schemaServiceFactory := func(_ context.Context, _ schemactl.SnowflakeClient, _ string) (schemactl.Service, func(context.Context), error) {
		return schemaMockSvc, func(context.Context) {}, nil
	}

	schemaReconciler := schemactl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, schemaServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := schemaReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup schema controller: " + err.Error())
	}

	// --- Table controller ---
	tableMockSvc = &mockTableService{}

	tableServiceFactory := func(_ context.Context, _ tablectl.SnowflakeClient, _ string) (tablectl.Service, func(context.Context), error) {
		return tableMockSvc, func(context.Context) {}, nil
	}

	tableReconciler := tablectl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, tableServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := tableReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup table controller: " + err.Error())
	}

	// --- View controller ---
	viewMockSvc = &mockViewService{}

	viewServiceFactory := func(_ context.Context, _ viewctl.SnowflakeClient, _ string) (viewctl.Service, func(context.Context), error) {
		return viewMockSvc, func(context.Context) {}, nil
	}

	viewReconciler := viewctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, viewServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := viewReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup view controller: " + err.Error())
	}

	// --- Stage controller ---
	stageMockSvc = &mockStageService{}

	stageServiceFactory := func(_ context.Context, _ stagectl.SnowflakeClient, _ string) (stagectl.Service, func(context.Context), error) {
		return stageMockSvc, func(context.Context) {}, nil
	}

	stageReconciler := stagectl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, stageServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := stageReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup stage controller: " + err.Error())
	}

	// --- Warehouse controller ---
	warehouseMockSvc = &mockWarehouseService{}

	warehouseServiceFactory := func(_ context.Context, _ warehousectl.SnowflakeClient, _ string) (warehousectl.Service, func(context.Context), error) {
		return warehouseMockSvc, func(context.Context) {}, nil
	}

	warehouseReconciler := warehousectl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, warehouseServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := warehouseReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup warehouse controller: " + err.Error())
	}

	// --- User controller ---
	userMockSvc = &mockUserService{}

	userServiceFactory := func(_ context.Context, _ userctl.SnowflakeClient, _ string) (userctl.Service, func(context.Context), error) {
		return userMockSvc, func(context.Context) {}, nil
	}

	userReconciler := userctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, userServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := userReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup user controller: " + err.Error())
	}

	// --- AccountRole controller ---
	accountRoleMockSvc = &mockAccountRoleService{}

	accountRoleServiceFactory := func(_ context.Context, _ accountrolectl.SnowflakeClient, _ string) (accountrolectl.Service, func(context.Context), error) {
		return accountRoleMockSvc, func(context.Context) {}, nil
	}

	accountRoleReconciler := accountrolectl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, accountRoleServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := accountRoleReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup accountrole controller: " + err.Error())
	}

	// --- DatabaseRole controller ---
	databaseRoleMockSvc = &mockDatabaseRoleService{}

	databaseRoleServiceFactory := func(_ context.Context, _ databaserolectl.SnowflakeClient, _ string) (databaserolectl.Service, func(context.Context), error) {
		return databaseRoleMockSvc, func(context.Context) {}, nil
	}

	databaseRoleReconciler := databaserolectl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, databaseRoleServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := databaseRoleReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup databaserole controller: " + err.Error())
	}

	// --- Grant controller ---
	grantMockSvc = &mockGrantService{}

	grantServiceFactory := func(_ context.Context, _ grantctl.SnowflakeClient, _ string) (grantctl.Service, func(context.Context), error) {
		return grantMockSvc, func(context.Context) {}, nil
	}

	grantReconciler := grantctl.NewAccountRoleGrantReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, grantServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := grantReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup grant controller: " + err.Error())
	}

	// --- FieldExport controller (standalone — no mock Snowflake service) ---
	fieldExportReconciler := fieldexportctl.NewReconciler(
		mgr.GetClient(),
		recorder,
	)

	if err := fieldExportReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup fieldexport controller: " + err.Error())
	}

	// Start the manager in a goroutine.
	go func() {
		if err := mgr.Start(ctx); err != nil {
			panic("manager exited with error: " + err.Error())
		}
	}()

	// Wait for caches to sync.
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		panic("cache sync failed")
	}

	// Run tests.
	code := m.Run()

	// Cleanup.
	cancel()

	if err := testEnv.Stop(); err != nil {
		// Log but don't fail — this is best-effort cleanup.
		_, _ = os.Stderr.WriteString("warning: failed to stop envtest: " + err.Error() + "\n")
	}

	os.Exit(code)
}

// newTestDatabase creates a Database CR with sensible defaults for integration tests.
func newTestDatabase(name, sfName string) *snowplanev1alpha1.Database {
	return &snowplanev1alpha1.Database{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: sfName,
		},
	}
}

// newTestSchema creates a Schema CR referencing a Database for integration tests.
func newTestSchema(name, sfName, dbRefName string) *snowplanev1alpha1.Schema {
	return &snowplanev1alpha1.Schema{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRefName},
		},
	}
}

// databaseObservation returns a standard existing-database observation.
func databaseObservation(name, comment, owner string) *snowflake.DatabaseObservation {
	return &snowflake.DatabaseObservation{
		Exists: true,
		ShowOutput: &snowflake.DatabaseShowOutput{
			CreatedOn:     "2024-01-01",
			Name:          name,
			Kind:          "STANDARD",
			Comment:       comment,
			Owner:         owner,
			RetentionTime: 1,
		},
		Parameters: &snowflake.DatabaseParameters{
			DataRetentionTimeInDays:    ptrInt32(1),
			MaxDataExtensionTimeInDays: ptrInt32(14),
			DefaultDDLCollation:        "",
			ReplaceInvalidCharacters:   ptrBool(false),
			StorageSerializationPolicy: "COMPATIBLE",
			LogLevel:                   "OFF",
			MetricLevel:                "NONE",
			TraceLevel:                 "OFF",
		},
	}
}

// schemaObservation returns a standard existing-schema observation.
func schemaObservation(name, dbName, comment, owner string) *snowflake.SchemaObservation {
	return &snowflake.SchemaObservation{
		Exists: true,
		ShowOutput: &snowflake.SchemaShowOutput{
			CreatedOn:     "2024-01-01",
			Name:          name,
			DatabaseName:  dbName,
			Kind:          "STANDARD",
			Comment:       comment,
			Owner:         owner,
			RetentionTime: 1,
		},
		Parameters: &snowflake.SchemaParameters{
			DataRetentionTimeInDays:    ptrInt32(1),
			MaxDataExtensionTimeInDays: ptrInt32(14),
			DefaultDDLCollation:        "",
			ReplaceInvalidCharacters:   ptrBool(false),
			StorageSerializationPolicy: "COMPATIBLE",
			LogLevel:                   "OFF",
			MetricLevel:                "NONE",
			TraceLevel:                 "OFF",
		},
	}
}

func ptrString(s string) *string { return &s }
func ptrInt32(i int32) *int32    { return &i }
func ptrBool(b bool) *bool       { return &b }

// newTestTable creates a Table CR referencing a Database and Schema for integration tests.
func newTestTable(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.Table {
	return &snowplanev1alpha1.Table{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.TableSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: schemaRefName},
			Columns: []snowplanev1alpha1.ColumnDefinition{
				{Name: "ID", Type: "NUMBER(38,0)"},
				{Name: "NAME", Type: "VARCHAR(256)"},
			},
		},
	}
}

// newTestView creates a View CR referencing a Database and Schema for integration tests.
func newTestView(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.View {
	return &snowplanev1alpha1.View{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.ViewSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: schemaRefName},
			Statement:   "SELECT 1",
		},
	}
}

// newTestStage creates a Stage CR referencing a Database and Schema for integration tests.
func newTestStage(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.Stage {
	return &snowplanev1alpha1.Stage{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.StageSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: schemaRefName},
		},
	}
}

// tableObservation returns a standard existing-table observation.
func tableObservation(name, dbName, schemaName, comment, owner string) *snowflake.TableObservation {
	return &snowflake.TableObservation{
		Exists: true,
		ShowOutput: &snowflake.TableShowOutput{
			CreatedOn:             "2024-01-01",
			Name:                  name,
			DatabaseName:          dbName,
			SchemaName:            schemaName,
			Kind:                  "TABLE",
			Comment:               comment,
			Owner:                 owner,
			RetentionTime:         1,
			ChangeTracking:        false,
			EnableSchemaEvolution: false,
		},
	}
}

// viewObservation returns a standard existing-view observation.
func viewObservation(name, dbName, schemaName, comment, owner, statement string, secure bool) *snowflake.ViewObservation {
	return &snowflake.ViewObservation{
		Exists: true,
		ShowOutput: &snowflake.ViewShowOutput{
			CreatedOn:      "2024-01-01",
			Name:           name,
			DatabaseName:   dbName,
			SchemaName:     schemaName,
			Comment:        comment,
			Owner:          owner,
			IsSecure:       secure,
			Text:           statement,
			ChangeTracking: false,
		},
	}
}

// stageObservation returns a standard existing-stage observation.
func stageObservation(name, dbName, schemaName, comment, owner, stageType string) *snowflake.StageObservation {
	return &snowflake.StageObservation{
		Exists: true,
		ShowOutput: &snowflake.StageShowOutput{
			CreatedOn:        "2024-01-01",
			Name:             name,
			DatabaseName:     dbName,
			SchemaName:       schemaName,
			URL:              "",
			Owner:            owner,
			Comment:          comment,
			Type:             stageType,
			DirectoryEnabled: false,
		},
	}
}

// newTestWarehouse creates a Warehouse CR with sensible defaults for integration tests.
func newTestWarehouse(name, sfName string) *snowplanev1alpha1.Warehouse {
	return &snowplanev1alpha1.Warehouse{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.WarehouseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: sfName,
		},
	}
}

// newTestUser creates a User CR with sensible defaults for integration tests.
func newTestUser(name, sfName string) *snowplanev1alpha1.User {
	return &snowplanev1alpha1.User{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.UserSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: sfName,
		},
	}
}

// newTestAccountRole creates an AccountRole CR with sensible defaults for integration tests.
func newTestAccountRole(name, sfName string) *snowplanev1alpha1.AccountRole {
	return &snowplanev1alpha1.AccountRole{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.AccountRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: sfName,
		},
	}
}

// newTestDatabaseRole creates a DatabaseRole CR referencing a Database for integration tests.
func newTestDatabaseRole(name, sfName, dbRefName string) *snowplanev1alpha1.DatabaseRole {
	return &snowplanev1alpha1.DatabaseRole{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.DatabaseRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRefName},
		},
	}
}

// newTestGrant creates an AccountRoleGrant CR for a database-level privilege.
func newTestGrant(name, privilege, objectType, objectName, toRole string) *snowplanev1alpha1.AccountRoleGrant {
	return &snowplanev1alpha1.AccountRoleGrant{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.AccountRoleGrantSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Privilege: privilege,
			On: snowplanev1alpha1.GrantOn{
				AccountObject: &snowplanev1alpha1.GrantOnAccountObject{
					ObjectType: objectType,
					ObjectName: objectName,
				},
			},
			AccountRole: toRole,
		},
	}
}

// warehouseObservation returns a standard existing-warehouse observation.
func warehouseObservation(name, comment, owner string) *snowflake.WarehouseObservation {
	return &snowflake.WarehouseObservation{
		Exists: true,
		ShowOutput: &snowflake.WarehouseShowOutput{
			CreatedOn:       "2024-01-01",
			Name:            name,
			State:           "STARTED",
			Type:            "STANDARD",
			Size:            "X-Small",
			Comment:         comment,
			Owner:           owner,
			AutoSuspend:     600,
			AutoResume:      true,
			MinClusterCount: 1,
			MaxClusterCount: 1,
			ScalingPolicy:   "STANDARD",
		},
		Parameters: &snowflake.WarehouseParameters{
			MaxConcurrencyLevel:             ptrInt32(8),
			StatementQueuedTimeoutInSeconds: ptrInt32(0),
			StatementTimeoutInSeconds:       ptrInt32(172800),
			EnableQueryAcceleration:         ptrBool(false),
			QueryAccelerationMaxScaleFactor: ptrInt32(8),
		},
	}
}

// userObservation returns a standard existing-user observation.
func userObservation(name, comment, owner string) *snowflake.UserObservation {
	return &snowflake.UserObservation{
		Exists: true,
		ShowOutput: &snowflake.UserShowOutput{
			CreatedOn:   "2024-01-01",
			Name:        name,
			LoginName:   name,
			DisplayName: name,
			Comment:     comment,
			Owner:       owner,
			Type:        "PERSON",
			DefaultRole: "PUBLIC",
		},
		DescribeOutput: &snowflake.UserDescribeOutput{},
	}
}

// accountRoleObservation returns a standard existing-account-role observation.
func accountRoleObservation(name, comment, owner string) *snowflake.AccountRoleObservation {
	return &snowflake.AccountRoleObservation{
		Exists: true,
		ShowOutput: &snowflake.AccountRoleShowOutput{
			CreatedOn: "2024-01-01",
			Name:      name,
			Comment:   comment,
			Owner:     owner,
		},
	}
}

// databaseRoleObservation returns a standard existing-database-role observation.
func databaseRoleObservation(name, dbName, comment, owner string) *snowflake.DatabaseRoleObservation {
	return &snowflake.DatabaseRoleObservation{
		Exists: true,
		ShowOutput: &snowflake.DatabaseRoleShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			Comment:      comment,
			Owner:        owner,
		},
	}
}

// grantObservation returns a standard existing-grant observation.
func grantObservation(privilege, grantedOn, objectName, grantedTo, granteeName string, grantOption bool) *snowflake.GrantObservation {
	return &snowflake.GrantObservation{
		Exists: true,
		ShowOutput: &snowflake.GrantShowOutput{
			CreatedOn:   "2024-01-01",
			Privilege:   privilege,
			GrantedOn:   grantedOn,
			Name:        objectName,
			GrantedTo:   grantedTo,
			GranteeName: granteeName,
			GrantOption: grantOption,
		},
	}
}

// setupReadyDatabase creates a Database that is Ready, returning a cleanup function.
// This is a prerequisite for DatabaseRole integration tests.
func setupReadyDatabase(t *testing.T, dbK8sName, sfDBName string) (dbKey types.NamespacedName, cleanup func()) {
	t.Helper()

	var dbCreated atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if id.Name() == sfDBName && dbCreated.Load() {
			return databaseObservation(sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		if opts.Name.Name() == sfDBName {
			dbCreated.Store(true)
		}

		return nil
	})

	db := newTestDatabase(dbK8sName, sfDBName)
	require.NoError(t, k8sClient.Create(ctx, db))

	dbKey = types.NamespacedName{Name: dbK8sName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "parent database should become Ready")

	cleanup = func() {
		dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var d snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &d); err == nil {
			_ = k8sClient.Delete(ctx, &d)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, dbKey, &snowplanev1alpha1.Database{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	}

	return dbKey, cleanup
}

// setupReadyDatabaseAndSchema creates a Database and Schema that are both Ready,
// returning cleanup functions and the keys. This is a common prerequisite for
// Table, View, and Stage integration tests.
func setupReadyDatabaseAndSchema(t *testing.T, dbK8sName, sfDBName, schemaK8sName, sfSchemaName string) (
	dbKey types.NamespacedName, schemaKey types.NamespacedName, cleanup func(),
) {
	t.Helper()

	var dbCreated atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if id.Name() == sfDBName && dbCreated.Load() {
			return databaseObservation(sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		if opts.Name.Name() == sfDBName {
			dbCreated.Store(true)
		}

		return nil
	})

	db := newTestDatabase(dbK8sName, sfDBName)
	require.NoError(t, k8sClient.Create(ctx, db))

	dbKey = types.NamespacedName{Name: dbK8sName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "parent database should become Ready")

	var schemaCreated atomic.Bool

	schemaMockSvc.SetObserve(func(_ context.Context, id snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
		if schemaCreated.Load() {
			return schemaObservation(sfSchemaName, sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.SchemaObservation{Exists: false}, nil
	})

	schemaMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSchemaOptions) error {
		schemaCreated.Store(true)

		return nil
	})

	schema := newTestSchema(schemaK8sName, sfSchemaName, dbK8sName)
	require.NoError(t, k8sClient.Create(ctx, schema))

	schemaKey = types.NamespacedName{Name: schemaK8sName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema
		if err := k8sClient.Get(ctx, schemaKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "parent schema should become Ready")

	cleanup = func() {
		schemaMockSvc.SetDrop(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error { return nil })

		var s snowplanev1alpha1.Schema
		if err := k8sClient.Get(ctx, schemaKey, &s); err == nil {
			_ = k8sClient.Delete(ctx, &s)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, schemaKey, &snowplanev1alpha1.Schema{}) != nil
			}, defaultTimeout, defaultInterval)
		}

		dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var d snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &d); err == nil {
			_ = k8sClient.Delete(ctx, &d)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, dbKey, &snowplanev1alpha1.Database{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	}

	return dbKey, schemaKey, cleanup
}
