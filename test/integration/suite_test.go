//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	accountrolectl "github.com/hupe1980/snowplane/internal/controller/accountrole"
	alertctl "github.com/hupe1980/snowplane/internal/controller/alert"
	apiauthcodegrantctl "github.com/hupe1980/snowplane/internal/controller/apiauthenticationintegrationwithauthorizationcodegrant"
	apiauthclientcredsctl "github.com/hupe1980/snowplane/internal/controller/apiauthenticationintegrationwithclientcredentials"
	apiauthjwtbearerctl "github.com/hupe1980/snowplane/internal/controller/apiauthenticationintegrationwithjwtbearer"
	apiintegrationctl "github.com/hupe1980/snowplane/internal/controller/apiintegration"
	authenticationpolicyctl "github.com/hupe1980/snowplane/internal/controller/authenticationpolicy"
	cortexsearchservicectl "github.com/hupe1980/snowplane/internal/controller/cortexsearchservice"
	database "github.com/hupe1980/snowplane/internal/controller/database"
	databaserolectl "github.com/hupe1980/snowplane/internal/controller/databaserole"
	dynamictablectl "github.com/hupe1980/snowplane/internal/controller/dynamictable"
	externaloauthctl "github.com/hupe1980/snowplane/internal/controller/externaloauthintegration"
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
	maskingpolicyctl "github.com/hupe1980/snowplane/internal/controller/maskingpolicy"
	maskingpolicyappctl "github.com/hupe1980/snowplane/internal/controller/maskingpolicyapplication"
	materializedviewctl "github.com/hupe1980/snowplane/internal/controller/materializedview"
	networkpolicyctl "github.com/hupe1980/snowplane/internal/controller/networkpolicy"
	networkpolicyattachctl "github.com/hupe1980/snowplane/internal/controller/networkpolicyattachment"
	networkrulectl "github.com/hupe1980/snowplane/internal/controller/networkrule"
	passwordpolicyctl "github.com/hupe1980/snowplane/internal/controller/passwordpolicy"
	passwordpolicyattachctl "github.com/hupe1980/snowplane/internal/controller/passwordpolicyattachment"
	pipectl "github.com/hupe1980/snowplane/internal/controller/pipe"
	procedurejavactl "github.com/hupe1980/snowplane/internal/controller/procedurejava"
	procedurejavascriptctl "github.com/hupe1980/snowplane/internal/controller/procedurejavascript"
	procedurepythonctl "github.com/hupe1980/snowplane/internal/controller/procedurepython"
	procedurescalactl "github.com/hupe1980/snowplane/internal/controller/procedurescala"
	proceduresqlctl "github.com/hupe1980/snowplane/internal/controller/proceduresql"
	resourcemonitorctl "github.com/hupe1980/snowplane/internal/controller/resourcemonitor"
	roleassignmentctl "github.com/hupe1980/snowplane/internal/controller/roleassignment"
	rowaccesspolicyctl "github.com/hupe1980/snowplane/internal/controller/rowaccesspolicy"
	saml2ctl "github.com/hupe1980/snowplane/internal/controller/saml2integration"
	schemactl "github.com/hupe1980/snowplane/internal/controller/schema"
	secondarydatabasectl "github.com/hupe1980/snowplane/internal/controller/secondarydatabase"
	secretauthcodectl "github.com/hupe1980/snowplane/internal/controller/secretwithauthorizationcodegrant"
	secretbasicauthctl "github.com/hupe1980/snowplane/internal/controller/secretwithbasicauthentication"
	secretclientcredsctl "github.com/hupe1980/snowplane/internal/controller/secretwithclientcredentials"
	secretgenericstringctl "github.com/hupe1980/snowplane/internal/controller/secretwithgenericstring"
	sequencectl "github.com/hupe1980/snowplane/internal/controller/sequence"
	shareddatabasectl "github.com/hupe1980/snowplane/internal/controller/shareddatabase"
	sqlstatementctl "github.com/hupe1980/snowplane/internal/controller/sqlstatement"
	streamlitctl "github.com/hupe1980/snowplane/internal/controller/streamlit"
	streamondirtablectl "github.com/hupe1980/snowplane/internal/controller/streamondirectorytable"
	streamondyntablectl "github.com/hupe1980/snowplane/internal/controller/streamondynamictable"
	streamonexttablectl "github.com/hupe1980/snowplane/internal/controller/streamonexternaltable"
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
	"github.com/hupe1980/snowplane/internal/testutil"
)

// Package-level state shared across all integration tests.
var (
	k8sClient                   client.Client
	testEnv                     *envtest.Environment
	ctx                         context.Context
	cancel                      context.CancelFunc
	dbMockSvc                   *mockDatabaseService
	schemaMockSvc               *mockSchemaService
	tableMockSvc                *mockTableService
	viewMockSvc                 *mockViewService
	warehouseMockSvc            *mockWarehouseService
	userMockSvc                 *mockUserService
	accountRoleMockSvc          *mockAccountRoleService
	databaseRoleMockSvc         *mockDatabaseRoleService
	grantMockSvc                *mockGrantService
	alertMockSvc                *mockAlertService
	taskMockSvc                 *mockTaskService
	dynamicTableMockSvc         *mockDynamicTableService
	networkPolicyMockSvc        *mockNetworkPolicyService
	maskingPolicyMockSvc        *mockMaskingPolicyService
	passwordPolicyMockSvc       *mockPasswordPolicyService
	resourceMonitorMockSvc      *mockResourceMonitorService
	pipeMockSvc                 *mockPipeService
	fileFormatMockSvc           *mockFileFormatService
	tagMockSvc                  *mockTagService
	rowAccessPolicyMockSvc      *mockRowAccessPolicyService
	grantOwnershipMockSvc       *mockGrantOwnershipService
	roleAssignmentMockSvc       *mockRoleAssignmentService
	authenticationPolicyMockSvc *mockAuthenticationPolicyService
	apiIntegrationMockSvc       *mockAPIIntegrationService
	secondaryDatabaseMockSvc    *mockSecondaryDatabaseService
	sharedDatabaseMockSvc       *mockSharedDatabaseService

	// Additional controller mocks
	functionJavaMockSvc           *mockFunctionService
	functionJavascriptMockSvc     *mockFunctionService
	functionPythonMockSvc         *mockFunctionService
	functionScalaMockSvc          *mockFunctionService
	functionSQLMockSvc            *mockFunctionService
	procedureJavaMockSvc          *mockProcedureService
	procedureJavascriptMockSvc    *mockProcedureService
	procedurePythonMockSvc        *mockProcedureService
	procedureScalaMockSvc         *mockProcedureService
	procedureSQLMockSvc           *mockProcedureService
	secretAuthCodeGrantMockSvc    *mockSecretService
	secretBasicAuthMockSvc        *mockSecretService
	secretClientCredsMockSvc      *mockSecretService
	secretGenericStringMockSvc    *mockSecretService
	streamOnDirectoryTableMockSvc *mockStreamService
	streamOnDynamicTableMockSvc   *mockStreamService
	streamOnExternalTableMockSvc  *mockStreamService
	streamOnTableMockSvc          *mockStreamService
	streamOnViewMockSvc           *mockStreamService
	apiAuthCodeGrantMockSvc       *mockAPIAuthIntegrationService
	apiAuthClientCredsMockSvc     *mockAPIAuthIntegrationService
	apiAuthJWTBearerMockSvc       *mockAPIAuthIntegrationService
	externalOAuthMockSvc          *mockExternalOAuthIntegrationService
	saml2MockSvc                  *mockSAML2IntegrationService
	externalTableMockSvc          *mockExternalTableService
	materializedViewMockSvc       *mockMaterializedViewService
	networkRuleMockSvc            *mockNetworkRuleService
	sequenceMockSvc               *mockSequenceService
	failoverGroupMockSvc          *mockFailoverGroupService
	maskingPolicyAppMockSvc       *mockMaskingPolicyApplicationService
	networkPolicyAttachMockSvc    *mockNetworkPolicyAttachmentService
	passwordPolicyAttachMockSvc   *mockPasswordPolicyAttachmentService
	tagAssociationMockSvc         *mockTagAssociationService
	tableConstraintMockSvc        *mockTableConstraintService
	sqlStatementMockSvc           *mockSQLStatementService
	externalVolumeMockSvc         *mockExternalVolumeService
	cortexSearchServiceMockSvc    *mockCortexSearchServiceService
	gitRepositoryMockSvc          *mockGitRepositoryService
	streamlitMockSvc              *mockStreamlitService
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
		func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
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

	grantReconciler := grantctl.NewGrantPrivilegesToAccountRoleReconcilerWithServiceFactory(
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

	if err := fieldExportReconciler.SetupWithManager(mgr, 1, nil); err != nil {
		panic("failed to setup fieldexport controller: " + err.Error())
	}

	// --- Alert controller ---
	alertMockSvc = &mockAlertService{}

	alertServiceFactory := func(_ context.Context, _ alertctl.SnowflakeClient, _ string) (alertctl.Service, func(context.Context), error) {
		return alertMockSvc, func(context.Context) {}, nil
	}

	alertReconciler := alertctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, alertServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := alertReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup alert controller: " + err.Error())
	}

	// --- Task controller ---
	taskMockSvc = &mockTaskService{}

	taskServiceFactory := func(_ context.Context, _ taskctl.SnowflakeClient, _ string) (taskctl.Service, func(context.Context), error) {
		return taskMockSvc, func(context.Context) {}, nil
	}

	taskReconciler := taskctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, taskServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := taskReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup task controller: " + err.Error())
	}

	// --- DynamicTable controller ---
	dynamicTableMockSvc = &mockDynamicTableService{}

	dynamicTableServiceFactory := func(_ context.Context, _ dynamictablectl.SnowflakeClient, _ string) (dynamictablectl.Service, func(context.Context), error) {
		return dynamicTableMockSvc, func(context.Context) {}, nil
	}

	dynamicTableReconciler := dynamictablectl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, dynamicTableServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := dynamicTableReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup dynamictable controller: " + err.Error())
	}

	// --- NetworkPolicy controller ---
	networkPolicyMockSvc = &mockNetworkPolicyService{}

	networkPolicyServiceFactory := func(_ context.Context, _ networkpolicyctl.SnowflakeClient, _ string) (networkpolicyctl.Service, func(context.Context), error) {
		return networkPolicyMockSvc, func(context.Context) {}, nil
	}

	networkPolicyReconciler := networkpolicyctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, networkPolicyServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := networkPolicyReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup networkpolicy controller: " + err.Error())
	}

	// --- MaskingPolicy controller ---
	maskingPolicyMockSvc = &mockMaskingPolicyService{}

	maskingPolicyServiceFactory := func(_ context.Context, _ maskingpolicyctl.SnowflakeClient, _ string) (maskingpolicyctl.Service, func(context.Context), error) {
		return maskingPolicyMockSvc, func(context.Context) {}, nil
	}

	maskingPolicyReconciler := maskingpolicyctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, maskingPolicyServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := maskingPolicyReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup maskingpolicy controller: " + err.Error())
	}

	// --- AuthenticationPolicy controller ---
	authenticationPolicyMockSvc = &mockAuthenticationPolicyService{}

	authenticationPolicyServiceFactory := func(_ context.Context, _ authenticationpolicyctl.SnowflakeClient, _ string) (authenticationpolicyctl.Service, func(context.Context), error) {
		return authenticationPolicyMockSvc, func(context.Context) {}, nil
	}

	authenticationPolicyReconciler := authenticationpolicyctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, authenticationPolicyServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := authenticationPolicyReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup authenticationpolicy controller: " + err.Error())
	}

	// --- PasswordPolicy controller ---
	passwordPolicyMockSvc = &mockPasswordPolicyService{}

	passwordPolicyServiceFactory := func(_ context.Context, _ passwordpolicyctl.SnowflakeClient, _ string) (passwordpolicyctl.Service, func(context.Context), error) {
		return passwordPolicyMockSvc, func(context.Context) {}, nil
	}

	passwordPolicyReconciler := passwordpolicyctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, passwordPolicyServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := passwordPolicyReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup passwordpolicy controller: " + err.Error())
	}

	// --- ResourceMonitor controller ---
	resourceMonitorMockSvc = &mockResourceMonitorService{}

	resourceMonitorServiceFactory := func(_ context.Context, _ resourcemonitorctl.SnowflakeClient, _ string) (resourcemonitorctl.Service, func(context.Context), error) {
		return resourceMonitorMockSvc, func(context.Context) {}, nil
	}

	resourceMonitorReconciler := resourcemonitorctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, resourceMonitorServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := resourceMonitorReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup resourcemonitor controller: " + err.Error())
	}

	// --- Pipe controller ---
	pipeMockSvc = &mockPipeService{}

	pipeServiceFactory := func(_ context.Context, _ pipectl.SnowflakeClient, _ string) (pipectl.Service, func(context.Context), error) {
		return pipeMockSvc, func(context.Context) {}, nil
	}

	pipeReconciler := pipectl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, pipeServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := pipeReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup pipe controller: " + err.Error())
	}

	// --- FileFormat controller ---
	fileFormatMockSvc = &mockFileFormatService{}

	fileFormatServiceFactory := func(_ context.Context, _ fileformatctl.SnowflakeClient, _ string) (fileformatctl.Service, func(context.Context), error) {
		return fileFormatMockSvc, func(context.Context) {}, nil
	}

	fileFormatReconciler := fileformatctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, fileFormatServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := fileFormatReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup fileformat controller: " + err.Error())
	}

	// --- Tag controller ---
	tagMockSvc = &mockTagService{}

	tagServiceFactory := func(_ context.Context, _ tagctl.SnowflakeClient, _ string) (tagctl.Service, func(context.Context), error) {
		return tagMockSvc, func(context.Context) {}, nil
	}

	tagReconciler := tagctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, tagServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := tagReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup tag controller: " + err.Error())
	}

	// --- RowAccessPolicy controller ---
	rowAccessPolicyMockSvc = &mockRowAccessPolicyService{}

	rowAccessPolicyServiceFactory := func(_ context.Context, _ rowaccesspolicyctl.SnowflakeClient, _ string) (rowaccesspolicyctl.Service, func(context.Context), error) {
		return rowAccessPolicyMockSvc, func(context.Context) {}, nil
	}

	rowAccessPolicyReconciler := rowaccesspolicyctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, rowAccessPolicyServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := rowAccessPolicyReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup rowaccesspolicy controller: " + err.Error())
	}

	// --- GrantOwnership controller ---
	grantOwnershipMockSvc = &mockGrantOwnershipService{}

	grantOwnershipServiceFactory := func(_ context.Context, _ grantownershipctl.SnowflakeClient, _ string) (grantownershipctl.Service, func(context.Context), error) {
		return grantOwnershipMockSvc, func(context.Context) {}, nil
	}

	grantOwnershipReconciler := grantownershipctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, grantOwnershipServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := grantOwnershipReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup grantownership controller: " + err.Error())
	}

	// --- AccountRoleAssignment controller ---
	roleAssignmentMockSvc = &mockRoleAssignmentService{}

	roleAssignmentServiceFactory := func(_ context.Context, _ roleassignmentctl.SnowflakeClient, _ string) (roleassignmentctl.Service, func(context.Context), error) {
		return roleAssignmentMockSvc, func(context.Context) {}, nil
	}

	accountRoleAssignmentReconciler := roleassignmentctl.NewAccountRoleAssignmentReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, roleAssignmentServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := accountRoleAssignmentReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup accountroleassignment controller: " + err.Error())
	}

	// --- DatabaseRoleAssignment controller ---
	databaseRoleAssignmentReconciler := roleassignmentctl.NewDatabaseRoleAssignmentReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, roleAssignmentServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := databaseRoleAssignmentReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup databaseroleassignment controller: " + err.Error())
	}

	// --- APIIntegration controller ---
	apiIntegrationMockSvc = &mockAPIIntegrationService{}

	apiIntegrationServiceFactory := func(_ context.Context, _ apiintegrationctl.SnowflakeClient, _ string) (apiintegrationctl.Service, func(context.Context), error) {
		return apiIntegrationMockSvc, func(context.Context) {}, nil
	}

	apiIntegrationReconciler := apiintegrationctl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, apiIntegrationServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := apiIntegrationReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup apiintegration controller: " + err.Error())
	}

	// --- SecondaryDatabase controller ---
	secondaryDatabaseMockSvc = &mockSecondaryDatabaseService{}

	secondaryDatabaseServiceFactory := func(_ context.Context, _ secondarydatabasectl.SnowflakeClient, _ string) (secondarydatabasectl.Service, func(context.Context), error) {
		return secondaryDatabaseMockSvc, func(context.Context) {}, nil
	}

	secondaryDatabaseReconciler := secondarydatabasectl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, secondaryDatabaseServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := secondaryDatabaseReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup secondarydatabase controller: " + err.Error())
	}

	// --- SharedDatabase controller ---
	sharedDatabaseMockSvc = &mockSharedDatabaseService{}

	sharedDatabaseServiceFactory := func(_ context.Context, _ shareddatabasectl.SnowflakeClient, _ string) (shareddatabasectl.Service, func(context.Context), error) {
		return sharedDatabaseMockSvc, func(context.Context) {}, nil
	}

	sharedDatabaseReconciler := shareddatabasectl.NewReconcilerWithServiceFactory(
		mgr.GetClient(), factory, recorder, rl, sharedDatabaseServiceFactory,
	).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)

	if err := sharedDatabaseReconciler.SetupWithManager(mgr, 1); err != nil {
		panic("failed to setup shareddatabase controller: " + err.Error())
	}

	// ===================================================================
	// Additional controllers
	// ===================================================================

	// --- FunctionJava controller ---
	functionJavaMockSvc = &mockFunctionService{}
	{
		svc := functionJavaMockSvc
		sf := func(_ context.Context, _ functionjavactl.SnowflakeClient, _ string) (functionjavactl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := functionjavactl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup functionjava controller: " + err.Error())
		}
	}

	// --- FunctionJavascript controller ---
	functionJavascriptMockSvc = &mockFunctionService{}
	{
		svc := functionJavascriptMockSvc
		sf := func(_ context.Context, _ functionjavascriptctl.SnowflakeClient, _ string) (functionjavascriptctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := functionjavascriptctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup functionjavascript controller: " + err.Error())
		}
	}

	// --- FunctionPython controller ---
	functionPythonMockSvc = &mockFunctionService{}
	{
		svc := functionPythonMockSvc
		sf := func(_ context.Context, _ functionpythonctl.SnowflakeClient, _ string) (functionpythonctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := functionpythonctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup functionpython controller: " + err.Error())
		}
	}

	// --- FunctionScala controller ---
	functionScalaMockSvc = &mockFunctionService{}
	{
		svc := functionScalaMockSvc
		sf := func(_ context.Context, _ functionscalactl.SnowflakeClient, _ string) (functionscalactl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := functionscalactl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup functionscala controller: " + err.Error())
		}
	}

	// --- FunctionSQL controller ---
	functionSQLMockSvc = &mockFunctionService{}
	{
		svc := functionSQLMockSvc
		sf := func(_ context.Context, _ functionsqlctl.SnowflakeClient, _ string) (functionsqlctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := functionsqlctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup functionsql controller: " + err.Error())
		}
	}

	// --- ProcedureJava controller ---
	procedureJavaMockSvc = &mockProcedureService{}
	{
		svc := procedureJavaMockSvc
		sf := func(_ context.Context, _ procedurejavactl.SnowflakeClient, _ string) (procedurejavactl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := procedurejavactl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup procedurejava controller: " + err.Error())
		}
	}

	// --- ProcedureJavascript controller ---
	procedureJavascriptMockSvc = &mockProcedureService{}
	{
		svc := procedureJavascriptMockSvc
		sf := func(_ context.Context, _ procedurejavascriptctl.SnowflakeClient, _ string) (procedurejavascriptctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := procedurejavascriptctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup procedurejavascript controller: " + err.Error())
		}
	}

	// --- ProcedurePython controller ---
	procedurePythonMockSvc = &mockProcedureService{}
	{
		svc := procedurePythonMockSvc
		sf := func(_ context.Context, _ procedurepythonctl.SnowflakeClient, _ string) (procedurepythonctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := procedurepythonctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup procedurepython controller: " + err.Error())
		}
	}

	// --- ProcedureScala controller ---
	procedureScalaMockSvc = &mockProcedureService{}
	{
		svc := procedureScalaMockSvc
		sf := func(_ context.Context, _ procedurescalactl.SnowflakeClient, _ string) (procedurescalactl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := procedurescalactl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup procedurescala controller: " + err.Error())
		}
	}

	// --- ProcedureSQL controller ---
	procedureSQLMockSvc = &mockProcedureService{}
	{
		svc := procedureSQLMockSvc
		sf := func(_ context.Context, _ proceduresqlctl.SnowflakeClient, _ string) (proceduresqlctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := proceduresqlctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup proceduresql controller: " + err.Error())
		}
	}

	// --- SecretWithAuthorizationCodeGrant controller ---
	secretAuthCodeGrantMockSvc = &mockSecretService{}
	{
		svc := secretAuthCodeGrantMockSvc
		sf := func(_ context.Context, _ secretauthcodectl.SnowflakeClient, _ string) (secretauthcodectl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := secretauthcodectl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup secretwithauthorizationcodegrant controller: " + err.Error())
		}
	}

	// --- SecretWithBasicAuthentication controller ---
	secretBasicAuthMockSvc = &mockSecretService{}
	{
		svc := secretBasicAuthMockSvc
		sf := func(_ context.Context, _ secretbasicauthctl.SnowflakeClient, _ string) (secretbasicauthctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := secretbasicauthctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup secretwithbasicauthentication controller: " + err.Error())
		}
	}

	// --- SecretWithClientCredentials controller ---
	secretClientCredsMockSvc = &mockSecretService{}
	{
		svc := secretClientCredsMockSvc
		sf := func(_ context.Context, _ secretclientcredsctl.SnowflakeClient, _ string) (secretclientcredsctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := secretclientcredsctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup secretwithclientcredentials controller: " + err.Error())
		}
	}

	// --- SecretWithGenericString controller ---
	secretGenericStringMockSvc = &mockSecretService{}
	{
		svc := secretGenericStringMockSvc
		sf := func(_ context.Context, _ secretgenericstringctl.SnowflakeClient, _ string) (secretgenericstringctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := secretgenericstringctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup secretwithgenericstring controller: " + err.Error())
		}
	}

	// --- StreamOnDirectoryTable controller ---
	streamOnDirectoryTableMockSvc = &mockStreamService{}
	{
		svc := streamOnDirectoryTableMockSvc
		sf := func(_ context.Context, _ streamondirtablectl.SnowflakeClient, _ string) (streamondirtablectl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := streamondirtablectl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup streamondirectorytable controller: " + err.Error())
		}
	}

	// --- StreamOnDynamicTable controller ---
	streamOnDynamicTableMockSvc = &mockStreamService{}
	{
		svc := streamOnDynamicTableMockSvc
		sf := func(_ context.Context, _ streamondyntablectl.SnowflakeClient, _ string) (streamondyntablectl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := streamondyntablectl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup streamondynamictable controller: " + err.Error())
		}
	}

	// --- StreamOnExternalTable controller ---
	streamOnExternalTableMockSvc = &mockStreamService{}
	{
		svc := streamOnExternalTableMockSvc
		sf := func(_ context.Context, _ streamonexttablectl.SnowflakeClient, _ string) (streamonexttablectl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := streamonexttablectl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup streamonexternaltable controller: " + err.Error())
		}
	}

	// --- StreamOnTable controller ---
	streamOnTableMockSvc = &mockStreamService{}
	{
		svc := streamOnTableMockSvc
		sf := func(_ context.Context, _ streamontablectl.SnowflakeClient, _ string) (streamontablectl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := streamontablectl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup streamontable controller: " + err.Error())
		}
	}

	// --- StreamOnView controller ---
	streamOnViewMockSvc = &mockStreamService{}
	{
		svc := streamOnViewMockSvc
		sf := func(_ context.Context, _ streamonviewctl.SnowflakeClient, _ string) (streamonviewctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := streamonviewctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup streamonview controller: " + err.Error())
		}
	}

	// --- APIAuthenticationIntegration with AuthorizationCodeGrant controller ---
	apiAuthCodeGrantMockSvc = &mockAPIAuthIntegrationService{}
	{
		svc := apiAuthCodeGrantMockSvc
		sf := func(_ context.Context, _ apiauthcodegrantctl.SnowflakeClient, _ string) (apiauthcodegrantctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := apiauthcodegrantctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup apiauthenticationintegrationwithauthorizationcodegrant controller: " + err.Error())
		}
	}

	// --- APIAuthenticationIntegration with ClientCredentials controller ---
	apiAuthClientCredsMockSvc = &mockAPIAuthIntegrationService{}
	{
		svc := apiAuthClientCredsMockSvc
		sf := func(_ context.Context, _ apiauthclientcredsctl.SnowflakeClient, _ string) (apiauthclientcredsctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := apiauthclientcredsctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup apiauthenticationintegrationwithclientcredentials controller: " + err.Error())
		}
	}

	// --- APIAuthenticationIntegration with JWTBearer controller ---
	apiAuthJWTBearerMockSvc = &mockAPIAuthIntegrationService{}
	{
		svc := apiAuthJWTBearerMockSvc
		sf := func(_ context.Context, _ apiauthjwtbearerctl.SnowflakeClient, _ string) (apiauthjwtbearerctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := apiauthjwtbearerctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup apiauthenticationintegrationwithjwtbearer controller: " + err.Error())
		}
	}

	// --- ExternalOAuthIntegration controller ---
	externalOAuthMockSvc = &mockExternalOAuthIntegrationService{}
	{
		svc := externalOAuthMockSvc
		sf := func(_ context.Context, _ externaloauthctl.SnowflakeClient, _ string) (externaloauthctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := externaloauthctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup externaloauthintegration controller: " + err.Error())
		}
	}

	// --- SAML2Integration controller ---
	saml2MockSvc = &mockSAML2IntegrationService{}
	{
		svc := saml2MockSvc
		sf := func(_ context.Context, _ saml2ctl.SnowflakeClient, _ string) (saml2ctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := saml2ctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup saml2integration controller: " + err.Error())
		}
	}

	// --- ExternalTable controller ---
	externalTableMockSvc = &mockExternalTableService{}
	{
		svc := externalTableMockSvc
		sf := func(_ context.Context, _ externaltablectl.SnowflakeClient, _ string) (externaltablectl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := externaltablectl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup externaltable controller: " + err.Error())
		}
	}

	// --- MaterializedView controller ---
	materializedViewMockSvc = &mockMaterializedViewService{}
	{
		svc := materializedViewMockSvc
		sf := func(_ context.Context, _ materializedviewctl.SnowflakeClient, _ string) (materializedviewctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := materializedviewctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup materializedview controller: " + err.Error())
		}
	}

	// --- NetworkRule controller ---
	networkRuleMockSvc = &mockNetworkRuleService{}
	{
		svc := networkRuleMockSvc
		sf := func(_ context.Context, _ networkrulectl.SnowflakeClient, _ string) (networkrulectl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := networkrulectl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup networkrule controller: " + err.Error())
		}
	}

	// --- Sequence controller ---
	sequenceMockSvc = &mockSequenceService{}
	{
		svc := sequenceMockSvc
		sf := func(_ context.Context, _ sequencectl.SnowflakeClient, _ string) (sequencectl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := sequencectl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup sequence controller: " + err.Error())
		}
	}

	// --- FailoverGroup controller ---
	failoverGroupMockSvc = &mockFailoverGroupService{}
	{
		svc := failoverGroupMockSvc
		sf := func(_ context.Context, _ failovergroupctl.SnowflakeClient, _ string) (failovergroupctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := failovergroupctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup failovergroup controller: " + err.Error())
		}
	}

	// --- MaskingPolicyApplication controller ---
	maskingPolicyAppMockSvc = &mockMaskingPolicyApplicationService{}
	{
		svc := maskingPolicyAppMockSvc
		sf := func(_ context.Context, _ maskingpolicyappctl.SnowflakeClient, _ string) (maskingpolicyappctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := maskingpolicyappctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup maskingpolicyapplication controller: " + err.Error())
		}
	}

	// --- NetworkPolicyAttachment controller ---
	networkPolicyAttachMockSvc = &mockNetworkPolicyAttachmentService{}
	{
		svc := networkPolicyAttachMockSvc
		sf := func(_ context.Context, _ networkpolicyattachctl.SnowflakeClient, _ string) (networkpolicyattachctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := networkpolicyattachctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup networkpolicyattachment controller: " + err.Error())
		}
	}

	// --- PasswordPolicyAttachment controller ---
	passwordPolicyAttachMockSvc = &mockPasswordPolicyAttachmentService{}
	{
		svc := passwordPolicyAttachMockSvc
		sf := func(_ context.Context, _ passwordpolicyattachctl.SnowflakeClient, _ string) (passwordpolicyattachctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := passwordpolicyattachctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup passwordpolicyattachment controller: " + err.Error())
		}
	}

	// --- TagAssociation controller ---
	tagAssociationMockSvc = &mockTagAssociationService{}
	{
		svc := tagAssociationMockSvc
		sf := func(_ context.Context, _ tagassociationctl.SnowflakeClient, _ string) (tagassociationctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := tagassociationctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup tagassociation controller: " + err.Error())
		}
	}

	// --- TableConstraint controller ---
	tableConstraintMockSvc = &mockTableConstraintService{}
	{
		svc := tableConstraintMockSvc
		sf := func(_ context.Context, _ tableconstraintctl.SnowflakeClient, _ string) (tableconstraintctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := tableconstraintctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup tableconstraint controller: " + err.Error())
		}
	}

	// --- SQLStatement controller ---
	sqlStatementMockSvc = &mockSQLStatementService{}
	{
		svc := sqlStatementMockSvc
		sf := func(_ context.Context, _ sqlstatementctl.SnowflakeClient, _ string) (sqlstatementctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := sqlstatementctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, nil, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup sqlstatement controller: " + err.Error())
		}
	}

	// --- ExternalVolume controller ---
	externalVolumeMockSvc = &mockExternalVolumeService{}
	{
		svc := externalVolumeMockSvc
		sf := func(_ context.Context, _ externalvolumectl.SnowflakeClient, _ string) (externalvolumectl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := externalvolumectl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup externalvolume controller: " + err.Error())
		}
	}

	// --- CortexSearchService controller ---
	cortexSearchServiceMockSvc = &mockCortexSearchServiceService{}
	{
		svc := cortexSearchServiceMockSvc
		sf := func(_ context.Context, _ cortexsearchservicectl.SnowflakeClient, _ string) (cortexsearchservicectl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := cortexsearchservicectl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup cortexsearchservice controller: " + err.Error())
		}
	}

	// --- GitRepository controller ---
	gitRepositoryMockSvc = &mockGitRepositoryService{}
	{
		svc := gitRepositoryMockSvc
		sf := func(_ context.Context, _ gitrepositoryctl.SnowflakeClient, _ string) (gitrepositoryctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := gitrepositoryctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup gitrepository controller: " + err.Error())
		}
	}

	// --- Streamlit controller ---
	streamlitMockSvc = &mockStreamlitService{}
	{
		svc := streamlitMockSvc
		sf := func(_ context.Context, _ streamlitctl.SnowflakeClient, _ string) (streamlitctl.Service, func(context.Context), error) {
			return svc, func(context.Context) {}, nil
		}
		r := streamlitctl.NewReconcilerWithServiceFactory(mgr.GetClient(), factory, recorder, rl, sf).WithRequeueInterval(500 * time.Millisecond).WithAlphaEnabled(true)
		if err := r.SetupWithManager(mgr, 1); err != nil {
			panic("failed to setup streamlit controller: " + err.Error())
		}
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
