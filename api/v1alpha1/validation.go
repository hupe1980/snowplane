package v1alpha1

import (
	"errors"
	"fmt"
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
// AccountRoleGrantSpec and DatabaseRoleGrantSpec (ShareGrant does not use
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
		if on.Schema.SchemaName != "" {
			schemaCount++
		}

		if on.Schema.SchemaRef != nil {
			schemaCount++
		}

		if on.Schema.AllInDatabase != "" {
			schemaCount++
		}

		if on.Schema.AllInDatabaseRef != nil {
			schemaCount++
		}

		if on.Schema.FutureInDatabase != "" {
			schemaCount++
		}

		if on.Schema.FutureInDatabaseRef != nil {
			schemaCount++
		}

		if schemaCount != 1 {
			errs = append(errs, errors.New("spec.on.schema: exactly one of schemaName, schemaRef, allInDatabase, allInDatabaseRef, futureInDatabase, or futureInDatabaseRef must be set"))
		}

		// Mutual exclusivity: ref vs raw string.
		if on.Schema.SchemaName != "" && on.Schema.SchemaRef != nil {
			errs = append(errs, errors.New("spec.on.schema: schemaName and schemaRef are mutually exclusive"))
		}

		if on.Schema.AllInDatabase != "" && on.Schema.AllInDatabaseRef != nil {
			errs = append(errs, errors.New("spec.on.schema: allInDatabase and allInDatabaseRef are mutually exclusive"))
		}

		if on.Schema.FutureInDatabase != "" && on.Schema.FutureInDatabaseRef != nil {
			errs = append(errs, errors.New("spec.on.schema: futureInDatabase and futureInDatabaseRef are mutually exclusive"))
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

// Validate checks the AccountRoleGrantSpec for configuration errors.
func (s *AccountRoleGrantSpec) Validate() error {
	var errs []error

	if s.Privilege == "" {
		errs = append(errs, errors.New("spec.privilege is required"))
	}

	// Validate On hierarchy.
	errs = append(errs, validateGrantOn(&s.On)...)

	// Exactly one of accountRole or accountRoleRef must be set.
	roleCount := 0
	if s.AccountRole != "" {
		roleCount++
	}

	if s.AccountRoleRef != nil {
		roleCount++
	}

	if roleCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of accountRole or accountRoleRef must be set"))
	}

	if s.AccountRole != "" && s.AccountRoleRef != nil {
		errs = append(errs, errors.New("spec: accountRole and accountRoleRef are mutually exclusive"))
	}

	// Best-effort privilege-to-object-type validation.
	if privErr := validatePrivilegeObjectCompat(&s.On, s.Privilege); privErr != nil {
		errs = append(errs, privErr)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the DatabaseRoleGrantSpec for configuration errors.
func (s *DatabaseRoleGrantSpec) Validate() error {
	var errs []error

	if s.Privilege == "" {
		errs = append(errs, errors.New("spec.privilege is required"))
	}

	// Validate On hierarchy.
	errs = append(errs, validateGrantOn(&s.On)...)

	// Exactly one of databaseRole or databaseRoleRef must be set.
	roleCount := 0
	if s.DatabaseRole != "" {
		roleCount++
	}

	if s.DatabaseRoleRef != nil {
		roleCount++
	}

	if roleCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of databaseRole or databaseRoleRef must be set"))
	}

	if s.DatabaseRole != "" && s.DatabaseRoleRef != nil {
		errs = append(errs, errors.New("spec: databaseRole and databaseRoleRef are mutually exclusive"))
	}

	// Best-effort privilege-to-object-type validation.
	if privErr := validatePrivilegeObjectCompat(&s.On, s.Privilege); privErr != nil {
		errs = append(errs, privErr)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the ShareGrantSpec for configuration errors.
func (s *ShareGrantSpec) Validate() error {
	var errs []error

	if s.Privilege == "" {
		errs = append(errs, errors.New("spec.privilege is required"))
	}

	if s.ObjectType == "" {
		errs = append(errs, errors.New("spec.objectType is required"))
	}

	if s.ObjectName == "" {
		errs = append(errs, errors.New("spec.objectName is required"))
	}

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
	if b.InDatabase != "" {
		scopeCount++
	}

	if b.InDatabaseRef != nil {
		scopeCount++
	}

	if b.InSchema != "" {
		scopeCount++
	}

	if b.InSchemaRef != nil {
		scopeCount++
	}

	if scopeCount != 1 {
		errs = append(errs, fmt.Errorf("%s: exactly one of inDatabase, inDatabaseRef, inSchema, or inSchemaRef must be set", prefix))
	}

	// Mutual exclusivity: ref vs raw string.
	if b.InDatabase != "" && b.InDatabaseRef != nil {
		errs = append(errs, fmt.Errorf("%s: inDatabase and inDatabaseRef are mutually exclusive", prefix))
	}

	if b.InSchema != "" && b.InSchemaRef != nil {
		errs = append(errs, fmt.Errorf("%s: inSchema and inSchemaRef are mutually exclusive", prefix))
	}

	return errs
}

// ---------------------------------------------------------------------------
// Privilege-to-object-type compatibility (L-15)
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
				if c.ForeignKey.Table == "" {
					errs = append(errs, fmt.Errorf("spec.constraints[%d].foreignKey.table is required", i))
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
	if s.Warehouse != nil && s.UserTaskManagedInitialWarehouseSize != nil {
		errs = append(errs, errors.New("spec.warehouse and spec.userTaskManagedInitialWarehouseSize are mutually exclusive"))
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

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the StreamSpec for configuration errors.
func (s *StreamSpec) Validate() error {
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

	if s.SourceName == "" {
		errs = append(errs, errors.New("spec.sourceName is required"))
	}

	// appendOnly is only valid for TABLE/VIEW streams.
	if s.AppendOnly != nil && *s.AppendOnly {
		if s.SourceType != StreamSourceTable && s.SourceType != StreamSourceView {
			errs = append(errs, errors.New("spec.appendOnly is only valid for TABLE or VIEW streams"))
		}
	}

	// insertOnly is only valid for EXTERNAL_TABLE streams.
	if s.InsertOnly != nil && *s.InsertOnly {
		if s.SourceType != StreamSourceExternalTable {
			errs = append(errs, errors.New("spec.insertOnly is only valid for EXTERNAL_TABLE streams"))
		}
	}

	// showInitialRows is only valid for TABLE/VIEW streams.
	if s.ShowInitialRows != nil && *s.ShowInitialRows {
		if s.SourceType != StreamSourceTable && s.SourceType != StreamSourceView {
			errs = append(errs, errors.New("spec.showInitialRows is only valid for TABLE or VIEW streams"))
		}
	}

	// appendOnly and insertOnly are mutually exclusive.
	if s.AppendOnly != nil && *s.AppendOnly && s.InsertOnly != nil && *s.InsertOnly {
		errs = append(errs, errors.New("spec.appendOnly and spec.insertOnly are mutually exclusive"))
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

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
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

// Validate checks the StageSpec for configuration errors.
func (s *StageSpec) Validate() error {
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

	isExternal := s.URL != nil && *s.URL != ""

	// StorageIntegration is only valid for external stages.
	if s.StorageIntegration != nil && !isExternal {
		errs = append(errs, errors.New("spec.storageIntegration requires spec.url (external stage)"))
	}

	// Cross-validate encryption type against stage type.
	if s.Encryption != nil {
		switch strings.ToUpper(s.Encryption.Type) {
		case "SNOWFLAKE_FULL", "SNOWFLAKE_SSE":
			if isExternal {
				errs = append(errs, fmt.Errorf("spec.encryption.type %q is only valid for internal stages", s.Encryption.Type))
			}
		case "AWS_CSE", "AWS_SSE_S3", "AWS_SSE_KMS", "GCS_SSE_KMS", "AZURE_CSE", "NONE":
			if !isExternal {
				errs = append(errs, fmt.Errorf("spec.encryption.type %q is only valid for external stages (spec.url must be set)", s.Encryption.Type))
			}
		}
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the StorageIntegrationSpec for configuration errors.
func (s *StorageIntegrationSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if len(s.StorageAllowedLocations) == 0 {
		errs = append(errs, errors.New("spec.storageAllowedLocations must not be empty"))
	}

	if s.StorageProvider == "" {
		errs = append(errs, errors.New("spec.storageProvider is required"))
	}

	// Provider-specific validation.
	switch strings.ToUpper(s.StorageProvider) {
	case "S3":
		if s.StorageAWSRoleARN == nil || *s.StorageAWSRoleARN == "" {
			errs = append(errs, errors.New("spec.storageAWSRoleARN is required when storageProvider is S3"))
		}
	case "AZURE":
		if s.AzureTenantID == nil || *s.AzureTenantID == "" {
			errs = append(errs, errors.New("spec.azureTenantID is required when storageProvider is AZURE"))
		}
	case "GCS":
		// GCS requires no additional provider-specific fields.
	default:
		errs = append(errs, fmt.Errorf("spec.storageProvider must be one of S3, GCS, AZURE (got: %q)", s.StorageProvider))
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

	if s.AutoIngest != nil && *s.AutoIngest && (s.Integration == nil || *s.Integration == "") {
		errs = append(errs, errors.New("spec.integration is required when spec.autoIngest is true"))
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
	}

	if s.Warehouse == "" {
		errs = append(errs, errors.New("spec.warehouse is required"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks the SecurityIntegrationSpec for configuration errors.
func (s *SecurityIntegrationSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("spec.name is required"))
	}

	if s.Type == "" {
		errs = append(errs, errors.New("spec.type is required"))
	}

	// Type-specific sub-config validation.
	switch s.Type {
	case SecurityIntegrationTypeExternalOAuth:
		if s.ExternalOAuth == nil {
			errs = append(errs, errors.New("spec.externalOAuth is required when type is EXTERNAL_OAUTH"))
		} else {
			if s.ExternalOAuth.Type == "" {
				errs = append(errs, errors.New("spec.externalOAuth.type is required"))
			}
			if s.ExternalOAuth.Issuer == "" {
				errs = append(errs, errors.New("spec.externalOAuth.issuer is required"))
			}
			if s.ExternalOAuth.TokenUserMappingClaim == "" {
				errs = append(errs, errors.New("spec.externalOAuth.tokenUserMappingClaim is required"))
			}
		}
	case SecurityIntegrationTypeSAML2:
		if s.SAML2 == nil {
			errs = append(errs, errors.New("spec.saml2 is required when type is SAML2"))
		} else {
			if s.SAML2.Issuer == "" {
				errs = append(errs, errors.New("spec.saml2.issuer is required"))
			}
			if s.SAML2.SSOURL == "" {
				errs = append(errs, errors.New("spec.saml2.ssoURL is required"))
			}
			if s.SAML2.Provider == "" {
				errs = append(errs, errors.New("spec.saml2.provider is required"))
			}
			if s.SAML2.X509Cert == "" {
				errs = append(errs, errors.New("spec.saml2.x509Cert is required"))
			}
		}
	case SecurityIntegrationTypeSCIM:
		if s.SCIM == nil {
			errs = append(errs, errors.New("spec.scim is required when type is SCIM"))
		} else {
			if s.SCIM.SCIMClient == "" {
				errs = append(errs, errors.New("spec.scim.scimClient is required"))
			}
			if s.SCIM.RunAsRole == "" {
				errs = append(errs, errors.New("spec.scim.runAsRole is required"))
			}
		}
	case SecurityIntegrationTypeAPIAuthentication:
		if s.APIAuthentication == nil {
			errs = append(errs, errors.New("spec.apiAuthentication is required when type is API_AUTHENTICATION"))
		} else {
			if s.APIAuthentication.OAuthClientID == "" {
				errs = append(errs, errors.New("spec.apiAuthentication.oauthClientID is required"))
			}
			if s.APIAuthentication.OAuthClientSecret == "" {
				errs = append(errs, errors.New("spec.apiAuthentication.oauthClientSecret is required"))
			}
			if s.APIAuthentication.OAuthTokenEndpoint == "" {
				errs = append(errs, errors.New("spec.apiAuthentication.oauthTokenEndpoint is required"))
			}
		}
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

// ValidFieldExportSourceKinds enumerates the Snowplane managed resource kinds
// supported as FieldExport sources.
//
//nolint:gochecknoglobals // package-level constant set
var ValidFieldExportSourceKinds = map[string]struct{}{
	"Database":               {},
	"Schema":                 {},
	"Warehouse":              {},
	"User":                   {},
	"AccountRole":            {},
	"DatabaseRole":           {},
	"AccountRoleGrant":       {},
	"DatabaseRoleGrant":      {},
	"ShareGrant":             {},
	"Table":                  {},
	"View":                   {},
	"Stage":                  {},
	"Task":                   {},
	"Stream":                 {},
	"Tag":                    {},
	"NetworkPolicy":          {},
	"ResourceMonitor":        {},
	"MaskingPolicy":          {},
	"RowAccessPolicy":        {},
	"GrantOwnership":         {},
	"StorageIntegration":     {},
	"SecurityIntegration":    {},
	"FileFormat":             {},
	"Pipe":                   {},
	"DynamicTable":           {},
	"PasswordPolicy":         {},
	"NetworkRule":            {},
	"AccountRoleAssignment":  {},
	"DatabaseRoleAssignment": {},
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
		if !strings.HasPrefix(s.From.Path, ".") {
			errs = append(errs, errors.New("spec.from.path must start with \".\" (e.g. \".status.showOutput.name\")"))
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

	return errors.Join(errs...)
}

// validateSecretKeyRef checks that a SecretKeyReference has both name and key set.
func validateSecretKeyRef(field string, ref *SecretKeyReference) error {
	if ref == nil {
		return nil
	}

	var errs []error

	if ref.Name == "" {
		errs = append(errs, fmt.Errorf("%s.name is required when %s is set", field, field))
	}

	if ref.Key == "" {
		errs = append(errs, fmt.Errorf("%s.key is required when %s is set", field, field))
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

// ValidateDangerousAccountRoleGrant checks whether an AccountRoleGrantSpec
// targets a dangerous system role or uses a dangerous privilege. It returns
// a non-nil error when the grant is considered dangerous and should be
// blocked unless the caller explicitly opts in via the allow-dangerous-grant
// annotation. Only account role grants can target system roles.
func ValidateDangerousAccountRoleGrant(spec *AccountRoleGrantSpec) error {
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
	target := strings.ToUpper(strings.TrimSpace(spec.AccountRole))
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
func validateDatabaseSource(ref *LocalObjectReference, name *string) error {
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
func validateSchemaSource(ref *LocalObjectReference, name *string) error {
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

// Validate checks the AccountRoleAssignmentSpec for configuration errors.
func (s *AccountRoleAssignmentSpec) Validate() error {
	var errs []error

	// Exactly one of roleName or roleRef must be set.
	roleCount := 0
	if s.RoleName != "" {
		roleCount++
	}

	if s.RoleRef != nil {
		roleCount++
	}

	if roleCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of roleName or roleRef must be set"))
	}

	if s.RoleName != "" && s.RoleRef != nil {
		errs = append(errs, errors.New("spec: roleName and roleRef are mutually exclusive"))
	}

	// Exactly one of toRole/toRoleRef or toUser/toUserRef must be set.
	targetCount := 0
	if s.ToRole != "" {
		targetCount++
	}

	if s.ToRoleRef != nil {
		targetCount++
	}

	if s.ToUser != "" {
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
	if s.DatabaseRoleName != "" {
		roleCount++
	}

	if s.DatabaseRoleRef != nil {
		roleCount++
	}

	if roleCount != 1 {
		errs = append(errs, errors.New("spec: exactly one of databaseRoleName or databaseRoleRef must be set"))
	}

	if s.DatabaseRoleName != "" && s.DatabaseRoleRef != nil {
		errs = append(errs, errors.New("spec: databaseRoleName and databaseRoleRef are mutually exclusive"))
	}

	// Exactly one of toRole/toRoleRef or toDatabaseRole/toDatabaseRoleRef must be set.
	targetCount := 0
	if s.ToRole != "" {
		targetCount++
	}

	if s.ToRoleRef != nil {
		targetCount++
	}

	if s.ToDatabaseRole != "" {
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
