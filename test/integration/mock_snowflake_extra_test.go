// mock_snowflake_extra_test.go adds mock services for all resource types
// not covered by the original mock_snowflake_test.go.
//
//go:build integration

package integration

import (
	"context"
	"sync"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

// ---------------------------------------------------------------------------
// mockAlertService
// ---------------------------------------------------------------------------

type mockAlertService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateAlertOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterAlertOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockAlertService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.AlertObservation{Exists: false}, nil
}

func (m *mockAlertService) Create(ctx context.Context, opts snowflake.CreateAlertOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockAlertService) Alter(ctx context.Context, opts snowflake.AlterAlertOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockAlertService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockAlertService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockAlertService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateAlertOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockAlertService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterAlertOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockAlertService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockAlertService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockTaskService
// ---------------------------------------------------------------------------

type mockTaskService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateTaskOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterTaskOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockTaskService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.TaskObservation{Exists: false}, nil
}

func (m *mockTaskService) Create(ctx context.Context, opts snowflake.CreateTaskOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockTaskService) Alter(ctx context.Context, opts snowflake.AlterTaskOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockTaskService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockTaskService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockTaskService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateTaskOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockTaskService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterTaskOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockTaskService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockTaskService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockDynamicTableService
// ---------------------------------------------------------------------------

type mockDynamicTableService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateDynamicTableOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterDynamicTableOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockDynamicTableService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.DynamicTableObservation{Exists: false}, nil
}

func (m *mockDynamicTableService) Create(ctx context.Context, opts snowflake.CreateDynamicTableOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockDynamicTableService) Alter(ctx context.Context, opts snowflake.AlterDynamicTableOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockDynamicTableService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockDynamicTableService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockDynamicTableService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateDynamicTableOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockDynamicTableService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterDynamicTableOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockDynamicTableService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockDynamicTableService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockNetworkPolicyService
// ---------------------------------------------------------------------------

type mockNetworkPolicyService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateNetworkPolicyOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterNetworkPolicyOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockNetworkPolicyService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.NetworkPolicyObservation{Exists: false}, nil
}

func (m *mockNetworkPolicyService) Create(ctx context.Context, opts snowflake.CreateNetworkPolicyOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockNetworkPolicyService) Alter(ctx context.Context, opts snowflake.AlterNetworkPolicyOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockNetworkPolicyService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockNetworkPolicyService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockNetworkPolicyService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateNetworkPolicyOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockNetworkPolicyService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterNetworkPolicyOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockNetworkPolicyService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockNetworkPolicyService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockMaskingPolicyService
// ---------------------------------------------------------------------------

type mockMaskingPolicyService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateMaskingPolicyOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterMaskingPolicyOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockMaskingPolicyService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.MaskingPolicyObservation{Exists: false}, nil
}

func (m *mockMaskingPolicyService) Create(ctx context.Context, opts snowflake.CreateMaskingPolicyOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockMaskingPolicyService) Alter(ctx context.Context, opts snowflake.AlterMaskingPolicyOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockMaskingPolicyService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockMaskingPolicyService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockMaskingPolicyService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateMaskingPolicyOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockMaskingPolicyService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterMaskingPolicyOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockMaskingPolicyService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockMaskingPolicyService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockPasswordPolicyService
// ---------------------------------------------------------------------------

type mockPasswordPolicyService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreatePasswordPolicyOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterPasswordPolicyOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockPasswordPolicyService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.PasswordPolicyObservation{Exists: false}, nil
}

func (m *mockPasswordPolicyService) Create(ctx context.Context, opts snowflake.CreatePasswordPolicyOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockPasswordPolicyService) Alter(ctx context.Context, opts snowflake.AlterPasswordPolicyOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockPasswordPolicyService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockPasswordPolicyService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockPasswordPolicyService) SetCreate(fn func(ctx context.Context, opts snowflake.CreatePasswordPolicyOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockPasswordPolicyService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterPasswordPolicyOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockPasswordPolicyService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockPasswordPolicyService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockSecurityIntegrationService
// ---------------------------------------------------------------------------

type mockSecurityIntegrationService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecurityIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSecurityIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSecurityIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockSecurityIntegrationService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecurityIntegrationObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.SecurityIntegrationObservation{Exists: false}, nil
}

func (m *mockSecurityIntegrationService) Create(ctx context.Context, opts snowflake.CreateSecurityIntegrationOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSecurityIntegrationService) Alter(ctx context.Context, opts snowflake.AlterSecurityIntegrationOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSecurityIntegrationService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockSecurityIntegrationService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecurityIntegrationObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockSecurityIntegrationService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateSecurityIntegrationOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockSecurityIntegrationService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterSecurityIntegrationOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockSecurityIntegrationService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockSecurityIntegrationService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockStorageIntegrationService
// ---------------------------------------------------------------------------

type mockStorageIntegrationService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateStorageIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterStorageIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockStorageIntegrationService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.StorageIntegrationObservation{Exists: false}, nil
}

func (m *mockStorageIntegrationService) Create(ctx context.Context, opts snowflake.CreateStorageIntegrationOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockStorageIntegrationService) Alter(ctx context.Context, opts snowflake.AlterStorageIntegrationOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockStorageIntegrationService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockStorageIntegrationService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockStorageIntegrationService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateStorageIntegrationOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockStorageIntegrationService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterStorageIntegrationOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockStorageIntegrationService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockStorageIntegrationService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockResourceMonitorService
// ---------------------------------------------------------------------------

type mockResourceMonitorService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateResourceMonitorOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterResourceMonitorOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockResourceMonitorService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.ResourceMonitorObservation{Exists: false}, nil
}

func (m *mockResourceMonitorService) Create(ctx context.Context, opts snowflake.CreateResourceMonitorOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockResourceMonitorService) Alter(ctx context.Context, opts snowflake.AlterResourceMonitorOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockResourceMonitorService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockResourceMonitorService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockResourceMonitorService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateResourceMonitorOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockResourceMonitorService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterResourceMonitorOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockResourceMonitorService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockResourceMonitorService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockPipeService
// ---------------------------------------------------------------------------

type mockPipeService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PipeObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreatePipeOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterPipeOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockPipeService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PipeObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.PipeObservation{Exists: false}, nil
}

func (m *mockPipeService) Create(ctx context.Context, opts snowflake.CreatePipeOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockPipeService) Alter(ctx context.Context, opts snowflake.AlterPipeOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockPipeService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockPipeService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PipeObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockPipeService) SetCreate(fn func(ctx context.Context, opts snowflake.CreatePipeOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockPipeService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterPipeOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockPipeService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockPipeService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockFileFormatService
// ---------------------------------------------------------------------------

type mockFileFormatService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateFileFormatOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterFileFormatOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockFileFormatService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.FileFormatObservation{Exists: false}, nil
}

func (m *mockFileFormatService) Create(ctx context.Context, opts snowflake.CreateFileFormatOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockFileFormatService) Alter(ctx context.Context, opts snowflake.AlterFileFormatOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockFileFormatService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockFileFormatService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockFileFormatService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateFileFormatOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockFileFormatService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterFileFormatOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockFileFormatService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockFileFormatService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockTagService
// ---------------------------------------------------------------------------

type mockTagService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateTagOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterTagOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockTagService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.TagObservation{Exists: false}, nil
}

func (m *mockTagService) Create(ctx context.Context, opts snowflake.CreateTagOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockTagService) Alter(ctx context.Context, opts snowflake.AlterTagOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockTagService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockTagService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockTagService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateTagOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockTagService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterTagOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockTagService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockTagService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockRowAccessPolicyService
// ---------------------------------------------------------------------------

type mockRowAccessPolicyService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateRowAccessPolicyOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterRowAccessPolicyOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockRowAccessPolicyService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.RowAccessPolicyObservation{Exists: false}, nil
}

func (m *mockRowAccessPolicyService) Create(ctx context.Context, opts snowflake.CreateRowAccessPolicyOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockRowAccessPolicyService) Alter(ctx context.Context, opts snowflake.AlterRowAccessPolicyOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockRowAccessPolicyService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockRowAccessPolicyService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockRowAccessPolicyService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateRowAccessPolicyOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockRowAccessPolicyService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterRowAccessPolicyOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockRowAccessPolicyService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockRowAccessPolicyService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockGrantOwnershipService
// ---------------------------------------------------------------------------

type mockGrantOwnershipService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateGrantOwnershipOptions) error
	dropFn    func(ctx context.Context, id snowflake.GrantOwnershipIdentifier) error
}

func (m *mockGrantOwnershipService) Observe(ctx context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, id)
	}

	return &snowflake.GrantOwnershipObservation{Exists: false}, nil
}

func (m *mockGrantOwnershipService) Create(ctx context.Context, opts snowflake.CreateGrantOwnershipOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockGrantOwnershipService) Drop(ctx context.Context, id snowflake.GrantOwnershipIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, id)
	}

	return nil
}

func (m *mockGrantOwnershipService) SetObserve(fn func(ctx context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockGrantOwnershipService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateGrantOwnershipOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockGrantOwnershipService) SetDrop(fn func(ctx context.Context, id snowflake.GrantOwnershipIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockGrantOwnershipService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockRoleAssignmentService
// ---------------------------------------------------------------------------

type mockRoleAssignmentService struct {
	mu           sync.Mutex
	observeFn    func(ctx context.Context, id snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error)
	grantRoleFn  func(ctx context.Context, opts snowflake.GrantRoleOptions) error
	revokeRoleFn func(ctx context.Context, opts snowflake.RevokeRoleOptions) error
}

func (m *mockRoleAssignmentService) Observe(ctx context.Context, id snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, id)
	}

	return &snowflake.RoleAssignmentObservation{Exists: false}, nil
}

func (m *mockRoleAssignmentService) GrantRole(ctx context.Context, opts snowflake.GrantRoleOptions) error {
	m.mu.Lock()
	fn := m.grantRoleFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockRoleAssignmentService) RevokeRole(ctx context.Context, opts snowflake.RevokeRoleOptions) error {
	m.mu.Lock()
	fn := m.revokeRoleFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockRoleAssignmentService) SetObserve(fn func(ctx context.Context, id snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockRoleAssignmentService) SetGrantRole(fn func(ctx context.Context, opts snowflake.GrantRoleOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.grantRoleFn = fn
}

func (m *mockRoleAssignmentService) SetRevokeRole(fn func(ctx context.Context, opts snowflake.RevokeRoleOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.revokeRoleFn = fn
}

func (m *mockRoleAssignmentService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.grantRoleFn = nil
	m.revokeRoleFn = nil
}

// ---------------------------------------------------------------------------
// mockNotificationIntegrationService
// ---------------------------------------------------------------------------

type mockNotificationIntegrationService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateNotificationIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterNotificationIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockNotificationIntegrationService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.NotificationIntegrationObservation{Exists: false}, nil
}

func (m *mockNotificationIntegrationService) Create(ctx context.Context, opts snowflake.CreateNotificationIntegrationOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockNotificationIntegrationService) Alter(ctx context.Context, opts snowflake.AlterNotificationIntegrationOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockNotificationIntegrationService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockNotificationIntegrationService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockNotificationIntegrationService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateNotificationIntegrationOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockNotificationIntegrationService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterNotificationIntegrationOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockNotificationIntegrationService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockNotificationIntegrationService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockAuthenticationPolicyService
// ---------------------------------------------------------------------------

type mockAuthenticationPolicyService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateAuthenticationPolicyOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterAuthenticationPolicyOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockAuthenticationPolicyService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.AuthenticationPolicyObservation{Exists: false}, nil
}

func (m *mockAuthenticationPolicyService) Create(ctx context.Context, opts snowflake.CreateAuthenticationPolicyOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockAuthenticationPolicyService) Alter(ctx context.Context, opts snowflake.AlterAuthenticationPolicyOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockAuthenticationPolicyService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockAuthenticationPolicyService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockAuthenticationPolicyService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateAuthenticationPolicyOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockAuthenticationPolicyService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterAuthenticationPolicyOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockAuthenticationPolicyService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockAuthenticationPolicyService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockAPIIntegrationService — thread-safe mock for APIIntegration Snowflake operations
// ---------------------------------------------------------------------------

type mockAPIIntegrationService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateAPIIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterAPIIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockAPIIntegrationService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.APIIntegrationObservation{Exists: false}, nil
}

func (m *mockAPIIntegrationService) Create(ctx context.Context, opts snowflake.CreateAPIIntegrationOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockAPIIntegrationService) Alter(ctx context.Context, opts snowflake.AlterAPIIntegrationOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockAPIIntegrationService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockAPIIntegrationService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockAPIIntegrationService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateAPIIntegrationOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockAPIIntegrationService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterAPIIntegrationOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockAPIIntegrationService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockAPIIntegrationService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockSecondaryDatabaseService — thread-safe mock for SecondaryDatabase Snowflake operations
// ---------------------------------------------------------------------------

type mockSecondaryDatabaseService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSecondaryDatabaseOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSecondaryDatabaseOptions) error
	refreshFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockSecondaryDatabaseService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.SecondaryDatabaseObservation{Exists: false}, nil
}

func (m *mockSecondaryDatabaseService) Create(ctx context.Context, opts snowflake.CreateSecondaryDatabaseOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSecondaryDatabaseService) Alter(ctx context.Context, opts snowflake.AlterSecondaryDatabaseOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSecondaryDatabaseService) Refresh(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.refreshFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockSecondaryDatabaseService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockSecondaryDatabaseService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockSecondaryDatabaseService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateSecondaryDatabaseOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockSecondaryDatabaseService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterSecondaryDatabaseOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockSecondaryDatabaseService) SetRefresh(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.refreshFn = fn
}

func (m *mockSecondaryDatabaseService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockSecondaryDatabaseService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.refreshFn = nil
	m.dropFn = nil
}

// ---------------------------------------------------------------------------
// mockSharedDatabaseService — thread-safe mock for SharedDatabase Snowflake operations
// ---------------------------------------------------------------------------

type mockSharedDatabaseService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSharedDatabaseOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSharedDatabaseOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockSharedDatabaseService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.SharedDatabaseObservation{Exists: false}, nil
}

func (m *mockSharedDatabaseService) Create(ctx context.Context, opts snowflake.CreateSharedDatabaseOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSharedDatabaseService) Alter(ctx context.Context, opts snowflake.AlterSharedDatabaseOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSharedDatabaseService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockSharedDatabaseService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = fn
}

func (m *mockSharedDatabaseService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateSharedDatabaseOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createFn = fn
}

func (m *mockSharedDatabaseService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterSharedDatabaseOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alterFn = fn
}

func (m *mockSharedDatabaseService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropFn = fn
}

func (m *mockSharedDatabaseService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}
