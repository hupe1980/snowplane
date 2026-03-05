package refresolver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

// mockExecutor is a minimal snowflake.SQLExecutor that records queries
// and returns configurable errors.
type mockExecutor struct {
	queryErr error
	queries  []string
}

func (m *mockExecutor) Exec(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}

func (m *mockExecutor) QueryRow(_ context.Context, _ string, _ ...any) *snowflake.Row {
	return nil
}

func (m *mockExecutor) Query(_ context.Context, query string, _ ...any) (*sql.Rows, error) {
	m.queries = append(m.queries, query)
	return nil, m.queryErr
}

// ---------------------------------------------------------------------------
// PreFlightCheckDatabaseExists
// ---------------------------------------------------------------------------

func TestPreFlightCheckDatabaseExists_SkipsWhenRefSet(t *testing.T) {
	exec := &mockExecutor{queryErr: fmt.Errorf("should not be called")}
	ref := &snowplanev1alpha1.ObjectReference{Name: "my-db-cr"}
	name := "SOME_DB"

	err := PreFlightCheckDatabaseExists(context.Background(), exec, ref, &name)
	if err != nil {
		t.Fatalf("expected nil error when ref is set, got: %v", err)
	}

	if len(exec.queries) > 0 {
		t.Fatalf("expected no queries when ref is set, got %d", len(exec.queries))
	}
}

func TestPreFlightCheckDatabaseExists_SkipsWhenNameNil(t *testing.T) {
	exec := &mockExecutor{queryErr: fmt.Errorf("should not be called")}

	err := PreFlightCheckDatabaseExists(context.Background(), exec, nil, nil)
	if err != nil {
		t.Fatalf("expected nil error when name is nil, got: %v", err)
	}
}

func TestPreFlightCheckDatabaseExists_SkipsWhenNameEmpty(t *testing.T) {
	exec := &mockExecutor{queryErr: fmt.Errorf("should not be called")}
	empty := ""

	err := PreFlightCheckDatabaseExists(context.Background(), exec, nil, &empty)
	if err != nil {
		t.Fatalf("expected nil error when name is empty, got: %v", err)
	}
}

func TestPreFlightCheckDatabaseExists_ErrorWhenNotFound(t *testing.T) {
	// ErrObjectNotFound is returned by ShowByID when SHOW DATABASES yields
	// no matching rows — the definitive "not found" signal.
	exec := &mockExecutor{queryErr: snowflake.ErrObjectNotFound}
	name := "ANALYTICS"

	err := PreFlightCheckDatabaseExists(context.Background(), exec, nil, &name)
	if err == nil {
		t.Fatal("expected error when database does not exist")
	}

	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Fatalf("expected ErrDatabaseNotFound, got: %v", err)
	}

	if len(exec.queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(exec.queries))
	}
}

func TestPreFlightCheckDatabaseExists_ConnectionError(t *testing.T) {
	// A non-ErrObjectNotFound error (e.g. connection refused) is NOT
	// wrapped as ErrDatabaseNotFound — callers can distinguish.
	exec := &mockExecutor{queryErr: fmt.Errorf("dial tcp: connection refused")}
	name := "MY_DB"

	err := PreFlightCheckDatabaseExists(context.Background(), exec, nil, &name)
	if err == nil {
		t.Fatal("expected error on connection failure")
	}

	if errors.Is(err, ErrDatabaseNotFound) {
		t.Fatalf("connection errors must NOT be ErrDatabaseNotFound, got: %v", err)
	}

	if len(exec.queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(exec.queries))
	}
}

func TestPreFlightCheckDatabaseExists_IssuesShowByIDQuery(t *testing.T) {
	// Verify the function actually calls ShowByID.
	exec := &mockExecutor{queryErr: snowflake.ErrObjectNotFound}
	name := "MY_DB"

	err := PreFlightCheckDatabaseExists(context.Background(), exec, nil, &name)
	if err == nil {
		t.Fatal("expected error from ShowByID failure")
	}

	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Fatalf("expected ErrDatabaseNotFound, got: %v", err)
	}

	if len(exec.queries) != 1 {
		t.Fatalf("expected 1 query issued, got %d", len(exec.queries))
	}
}

// ---------------------------------------------------------------------------
// PreFlightCheckSchemaExists
// ---------------------------------------------------------------------------

func TestPreFlightCheckSchemaExists_SkipsWhenRefSet(t *testing.T) {
	exec := &mockExecutor{queryErr: fmt.Errorf("should not be called")}
	ref := &snowplanev1alpha1.ObjectReference{Name: "my-schema-cr"}
	name := "PUBLIC"

	err := PreFlightCheckSchemaExists(context.Background(), exec, "DB", ref, &name)
	if err != nil {
		t.Fatalf("expected nil error when ref is set, got: %v", err)
	}

	if len(exec.queries) > 0 {
		t.Fatalf("expected no queries when ref is set, got %d", len(exec.queries))
	}
}

func TestPreFlightCheckSchemaExists_SkipsWhenNameNil(t *testing.T) {
	exec := &mockExecutor{queryErr: fmt.Errorf("should not be called")}

	err := PreFlightCheckSchemaExists(context.Background(), exec, "DB", nil, nil)
	if err != nil {
		t.Fatalf("expected nil error when name is nil, got: %v", err)
	}
}

func TestPreFlightCheckSchemaExists_SkipsWhenNameEmpty(t *testing.T) {
	exec := &mockExecutor{queryErr: fmt.Errorf("should not be called")}
	empty := ""

	err := PreFlightCheckSchemaExists(context.Background(), exec, "DB", nil, &empty)
	if err != nil {
		t.Fatalf("expected nil error when name is empty, got: %v", err)
	}
}

func TestPreFlightCheckSchemaExists_ErrorWhenNotFound(t *testing.T) {
	exec := &mockExecutor{queryErr: snowflake.ErrObjectNotFound}
	name := "MY_SCHEMA"

	err := PreFlightCheckSchemaExists(context.Background(), exec, "ANALYTICS", nil, &name)
	if err == nil {
		t.Fatal("expected error when schema does not exist")
	}

	if !errors.Is(err, ErrSchemaNotFound) {
		t.Fatalf("expected ErrSchemaNotFound, got: %v", err)
	}

	if len(exec.queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(exec.queries))
	}
}

func TestPreFlightCheckSchemaExists_ConnectionError(t *testing.T) {
	// Connection errors are NOT wrapped as ErrSchemaNotFound.
	exec := &mockExecutor{queryErr: fmt.Errorf("i/o timeout")}
	name := "MY_SCHEMA"

	err := PreFlightCheckSchemaExists(context.Background(), exec, "DB", nil, &name)
	if err == nil {
		t.Fatal("expected error on connection failure")
	}

	if errors.Is(err, ErrSchemaNotFound) {
		t.Fatalf("connection errors must NOT be ErrSchemaNotFound, got: %v", err)
	}

	if len(exec.queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(exec.queries))
	}
}

func TestPreFlightCheckSchemaExists_IssuesShowByIDQuery(t *testing.T) {
	exec := &mockExecutor{queryErr: snowflake.ErrObjectNotFound}
	name := "PUBLIC"

	err := PreFlightCheckSchemaExists(context.Background(), exec, "ANALYTICS", nil, &name)
	if err == nil {
		t.Fatal("expected error from ShowByID failure")
	}

	if !errors.Is(err, ErrSchemaNotFound) {
		t.Fatalf("expected ErrSchemaNotFound, got: %v", err)
	}

	if len(exec.queries) != 1 {
		t.Fatalf("expected 1 query issued, got %d", len(exec.queries))
	}
}
