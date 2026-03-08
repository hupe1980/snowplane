//go:build integration

package integration

import (
	"context"
	"sync"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	sqlstmtclient "github.com/hupe1980/snowplane/internal/clients/snowflake/sqlstatement"
)

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group A: Functions (5 controllers share identical interface)            │
// └──────────────────────────────────────────────────────────────────────────┘

type mockFunctionService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) (*snowflake.FunctionObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateFunctionOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterFunctionOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) error
}

func (m *mockFunctionService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) (*snowflake.FunctionObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name, argTypes)
	}

	return &snowflake.FunctionObservation{Exists: false}, nil
}

func (m *mockFunctionService) Create(ctx context.Context, opts snowflake.CreateFunctionOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockFunctionService) Alter(ctx context.Context, opts snowflake.AlterFunctionOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockFunctionService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name, argTypes)
	}

	return nil
}

func (m *mockFunctionService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) (*snowflake.FunctionObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockFunctionService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateFunctionOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockFunctionService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockFunctionService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group B: Procedures (5 controllers share identical interface)           │
// └──────────────────────────────────────────────────────────────────────────┘

type mockProcedureService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) (*snowflake.ProcedureObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateProcedureOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterProcedureOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) error
}

func (m *mockProcedureService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) (*snowflake.ProcedureObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name, argTypes)
	}

	return &snowflake.ProcedureObservation{Exists: false}, nil
}

func (m *mockProcedureService) Create(ctx context.Context, opts snowflake.CreateProcedureOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockProcedureService) Alter(ctx context.Context, opts snowflake.AlterProcedureOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockProcedureService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name, argTypes)
	}

	return nil
}

func (m *mockProcedureService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) (*snowflake.ProcedureObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockProcedureService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateProcedureOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockProcedureService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockProcedureService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group C: Secrets (4 controllers share identical interface)              │
// └──────────────────────────────────────────────────────────────────────────┘

type mockSecretService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSecretOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSecretOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockSecretService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.SecretObservation{Exists: false}, nil
}

func (m *mockSecretService) Create(ctx context.Context, opts snowflake.CreateSecretOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSecretService) Alter(ctx context.Context, opts snowflake.AlterSecretOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSecretService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockSecretService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockSecretService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateSecretOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockSecretService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockSecretService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group D: Streams (5 controllers share identical interface)              │
// └──────────────────────────────────────────────────────────────────────────┘

type mockStreamService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateStreamOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterStreamOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockStreamService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.StreamObservation{Exists: false}, nil
}

func (m *mockStreamService) Create(ctx context.Context, opts snowflake.CreateStreamOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockStreamService) Alter(ctx context.Context, opts snowflake.AlterStreamOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockStreamService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockStreamService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockStreamService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateStreamOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockStreamService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockStreamService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group E: API Authentication Integrations (3 controllers, same iface)   │
// └──────────────────────────────────────────────────────────────────────────┘

type mockAPIAuthIntegrationService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateAPIAuthenticationIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterAPIAuthenticationIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockAPIAuthIntegrationService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.APIAuthenticationIntegrationObservation{Exists: false}, nil
}

func (m *mockAPIAuthIntegrationService) Create(ctx context.Context, opts snowflake.CreateAPIAuthenticationIntegrationOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockAPIAuthIntegrationService) Alter(ctx context.Context, opts snowflake.AlterAPIAuthenticationIntegrationOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockAPIAuthIntegrationService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockAPIAuthIntegrationService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockAPIAuthIntegrationService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateAPIAuthenticationIntegrationOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockAPIAuthIntegrationService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockAPIAuthIntegrationService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group F: Unique account-scoped — ExternalOAuth, SAML2, FailoverGroup  │
// └──────────────────────────────────────────────────────────────────────────┘

type mockExternalOAuthIntegrationService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateExternalOAuthIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterExternalOAuthIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockExternalOAuthIntegrationService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.ExternalOAuthIntegrationObservation{Exists: false}, nil
}

func (m *mockExternalOAuthIntegrationService) Create(ctx context.Context, opts snowflake.CreateExternalOAuthIntegrationOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockExternalOAuthIntegrationService) Alter(ctx context.Context, opts snowflake.AlterExternalOAuthIntegrationOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockExternalOAuthIntegrationService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockExternalOAuthIntegrationService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockExternalOAuthIntegrationService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateExternalOAuthIntegrationOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockExternalOAuthIntegrationService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockExternalOAuthIntegrationService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// --- SAML2 ---

type mockSAML2IntegrationService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SAML2IntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSAML2IntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSAML2IntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockSAML2IntegrationService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SAML2IntegrationObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.SAML2IntegrationObservation{Exists: false}, nil
}

func (m *mockSAML2IntegrationService) Create(ctx context.Context, opts snowflake.CreateSAML2IntegrationOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSAML2IntegrationService) Alter(ctx context.Context, opts snowflake.AlterSAML2IntegrationOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSAML2IntegrationService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockSAML2IntegrationService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SAML2IntegrationObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockSAML2IntegrationService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateSAML2IntegrationOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockSAML2IntegrationService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockSAML2IntegrationService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// --- FailoverGroup ---

type mockFailoverGroupService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateFailoverGroupOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterFailoverGroupOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockFailoverGroupService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.FailoverGroupObservation{Exists: false}, nil
}

func (m *mockFailoverGroupService) Create(ctx context.Context, opts snowflake.CreateFailoverGroupOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockFailoverGroupService) Alter(ctx context.Context, opts snowflake.AlterFailoverGroupOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockFailoverGroupService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockFailoverGroupService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockFailoverGroupService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateFailoverGroupOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockFailoverGroupService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockFailoverGroupService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group G: Schema-scoped standard — ExternalTable, MaterializedView,     │
// │          NetworkRule, Sequence                                          │
// └──────────────────────────────────────────────────────────────────────────┘

type mockExternalTableService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ExternalTableObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateExternalTableOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterExternalTableOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockExternalTableService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ExternalTableObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.ExternalTableObservation{Exists: false}, nil
}

func (m *mockExternalTableService) Create(ctx context.Context, opts snowflake.CreateExternalTableOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockExternalTableService) Alter(ctx context.Context, opts snowflake.AlterExternalTableOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockExternalTableService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockExternalTableService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ExternalTableObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockExternalTableService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateExternalTableOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockExternalTableService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockExternalTableService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// --- MaterializedView ---

type mockMaterializedViewService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateMaterializedViewOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterMaterializedViewOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockMaterializedViewService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.MaterializedViewObservation{Exists: false}, nil
}

func (m *mockMaterializedViewService) Create(ctx context.Context, opts snowflake.CreateMaterializedViewOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockMaterializedViewService) Alter(ctx context.Context, opts snowflake.AlterMaterializedViewOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockMaterializedViewService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockMaterializedViewService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockMaterializedViewService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateMaterializedViewOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockMaterializedViewService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockMaterializedViewService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// --- NetworkRule ---

type mockNetworkRuleService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateNetworkRuleOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterNetworkRuleOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockNetworkRuleService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.NetworkRuleObservation{Exists: false}, nil
}

func (m *mockNetworkRuleService) Create(ctx context.Context, opts snowflake.CreateNetworkRuleOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockNetworkRuleService) Alter(ctx context.Context, opts snowflake.AlterNetworkRuleOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockNetworkRuleService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockNetworkRuleService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockNetworkRuleService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateNetworkRuleOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockNetworkRuleService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockNetworkRuleService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// --- Sequence ---

type mockSequenceService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SequenceObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSequenceOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSequenceOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockSequenceService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SequenceObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.SequenceObservation{Exists: false}, nil
}

func (m *mockSequenceService) Create(ctx context.Context, opts snowflake.CreateSequenceOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSequenceService) Alter(ctx context.Context, opts snowflake.AlterSequenceOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockSequenceService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockSequenceService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SequenceObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockSequenceService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateSequenceOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockSequenceService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockSequenceService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group H: Policy Attachments — Set/Unset pattern                        │
// └──────────────────────────────────────────────────────────────────────────┘

type mockMaskingPolicyApplicationService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, id snowflake.MaskingPolicyApplicationIdentifier) (*snowflake.MaskingPolicyApplicationObservation, error)
	setFn     func(ctx context.Context, opts snowflake.SetMaskingPolicyOptions) error
	unsetFn   func(ctx context.Context, opts snowflake.UnsetMaskingPolicyOptions) error
}

func (m *mockMaskingPolicyApplicationService) Observe(ctx context.Context, id snowflake.MaskingPolicyApplicationIdentifier) (*snowflake.MaskingPolicyApplicationObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, id)
	}

	return &snowflake.MaskingPolicyApplicationObservation{Exists: false}, nil
}

func (m *mockMaskingPolicyApplicationService) SetMaskingPolicy(ctx context.Context, opts snowflake.SetMaskingPolicyOptions) error {
	m.mu.Lock()
	fn := m.setFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockMaskingPolicyApplicationService) UnsetMaskingPolicy(ctx context.Context, opts snowflake.UnsetMaskingPolicyOptions) error {
	m.mu.Lock()
	fn := m.unsetFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockMaskingPolicyApplicationService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.setFn = nil
	m.unsetFn = nil
}

// --- NetworkPolicyAttachment ---

type mockNetworkPolicyAttachmentService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, id snowflake.NetworkPolicyAttachmentIdentifier) (*snowflake.NetworkPolicyAttachmentObservation, error)
	setFn     func(ctx context.Context, opts snowflake.SetNetworkPolicyOptions) error
	unsetFn   func(ctx context.Context, opts snowflake.UnsetNetworkPolicyOptions) error
}

func (m *mockNetworkPolicyAttachmentService) Observe(ctx context.Context, id snowflake.NetworkPolicyAttachmentIdentifier) (*snowflake.NetworkPolicyAttachmentObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, id)
	}

	return &snowflake.NetworkPolicyAttachmentObservation{Exists: false}, nil
}

func (m *mockNetworkPolicyAttachmentService) SetNetworkPolicy(ctx context.Context, opts snowflake.SetNetworkPolicyOptions) error {
	m.mu.Lock()
	fn := m.setFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockNetworkPolicyAttachmentService) UnsetNetworkPolicy(ctx context.Context, opts snowflake.UnsetNetworkPolicyOptions) error {
	m.mu.Lock()
	fn := m.unsetFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockNetworkPolicyAttachmentService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.setFn = nil
	m.unsetFn = nil
}

// --- PasswordPolicyAttachment ---

type mockPasswordPolicyAttachmentService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, id snowflake.PasswordPolicyAttachmentIdentifier) (*snowflake.PasswordPolicyAttachmentObservation, error)
	setFn     func(ctx context.Context, opts snowflake.SetPasswordPolicyOptions) error
	unsetFn   func(ctx context.Context, opts snowflake.UnsetPasswordPolicyOptions) error
}

func (m *mockPasswordPolicyAttachmentService) Observe(ctx context.Context, id snowflake.PasswordPolicyAttachmentIdentifier) (*snowflake.PasswordPolicyAttachmentObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, id)
	}

	return &snowflake.PasswordPolicyAttachmentObservation{Exists: false}, nil
}

func (m *mockPasswordPolicyAttachmentService) SetPasswordPolicy(ctx context.Context, opts snowflake.SetPasswordPolicyOptions) error {
	m.mu.Lock()
	fn := m.setFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockPasswordPolicyAttachmentService) UnsetPasswordPolicy(ctx context.Context, opts snowflake.UnsetPasswordPolicyOptions) error {
	m.mu.Lock()
	fn := m.unsetFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockPasswordPolicyAttachmentService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.setFn = nil
	m.unsetFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group I: TagAssociation — SetTag/UnsetTag pattern                      │
// └──────────────────────────────────────────────────────────────────────────┘

type mockTagAssociationService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, id snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error)
	setFn     func(ctx context.Context, opts snowflake.SetTagOptions) error
	unsetFn   func(ctx context.Context, opts snowflake.UnsetTagOptions) error
}

func (m *mockTagAssociationService) Observe(ctx context.Context, id snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, id)
	}

	return &snowflake.TagAssociationObservation{Exists: false}, nil
}

func (m *mockTagAssociationService) SetTag(ctx context.Context, opts snowflake.SetTagOptions) error {
	m.mu.Lock()
	fn := m.setFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockTagAssociationService) UnsetTag(ctx context.Context, opts snowflake.UnsetTagOptions) error {
	m.mu.Lock()
	fn := m.unsetFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockTagAssociationService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.setFn = nil
	m.unsetFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group J: TableConstraint — AddConstraint/AlterConstraint/Drop          │
// └──────────────────────────────────────────────────────────────────────────┘

type mockTableConstraintService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, id snowflake.TableConstraintIdentifier, constraintType string) (*snowflake.TableConstraintObservation, error)
	addFn     func(ctx context.Context, opts snowflake.AddConstraintOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterConstraintOptions) error
	dropFn    func(ctx context.Context, id snowflake.TableConstraintIdentifier) error
}

func (m *mockTableConstraintService) Observe(ctx context.Context, id snowflake.TableConstraintIdentifier, constraintType string) (*snowflake.TableConstraintObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, id, constraintType)
	}

	return &snowflake.TableConstraintObservation{Exists: false}, nil
}

func (m *mockTableConstraintService) AddConstraint(ctx context.Context, opts snowflake.AddConstraintOptions) error {
	m.mu.Lock()
	fn := m.addFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockTableConstraintService) AlterConstraint(ctx context.Context, opts snowflake.AlterConstraintOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockTableConstraintService) DropConstraint(ctx context.Context, id snowflake.TableConstraintIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, id)
	}

	return nil
}

func (m *mockTableConstraintService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.addFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group K: SQLStatement — Execute/Revert/Observe                         │
// └──────────────────────────────────────────────────────────────────────────┘

type mockSQLStatementService struct {
	mu        sync.Mutex
	executeFn func(ctx context.Context, sql string) error
	revertFn  func(ctx context.Context, sql string) error
	observeFn func(ctx context.Context, observeSQL string, expectations []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error)
}

func (m *mockSQLStatementService) Execute(ctx context.Context, sql string) error {
	m.mu.Lock()
	fn := m.executeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, sql)
	}

	return nil
}

func (m *mockSQLStatementService) Revert(ctx context.Context, sql string) error {
	m.mu.Lock()
	fn := m.revertFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, sql)
	}

	return nil
}

func (m *mockSQLStatementService) Observe(ctx context.Context, observeSQL string, expectations []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, observeSQL, expectations)
	}

	return &sqlstmtclient.Observation{Exists: true, Matched: true}, nil
}

func (m *mockSQLStatementService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executeFn = nil
	m.revertFn = nil
	m.observeFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group I: Account-scoped — ExternalVolume                               │
// └──────────────────────────────────────────────────────────────────────────┘

type mockExternalVolumeService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ExternalVolumeObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateExternalVolumeOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterExternalVolumeOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockExternalVolumeService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ExternalVolumeObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.ExternalVolumeObservation{Exists: false}, nil
}

func (m *mockExternalVolumeService) Create(ctx context.Context, opts snowflake.CreateExternalVolumeOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockExternalVolumeService) Alter(ctx context.Context, opts snowflake.AlterExternalVolumeOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockExternalVolumeService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockExternalVolumeService) SetObserve(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ExternalVolumeObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockExternalVolumeService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateExternalVolumeOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockExternalVolumeService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterExternalVolumeOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alterFn = fn
}

func (m *mockExternalVolumeService) SetDrop(fn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockExternalVolumeService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group J: Schema-scoped — CortexSearchService                           │
// └──────────────────────────────────────────────────────────────────────────┘

type mockCortexSearchServiceService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.CortexSearchServiceObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateCortexSearchServiceOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterCortexSearchServiceOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockCortexSearchServiceService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.CortexSearchServiceObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.CortexSearchServiceObservation{Exists: false}, nil
}

func (m *mockCortexSearchServiceService) Create(ctx context.Context, opts snowflake.CreateCortexSearchServiceOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockCortexSearchServiceService) Alter(ctx context.Context, opts snowflake.AlterCortexSearchServiceOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockCortexSearchServiceService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockCortexSearchServiceService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.CortexSearchServiceObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockCortexSearchServiceService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateCortexSearchServiceOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockCortexSearchServiceService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterCortexSearchServiceOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alterFn = fn
}

func (m *mockCortexSearchServiceService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockCortexSearchServiceService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group K: Schema-scoped — GitRepository                                 │
// └──────────────────────────────────────────────────────────────────────────┘

type mockGitRepositoryService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.GitRepositoryObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateGitRepositoryOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterGitRepositoryOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockGitRepositoryService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.GitRepositoryObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.GitRepositoryObservation{Exists: false}, nil
}

func (m *mockGitRepositoryService) Create(ctx context.Context, opts snowflake.CreateGitRepositoryOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockGitRepositoryService) Alter(ctx context.Context, opts snowflake.AlterGitRepositoryOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockGitRepositoryService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockGitRepositoryService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.GitRepositoryObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockGitRepositoryService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateGitRepositoryOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockGitRepositoryService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterGitRepositoryOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alterFn = fn
}

func (m *mockGitRepositoryService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockGitRepositoryService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ Group L: Schema-scoped — Streamlit                                     │
// └──────────────────────────────────────────────────────────────────────────┘

type mockStreamlitService struct {
	mu        sync.Mutex
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamlitObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateStreamlitOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterStreamlitOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockStreamlitService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamlitObservation, error) {
	m.mu.Lock()
	fn := m.observeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return &snowflake.StreamlitObservation{Exists: false}, nil
}

func (m *mockStreamlitService) Create(ctx context.Context, opts snowflake.CreateStreamlitOptions) error {
	m.mu.Lock()
	fn := m.createFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockStreamlitService) Alter(ctx context.Context, opts snowflake.AlterStreamlitOptions) error {
	m.mu.Lock()
	fn := m.alterFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, opts)
	}

	return nil
}

func (m *mockStreamlitService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	m.mu.Lock()
	fn := m.dropFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, name)
	}

	return nil
}

func (m *mockStreamlitService) SetObserve(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamlitObservation, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = fn
}

func (m *mockStreamlitService) SetCreate(fn func(ctx context.Context, opts snowflake.CreateStreamlitOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFn = fn
}

func (m *mockStreamlitService) SetAlter(fn func(ctx context.Context, opts snowflake.AlterStreamlitOptions) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alterFn = fn
}

func (m *mockStreamlitService) SetDrop(fn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropFn = fn
}

func (m *mockStreamlitService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeFn = nil
	m.createFn = nil
	m.alterFn = nil
	m.dropFn = nil
}

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ resetExtendedMocks — called from resetMocks                            │
// └──────────────────────────────────────────────────────────────────────────┘

func resetExtendedMocks() {
	// Functions (5 instances)
	functionJavaMockSvc.Reset()
	functionJavascriptMockSvc.Reset()
	functionPythonMockSvc.Reset()
	functionScalaMockSvc.Reset()
	functionSQLMockSvc.Reset()
	// Procedures (5 instances)
	procedureJavaMockSvc.Reset()
	procedureJavascriptMockSvc.Reset()
	procedurePythonMockSvc.Reset()
	procedureScalaMockSvc.Reset()
	procedureSQLMockSvc.Reset()
	// Secrets (4 instances)
	secretAuthCodeGrantMockSvc.Reset()
	secretBasicAuthMockSvc.Reset()
	secretClientCredsMockSvc.Reset()
	secretGenericStringMockSvc.Reset()
	// Streams (5 instances)
	streamOnDirectoryTableMockSvc.Reset()
	streamOnDynamicTableMockSvc.Reset()
	streamOnExternalTableMockSvc.Reset()
	streamOnTableMockSvc.Reset()
	streamOnViewMockSvc.Reset()
	// API Auth Integrations (3 instances)
	apiAuthCodeGrantMockSvc.Reset()
	apiAuthClientCredsMockSvc.Reset()
	apiAuthJWTBearerMockSvc.Reset()
	// Security integrations
	externalOAuthMockSvc.Reset()
	saml2MockSvc.Reset()
	// Schema-scoped standard
	externalTableMockSvc.Reset()
	materializedViewMockSvc.Reset()
	networkRuleMockSvc.Reset()
	sequenceMockSvc.Reset()
	// Account-scoped
	failoverGroupMockSvc.Reset()
	// Policy attachments
	maskingPolicyAppMockSvc.Reset()
	networkPolicyAttachMockSvc.Reset()
	passwordPolicyAttachMockSvc.Reset()
	// Special
	tagAssociationMockSvc.Reset()
	tableConstraintMockSvc.Reset()
	sqlStatementMockSvc.Reset()
	// New CRDs
	externalVolumeMockSvc.Reset()
	cortexSearchServiceMockSvc.Reset()
	gitRepositoryMockSvc.Reset()
	streamlitMockSvc.Reset()
}
