// Package integration provides envtest-based integration tests for Snowplane
// controllers. These tests exercise the full reconciliation lifecycle with a
// real kube-apiserver and etcd (via envtest), while mocking only the Snowflake
// backend. This catches bugs that unit tests with fake.Client cannot, including
// real status subresource patching, field indexer registration, informer caches,
// and cross-resource watch propagation.
//
//go:build integration

package integration

import (
	"context"
	"database/sql"
	"sync"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

// ---------------------------------------------------------------------------
// resetMocks clears all mock service functions to prevent state leaking
// between sequential tests. Call at the start of every test function.
// ---------------------------------------------------------------------------

func resetMocks() {
	dbMockSvc.Reset()
	schemaMockSvc.Reset()
	tableMockSvc.Reset()
	viewMockSvc.Reset()
	stageMockSvc.Reset()
	warehouseMockSvc.Reset()
	userMockSvc.Reset()
	accountRoleMockSvc.Reset()
	databaseRoleMockSvc.Reset()
	grantMockSvc.Reset()
	alertMockSvc.Reset()
	taskMockSvc.Reset()
	dynamicTableMockSvc.Reset()
	networkPolicyMockSvc.Reset()
	maskingPolicyMockSvc.Reset()
	passwordPolicyMockSvc.Reset()
	securityIntegrationMockSvc.Reset()
	notificationIntegrationMockSvc.Reset()
	storageIntegrationMockSvc.Reset()
	resourceMonitorMockSvc.Reset()
	pipeMockSvc.Reset()
	fileFormatMockSvc.Reset()
	tagMockSvc.Reset()
	rowAccessPolicyMockSvc.Reset()
	grantOwnershipMockSvc.Reset()
	roleAssignmentMockSvc.Reset()
	authenticationPolicyMockSvc.Reset()
}

// ---------------------------------------------------------------------------
// mockSnowflakeClient — satisfies clientfactory.SnowflakeClient
// ---------------------------------------------------------------------------

// mockSnowflakeClient satisfies the clientfactory.SnowflakeClient interface.
// It is shared across all reconciled resources but delegates actual operations
// to per-resource mock services configured in each test.
type mockSnowflakeClient struct{}

func (m *mockSnowflakeClient) Ping(_ context.Context) error { return nil }
func (m *mockSnowflakeClient) Close() error                 { return nil }

func (m *mockSnowflakeClient) Exec(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}

func (m *mockSnowflakeClient) QueryRow(_ context.Context, _ string, _ ...any) *snowflake.Row {
	return nil
}

func (m *mockSnowflakeClient) Query(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}

func (m *mockSnowflakeClient) WithRole(_ context.Context, _ string) (*snowflake.Client, func(context.Context), error) {
	// Return nil for the *Client — the mock service factory bypasses this.
	// Provide a no-op cleanup to avoid nil-pointer panics if called unexpectedly.
	return nil, func(context.Context) {}, nil
}

// ---------------------------------------------------------------------------
// mockDatabaseService — thread-safe mock for Database Snowflake operations
// ---------------------------------------------------------------------------

type mockDatabaseService struct {
	mu            sync.Mutex
	observeFn     func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error)
	createFn      func(ctx context.Context, opts snowflake.CreateDatabaseOptions) error
	alterFn       func(ctx context.Context, opts snowflake.AlterDatabaseOptions) error
	dropFn        func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
	dropCascadeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockDatabaseService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.DatabaseObservation{Exists: false}, nil
}

func (m *mockDatabaseService) Create(ctx context.Context, opts snowflake.CreateDatabaseOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockDatabaseService) Alter(ctx context.Context, opts snowflake.AlterDatabaseOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockDatabaseService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockDatabaseService) DropCascade(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropCascadeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockDatabaseService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockDatabaseService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateDatabaseOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockDatabaseService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterDatabaseOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockDatabaseService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockDatabaseService) SetDropCascade(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropCascadeFn = fn
}

// Reset clears all mock functions to their nil defaults.
func (m *mockDatabaseService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
	m.dropCascadeFn = nil
}

// ---------------------------------------------------------------------------
// mockSchemaService — thread-safe mock for Schema Snowflake operations
// ---------------------------------------------------------------------------

type mockSchemaService struct {
	mu            sync.Mutex
	observeFn     func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error)
	createFn      func(ctx context.Context, opts snowflake.CreateSchemaOptions) error
	alterFn       func(ctx context.Context, opts snowflake.AlterSchemaOptions) error
	dropFn        func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error
	dropCascadeFn func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error
}

func (m *mockSchemaService) Observe(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.SchemaObservation{Exists: false}, nil
}

func (m *mockSchemaService) Create(ctx context.Context, opts snowflake.CreateSchemaOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSchemaService) Alter(ctx context.Context, opts snowflake.AlterSchemaOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSchemaService) Drop(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockSchemaService) DropCascade(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropCascadeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockSchemaService) SetObserve(fn func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockSchemaService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateSchemaOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockSchemaService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterSchemaOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockSchemaService) SetDrop(fn func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockSchemaService) SetDropCascade(fn func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropCascadeFn = fn
}

// Reset clears all mock functions to their nil defaults.
func (m *mockSchemaService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
	m.dropCascadeFn = nil
}

// ---------------------------------------------------------------------------
// mockTableService — thread-safe mock for Table Snowflake operations
// ---------------------------------------------------------------------------

type mockTableService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateTableOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterTableOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockTableService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.TableObservation{Exists: false}, nil
}

func (m *mockTableService) Create(ctx context.Context, opts snowflake.CreateTableOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockTableService) Alter(ctx context.Context, opts snowflake.AlterTableOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockTableService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockTableService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockTableService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateTableOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockTableService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterTableOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockTableService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockTableService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockViewService — thread-safe mock for View Snowflake operations
// ---------------------------------------------------------------------------

type mockViewService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateViewOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterViewOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockViewService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.ViewObservation{Exists: false}, nil
}

func (m *mockViewService) Create(ctx context.Context, opts snowflake.CreateViewOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockViewService) Alter(ctx context.Context, opts snowflake.AlterViewOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockViewService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockViewService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockViewService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateViewOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockViewService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterViewOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockViewService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockViewService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockStageService — thread-safe mock for Stage Snowflake operations
// ---------------------------------------------------------------------------

type mockStageService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateStageOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterStageOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockStageService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.StageObservation{Exists: false}, nil
}

func (m *mockStageService) Create(ctx context.Context, opts snowflake.CreateStageOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockStageService) Alter(ctx context.Context, opts snowflake.AlterStageOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockStageService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockStageService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockStageService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateStageOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockStageService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterStageOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockStageService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockStageService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockWarehouseService — thread-safe mock for Warehouse Snowflake operations
// ---------------------------------------------------------------------------

type mockWarehouseService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateWarehouseOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterWarehouseOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockWarehouseService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.WarehouseObservation{Exists: false}, nil
}

func (m *mockWarehouseService) Create(ctx context.Context, opts snowflake.CreateWarehouseOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockWarehouseService) Alter(ctx context.Context, opts snowflake.AlterWarehouseOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockWarehouseService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockWarehouseService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockWarehouseService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateWarehouseOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockWarehouseService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterWarehouseOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockWarehouseService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockWarehouseService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockUserService — thread-safe mock for User Snowflake operations
// ---------------------------------------------------------------------------

type mockUserService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateUserOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterUserOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockUserService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.UserObservation{Exists: false}, nil
}

func (m *mockUserService) Create(ctx context.Context, opts snowflake.CreateUserOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockUserService) Alter(ctx context.Context, opts snowflake.AlterUserOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockUserService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockUserService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockUserService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateUserOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockUserService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterUserOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockUserService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockUserService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockAccountRoleService — thread-safe mock for AccountRole Snowflake operations
// ---------------------------------------------------------------------------

type mockAccountRoleService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateAccountRoleOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterAccountRoleOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockAccountRoleService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.AccountRoleObservation{Exists: false}, nil
}

func (m *mockAccountRoleService) Create(ctx context.Context, opts snowflake.CreateAccountRoleOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockAccountRoleService) Alter(ctx context.Context, opts snowflake.AlterAccountRoleOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockAccountRoleService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockAccountRoleService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockAccountRoleService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateAccountRoleOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockAccountRoleService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterAccountRoleOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockAccountRoleService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockAccountRoleService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockDatabaseRoleService — thread-safe mock for DatabaseRole Snowflake operations
// ---------------------------------------------------------------------------

type mockDatabaseRoleService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateDatabaseRoleOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterDatabaseRoleOptions) error
	dropFn    func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error
}

func (m *mockDatabaseRoleService) Observe(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.DatabaseRoleObservation{Exists: false}, nil
}

func (m *mockDatabaseRoleService) Create(ctx context.Context, opts snowflake.CreateDatabaseRoleOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockDatabaseRoleService) Alter(ctx context.Context, opts snowflake.AlterDatabaseRoleOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockDatabaseRoleService) Drop(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockDatabaseRoleService) SetObserve(fn func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockDatabaseRoleService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateDatabaseRoleOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockDatabaseRoleService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterDatabaseRoleOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockDatabaseRoleService) SetDrop(fn func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockDatabaseRoleService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockGrantService — thread-safe mock for Grant Snowflake operations.
// Grants use Grant/Revoke instead of Create/Drop.
// ---------------------------------------------------------------------------

type mockGrantService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, id snowflake.GrantIdentifier) (*snowflake.GrantObservation, error)
	grantFn   func(ctx context.Context, opts snowflake.CreateGrantOptions) error
	revokeFn  func(ctx context.Context, opts snowflake.RevokeGrantOptions) error
}

func (m *mockGrantService) Observe(ctx context.Context, id snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, id)
	}

	return &snowflake.GrantObservation{Exists: false}, nil
}

func (m *mockGrantService) Grant(ctx context.Context, opts snowflake.CreateGrantOptions) error {
	m.mu.Lock()
	fn := m.grantFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockGrantService) Revoke(ctx context.Context, opts snowflake.RevokeGrantOptions) error {
	m.mu.Lock()
	fn := m.revokeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockGrantService) SetObserve(fn func(ctx context.Context, id snowflake.GrantIdentifier) (*snowflake.GrantObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockGrantService) SetGrant(fn func(ctx context.Context, opts snowflake.CreateGrantOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.grantFn = fn
}

func (m *mockGrantService) SetRevoke(fn func(ctx context.Context, opts snowflake.RevokeGrantOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.revokeFn = fn
}

func (m *mockGrantService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.grantFn = nil
	m.revokeFn = nil
}
