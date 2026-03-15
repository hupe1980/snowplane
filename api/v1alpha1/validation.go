package v1alpha1

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"regexp"
	"sort"
	"strings"
)

// validSnowflakeColumnTypes lists all valid Snowflake data type base names.
// Types may be followed by optional parameters in parentheses, e.g. VARCHAR(100),
// NUMBER(38,0), TIMESTAMP_LTZ(9).
//
//nolint:gochecknoglobals // package-level constant set
var validSnowflakeColumnTypes = map[string]struct{}{
	// Numeric
	"NUMBER": {}, "DECIMAL": {}, "DEC": {}, "NUMERIC": {},
	"INT": {}, "INTEGER": {}, "BIGINT": {}, "SMALLINT": {}, "TINYINT": {}, "BYTEINT": {},
	"FLOAT": {}, "FLOAT4": {}, "FLOAT8": {},
	"DOUBLE": {}, "DOUBLE PRECISION": {}, "REAL": {},
	// String
	"VARCHAR": {}, "CHAR": {}, "CHARACTER": {}, "STRING": {}, "TEXT": {},
	// Binary
	"BINARY": {}, "VARBINARY": {},
	// Boolean
	"BOOLEAN": {},
	// Date & Time
	"DATE":      {},
	"DATETIME":  {},
	"TIME":      {},
	"TIMESTAMP": {}, "TIMESTAMP_LTZ": {}, "TIMESTAMP_NTZ": {}, "TIMESTAMP_TZ": {},
	// Semi-structured
	"VARIANT": {}, "OBJECT": {}, "ARRAY": {},
	// Geospatial
	"GEOGRAPHY": {}, "GEOMETRY": {},
	// Vector
	"VECTOR": {},
}

// columnTypePattern matches a Snowflake type like "VARCHAR(100)" or "NUMBER(38,0)"
// or a bare type like "BOOLEAN". Allows optional parenthesized parameters.
var columnTypePattern = regexp.MustCompile(`^([A-Z][A-Z0-9_ ]*)(?:\(([^)]+)\))?$`)

// isValidColumnType checks whether typ is a recognized Snowflake data type.
func isValidColumnType(typ string) bool {
	upper := strings.TrimSpace(strings.ToUpper(typ))
	if upper == "" {
		return false
	}

	m := columnTypePattern.FindStringSubmatch(upper)
	if m == nil {
		return false
	}

	baseName := strings.TrimSpace(m[1])
	_, ok := validSnowflakeColumnTypes[baseName]

	return ok
}

// validateEnum checks that *val is one of the allowed values. Returns nil
// when val is nil (not set). This eliminates per-type switch-case boilerplate.
func validateEnum[T ~string](field string, val *T, valid ...T) error {
	if val == nil || *val == "" {
		return nil
	}

	for _, v := range valid {
		if *val == v {
			return nil
		}
	}

	return fmt.Errorf("%s must be one of %v (got: %q)", field, valid, *val)
}

// Validate checks CommonSpec fields shared across all managed resources.
// Each concrete Spec.Validate() should call this and join the result.
func (s *CommonSpec) Validate() error {
	var errs []error

	if s.ProviderRef.Name == "" {
		errs = append(errs, errors.New("spec.providerRef.name is required"))
	}

	if s.UseRole != nil && *s.UseRole == "" {
		errs = append(errs, errors.New("spec.useRole must not be an empty string when set"))
	}

	if err := validateEnum("spec.deletionPolicy", &s.DeletionPolicy,
		DeletionPolicyDelete, DeletionPolicyOrphan); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the DatabaseSpec for configuration errors.
// Returns an errors.Join aggregate of all validation issues found.
func (s *DatabaseSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.DataRetentionTimeInDays != nil {
		v := *s.DataRetentionTimeInDays
		if v < 0 || v > 90 {
			errs = append(errs, fmt.Errorf("spec.dataRetentionTimeInDays must be between 0 and 90 (got: %d)", v))
		}
	}

	if s.MaxDataExtensionTimeInDays != nil {
		v := *s.MaxDataExtensionTimeInDays
		if v < 0 || v > 90 {
			errs = append(errs, fmt.Errorf("spec.maxDataExtensionTimeInDays must be between 0 and 90 (got: %d)", v))
		}
	}

	if err := validateEnum("spec.storageSerializationPolicy", s.StorageSerializationPolicy,
		StorageSerializationPolicyCompatible, StorageSerializationPolicyOptimized); err != nil {
		errs = append(errs, err)
	}

	if err := validateEnum("spec.logLevel", s.LogLevel,
		LogLevelTrace, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelFatal, LogLevelOff); err != nil {
		errs = append(errs, err)
	}

	if err := validateEnum("spec.metricLevel", s.MetricLevel,
		MetricLevelNone, MetricLevelAll); err != nil {
		errs = append(errs, err)
	}

	if err := validateEnum("spec.traceLevel", s.TraceLevel,
		TraceLevelAlways, TraceLevelOnEvent, TraceLevelOff); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the SchemaSpec for configuration errors.
func (s *SchemaSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	// Exactly one of databaseRef or databaseName must be set.
	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if s.DataRetentionTimeInDays != nil {
		v := *s.DataRetentionTimeInDays
		if v < 0 || v > 90 {
			errs = append(errs, fmt.Errorf("spec.dataRetentionTimeInDays must be between 0 and 90 (got: %d)", v))
		}
	}

	if s.MaxDataExtensionTimeInDays != nil {
		v := *s.MaxDataExtensionTimeInDays
		if v < 0 || v > 90 {
			errs = append(errs, fmt.Errorf("spec.maxDataExtensionTimeInDays must be between 0 and 90 (got: %d)", v))
		}
	}

	if err := validateEnum("spec.storageSerializationPolicy", s.StorageSerializationPolicy,
		StorageSerializationPolicyCompatible, StorageSerializationPolicyOptimized); err != nil {
		errs = append(errs, err)
	}

	if err := validateEnum("spec.logLevel", s.LogLevel,
		LogLevelTrace, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelFatal, LogLevelOff); err != nil {
		errs = append(errs, err)
	}

	if err := validateEnum("spec.metricLevel", s.MetricLevel,
		MetricLevelNone, MetricLevelAll); err != nil {
		errs = append(errs, err)
	}

	if err := validateEnum("spec.traceLevel", s.TraceLevel,
		TraceLevelAlways, TraceLevelOnEvent, TraceLevelOff); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the WarehouseSpec for configuration errors.
func (s *WarehouseSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.QueryAccelerationMaxScaleFactor != nil {
		v := *s.QueryAccelerationMaxScaleFactor
		if v < 0 || v > 100 {
			errs = append(errs, fmt.Errorf("spec.queryAccelerationMaxScaleFactor must be between 0 and 100 (got: %d)", v))
		}
	}

	if s.MinClusterCount != nil {
		if *s.MinClusterCount < 1 || *s.MinClusterCount > 10 {
			errs = append(errs, fmt.Errorf("spec.minClusterCount must be between 1 and 10 (got: %d)", *s.MinClusterCount))
		}
	}

	if s.MaxClusterCount != nil {
		if *s.MaxClusterCount < 1 || *s.MaxClusterCount > 10 {
			errs = append(errs, fmt.Errorf("spec.maxClusterCount must be between 1 and 10 (got: %d)", *s.MaxClusterCount))
		}
	}

	if s.MinClusterCount != nil && s.MaxClusterCount != nil {
		if *s.MinClusterCount > *s.MaxClusterCount {
			errs = append(errs, fmt.Errorf(
				"spec.minClusterCount (%d) must not exceed spec.maxClusterCount (%d)",
				*s.MinClusterCount, *s.MaxClusterCount,
			))
		}
	}

	if s.AutoSuspend != nil && *s.AutoSuspend < 0 {
		errs = append(errs, fmt.Errorf("spec.autoSuspend must be non-negative (got: %d)", *s.AutoSuspend))
	}

	if s.MaxConcurrencyLevel != nil {
		v := *s.MaxConcurrencyLevel
		if v < 1 || v > 32 {
			errs = append(errs, fmt.Errorf("spec.maxConcurrencyLevel must be between 1 and 32 (got: %d)", v))
		}
	}

	if s.StatementQueuedTimeoutInSeconds != nil && *s.StatementQueuedTimeoutInSeconds < 0 {
		errs = append(errs, fmt.Errorf("spec.statementQueuedTimeoutInSeconds must be non-negative (got: %d)", *s.StatementQueuedTimeoutInSeconds))
	}

	if s.StatementTimeoutInSeconds != nil && *s.StatementTimeoutInSeconds < 0 {
		errs = append(errs, fmt.Errorf("spec.statementTimeoutInSeconds must be non-negative (got: %d)", *s.StatementTimeoutInSeconds))
	}

	if err := validateEnum("spec.warehouseType", s.WarehouseType,
		WarehouseTypeStandard, WarehouseTypeSnowparkOptimized); err != nil {
		errs = append(errs, err)
	}

	if err := validateEnum("spec.warehouseSize", s.WarehouseSize,
		WarehouseSizeXSmall, WarehouseSizeSmall, WarehouseSizeMedium, WarehouseSizeLarge,
		WarehouseSizeXLarge, WarehouseSize2XLarge, WarehouseSize3XLarge,
		WarehouseSize4XLarge, WarehouseSize5XLarge, WarehouseSize6XLarge); err != nil {
		errs = append(errs, err)
	}

	if err := validateEnum("spec.scalingPolicy", s.ScalingPolicy,
		ScalingPolicyStandard, ScalingPolicyEconomy); err != nil {
		errs = append(errs, err)
	}

	if err := validateEnum("spec.resourceConstraint", s.ResourceConstraint,
		ResourceConstraintMemory,
		ResourceConstraintStandardGen1,
		ResourceConstraintStandardGen2,
		ResourceConstraintMemory1X,
		ResourceConstraintMemory1Xx86,
		ResourceConstraintMemory16X,
		ResourceConstraintMemory16Xx86,
		ResourceConstraintMemory64X,
		ResourceConstraintMemory64Xx86); err != nil {
		errs = append(errs, err)
	}

	if s.Generation != nil && *s.Generation != "1" && *s.Generation != "2" {
		errs = append(errs, fmt.Errorf("spec.generation must be %q or %q (got: %q)", "1", "2", *s.Generation))
	}

	if s.Generation != nil && s.ResourceConstraint != nil {
		errs = append(errs, errors.New("spec.generation and spec.resourceConstraint are mutually exclusive"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the AccountRoleSpec for configuration errors.
func (s *AccountRoleSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the DatabaseRoleSpec for configuration errors.
func (s *DatabaseRoleSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	// Exactly one of databaseRef or databaseName must be set.
	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// validateGrantOn validates the GrantOn hierarchy. It is shared by
// GrantPrivilegesToAccountRoleSpec and GrantPrivilegesToDatabaseRoleSpec (GrantPrivilegesToShare does not use
// the On hierarchy).
func validateGrantOn(on *GrantOn) []error {
	var errs []error

	onCount := 0
	if on.Account {
		onCount++
	}

	if on.AccountObject != nil {
		onCount++

		if on.AccountObject.ObjectType == "" {
			errs = append(errs, errors.New("spec.on.accountObject.objectType is required"))
		}

		if on.AccountObject.ObjectName == "" {
			errs = append(errs, errors.New("spec.on.accountObject.objectName is required"))
		}
	}

	if on.Schema != nil {
		onCount++

		schemaCount := 0
		if on.Schema.SchemaName != nil {
			schemaCount++
		}

		if on.Schema.SchemaRef != nil {
			schemaCount++
		}

		if on.Schema.AllInDatabase != nil {
			schemaCount++
		}

		if on.Schema.AllInDatabaseRef != nil {
			schemaCount++
		}

		if on.Schema.FutureInDatabase != nil {
			schemaCount++
		}

		if on.Schema.FutureInDatabaseRef != nil {
			schemaCount++
		}

		if schemaCount != 1 {
			errs = append(errs, errors.New("spec.on.schema: exactly one of schemaName, schemaRef, allInDatabase, allInDatabaseRef, futureInDatabase, or futureInDatabaseRef must be set"))
		}
	}

	if on.SchemaObject != nil {
		onCount++

		soCount := 0
		if on.SchemaObject.ObjectType != "" || on.SchemaObject.ObjectName != "" {
			soCount++

			if on.SchemaObject.ObjectType == "" {
				errs = append(errs, errors.New("spec.on.schemaObject.objectType is required when objectName is set"))
			}

			if on.SchemaObject.ObjectName == "" {
				errs = append(errs, errors.New("spec.on.schemaObject.objectName is required when objectType is set"))
			}
		}

		if on.SchemaObject.All != nil {
			soCount++

			errs = append(errs, validateGrantOnBulk("spec.on.schemaObject.all", on.SchemaObject.All)...)
		}

		if on.SchemaObject.Future != nil {
			soCount++

			errs = append(errs, validateGrantOnBulk("spec.on.schemaObject.future", on.SchemaObject.Future)...)
		}

		if soCount != 1 {
			errs = append(errs, errors.New("spec.on.schemaObject: exactly one of (objectType+objectName), all, or future must be set"))
		}
	}

	if onCount != 1 {
		errs = append(errs, errors.New("spec.on: exactly one of account, accountObject, schema, or schemaObject must be set"))
	}

	return errs
}

// Validate checks the GrantPrivilegesToAccountRoleSpec for configuration errors.
func (s *GrantPrivilegesToAccountRoleSpec) Validate() error {
	var errs []error

	// Exactly one of privilege or allPrivileges must be set.
	privCount := 0
	if s.Privilege != "" {
		privCount++
	}

	if s.AllPrivileges {
		privCount++
	}

	if privCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of privilege or allPrivileges must be set"))
	}

	// Validate On hierarchy.
	errs = append(errs, validateGrantOn(&s.On)...)

	// Exactly one of accountRole or accountRoleRef must be set.
	roleCount := 0
	if s.AccountRole != nil {
		roleCount++
	}

	if s.AccountRoleRef != nil {
		roleCount++
	}

	if roleCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of accountRole or accountRoleRef must be set"))
	}

	// Best-effort privilege-to-object-type validation (skip for ALL PRIVILEGES).
	if !s.AllPrivileges {
		if privErr := validatePrivilegeObjectCompat(&s.On, s.Privilege); privErr != nil {
			errs = append(errs, privErr)
		}
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// validateGrantPrivilegesToDatabaseRoleOn validates the GrantPrivilegesToDatabaseRoleOn hierarchy.
func validateGrantPrivilegesToDatabaseRoleOn(on *GrantPrivilegesToDatabaseRoleOn) []error {
	var errs []error

	onCount := 0

	if on.Database != nil {
		onCount++
	}

	if on.Schema != nil {
		onCount++

		schemaCount := 0
		if on.Schema.SchemaName != nil {
			schemaCount++
		}

		if on.Schema.SchemaRef != nil {
			schemaCount++
		}

		if on.Schema.AllInDatabase != nil {
			schemaCount++
		}

		if on.Schema.AllInDatabaseRef != nil {
			schemaCount++
		}

		if on.Schema.FutureInDatabase != nil {
			schemaCount++
		}

		if on.Schema.FutureInDatabaseRef != nil {
			schemaCount++
		}

		if schemaCount != 1 {
			errs = append(errs, errors.New("spec.on.schema: exactly one of schemaName, schemaRef, allInDatabase, allInDatabaseRef, futureInDatabase, or futureInDatabaseRef must be set"))
		}
	}

	if on.SchemaObject != nil {
		onCount++

		soCount := 0
		if on.SchemaObject.ObjectType != "" || on.SchemaObject.ObjectName != "" {
			soCount++

			if on.SchemaObject.ObjectType == "" {
				errs = append(errs, errors.New("spec.on.schemaObject.objectType is required when objectName is set"))
			}

			if on.SchemaObject.ObjectName == "" {
				errs = append(errs, errors.New("spec.on.schemaObject.objectName is required when objectType is set"))
			}
		}

		if on.SchemaObject.All != nil {
			soCount++
			errs = append(errs, validateGrantOnBulk("spec.on.schemaObject.all", on.SchemaObject.All)...)
		}

		if on.SchemaObject.Future != nil {
			soCount++
			errs = append(errs, validateGrantOnBulk("spec.on.schemaObject.future", on.SchemaObject.Future)...)
		}

		if soCount != 1 {
			errs = append(errs, errors.New("spec.on.schemaObject: exactly one of (objectType+objectName), all, or future must be set"))
		}
	}

	if onCount != 1 {
		errs = append(errs, errors.New("spec.on: exactly one of database, schema, or schemaObject must be set"))
	}

	return errs
}

// Validate checks the GrantPrivilegesToDatabaseRoleSpec for configuration errors.
func (s *GrantPrivilegesToDatabaseRoleSpec) Validate() error {
	var errs []error

	// Exactly one of privilege or allPrivileges must be set.
	privCount := 0
	if s.Privilege != "" {
		privCount++
	}

	if s.AllPrivileges {
		privCount++
	}

	if privCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of privilege or allPrivileges must be set"))
	}

	// Validate On hierarchy (database role specific).
	errs = append(errs, validateGrantPrivilegesToDatabaseRoleOn(&s.On)...)

	// Exactly one of databaseRole or databaseRoleRef must be set.
	roleCount := 0
	if s.DatabaseRole != nil {
		roleCount++
	}

	if s.DatabaseRoleRef != nil {
		roleCount++
	}

	if roleCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of databaseRole or databaseRoleRef must be set"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// validateGrantPrivilegesToShareOn validates the GrantPrivilegesToShareOn hierarchy.
func validateGrantPrivilegesToShareOn(on *GrantPrivilegesToShareOn) []error {
	var errs []error

	onCount := 0
	if on.Database != nil {
		onCount++
	}

	if on.Schema != nil {
		onCount++
	}

	if on.Table != nil {
		onCount++
	}

	if on.AllTablesInSchema != nil {
		onCount++
	}

	if on.Function != nil {
		onCount++
	}

	if on.Tag != nil {
		onCount++
	}

	if on.View != nil {
		onCount++
	}

	if onCount != 1 {
		errs = append(errs, errors.New("spec.on: exactly one of database, schema, table, allTablesInSchema, function, tag, or view must be set"))
	}

	return errs
}

// Validate checks the GrantPrivilegesToShareSpec for configuration errors.
func (s *GrantPrivilegesToShareSpec) Validate() error {
	var errs []error

	if s.Privilege == "" {
		errs = append(errs, errors.New("spec.privilege is required"))
	}

	errs = append(errs, validateGrantPrivilegesToShareOn(&s.On)...)

	if s.Share == "" {
		errs = append(errs, errors.New("spec.share is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// validateGrantOnBulk validates a GrantOnBulk struct.
func validateGrantOnBulk(prefix string, b *GrantOnBulk) []error {
	var errs []error

	if b.ObjectTypePlural == "" {
		errs = append(errs, fmt.Errorf("%s.objectTypePlural is required", prefix))
	}

	// Count all scope variants (raw + ref).
	scopeCount := 0
	if b.InDatabase != nil {
		scopeCount++
	}

	if b.InDatabaseRef != nil {
		scopeCount++
	}

	if b.InSchema != nil {
		scopeCount++
	}

	if b.InSchemaRef != nil {
		scopeCount++
	}

	if scopeCount != 1 {
		errs = append(errs, fmt.Errorf("%s: exactly one of inDatabase, inDatabaseRef, inSchema, or inSchemaRef must be set", prefix))
	}

	return errs
}

// ---------------------------------------------------------------------------
// Privilege-to-object-type compatibility
// ---------------------------------------------------------------------------

// validAccountPrivileges lists privileges valid for ON ACCOUNT.
// Ref: https://docs.snowflake.com/en/sql-reference/sql/grant-privilege#global-privileges
//
//nolint:gochecknoglobals // package-level constant set
var validAccountPrivileges = map[string]struct{}{
	"CREATE DATABASE": {}, "CREATE WAREHOUSE": {}, "CREATE ROLE": {},
	"CREATE USER": {}, "CREATE INTEGRATION": {}, "CREATE SHARE": {},
	"CREATE NETWORK POLICY": {}, "CREATE ACCOUNT": {}, "CREATE DATA EXCHANGE LISTING": {},
	"CREATE CONNECTION": {}, "CREATE FAILOVER GROUP": {}, "CREATE REPLICATION GROUP": {},
	"MANAGE GRANTS": {}, "MONITOR USAGE": {}, "MONITOR": {},
	"EXECUTE TASK": {}, "EXECUTE MANAGED TASK": {}, "EXECUTE DATA METRIC FUNCTION": {},
	"IMPORT SHARE": {}, "APPLY MASKING POLICY": {}, "APPLY ROW ACCESS POLICY": {},
	"APPLY TAG": {}, "APPLY SESSION POLICY": {}, "APPLY PASSWORD POLICY": {},
	"APPLY AUTHENTICATION POLICY": {}, "APPLY AGGREGATION POLICY": {},
	"APPLY PROJECTION POLICY": {}, "APPLY PACKAGES POLICY": {},
	"OVERRIDE SHARE RESTRICTIONS": {}, "BIND SERVICE ENDPOINT": {},
	"MANAGE ORGANIZATION SUPPORT CASES": {}, "MANAGE USER SUPPORT CASES": {},
	"MANAGE ACCOUNT SUPPORT CASES": {}, "PURCHASE DATA EXCHANGE LISTING": {},
	"RESOLVE ALL": {}, "ATTACH POLICY": {},
	"ALL": {}, "ALL PRIVILEGES": {},
}

// validObjectPrivileges maps common account-object and schema-object types
// to their valid privilege sets. Only the most commonly used types are
// included — unlisted types skip validation.
// Ref: https://docs.snowflake.com/en/sql-reference/sql/grant-privilege
//
//nolint:gochecknoglobals // package-level constant set
var validObjectPrivileges = map[string]map[string]struct{}{
	// Account objects
	"DATABASE": {
		"USAGE": {}, "MONITOR": {}, "CREATE SCHEMA": {},
		"MODIFY": {}, "IMPORTED PRIVILEGES": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"WAREHOUSE": {
		"USAGE": {}, "OPERATE": {}, "MONITOR": {}, "MODIFY": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"USER": {
		"MONITOR": {}, "OWNERSHIP": {},
		"ALL": {}, "ALL PRIVILEGES": {},
	},
	"RESOURCE MONITOR": {
		"MONITOR": {}, "MODIFY": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"INTEGRATION": {
		"USAGE": {}, "USE_ANY_ROLE": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},

	// Schema objects
	"SCHEMA": {
		"USAGE": {}, "MONITOR": {}, "MODIFY": {},
		"CREATE TABLE": {}, "CREATE VIEW": {}, "CREATE MATERIALIZED VIEW": {},
		"CREATE TEMPORARY TABLE": {}, "CREATE STAGE": {}, "CREATE FILE FORMAT": {},
		"CREATE SEQUENCE": {}, "CREATE FUNCTION": {}, "CREATE PROCEDURE": {},
		"CREATE PIPE": {}, "CREATE STREAM": {}, "CREATE TASK": {},
		"CREATE TAG": {}, "CREATE MASKING POLICY": {}, "CREATE ROW ACCESS POLICY": {},
		"CREATE SECRET": {}, "CREATE EXTERNAL TABLE": {},
		"ADD SEARCH OPTIMIZATION": {},
		"ALL":                     {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"TABLE": {
		"SELECT": {}, "INSERT": {}, "UPDATE": {}, "DELETE": {}, "TRUNCATE": {},
		"REFERENCES": {}, "EVOLVE SCHEMA": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"TABLES": {
		"SELECT": {}, "INSERT": {}, "UPDATE": {}, "DELETE": {}, "TRUNCATE": {},
		"REFERENCES": {}, "EVOLVE SCHEMA": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"VIEW": {
		"SELECT": {}, "REFERENCES": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"VIEWS": {
		"SELECT": {}, "REFERENCES": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"MATERIALIZED VIEW": {
		"SELECT": {}, "REFERENCES": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"MATERIALIZED VIEWS": {
		"SELECT": {}, "REFERENCES": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"STAGE": {
		"USAGE": {}, "READ": {}, "WRITE": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"STAGES": {
		"USAGE": {}, "READ": {}, "WRITE": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"FUNCTION": {
		"USAGE": {},
		"ALL":   {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"FUNCTIONS": {
		"USAGE": {},
		"ALL":   {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"PROCEDURE": {
		"USAGE": {},
		"ALL":   {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"PROCEDURES": {
		"USAGE": {},
		"ALL":   {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"STREAM": {
		"SELECT": {},
		"ALL":    {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"STREAMS": {
		"SELECT": {},
		"ALL":    {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"TASK": {
		"MONITOR": {}, "OPERATE": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"TASKS": {
		"MONITOR": {}, "OPERATE": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"PIPE": {
		"MONITOR": {}, "OPERATE": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"SEQUENCE": {
		"USAGE": {},
		"ALL":   {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"FILE FORMAT": {
		"USAGE": {},
		"ALL":   {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"EXTERNAL TABLE": {
		"SELECT": {}, "REFERENCES": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"EXTERNAL TABLES": {
		"SELECT": {}, "REFERENCES": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"ALERT": {
		"MONITOR": {}, "OPERATE": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"ALERTS": {
		"MONITOR": {}, "OPERATE": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"DYNAMIC TABLE": {
		"SELECT": {}, "MONITOR": {}, "OPERATE": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
	"DYNAMIC TABLES": {
		"SELECT": {}, "MONITOR": {}, "OPERATE": {},
		"ALL": {}, "ALL PRIVILEGES": {}, "OWNERSHIP": {},
	},
}

// validatePrivilegeObjectCompat checks whether the privilege is compatible
// with the grant target object type. It only rejects known-invalid combinations
// and allows unrecognized types through (validated by Snowflake at runtime).
func validatePrivilegeObjectCompat(on *GrantOn, privilege string) error {
	upper := strings.ToUpper(strings.TrimSpace(privilege))

	// Account-level grants.
	if on.Account {
		if _, ok := validAccountPrivileges[upper]; !ok {
			return fmt.Errorf("privilege %q is not a valid global account privilege", privilege)
		}

		return nil
	}

	// Account-object grants.
	if on.AccountObject != nil {
		objType := strings.ToUpper(strings.TrimSpace(on.AccountObject.ObjectType))

		privs, known := validObjectPrivileges[objType]
		if !known {
			return nil // Unknown object type — defer to Snowflake.
		}

		if _, ok := privs[upper]; !ok {
			return fmt.Errorf("privilege %q is not valid for %s (valid: %s)",
				privilege, on.AccountObject.ObjectType, joinPrivileges(privs))
		}

		return nil
	}

	// Schema-level grants (ON SCHEMA / ALL / FUTURE schemas).
	if on.Schema != nil {
		privs, known := validObjectPrivileges["SCHEMA"]
		if !known {
			return nil
		}

		if _, ok := privs[upper]; !ok {
			return fmt.Errorf("privilege %q is not valid for SCHEMA (valid: %s)",
				privilege, joinPrivileges(privs))
		}

		return nil
	}

	// Schema-object grants.
	if on.SchemaObject != nil {
		var objType string

		switch {
		case on.SchemaObject.ObjectType != "":
			objType = strings.ToUpper(strings.TrimSpace(on.SchemaObject.ObjectType))
		case on.SchemaObject.All != nil:
			objType = strings.ToUpper(strings.TrimSpace(on.SchemaObject.All.ObjectTypePlural))
		case on.SchemaObject.Future != nil:
			objType = strings.ToUpper(strings.TrimSpace(on.SchemaObject.Future.ObjectTypePlural))
		default:
			return nil
		}

		privs, known := validObjectPrivileges[objType]
		if !known {
			return nil // Unknown object type — defer to Snowflake.
		}

		if _, ok := privs[upper]; !ok {
			return fmt.Errorf("privilege %q is not valid for %s (valid: %s)",
				privilege, objType, joinPrivileges(privs))
		}

		return nil
	}

	return nil
}

// joinPrivileges returns a sorted, comma-separated list of privilege names.
func joinPrivileges(privs map[string]struct{}) string {
	names := make([]string, 0, len(privs))
	for p := range privs {
		names = append(names, p)
	}

	// Sort for deterministic output.
	sort.Strings(names)

	return strings.Join(names, ", ")
}

// Validate checks the UserSpec for configuration errors.
func (s *UserSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateEnum("spec.type", s.Type,
		UserTypePerson, UserTypeService, UserTypeLegacyService); err != nil {
		errs = append(errs, err)
	}

	if err := validateSecretKeyRef("spec.password", s.Password); err != nil {
		errs = append(errs, err)
	}

	if err := validateSecretKeyRef("spec.rsaPublicKey", s.RSAPublicKey); err != nil {
		errs = append(errs, err)
	}

	if err := validateSecretKeyRef("spec.rsaPublicKey2", s.RSAPublicKey2); err != nil {
		errs = append(errs, err)
	}

	if s.Email != nil && *s.Email != "" {
		if _, err := mail.ParseAddress(*s.Email); err != nil {
			errs = append(errs, fmt.Errorf("spec.email is not a valid email address: %w", err))
		}
	}

	if s.DaysToExpiry != nil && *s.DaysToExpiry < 0 {
		errs = append(errs, errors.New("spec.daysToExpiry must be >= 0"))
	}

	if s.MinsToUnlock != nil && *s.MinsToUnlock < 0 {
		errs = append(errs, errors.New("spec.minsToUnlock must be >= 0"))
	}

	if s.MinsToBypassMFA != nil && *s.MinsToBypassMFA < 0 {
		errs = append(errs, errors.New("spec.minsToBypassMFA must be >= 0"))
	}

	if s.NetworkPolicy != nil && *s.NetworkPolicy == "" {
		errs = append(errs, errors.New("spec.networkPolicy must not be empty when set"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the TableSpec for configuration errors.
func (s *TableSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	// Exactly one of databaseRef or databaseName must be set.
	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	// Exactly one of schemaRef or schemaName must be set.
	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if len(s.Columns) == 0 {
		errs = append(errs, errors.New("spec.columns must have at least one column"))
	}

	for i, col := range s.Columns {
		if col.Name == "" {
			errs = append(errs, fmt.Errorf("spec.columns[%d].name is required", i))
		}

		if col.Type == "" {
			errs = append(errs, fmt.Errorf("spec.columns[%d].type is required", i))
		} else if !isValidColumnType(col.Type) {
			errs = append(errs, fmt.Errorf("spec.columns[%d].type %q is not a recognized Snowflake data type", i, col.Type))
		}
	}

	// Detect duplicate column names (case-insensitive, matching Snowflake semantics).
	seen := make(map[string]int, len(s.Columns))
	for i, col := range s.Columns {
		if col.Name == "" {
			continue
		}
		upper := strings.ToUpper(col.Name)
		if prev, ok := seen[upper]; ok {
			errs = append(errs, fmt.Errorf("spec.columns[%d].name %q duplicates spec.columns[%d] (Snowflake identifiers are case-insensitive)", i, col.Name, prev))
		} else {
			seen[upper] = i
		}
	}

	if s.DataRetentionTimeInDays != nil {
		v := *s.DataRetentionTimeInDays
		if v < 0 || v > 90 {
			errs = append(errs, fmt.Errorf("spec.dataRetentionTimeInDays must be between 0 and 90 (got: %d)", v))
		}
	}

	if s.MaxDataExtensionTimeInDays != nil {
		v := *s.MaxDataExtensionTimeInDays
		if v < 0 || v > 90 {
			errs = append(errs, fmt.Errorf("spec.maxDataExtensionTimeInDays must be between 0 and 90 (got: %d)", v))
		}
	}

	// Validate table constraints.
	for i, c := range s.Constraints {
		if len(c.Columns) == 0 {
			errs = append(errs, fmt.Errorf("spec.constraints[%d].columns must have at least one column", i))
		}

		// Verify constraint columns reference defined columns.
		for _, colName := range c.Columns {
			found := false
			for _, col := range s.Columns {
				if strings.EqualFold(col.Name, colName) {
					found = true
					break
				}
			}

			if !found {
				errs = append(errs, fmt.Errorf("spec.constraints[%d] references undefined column %q", i, colName))
			}
		}

		switch c.Type {
		case TableConstraintPrimaryKey, TableConstraintUnique:
			if c.ForeignKey != nil {
				errs = append(errs, fmt.Errorf("spec.constraints[%d]: foreignKey must not be set for %s constraints", i, c.Type))
			}
		case TableConstraintForeignKey:
			if c.ForeignKey == nil {
				errs = append(errs, fmt.Errorf("spec.constraints[%d]: foreignKey is required for ForeignKey constraints", i))
			} else {
				hasTable := c.ForeignKey.Table != nil && *c.ForeignKey.Table != ""
				hasTableRef := c.ForeignKey.TableRef != nil

				if !hasTable && !hasTableRef {
					errs = append(errs, fmt.Errorf("spec.constraints[%d].foreignKey: exactly one of table or tableRef must be set", i))
				}

				if hasTable && hasTableRef {
					errs = append(errs, fmt.Errorf("spec.constraints[%d].foreignKey: table and tableRef are mutually exclusive", i))
				}

				if len(c.ForeignKey.Columns) == 0 {
					errs = append(errs, fmt.Errorf("spec.constraints[%d].foreignKey.columns must have at least one column", i))
				}

				if len(c.Columns) != len(c.ForeignKey.Columns) {
					errs = append(errs, fmt.Errorf("spec.constraints[%d]: column count (%d) must match foreignKey column count (%d)", i, len(c.Columns), len(c.ForeignKey.Columns)))
				}
			}
		default:
			errs = append(errs, fmt.Errorf("spec.constraints[%d].type %q is not valid (expected PrimaryKey, Unique, or ForeignKey)", i, c.Type))
		}
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the AlertSpec for configuration errors.
func (s *AlertSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	// Exactly one of databaseRef or databaseName must be set.
	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	// Exactly one of schemaRef or schemaName must be set.
	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if s.Condition == "" {
		errs = append(errs, errors.New("spec.condition is required"))
	}

	if s.Action == "" {
		errs = append(errs, errors.New("spec.action is required"))
	}

	if err := validateOptionalSourceRef("warehouse", s.WarehouseRef, s.WarehouseName); err != nil {
		errs = append(errs, err)
	}

	// Validate schedule format: "N MINUTE" or "USING CRON <expr> <tz>".
	if s.Schedule != nil && *s.Schedule != "" {
		if err := validateSchedule(*s.Schedule); err != nil {
			errs = append(errs, fmt.Errorf("spec.schedule: %w", err))
		}
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the TaskSpec for configuration errors.
func (s *TaskSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	// Exactly one of databaseRef or databaseName must be set.
	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	// Exactly one of schemaRef or schemaName must be set.
	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if s.SQLStatement == "" {
		errs = append(errs, errors.New("spec.sqlStatement is required"))
	}

	// Warehouse and UserTaskManagedInitialWarehouseSize are mutually exclusive.
	if err := validateOptionalSourceRef("warehouse", s.WarehouseRef, s.WarehouseName); err != nil {
		errs = append(errs, err)
	}

	if (s.WarehouseRef != nil || s.WarehouseName != nil) && s.UserTaskManagedInitialWarehouseSize != nil {
		errs = append(errs, errors.New("spec.warehouseRef/warehouseName and spec.userTaskManagedInitialWarehouseSize are mutually exclusive"))
	}

	if err := validateOptionalSourceRef("errorIntegration", s.ErrorIntegrationRef, s.ErrorIntegrationName); err != nil {
		errs = append(errs, err)
	}

	if err := validateOptionalSourceRef("successIntegration", s.SuccessIntegrationRef, s.SuccessIntegrationName); err != nil {
		errs = append(errs, err)
	}

	if err := validateOptionalSourceRef("finalize", s.FinalizeRef, s.FinalizeName); err != nil {
		errs = append(errs, err)
	}

	// Validate After entries: each must have exactly one of ref or name.
	for i, p := range s.After {
		if (p.Ref == nil) == (p.Name == nil) {
			errs = append(errs, fmt.Errorf("spec.after[%d]: exactly one of ref or name must be set", i))
		}
	}

	if s.UserTaskTimeoutMs != nil {
		v := *s.UserTaskTimeoutMs
		if v < 0 || v > 604800000 {
			errs = append(errs, fmt.Errorf("spec.userTaskTimeoutMs must be between 0 and 604800000 (got: %d)", v))
		}
	}

	if s.TaskAutoRetryAttempts != nil {
		v := *s.TaskAutoRetryAttempts
		if v < 0 || v > 30 {
			errs = append(errs, fmt.Errorf("spec.taskAutoRetryAttempts must be between 0 and 30 (got: %d)", v))
		}
	}

	if s.Schedule != nil && *s.Schedule != "" {
		if err := validateSchedule(*s.Schedule); err != nil {
			errs = append(errs, fmt.Errorf("spec.schedule: %w", err))
		}
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the StreamOnTableSpec for configuration errors.
func (s *StreamOnTableSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSourceRef("table", s.TableRef, s.TableName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the StreamOnViewSpec for configuration errors.
func (s *StreamOnViewSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSourceRef("view", s.ViewRef, s.ViewName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the StreamOnExternalTableSpec for configuration errors.
func (s *StreamOnExternalTableSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := validateExternalTableSource(s.ExternalTableRef, s.ExternalTableName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the StreamOnDirectoryTableSpec for configuration errors.
func (s *StreamOnDirectoryTableSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSourceRef("stage", s.StageRef, s.StageName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the StreamOnDynamicTableSpec for configuration errors.
func (s *StreamOnDynamicTableSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSourceRef("dynamicTable", s.DynamicTableRef, s.DynamicTableName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the TagSpec for configuration errors.
func (s *TagSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	// Exactly one of databaseRef or databaseName must be set.
	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	// Exactly one of schemaRef or schemaName must be set.
	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the NetworkPolicySpec for configuration errors.
func (s *NetworkPolicySpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	// At least one allowed source is required.
	if len(s.AllowedIPList) == 0 && len(s.AllowedNetworkRuleList) == 0 {
		errs = append(errs, errors.New("spec: at least one of allowedIPList or allowedNetworkRuleList must be non-empty"))
	}

	// Validate IP address / CIDR format.
	for i, ip := range s.AllowedIPList {
		if err := validateIPOrCIDR(ip); err != nil {
			errs = append(errs, fmt.Errorf("spec.allowedIPList[%d]: %w", i, err))
		}
	}

	for i, ip := range s.BlockedIPList {
		if err := validateIPOrCIDR(ip); err != nil {
			errs = append(errs, fmt.Errorf("spec.blockedIPList[%d]: %w", i, err))
		}
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// validateIPOrCIDR checks that s is a valid IPv4/IPv6 address or CIDR notation.
func validateIPOrCIDR(s string) error {
	if net.ParseIP(s) != nil {
		return nil
	}

	if _, _, err := net.ParseCIDR(s); err == nil {
		return nil
	}

	return fmt.Errorf("%q is not a valid IP address or CIDR", s)
}

// scheduleMinutePattern matches Snowflake "N MINUTE" schedules.
var scheduleMinutePattern = regexp.MustCompile(`(?i)^\d+\s+MINUTE$`)

// scheduleCronPattern matches Snowflake "USING CRON <expr> <timezone>" schedules.
var scheduleCronPattern = regexp.MustCompile(`(?i)^USING\s+CRON\s+.+\s+\S+$`)

// validateSchedule checks that s matches one of the two Snowflake schedule formats:
//   - "N MINUTE"  (e.g. "5 MINUTE", "60 MINUTE")
//   - "USING CRON <cron_expr> <timezone>"  (e.g. "USING CRON 0 9 * * MON-FRI America/New_York")
func validateSchedule(s string) error {
	if scheduleMinutePattern.MatchString(s) || scheduleCronPattern.MatchString(s) {
		return nil
	}

	return fmt.Errorf("must be %q or %q format (got: %q)", "N MINUTE", "USING CRON <expr> <tz>", s)
}

// targetLagPattern matches Snowflake target lag duration formats (e.g. "10 seconds", "5 minutes", "1 hour", "2 days").
var targetLagPattern = regexp.MustCompile(`(?i)^\d+\s+(seconds?|minutes?|hours?|days?)$`)

// validateTargetLag checks that s matches a valid Snowflake dynamic table target lag:
//   - Duration: "N seconds/minutes/hours/days"  (singular or plural)
//   - Downstream: "DOWNSTREAM"
func validateTargetLag(s string) error {
	if strings.EqualFold(s, "DOWNSTREAM") || targetLagPattern.MatchString(s) {
		return nil
	}

	return fmt.Errorf("must be a duration (e.g. %q) or %q (got: %q)", "10 minutes", "DOWNSTREAM", s)
}

// Validate checks the ViewSpec for configuration errors.
func (s *ViewSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	// Exactly one of databaseRef or databaseName must be set.
	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	// Exactly one of schemaRef or schemaName must be set.
	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if s.Statement == "" {
		errs = append(errs, errors.New("spec.statement is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the FileFormatSpec for configuration errors.
func (s *FileFormatSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if s.Type == "" {
		errs = append(errs, errors.New("spec.type is required"))
	} else {
		validTypes := []FileFormatType{
			FileFormatTypeCSV, FileFormatTypeJSON, FileFormatTypeAVRO,
			FileFormatTypeORC, FileFormatTypePARQUET, FileFormatTypeXML,
		}

		valid := false
		for _, v := range validTypes {
			if s.Type == v {
				valid = true

				break
			}
		}

		if !valid {
			errs = append(errs, fmt.Errorf("spec.type must be one of %v (got: %q)", validTypes, s.Type))
		}
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the PipeSpec for configuration errors.
func (s *PipeSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if s.CopyStatement == "" {
		errs = append(errs, errors.New("spec.copyStatement is required"))
	}

	if err := validateOptionalSourceRef("integration", s.IntegrationRef, s.IntegrationName); err != nil {
		errs = append(errs, err)
	}

	if s.AutoIngest != nil && *s.AutoIngest && s.IntegrationRef == nil && s.IntegrationName == nil {
		errs = append(errs, errors.New("one of spec.integrationRef or spec.integrationName is required when spec.autoIngest is true"))
	}

	if err := validateOptionalSourceRef("errorIntegration", s.ErrorIntegrationRef, s.ErrorIntegrationName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the DynamicTableSpec for configuration errors.
func (s *DynamicTableSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if s.Query == "" {
		errs = append(errs, errors.New("spec.query is required"))
	}

	if s.TargetLag == "" {
		errs = append(errs, errors.New("spec.targetLag is required"))
	} else if err := validateTargetLag(s.TargetLag); err != nil {
		errs = append(errs, fmt.Errorf("spec.targetLag: %w", err))
	}

	if err := validateSourceRef("warehouse", s.WarehouseRef, s.WarehouseName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the AuthenticationPolicySpec for configuration errors.
func (s *AuthenticationPolicySpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the PasswordPolicySpec for configuration errors.
func (s *PasswordPolicySpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	// Cross-field constraint: min <= max.
	if s.PasswordMinLength != nil && s.PasswordMaxLength != nil && *s.PasswordMinLength > *s.PasswordMaxLength {
		errs = append(errs, errors.New("spec.passwordMinLength must not exceed spec.passwordMaxLength"))
	}

	// Cross-field constraint: minAgeDays <= maxAgeDays.
	if s.PasswordMinAgeDays != nil && s.PasswordMaxAgeDays != nil && *s.PasswordMinAgeDays > *s.PasswordMaxAgeDays {
		errs = append(errs, errors.New("spec.passwordMinAgeDays must not exceed spec.passwordMaxAgeDays"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the NetworkRuleSpec for configuration errors.
func (s *NetworkRuleSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if s.Type == "" {
		errs = append(errs, errors.New("spec.type is required"))
	}

	if s.Mode == "" {
		errs = append(errs, errors.New("spec.mode is required"))
	}

	if len(s.ValueList) == 0 {
		errs = append(errs, errors.New("spec.valueList must not be empty"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the SequenceSpec for configuration errors.
func (s *SequenceSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the ExternalTableSpec for configuration errors.
func (s *ExternalTableSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.Location == "" {
		errs = append(errs, errors.New("spec.location is required"))
	}

	if s.FileFormat == "" {
		errs = append(errs, errors.New("spec.fileFormat is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the MaterializedViewSpec for configuration errors.
func (s *MaterializedViewSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.Statement == "" {
		errs = append(errs, errors.New("spec.statement is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the SecretWithClientCredentialsSpec for configuration errors.
func (s *SecretWithClientCredentialsSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if s.APIAuthentication == "" {
		errs = append(errs, errors.New("spec.apiAuthentication is required"))
	}

	if len(s.OAuthScopes) == 0 {
		errs = append(errs, errors.New("spec.oauthScopes must contain at least one scope"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the SecretWithAuthorizationCodeGrantSpec for configuration errors.
func (s *SecretWithAuthorizationCodeGrantSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if s.APIAuthentication == "" {
		errs = append(errs, errors.New("spec.apiAuthentication is required"))
	}

	if s.OAuthRefreshToken == "" {
		errs = append(errs, errors.New("spec.oauthRefreshToken is required"))
	}

	if s.OAuthRefreshTokenExpiryTime == "" {
		errs = append(errs, errors.New("spec.oauthRefreshTokenExpiryTime is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the SecretWithBasicAuthenticationSpec for configuration errors.
func (s *SecretWithBasicAuthenticationSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if s.Username == "" {
		errs = append(errs, errors.New("spec.username is required"))
	}

	if s.Password == "" {
		errs = append(errs, errors.New("spec.password is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the SecretWithGenericStringSpec for configuration errors.
func (s *SecretWithGenericStringSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if s.SecretString == "" {
		errs = append(errs, errors.New("spec.secretString is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the APIAuthenticationIntegrationWithClientCredentialsSpec for configuration errors.
func (s *APIAuthenticationIntegrationWithClientCredentialsSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.OAuthClientID == "" {
		errs = append(errs, errors.New("spec.oauthClientId is required"))
	}

	if err := validateRequiredSecretKeyRef("spec.oauthClientSecretRef", s.OAuthClientSecretRef); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the APIAuthenticationIntegrationWithAuthorizationCodeGrantSpec for configuration errors.
func (s *APIAuthenticationIntegrationWithAuthorizationCodeGrantSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.OAuthClientID == "" {
		errs = append(errs, errors.New("spec.oauthClientId is required"))
	}

	if err := validateRequiredSecretKeyRef("spec.oauthClientSecretRef", s.OAuthClientSecretRef); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the APIAuthenticationIntegrationWithJWTBearerSpec for configuration errors.
func (s *APIAuthenticationIntegrationWithJWTBearerSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.OAuthClientID == "" {
		errs = append(errs, errors.New("spec.oauthClientId is required"))
	}

	if err := validateRequiredSecretKeyRef("spec.oauthClientSecretRef", s.OAuthClientSecretRef); err != nil {
		errs = append(errs, err)
	}

	if s.OAuthAssertionIssuer == "" {
		errs = append(errs, errors.New("spec.oauthAssertionIssuer is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// ValidFieldExportSourceKinds enumerates the Snowplane managed resource kinds
// supported as FieldExport sources.
//
//nolint:gochecknoglobals // package-level constant set
var ValidFieldExportSourceKinds = map[string]struct{}{
	"Alert":                            {},
	"AuthenticationPolicy":             {},
	"Database":                         {},
	"Schema":                           {},
	"Warehouse":                        {},
	"User":                             {},
	"AccountRole":                      {},
	"DatabaseRole":                     {},
	"GrantPrivilegesToAccountRole":     {},
	"GrantPrivilegesToDatabaseRole":    {},
	"GrantPrivilegesToShare":           {},
	"Table":                            {},
	"View":                             {},
	"ExternalStage":                    {},
	"InternalStage":                    {},
	"Task":                             {},
	"StreamOnTable":                    {},
	"StreamOnView":                     {},
	"StreamOnExternalTable":            {},
	"StreamOnDirectoryTable":           {},
	"StreamOnDynamicTable":             {},
	"Tag":                              {},
	"NetworkPolicy":                    {},
	"ResourceMonitor":                  {},
	"MaskingPolicy":                    {},
	"RowAccessPolicy":                  {},
	"GrantOwnership":                   {},
	"StorageIntegrationAWS":            {},
	"StorageIntegrationGCS":            {},
	"StorageIntegrationAzure":          {},
	"ExternalVolume":                   {},
	"CortexSearchService":              {},
	"GitRepository":                    {},
	"Streamlit":                        {},
	"FileFormat":                       {},
	"Pipe":                             {},
	"DynamicTable":                     {},
	"PasswordPolicy":                   {},
	"NetworkRule":                      {},
	"AccountRoleAssignment":            {},
	"DatabaseRoleAssignment":           {},
	"TagAssociation":                   {},
	"NetworkPolicyAttachment":          {},
	"PasswordPolicyAttachment":         {},
	"MaskingPolicyApplication":         {},
	"Sequence":                         {},
	"TableConstraint":                  {},
	"ExternalTable":                    {},
	"MaterializedView":                 {},
	"ProcedureSQL":                     {},
	"ProcedureJavascript":              {},
	"ProcedurePython":                  {},
	"ProcedureJava":                    {},
	"ProcedureScala":                   {},
	"FunctionSQL":                      {},
	"FunctionJavascript":               {},
	"FunctionPython":                   {},
	"FunctionJava":                     {},
	"FunctionScala":                    {},
	"SecretWithClientCredentials":      {},
	"SecretWithAuthorizationCodeGrant": {},
	"SecretWithBasicAuthentication":    {},
	"SecretWithGenericString":          {},
	"APIAuthenticationIntegrationWithClientCredentials":      {},
	"APIAuthenticationIntegrationWithAuthorizationCodeGrant": {},
	"APIAuthenticationIntegrationWithJWTBearer":              {},
	"SQLStatement":                           {},
	"SAML2Integration":                       {},
	"ExternalOAuthIntegration":               {},
	"SCIMIntegration":                        {},
	"EmailNotificationIntegration":           {},
	"QueueNotificationIntegration":           {},
	"WebhookNotificationIntegration":         {},
	"APIIntegration":                         {},
	"SecondaryDatabase":                      {},
	"SharedDatabase":                         {},
	"FailoverGroup":                          {},
	"ComputePool":                            {},
	"ExternalFunction":                       {},
	"ImageRepository":                        {},
	"Service":                                {},
	"Share":                                  {},
	"OAuthIntegrationForCustomClients":       {},
	"OAuthIntegrationForPartnerApplications": {},
	"PrimaryConnection":                      {},
	"SecondaryConnection":                    {},
}

// Validate checks that the FieldExport spec fields are semantically valid.
func (s *FieldExportSpec) Validate() error {
	var errs []error

	// Validate source kind is a known Snowplane managed resource.
	if s.From.Resource.Kind == "" {
		errs = append(errs, errors.New("spec.from.resource.kind is required"))
	} else if _, ok := ValidFieldExportSourceKinds[s.From.Resource.Kind]; !ok {
		errs = append(errs, fmt.Errorf("spec.from.resource.kind %q is not a supported Snowplane resource kind", s.From.Resource.Kind))
	}

	if s.From.Resource.Name == "" {
		errs = append(errs, errors.New("spec.from.resource.name is required"))
	}

	// Validate path format.
	if s.From.Path == "" {
		errs = append(errs, errors.New("spec.from.path is required"))
	} else {
		if !strings.HasPrefix(s.From.Path, ".status.") {
			errs = append(errs, errors.New(`spec.from.path must start with ".status." (e.g. ".status.showOutput.name")`))
		}

		if strings.Contains(s.From.Path, "[") {
			errs = append(errs, errors.New("spec.from.path does not support array indexing"))
		}
	}

	// Validate target fields.
	if s.To.Kind == "" {
		errs = append(errs, errors.New("spec.to.kind is required"))
	} else if s.To.Kind != FieldExportTargetConfigMap && s.To.Kind != FieldExportTargetSecret {
		errs = append(errs, fmt.Errorf("spec.to.kind %q is invalid: must be %q or %q",
			s.To.Kind, FieldExportTargetConfigMap, FieldExportTargetSecret))
	}

	if s.To.Name == "" {
		errs = append(errs, errors.New("spec.to.name is required"))
	}

	if s.To.Key == "" {
		errs = append(errs, errors.New("spec.to.key is required"))
	}

	// Reject sensitive status paths when target is a ConfigMap (unencrypted).
	if s.To.Kind == FieldExportTargetConfigMap && IsSensitiveStatusPath(s.From.Resource.Kind, s.From.Path) {
		errs = append(errs, fmt.Errorf(
			"spec.from.path %q is sensitive for kind %q and must not be exported to a ConfigMap; use a Secret target instead",
			s.From.Path, s.From.Resource.Kind))
	}

	return errors.Join(errs...)
}

// validateSecretKeyRef checks that a SecretKeyReference has both name and key set.
func validateSecretKeyRef(field string, ref *SecretKeyReference) error {
	if ref == nil {
		return nil
	}

	return validateRequiredSecretKeyRef(field, *ref)
}

// validateRequiredSecretKeyRef checks that a required SecretKeyReference has both name and key set.
func validateRequiredSecretKeyRef(field string, ref SecretKeyReference) error {
	var errs []error

	if ref.Name == "" {
		errs = append(errs, fmt.Errorf("%s.name is required", field))
	}

	if ref.Key == "" {
		errs = append(errs, fmt.Errorf("%s.key is required", field))
	}

	return errors.Join(errs...)
}

// DangerousSystemRoles contains Snowflake system roles that should not
// be targeted by grants unless explicitly opted in. Granting privileges
// to these roles can lead to privilege escalation.
var DangerousSystemRoles = map[string]bool{
	"ACCOUNTADMIN":  true,
	"SECURITYADMIN": true,
	"ORGADMIN":      true,
}

// DangerousPrivileges contains Snowflake privileges that are inherently
// dangerous because they enable privilege escalation or full object
// takeover.
var DangerousPrivileges = map[string]bool{
	"MANAGE GRANTS": true,
	"OWNERSHIP":     true,
}

// ValidateDangerousGrantPrivilegesToAccountRole checks whether an GrantPrivilegesToAccountRoleSpec
// targets a dangerous system role or uses a dangerous privilege. It returns
// a non-nil error when the grant is considered dangerous and should be
// blocked unless the caller explicitly opts in via the allow-dangerous-grant
// annotation. Only account role grants can target system roles.
func ValidateDangerousGrantPrivilegesToAccountRole(spec *GrantPrivilegesToAccountRoleSpec) error {
	var errs []error

	priv := strings.ToUpper(strings.TrimSpace(spec.Privilege))

	// Block dangerous privileges.
	if DangerousPrivileges[priv] {
		if spec.On.Account {
			errs = append(errs, fmt.Errorf(
				"granting %s on ACCOUNT is dangerous and blocked by default; "+
					"set annotation %q to \"true\" to allow",
				priv, AnnotationAllowDangerousGrant,
			))
		}
	}

	// Block grants targeting dangerous system roles.
	var target string
	if spec.AccountRole != nil {
		target = strings.ToUpper(strings.TrimSpace(*spec.AccountRole))
	}
	if target != "" && DangerousSystemRoles[target] {
		errs = append(errs, fmt.Errorf(
			"granting privileges to system role %s is dangerous and blocked by default; "+
				"set annotation %q to \"true\" to allow",
			target, AnnotationAllowDangerousGrant,
		))
	}

	return errors.Join(errs...)
}

// validateDatabaseSource enforces exactly-one-of semantics for
// databaseRef / databaseName across Schema, Table, View, Stage, DatabaseRole.
func validateDatabaseSource(ref *ObjectReference, name *string) error {
	hasRef := ref != nil && ref.Name != ""
	hasName := name != nil && *name != ""

	if hasRef && hasName {
		return errors.New("spec.databaseRef and spec.databaseName are mutually exclusive")
	}

	if !hasRef && !hasName {
		return errors.New("exactly one of spec.databaseRef or spec.databaseName must be set")
	}

	if ref != nil && ref.Name == "" {
		return errors.New("spec.databaseRef.name must not be empty when databaseRef is set")
	}

	return nil
}

// validateSchemaSource enforces exactly-one-of semantics for
// schemaRef / schemaName across Table, View, Stage.
func validateSchemaSource(ref *ObjectReference, name *string) error {
	hasRef := ref != nil && ref.Name != ""
	hasName := name != nil && *name != ""

	if hasRef && hasName {
		return errors.New("spec.schemaRef and spec.schemaName are mutually exclusive")
	}

	if !hasRef && !hasName {
		return errors.New("exactly one of spec.schemaRef or spec.schemaName must be set")
	}

	if ref != nil && ref.Name == "" {
		return errors.New("spec.schemaRef.name must not be empty when schemaRef is set")
	}

	return nil
}

// validateExternalTableSource enforces exactly-one-of semantics for
// externalTableRef / externalTableName.
func validateExternalTableSource(ref *ObjectReference, name *string) error {
	return validateSourceRef("externalTable", ref, name)
}

// validateSourceRef enforces exactly-one-of semantics for a
// <kindLabel>Ref / <kindLabel>Name pair.
func validateSourceRef(kindLabel string, ref *ObjectReference, name *string) error {
	hasRef := ref != nil && ref.Name != ""
	hasName := name != nil && *name != ""

	if hasRef && hasName {
		return fmt.Errorf("spec.%sRef and spec.%sName are mutually exclusive", kindLabel, kindLabel)
	}

	if !hasRef && !hasName {
		return fmt.Errorf("exactly one of spec.%sRef or spec.%sName must be set", kindLabel, kindLabel)
	}

	if ref != nil && ref.Name == "" {
		return fmt.Errorf("spec.%sRef.name must not be empty when %sRef is set", kindLabel, kindLabel)
	}

	return nil
}

// validateOptionalSourceRef validates an optional ref/name pair.
// Both may be nil (field omitted), but if one is set, the other must be nil.
func validateOptionalSourceRef(kindLabel string, ref *ObjectReference, name *string) error {
	hasRef := ref != nil && ref.Name != ""
	hasName := name != nil && *name != ""

	if hasRef && hasName {
		return fmt.Errorf("spec.%sRef and spec.%sName are mutually exclusive", kindLabel, kindLabel)
	}

	if ref != nil && ref.Name == "" {
		return fmt.Errorf("spec.%sRef.name must not be empty when %sRef is set", kindLabel, kindLabel)
	}

	return nil
}

// Validate checks the AccountRoleAssignmentSpec for configuration errors.
func (s *AccountRoleAssignmentSpec) Validate() error {
	var errs []error

	// Exactly one of roleName or roleRef must be set.
	roleCount := 0
	if s.RoleName != nil {
		roleCount++
	}

	if s.RoleRef != nil {
		roleCount++
	}

	if roleCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of roleName or roleRef must be set"))
	}

	// Exactly one of toRole/toRoleRef or toUser/toUserRef must be set.
	targetCount := 0
	if s.ToRole != nil {
		targetCount++
	}

	if s.ToRoleRef != nil {
		targetCount++
	}

	if s.ToUser != nil {
		targetCount++
	}

	if s.ToUserRef != nil {
		targetCount++
	}

	if targetCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of toRole, toRoleRef, toUser, or toUserRef must be set"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the DatabaseRoleAssignmentSpec for configuration errors.
func (s *DatabaseRoleAssignmentSpec) Validate() error {
	var errs []error

	// Exactly one of databaseRoleName or databaseRoleRef must be set.
	roleCount := 0
	if s.DatabaseRoleName != nil {
		roleCount++
	}

	if s.DatabaseRoleRef != nil {
		roleCount++
	}

	if roleCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of databaseRoleName or databaseRoleRef must be set"))
	}

	// Exactly one of toRole/toRoleRef or toDatabaseRole/toDatabaseRoleRef must be set.
	targetCount := 0
	if s.ToRole != nil {
		targetCount++
	}

	if s.ToRoleRef != nil {
		targetCount++
	}

	if s.ToDatabaseRole != nil {
		targetCount++
	}

	if s.ToDatabaseRoleRef != nil {
		targetCount++
	}

	if targetCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of toRole, toRoleRef, toDatabaseRole, or toDatabaseRoleRef must be set"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the TagAssociationSpec for configuration errors.
func (s *TagAssociationSpec) Validate() error {
	var errs []error

	// Exactly one of tagName or tagRef must be set.
	tagCount := 0
	if s.TagName != nil {
		tagCount++
	}

	if s.TagRef != nil {
		tagCount++
	}

	if tagCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of tagName or tagRef must be set"))
	}

	if s.TagValue == "" {
		errs = append(errs, errors.New("spec.tagValue is required"))
	} else if len(s.TagValue) > 256 {
		errs = append(errs, errors.New("spec.tagValue must be at most 256 characters"))
	}

	if s.ObjectType == "" {
		errs = append(errs, errors.New("spec.objectType is required"))
	} else {
		validObjectTypes := []string{
			"ACCOUNT", "DATABASE", "SCHEMA", "TABLE", "VIEW", "COLUMN",
			"WAREHOUSE", "ROLE", "USER", "STAGE", "STREAM", "TASK",
			"ALERT", "PIPE", "FUNCTION", "PROCEDURE", "INTEGRATION",
			"NETWORK POLICY", "DATABASE ROLE",
		}

		valid := false
		for _, v := range validObjectTypes {
			if s.ObjectType == v {
				valid = true

				break
			}
		}

		if !valid {
			errs = append(errs, fmt.Errorf("spec.objectType must be one of %v (got: %q)", validObjectTypes, s.ObjectType))
		}
	}

	if s.ObjectName == "" {
		errs = append(errs, errors.New("spec.objectName is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the NetworkPolicyAttachmentSpec for configuration errors.
func (s *NetworkPolicyAttachmentSpec) Validate() error {
	var errs []error

	// Exactly one of policyName or policyRef must be set.
	policyCount := 0
	if s.PolicyName != nil {
		policyCount++
	}

	if s.PolicyRef != nil {
		policyCount++
	}

	if policyCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of policyName or policyRef must be set"))
	}

	if s.TargetType == "" {
		errs = append(errs, errors.New("spec.targetType is required"))
	} else if s.TargetType != "ACCOUNT" && s.TargetType != "USER" {
		errs = append(errs, fmt.Errorf("spec.targetType must be one of [ACCOUNT USER] (got: %q)", s.TargetType))
	}

	if s.TargetType == "USER" && s.TargetName == "" {
		errs = append(errs, errors.New("spec.targetName is required when targetType is USER"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the PasswordPolicyAttachmentSpec for configuration errors.
func (s *PasswordPolicyAttachmentSpec) Validate() error {
	var errs []error

	// Exactly one of policyName or policyRef must be set.
	policyCount := 0
	if s.PolicyName != nil {
		policyCount++
	}

	if s.PolicyRef != nil {
		policyCount++
	}

	if policyCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of policyName or policyRef must be set"))
	}

	if s.TargetType == "" {
		errs = append(errs, errors.New("spec.targetType is required"))
	} else if s.TargetType != "ACCOUNT" && s.TargetType != "USER" {
		errs = append(errs, fmt.Errorf("spec.targetType must be one of [ACCOUNT USER] (got: %q)", s.TargetType))
	}

	if s.TargetType == "USER" && s.TargetName == "" {
		errs = append(errs, errors.New("spec.targetName is required when targetType is USER"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the MaskingPolicyApplicationSpec for configuration errors.
func (s *MaskingPolicyApplicationSpec) Validate() error {
	var errs []error

	// Exactly one of policyName or policyRef must be set.
	policyCount := 0
	if s.PolicyName != nil {
		policyCount++
	}

	if s.PolicyRef != nil {
		policyCount++
	}

	if policyCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of policyName or policyRef must be set"))
	}

	if s.TableName == "" {
		errs = append(errs, errors.New("spec.tableName is required"))
	}

	if s.ColumnName == "" {
		errs = append(errs, errors.New("spec.columnName is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the TableConstraintSpec for configuration errors.
func (s *TableConstraintSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.TableName == "" {
		errs = append(errs, errors.New("spec.tableName is required"))
	}

	if s.Type == "" {
		errs = append(errs, errors.New("spec.type is required"))
	}

	if len(s.Columns) == 0 {
		errs = append(errs, errors.New("spec.columns must have at least one entry"))
	}

	// Foreign key properties validation.
	if s.Type == ConstraintTypeForeignKey {
		if s.ForeignKeyProperties == nil {
			errs = append(errs, errors.New("spec.foreignKeyProperties is required when type is FOREIGN KEY"))
		} else {
			if s.ForeignKeyProperties.ReferencesTableName == "" {
				errs = append(errs, errors.New("spec.foreignKeyProperties.referencesTableName is required"))
			}

			if len(s.ForeignKeyProperties.ReferencesColumns) == 0 {
				errs = append(errs, errors.New("spec.foreignKeyProperties.referencesColumns must have at least one entry"))
			}
		}
	} else if s.ForeignKeyProperties != nil {
		errs = append(errs, errors.New("spec.foreignKeyProperties must not be set when type is not FOREIGN KEY"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// destructiveKeywordsRe matches SQL keywords that indicate potentially
// destructive operations requiring explicit opt-in via dangerousAllowDestructive.
// Uses word boundaries (\b) to avoid false positives (e.g. "BACKDROP") and
// handles all whitespace variants (DROP\n, DROP\t, etc.).
//
//nolint:gochecknoglobals // package-level compiled regex
var destructiveKeywordsRe = regexp.MustCompile(`(?i)\b(DROP|TRUNCATE|DELETE|REMOVE)\b`)

// containsDestructiveSQL checks whether a SQL string contains destructive keywords.
func containsDestructiveSQL(sql string) bool {
	return destructiveKeywordsRe.MatchString(sql)
}

// Validate checks the SQLStatementSpec for configuration errors.
func (s *SQLStatementSpec) Validate() error {
	var errs []error

	if strings.TrimSpace(s.Execute) == "" {
		errs = append(errs, errors.New("spec.execute is required and must not be blank"))
	}

	// ObserveExpect without observe makes no sense.
	if s.Observe == nil && len(s.ObserveExpect) > 0 {
		errs = append(errs, errors.New("spec.observeExpect requires spec.observe to be set"))
	}

	// Validate observe is not blank when set.
	if s.Observe != nil && strings.TrimSpace(*s.Observe) == "" {
		errs = append(errs, errors.New("spec.observe must not be blank when set"))
	}

	// Validate revert is not blank when set.
	if s.Revert != nil && strings.TrimSpace(*s.Revert) == "" {
		errs = append(errs, errors.New("spec.revert must not be blank when set"))
	}

	// Destructive SQL guard: require explicit opt-in for execute SQL.
	// Note: spec.revert is exempt from this check because revert SQL is
	// inherently destructive by design (DROP, REVOKE, etc.). Requiring
	// dangerousAllowDestructive for every revert would cause unnecessary
	// friction without adding safety.
	if !s.DangerousAllowDestructive {
		if containsDestructiveSQL(s.Execute) {
			errs = append(errs, errors.New("spec.execute contains destructive SQL (DROP/TRUNCATE/DELETE/REMOVE); set spec.dangerousAllowDestructive=true to proceed"))
		}
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the SAML2IntegrationSpec for configuration errors.
func (s *SAML2IntegrationSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.Issuer == "" {
		errs = append(errs, errors.New("spec.issuer is required"))
	}

	if s.SSOURL == "" {
		errs = append(errs, errors.New("spec.ssoURL is required"))
	}

	if s.Provider == "" {
		errs = append(errs, errors.New("spec.provider is required"))
	}

	if s.X509Cert == "" {
		errs = append(errs, errors.New("spec.x509Cert is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the ShareSpec for configuration errors.
func (s *ShareSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	for i, acct := range s.Accounts {
		if acct == "" {
			errs = append(errs, fmt.Errorf("spec.accounts[%d]: account identifier must not be empty", i))
		}
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the ExternalFunctionSpec for configuration errors.
func (s *ExternalFunctionSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.ReturnType == "" {
		errs = append(errs, errors.New("spec.returnType is required"))
	}

	if s.APIIntegrationName == nil && s.APIIntegrationRef == nil {
		errs = append(errs, errors.New("one of spec.apiIntegrationRef or spec.apiIntegrationName is required"))
	}

	if s.APIIntegrationName != nil && s.APIIntegrationRef != nil {
		errs = append(errs, errors.New("spec.apiIntegrationRef and spec.apiIntegrationName are mutually exclusive"))
	}

	if s.URL == "" {
		errs = append(errs, errors.New("spec.url is required"))
	}

	if s.Compression != nil {
		if err := validateEnum("compression", s.Compression,
			ExternalFunctionCompressionAuto,
			ExternalFunctionCompressionGZIP,
			ExternalFunctionCompressionDeflate,
			ExternalFunctionCompressionNone,
		); err != nil {
			errs = append(errs, err)
		}
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the ComputePoolSpec for configuration errors.
func (s *ComputePoolSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.MinNodes < 1 {
		errs = append(errs, errors.New("spec.minNodes must be >= 1"))
	}

	if s.MaxNodes < 1 {
		errs = append(errs, errors.New("spec.maxNodes must be >= 1"))
	}

	if s.MinNodes > s.MaxNodes {
		errs = append(errs, fmt.Errorf("spec.minNodes (%d) must be <= spec.maxNodes (%d)", s.MinNodes, s.MaxNodes))
	}

	if s.InstanceFamily == "" {
		errs = append(errs, errors.New("spec.instanceFamily is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the ServiceSpec for configuration errors.
func (s *ServiceSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.ComputePoolName == nil && s.ComputePoolRef == nil {
		errs = append(errs, errors.New("one of spec.computePoolRef or spec.computePoolName is required"))
	}

	if s.ComputePoolName != nil && s.ComputePoolRef != nil {
		errs = append(errs, errors.New("spec.computePoolRef and spec.computePoolName are mutually exclusive"))
	}

	if s.Specification == "" && s.SpecificationReference == "" {
		errs = append(errs, errors.New("one of spec.specification or spec.specificationReference is required"))
	}

	if s.Specification != "" && s.SpecificationReference != "" {
		errs = append(errs, errors.New("spec.specification and spec.specificationReference are mutually exclusive"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the ImageRepositorySpec for configuration errors.
func (s *ImageRepositorySpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
