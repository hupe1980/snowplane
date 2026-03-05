// Package refresolver provides centralized reference resolution and pre-flight
// validation utilities for Snowplane controllers.
package refresolver

import (
	"context"
	"errors"
	"fmt"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

// ErrDatabaseNotFound is returned when a raw databaseName references a
// non-existent Snowflake database.
var ErrDatabaseNotFound = errors.New("database not found in Snowflake")

// ErrSchemaNotFound is returned when a raw schemaName references a
// non-existent Snowflake schema.
var ErrSchemaNotFound = errors.New("schema not found in Snowflake")

// PreFlightCheckDatabaseExists verifies that a database exists in Snowflake
// when a raw databaseName string is used (not a DatabaseRef CR reference).
// When databaseRef is set, the CR reference resolution in PreReconcile
// already validates existence, so this check is skipped.
//
// Returns ErrDatabaseNotFound only when ShowByID definitively confirms the
// database does not exist (ErrObjectNotFound). Other errors (connection
// failures, timeouts) are returned unwrapped so callers can distinguish
// "not found" from infrastructure issues.
func PreFlightCheckDatabaseExists(
	ctx context.Context,
	exec snowflake.SQLExecutor,
	dbRef *snowplanev1alpha1.ObjectReference,
	rawDBName *string,
) error {
	// Ref-based resolution already validates existence via CR readiness.
	if dbRef != nil {
		return nil
	}

	if rawDBName == nil || *rawDBName == "" {
		return nil
	}

	dbClient := snowflake.NewDatabaseClient(exec)

	id := snowflake.NewAccountObjectIdentifier(*rawDBName)

	_, err := dbClient.ShowByID(ctx, id)
	if err != nil {
		if errors.Is(err, snowflake.ErrObjectNotFound) {
			return fmt.Errorf("%w: %q — ensure the database exists in Snowflake or use a databaseRef instead", ErrDatabaseNotFound, *rawDBName)
		}

		// Connection errors, timeouts, etc. — not a definitive "not found".
		return fmt.Errorf("pre-flight database check for %q: %w", *rawDBName, err)
	}

	return nil
}

// PreFlightCheckSchemaExists verifies that a schema exists in Snowflake
// when raw databaseName + schemaName strings are used (not CR references).
// When schemaRef is set, the CR reference resolution in PreReconcile
// already validates existence, so this check is skipped.
//
// Returns ErrSchemaNotFound only when ShowByID definitively confirms the
// schema does not exist. Other errors are returned unwrapped.
func PreFlightCheckSchemaExists(
	ctx context.Context,
	exec snowflake.SQLExecutor,
	dbName string,
	schemaRef *snowplanev1alpha1.ObjectReference,
	rawSchemaName *string,
) error {
	// Ref-based resolution already validates existence via CR readiness.
	if schemaRef != nil {
		return nil
	}

	if rawSchemaName == nil || *rawSchemaName == "" {
		return nil
	}

	schemaClient := snowflake.NewSchemaClient(exec)

	id := snowflake.NewDatabaseObjectIdentifier(dbName, *rawSchemaName)

	_, err := schemaClient.ShowByID(ctx, id)
	if err != nil {
		if errors.Is(err, snowflake.ErrObjectNotFound) {
			return fmt.Errorf("%w: %q.%q — ensure the schema exists in Snowflake or use a schemaRef instead", ErrSchemaNotFound, dbName, *rawSchemaName)
		}

		// Connection errors, timeouts, etc. — not a definitive "not found".
		return fmt.Errorf("pre-flight schema check for %q.%q: %w", dbName, *rawSchemaName, err)
	}

	return nil
}
