// Package main implements a CLI tool that reads a Terraform state file and
// generates Snowplane Kubernetes CRD manifests for migration.
//
// Supported Terraform resource types:
//   - snowflake_database        → Database
//   - snowflake_schema          → Schema
//   - snowflake_warehouse       → Warehouse
//   - snowflake_user            → User
//   - snowflake_account_role / snowflake_role → AccountRole
//   - snowflake_database_role   → DatabaseRole
//   - snowflake_grant_privileges_to_account_role → GrantPrivilegesToAccountRole
//   - snowflake_grant_privileges_to_database_role → GrantPrivilegesToDatabaseRole
//   - snowflake_grant_privileges_to_share → GrantPrivilegesToShare
//   - snowflake_table            → Table
//   - snowflake_view             → View
//   - snowflake_stage            → Stage
//
// Usage:
//
//	go run ./cmd/tfimport -state terraform.tfstate -provider default > manifests.yaml
//
// To also generate a state-removal script for Terraform / OpenTofu:
//
//	go run ./cmd/tfimport -state terraform.tfstate -remove-script remove-from-state.sh > manifests.yaml
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// Terraform state model (subset)
// ---------------------------------------------------------------------------

// TerraformState represents the top-level structure of a Terraform state file.
type TerraformState struct {
	Version   int             `json:"version"`
	Resources []StateResource `json:"resources"`
}

// StateResource is a single resource block inside the state.
type StateResource struct {
	Module    string          `json:"module"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Instances []StateInstance `json:"instances"`
}

// StateInstance is one instance of a resource (usually exactly one for
// non-count / non-for_each resources).
type StateInstance struct {
	IndexKey   any            `json:"index_key"`
	Attributes map[string]any `json:"attributes"`
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func stringAttr(attrs map[string]any, key string) *string {
	v, ok := attrs[key]
	if !ok || v == nil {
		return nil
	}

	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}

	return &s
}

func boolAttr(attrs map[string]any, key string) *bool {
	v, ok := attrs[key]
	if !ok || v == nil {
		return nil
	}

	b, ok := v.(bool)
	if !ok {
		return nil
	}

	return &b
}

func int32Attr(attrs map[string]any, key string) *int32 {
	v, ok := attrs[key]
	if !ok || v == nil {
		return nil
	}

	// JSON numbers are float64
	f, ok := v.(float64)
	if !ok {
		return nil
	}

	i := int32(f)

	return &i
}

const apiVersion = "snowplane.hupe1980.github.io/v1alpha1"

// yamlString returns a YAML-safe (single-quoted) string with embedded
// single-quotes doubled.
func yamlString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// emitField writes an optional spec field with correct YAML indentation.
func emitField(b *strings.Builder, indent, key string, value *string) {
	if value != nil {
		fmt.Fprintf(b, "%s%s: %s\n", indent, key, yamlString(*value))
	}
}

func emitBool(b *strings.Builder, indent, key string, value *bool) {
	if value != nil {
		if *value {
			fmt.Fprintf(b, "%s%s: true\n", indent, key)
		} else {
			fmt.Fprintf(b, "%s%s: false\n", indent, key)
		}
	}
}

func emitInt32(b *strings.Builder, indent, key string, value *int32) {
	if value != nil {
		fmt.Fprintf(b, "%s%s: %d\n", indent, key, *value)
	}
}

// isTruthy checks if an attribute is truthy (handles both bool and string "true").
func isTruthy(attrs map[string]any, key string) bool {
	v, ok := attrs[key]
	if !ok || v == nil {
		return false
	}

	switch bv := v.(type) {
	case bool:
		return bv
	case string:
		return strings.EqualFold(bv, "true")
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------

func convertDatabase(attrs map[string]any, provider, namespace string) string {
	name, ok := attrs["name"].(string)
	if !ok || name == "" {
		return ""
	}

	crName := sanitizeName(name)

	var b strings.Builder

	fmt.Fprintf(&b, "apiVersion: %s\nkind: Database\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n", apiVersion, crName, namespace)
	fmt.Fprintf(&b, "  providerRef:\n    name: %s\n", provider)
	fmt.Fprintf(&b, "  deletionPolicy: Orphan\n")
	fmt.Fprintf(&b, "  name: %s\n", yamlString(name))

	indent := "  "

	emitField(&b, indent, "comment", stringAttr(attrs, "comment"))
	emitInt32(&b, indent, "dataRetentionTimeInDays", int32Attr(attrs, "data_retention_time_in_days"))
	emitInt32(&b, indent, "maxDataExtensionTimeInDays", int32Attr(attrs, "max_data_extension_time_in_days"))

	if isTruthy(attrs, "is_transient") {
		fmt.Fprintf(&b, "  transient: true\n")
	}

	emitField(&b, indent, "catalog", stringAttr(attrs, "catalog"))
	emitField(&b, indent, "externalVolume", stringAttr(attrs, "external_volume"))
	emitBool(&b, indent, "replaceInvalidCharacters", boolAttr(attrs, "replace_invalid_characters"))
	emitField(&b, indent, "defaultDDLCollation", stringAttr(attrs, "default_ddl_collation"))
	emitField(&b, indent, "storageSerializationPolicy", stringAttr(attrs, "storage_serialization_policy"))
	emitField(&b, indent, "logLevel", stringAttr(attrs, "log_level"))
	emitField(&b, indent, "metricLevel", stringAttr(attrs, "metric_level"))
	emitField(&b, indent, "traceLevel", stringAttr(attrs, "trace_level"))

	return b.String()
}

func convertSchema(attrs map[string]any, provider, namespace string) string {
	name, ok := attrs["name"].(string)
	if !ok || name == "" {
		return ""
	}

	database := ""

	if v := stringAttr(attrs, "database"); v != nil {
		database = *v
	}

	crName := sanitizeName(database + "-" + name)

	var b strings.Builder

	fmt.Fprintf(&b, "apiVersion: %s\nkind: Schema\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n", apiVersion, crName, namespace)
	fmt.Fprintf(&b, "  providerRef:\n    name: %s\n", provider)
	fmt.Fprintf(&b, "  deletionPolicy: Orphan\n")
	fmt.Fprintf(&b, "  name: %s\n", yamlString(name))

	if database != "" {
		fmt.Fprintf(&b, "  databaseRef:\n    name: %s\n", sanitizeName(database))
	}

	indent := "  "

	emitField(&b, indent, "comment", stringAttr(attrs, "comment"))
	emitInt32(&b, indent, "dataRetentionTimeInDays", int32Attr(attrs, "data_retention_time_in_days"))
	emitInt32(&b, indent, "maxDataExtensionTimeInDays", int32Attr(attrs, "max_data_extension_time_in_days"))

	if isTruthy(attrs, "is_transient") {
		fmt.Fprintf(&b, "  transient: true\n")
	}

	if isTruthy(attrs, "with_managed_access") {
		fmt.Fprintf(&b, "  managedAccess: true\n")
	}

	emitField(&b, indent, "defaultDDLCollation", stringAttr(attrs, "default_ddl_collation"))
	emitBool(&b, indent, "replaceInvalidCharacters", boolAttr(attrs, "replace_invalid_characters"))
	emitField(&b, indent, "storageSerializationPolicy", stringAttr(attrs, "storage_serialization_policy"))
	emitField(&b, indent, "logLevel", stringAttr(attrs, "log_level"))
	emitField(&b, indent, "metricLevel", stringAttr(attrs, "metric_level"))
	emitField(&b, indent, "traceLevel", stringAttr(attrs, "trace_level"))

	return b.String()
}

func convertWarehouse(attrs map[string]any, provider, namespace string) string {
	name, ok := attrs["name"].(string)
	if !ok || name == "" {
		return ""
	}

	crName := sanitizeName(name)

	var b strings.Builder

	fmt.Fprintf(&b, "apiVersion: %s\nkind: Warehouse\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n", apiVersion, crName, namespace)
	fmt.Fprintf(&b, "  providerRef:\n    name: %s\n", provider)
	fmt.Fprintf(&b, "  deletionPolicy: Orphan\n")
	fmt.Fprintf(&b, "  name: %s\n", yamlString(name))

	indent := "  "

	emitField(&b, indent, "warehouseType", stringAttr(attrs, "warehouse_type"))
	emitField(&b, indent, "warehouseSize", stringAttr(attrs, "warehouse_size"))
	emitInt32(&b, indent, "minClusterCount", int32Attr(attrs, "min_cluster_count"))
	emitInt32(&b, indent, "maxClusterCount", int32Attr(attrs, "max_cluster_count"))
	emitField(&b, indent, "scalingPolicy", stringAttr(attrs, "scaling_policy"))
	emitInt32(&b, indent, "autoSuspend", int32Attr(attrs, "auto_suspend"))
	emitBool(&b, indent, "autoResume", boolAttr(attrs, "auto_resume"))

	if isTruthy(attrs, "initially_suspended") {
		fmt.Fprintf(&b, "  initiallySuspended: true\n")
	}

	emitField(&b, indent, "resourceMonitor", stringAttr(attrs, "resource_monitor"))
	emitField(&b, indent, "comment", stringAttr(attrs, "comment"))
	emitBool(&b, indent, "enableQueryAcceleration", boolAttr(attrs, "enable_query_acceleration"))
	emitInt32(&b, indent, "queryAccelerationMaxScaleFactor", int32Attr(attrs, "query_acceleration_max_scale_factor"))
	emitInt32(&b, indent, "maxConcurrencyLevel", int32Attr(attrs, "max_concurrency_level"))
	emitInt32(&b, indent, "statementQueuedTimeoutInSeconds", int32Attr(attrs, "statement_queued_timeout_in_seconds"))
	emitInt32(&b, indent, "statementTimeoutInSeconds", int32Attr(attrs, "statement_timeout_in_seconds"))

	return b.String()
}

func convertUser(attrs map[string]any, provider, namespace string) string {
	name, ok := attrs["name"].(string)
	if !ok || name == "" {
		return ""
	}

	crName := sanitizeName(name)

	var b strings.Builder

	fmt.Fprintf(&b, "apiVersion: %s\nkind: User\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n", apiVersion, crName, namespace)
	fmt.Fprintf(&b, "  providerRef:\n    name: %s\n", provider)
	fmt.Fprintf(&b, "  deletionPolicy: Orphan\n")
	fmt.Fprintf(&b, "  name: %s\n", yamlString(name))

	indent := "  "

	emitField(&b, indent, "loginName", stringAttr(attrs, "login_name"))
	emitField(&b, indent, "displayName", stringAttr(attrs, "display_name"))
	emitField(&b, indent, "email", stringAttr(attrs, "email"))
	emitField(&b, indent, "firstName", stringAttr(attrs, "first_name"))
	emitField(&b, indent, "lastName", stringAttr(attrs, "last_name"))
	emitField(&b, indent, "comment", stringAttr(attrs, "comment"))
	emitField(&b, indent, "type", stringAttr(attrs, "user_type"))
	emitField(&b, indent, "defaultRole", stringAttr(attrs, "default_role"))
	emitField(&b, indent, "defaultSecondaryRoles", stringAttr(attrs, "default_secondary_roles_option"))
	emitField(&b, indent, "defaultWarehouse", stringAttr(attrs, "default_warehouse"))
	emitField(&b, indent, "defaultNamespace", stringAttr(attrs, "default_namespace"))
	emitBool(&b, indent, "mustChangePassword", boolAttr(attrs, "must_change_password"))
	emitBool(&b, indent, "disabled", boolAttr(attrs, "disabled"))

	// password, rsaPublicKey, rsaPublicKey2 are not imported — they use
	// SecretKeyReference in Snowplane and must be created manually.

	return b.String()
}

func convertAccountRole(attrs map[string]any, provider, namespace string) string {
	name, ok := attrs["name"].(string)
	if !ok || name == "" {
		return ""
	}

	crName := sanitizeName(name)

	var b strings.Builder

	fmt.Fprintf(&b, "apiVersion: %s\nkind: AccountRole\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n", apiVersion, crName, namespace)
	fmt.Fprintf(&b, "  providerRef:\n    name: %s\n", provider)
	fmt.Fprintf(&b, "  deletionPolicy: Orphan\n")
	fmt.Fprintf(&b, "  name: %s\n", yamlString(name))

	emitField(&b, "  ", "comment", stringAttr(attrs, "comment"))

	return b.String()
}

func convertDatabaseRole(attrs map[string]any, provider, namespace string) string {
	name, ok := attrs["name"].(string)
	if !ok || name == "" {
		return ""
	}

	database := ""
	if v := stringAttr(attrs, "database"); v != nil {
		database = *v
	}

	crName := sanitizeName(database + "-" + name)

	var b strings.Builder

	fmt.Fprintf(&b, "apiVersion: %s\nkind: DatabaseRole\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n", apiVersion, crName, namespace)
	fmt.Fprintf(&b, "  providerRef:\n    name: %s\n", provider)
	fmt.Fprintf(&b, "  deletionPolicy: Orphan\n")
	fmt.Fprintf(&b, "  name: %s\n", yamlString(name))

	if database != "" {
		fmt.Fprintf(&b, "  databaseRef:\n    name: %s\n", sanitizeName(database))
	}

	emitField(&b, "  ", "comment", stringAttr(attrs, "comment"))

	return b.String()
}

func convertGrantPrivilegesToAccountRole(attrs map[string]any, provider, namespace string) string {
	return convertGrant(attrs, provider, namespace, "GrantPrivilegesToAccountRole", "account_role_name", "accountRole", buildGrantPrivilegesToAccountRoleOnYAML)
}

func convertGrantPrivilegesToDatabaseRole(attrs map[string]any, provider, namespace string) string {
	return convertGrant(attrs, provider, namespace, "GrantPrivilegesToDatabaseRole", "database_role_name", "databaseRole", buildGrantPrivilegesToDatabaseRoleOnYAML)
}

// onYAMLBuilder is a function that inspects TF attrs and returns
// the YAML fragment for the "on:" section plus a short description for naming.
type onYAMLBuilder func(attrs map[string]any) (string, string)

// convertGrant handles the shared logic for GrantPrivilegesToAccountRole and GrantPrivilegesToDatabaseRole.
func convertGrant(attrs map[string]any, provider, namespace, kind, roleAttrKey, roleSpecKey string, buildOnYAML onYAMLBuilder) string {
	// The Terraform provider stores privileges as a list.
	privileges := sliceAttr(attrs, "privileges")
	if len(privileges) == 0 {
		// all_privileges shorthand
		if isTruthy(attrs, "all_privileges") {
			privileges = []string{"ALL PRIVILEGES"}
		}
	}

	roleName := ""
	if v := stringAttr(attrs, roleAttrKey); v != nil {
		roleName = *v
	}

	if roleName == "" || len(privileges) == 0 {
		return ""
	}

	// Determine the "on" target from nested blocks.
	onYAML, targetDesc := buildOnYAML(attrs)
	if onYAML == "" {
		return ""
	}

	var manifests []string

	for _, priv := range privileges {
		crName := sanitizeName(roleName + "-" + priv + "-" + targetDesc)

		var b strings.Builder

		fmt.Fprintf(&b, "apiVersion: %s\nkind: %s\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n", apiVersion, kind, crName, namespace)
		fmt.Fprintf(&b, "  providerRef:\n    name: %s\n", provider)
		fmt.Fprintf(&b, "  deletionPolicy: Orphan\n")

		if priv == "ALL PRIVILEGES" || priv == "ALL" {
			fmt.Fprintf(&b, "  allPrivileges: true\n")
		} else {
			fmt.Fprintf(&b, "  privilege: %s\n", yamlString(priv))
		}

		fmt.Fprintf(&b, "  %s: %s\n", roleSpecKey, yamlString(roleName))
		b.WriteString(onYAML)

		if isTruthy(attrs, "with_grant_option") {
			fmt.Fprintf(&b, "  withGrantOption: true\n")
		}

		manifests = append(manifests, b.String())
	}

	return strings.Join(manifests, "---\n")
}

// buildGrantPrivilegesToAccountRoleOnYAML inspects the Terraform "on_*" nested blocks and returns
// the YAML fragment for the "on:" section plus a short description for naming.
// GrantPrivilegesToAccountRole supports: on_account, on_account_object, on_schema, on_schema_object.
func buildGrantPrivilegesToAccountRoleOnYAML(attrs map[string]any) (string, string) {
	// on_account_object { object_type, object_name }
	if block := firstBlock(attrs, "on_account_object"); block != nil {
		objType := ""
		objName := ""

		if v := stringAttr(block, "object_type"); v != nil {
			objType = *v
		}

		if v := stringAttr(block, "object_name"); v != nil {
			objName = *v
		}

		if objType != "" && objName != "" {
			yaml := fmt.Sprintf("  on:\n    accountObject:\n      objectType: %s\n      objectName: %s\n",
				yamlString(objType), yamlString(objName))
			return yaml, objType + "-" + objName
		}
	}

	// on_schema_object { object_type, object_name } or { all / future }
	if block := firstBlock(attrs, "on_schema_object"); block != nil {
		objType := ""
		objName := ""

		if v := stringAttr(block, "object_type"); v != nil {
			objType = *v
		}

		if v := stringAttr(block, "object_name"); v != nil {
			objName = *v
		}

		if objType != "" && objName != "" {
			yaml := fmt.Sprintf("  on:\n    schemaObject:\n      objectType: %s\n      objectName: %s\n",
				yamlString(objType), yamlString(objName))
			return yaml, objType + "-" + objName
		}

		// future { object_type_plural, in_database / in_schema }
		if futureBlock := firstBlock(block, "future"); futureBlock != nil {
			return buildBulkOnYAML("future", futureBlock)
		}

		// all { object_type_plural, in_database / in_schema }
		if allBlock := firstBlock(block, "all"); allBlock != nil {
			return buildBulkOnYAML("all", allBlock)
		}
	}

	// on_schema { schema_name | all_schemas_in_database | future_schemas_in_database }
	if block := firstBlock(attrs, "on_schema"); block != nil {
		if v := stringAttr(block, "schema_name"); v != nil {
			yaml := fmt.Sprintf("  on:\n    schema:\n      schemaName: %s\n", yamlString(*v))
			return yaml, "schema-" + *v
		}

		if v := stringAttr(block, "all_schemas_in_database"); v != nil {
			yaml := fmt.Sprintf("  on:\n    schema:\n      allInDatabase: %s\n", yamlString(*v))
			return yaml, "all-schemas-" + *v
		}

		if v := stringAttr(block, "future_schemas_in_database"); v != nil {
			yaml := fmt.Sprintf("  on:\n    schema:\n      futureInDatabase: %s\n", yamlString(*v))
			return yaml, "future-schemas-" + *v
		}
	}

	// on_account
	if isTruthy(attrs, "on_account") {
		return "  on:\n    account: true\n", "account"
	}

	return "", ""
}

// buildGrantPrivilegesToDatabaseRoleOnYAML builds the "on:" YAML for GrantPrivilegesToDatabaseRole.
// GrantPrivilegesToDatabaseRole supports: on_database, on_schema, on_schema_object (no account-level).
// Maps TF's on_database to the flat `database:` field in GrantPrivilegesToDatabaseRoleOn.
func buildGrantPrivilegesToDatabaseRoleOnYAML(attrs map[string]any) (string, string) {
	// on_database is a simple string attribute in the TF provider for database role grants.
	if v := stringAttr(attrs, "on_database"); v != nil {
		yaml := fmt.Sprintf("  on:\n    database: %s\n", yamlString(*v))
		return yaml, "DATABASE-" + *v
	}

	// on_schema { schema_name | all_schemas_in_database | future_schemas_in_database }
	if block := firstBlock(attrs, "on_schema"); block != nil {
		if v := stringAttr(block, "schema_name"); v != nil {
			yaml := fmt.Sprintf("  on:\n    schema:\n      schemaName: %s\n", yamlString(*v))
			return yaml, "schema-" + *v
		}

		if v := stringAttr(block, "all_schemas_in_database"); v != nil {
			yaml := fmt.Sprintf("  on:\n    schema:\n      allInDatabase: %s\n", yamlString(*v))
			return yaml, "all-schemas-" + *v
		}

		if v := stringAttr(block, "future_schemas_in_database"); v != nil {
			yaml := fmt.Sprintf("  on:\n    schema:\n      futureInDatabase: %s\n", yamlString(*v))
			return yaml, "future-schemas-" + *v
		}
	}

	// on_schema_object { object_type, object_name } or { all / future }
	if block := firstBlock(attrs, "on_schema_object"); block != nil {
		objType := ""
		objName := ""

		if v := stringAttr(block, "object_type"); v != nil {
			objType = *v
		}

		if v := stringAttr(block, "object_name"); v != nil {
			objName = *v
		}

		if objType != "" && objName != "" {
			yaml := fmt.Sprintf("  on:\n    schemaObject:\n      objectType: %s\n      objectName: %s\n",
				yamlString(objType), yamlString(objName))
			return yaml, objType + "-" + objName
		}

		if futureBlock := firstBlock(block, "future"); futureBlock != nil {
			return buildBulkOnYAML("future", futureBlock)
		}

		if allBlock := firstBlock(block, "all"); allBlock != nil {
			return buildBulkOnYAML("all", allBlock)
		}
	}

	return "", ""
}

// buildBulkOnYAML builds the YAML for all/future grant targets.
func buildBulkOnYAML(bulkType string, block map[string]any) (string, string) {
	objPlural := ""
	if v := stringAttr(block, "object_type_plural"); v != nil {
		objPlural = *v
	}

	if objPlural == "" {
		return "", ""
	}

	var target, desc string

	if v := stringAttr(block, "in_schema"); v != nil {
		target = fmt.Sprintf("      inSchema: %s\n", yamlString(*v))
		desc = bulkType + "-" + objPlural + "-" + *v
	} else if v := stringAttr(block, "in_database"); v != nil {
		target = fmt.Sprintf("      inDatabase: %s\n", yamlString(*v))
		desc = bulkType + "-" + objPlural + "-" + *v
	} else {
		desc = bulkType + "-" + objPlural
	}

	yaml := fmt.Sprintf("  on:\n    schemaObject:\n      %s:\n        objectTypePlural: %s\n%s",
		bulkType, yamlString(objPlural), target)

	return yaml, desc
}

func convertGrantPrivilegesToShare(attrs map[string]any, provider, namespace string) string {
	privileges := sliceAttr(attrs, "privileges")
	if len(privileges) == 0 {
		if isTruthy(attrs, "all_privileges") {
			privileges = []string{"ALL PRIVILEGES"}
		}
	}

	share := ""
	if v := stringAttr(attrs, "share_name"); v != nil {
		share = *v
	}

	if share == "" || len(privileges) == 0 {
		return ""
	}

	// Determine the on-target field and value, matching GrantPrivilegesToShareOn flat fields.
	onField := ""
	onValue := ""

	if block := firstBlock(attrs, "on_database"); block != nil {
		onField = "database"

		if v := stringAttr(block, "database_name"); v != nil {
			onValue = *v
		}
	} else if block := firstBlock(attrs, "on_schema"); block != nil {
		onField = "schema"

		if v := stringAttr(block, "schema_name"); v != nil {
			onValue = *v
		}
	} else if block := firstBlock(attrs, "on_table"); block != nil {
		onField = "table"

		if v := stringAttr(block, "table_name"); v != nil {
			onValue = *v
		}
	} else if block := firstBlock(attrs, "on_all_tables_in_schema"); block != nil {
		onField = "allTablesInSchema"

		if v := stringAttr(block, "schema_name"); v != nil {
			onValue = *v
		}
	} else if block := firstBlock(attrs, "on_function"); block != nil {
		onField = "functionName"

		if v := stringAttr(block, "function_name"); v != nil {
			onValue = *v
		}
	} else if block := firstBlock(attrs, "on_tag"); block != nil {
		onField = "tag"

		if v := stringAttr(block, "tag_name"); v != nil {
			onValue = *v
		}
	} else if block := firstBlock(attrs, "on_view"); block != nil {
		onField = "view"

		if v := stringAttr(block, "view_name"); v != nil {
			onValue = *v
		}
	}

	if onField == "" || onValue == "" {
		return ""
	}

	var manifests []string

	for _, priv := range privileges {
		crName := sanitizeName(share + "-" + priv + "-" + onField + "-" + onValue)

		var b strings.Builder

		fmt.Fprintf(&b, "apiVersion: %s\nkind: GrantPrivilegesToShare\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n", apiVersion, crName, namespace)
		fmt.Fprintf(&b, "  providerRef:\n    name: %s\n", provider)
		fmt.Fprintf(&b, "  deletionPolicy: Orphan\n")
		fmt.Fprintf(&b, "  privilege: %s\n", yamlString(priv))
		fmt.Fprintf(&b, "  on:\n    %s: %s\n", onField, yamlString(onValue))
		fmt.Fprintf(&b, "  share: %s\n", yamlString(share))

		manifests = append(manifests, b.String())
	}

	return strings.Join(manifests, "---\n")
}

func convertTable(attrs map[string]any, provider, namespace string) string {
	name, ok := attrs["name"].(string)
	if !ok || name == "" {
		return ""
	}

	database := ""
	schema := ""

	if v := stringAttr(attrs, "database"); v != nil {
		database = *v
	}

	if v := stringAttr(attrs, "schema"); v != nil {
		schema = *v
	}

	crName := sanitizeName(database + "-" + schema + "-" + name)

	var b strings.Builder

	fmt.Fprintf(&b, "apiVersion: %s\nkind: Table\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n", apiVersion, crName, namespace)
	fmt.Fprintf(&b, "  providerRef:\n    name: %s\n", provider)
	fmt.Fprintf(&b, "  deletionPolicy: Orphan\n")
	fmt.Fprintf(&b, "  name: %s\n", yamlString(name))

	if database != "" {
		fmt.Fprintf(&b, "  databaseRef:\n    name: %s\n", sanitizeName(database))
	}

	if schema != "" {
		fmt.Fprintf(&b, "  schemaRef:\n    name: %s\n", sanitizeName(database+"-"+schema))
	}

	indent := "  "

	emitField(&b, indent, "comment", stringAttr(attrs, "comment"))

	if isTruthy(attrs, "is_transient") {
		fmt.Fprintf(&b, "  transient: true\n")
	}

	emitInt32(&b, indent, "dataRetentionTimeInDays", int32Attr(attrs, "data_retention_time_in_days"))
	emitBool(&b, indent, "changeTracking", boolAttr(attrs, "change_tracking"))
	emitField(&b, indent, "defaultDDLCollation", stringAttr(attrs, "default_ddl_collation"))
	emitBool(&b, indent, "enableSchemaEvolution", boolAttr(attrs, "enable_schema_evolution"))

	// cluster_by is a list of strings in Terraform.
	if clusterBy := sliceAttr(attrs, "cluster_by"); len(clusterBy) > 0 {
		fmt.Fprintf(&b, "  clusterBy:\n")
		for _, col := range clusterBy {
			fmt.Fprintf(&b, "    - %s\n", yamlString(col))
		}
	}

	// Columns
	if cols, ok := attrs["column"].([]any); ok && len(cols) > 0 {
		fmt.Fprintf(&b, "  columns:\n")

		for _, c := range cols {
			col, ok := c.(map[string]any)
			if !ok {
				continue
			}

			colName := ""
			colType := ""

			if v := stringAttr(col, "name"); v != nil {
				colName = *v
			}

			if v := stringAttr(col, "type"); v != nil {
				colType = *v
			}

			if colName == "" || colType == "" {
				continue
			}

			fmt.Fprintf(&b, "    - name: %s\n      type: %s\n", yamlString(colName), yamlString(colType))

			if v := boolAttr(col, "nullable"); v != nil {
				if *v {
					fmt.Fprintf(&b, "      nullable: true\n")
				} else {
					fmt.Fprintf(&b, "      nullable: false\n")
				}
			}

			emitField(&b, "      ", "default", stringAttr(col, "default"))
			emitField(&b, "      ", "comment", stringAttr(col, "comment"))
		}
	}

	return b.String()
}

func convertView(attrs map[string]any, provider, namespace string) string {
	name, ok := attrs["name"].(string)
	if !ok || name == "" {
		return ""
	}

	database := ""
	schema := ""

	if v := stringAttr(attrs, "database"); v != nil {
		database = *v
	}

	if v := stringAttr(attrs, "schema"); v != nil {
		schema = *v
	}

	crName := sanitizeName(database + "-" + schema + "-" + name)

	var b strings.Builder

	fmt.Fprintf(&b, "apiVersion: %s\nkind: View\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n", apiVersion, crName, namespace)
	fmt.Fprintf(&b, "  providerRef:\n    name: %s\n", provider)
	fmt.Fprintf(&b, "  deletionPolicy: Orphan\n")
	fmt.Fprintf(&b, "  name: %s\n", yamlString(name))

	if database != "" {
		fmt.Fprintf(&b, "  databaseRef:\n    name: %s\n", sanitizeName(database))
	}

	if schema != "" {
		fmt.Fprintf(&b, "  schemaRef:\n    name: %s\n", sanitizeName(database+"-"+schema))
	}

	indent := "  "

	// statement — the view's SQL definition
	if v := stringAttr(attrs, "statement"); v != nil {
		// Use YAML literal block scalar for multi-line SQL.
		fmt.Fprintf(&b, "  statement: |\n")
		for _, line := range strings.Split(*v, "\n") {
			fmt.Fprintf(&b, "    %s\n", line)
		}
	}

	if isTruthy(attrs, "is_secure") {
		fmt.Fprintf(&b, "  secure: true\n")
	}

	emitField(&b, indent, "comment", stringAttr(attrs, "comment"))
	emitBool(&b, indent, "changeTracking", boolAttr(attrs, "change_tracking"))

	return b.String()
}

func convertStage(attrs map[string]any, provider, namespace string) string {
	name, ok := attrs["name"].(string)
	if !ok || name == "" {
		return ""
	}

	database := ""
	schema := ""

	if v := stringAttr(attrs, "database"); v != nil {
		database = *v
	}

	if v := stringAttr(attrs, "schema"); v != nil {
		schema = *v
	}

	crName := sanitizeName(database + "-" + schema + "-" + name)

	var b strings.Builder

	fmt.Fprintf(&b, "apiVersion: %s\nkind: Stage\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n", apiVersion, crName, namespace)
	fmt.Fprintf(&b, "  providerRef:\n    name: %s\n", provider)
	fmt.Fprintf(&b, "  deletionPolicy: Orphan\n")
	fmt.Fprintf(&b, "  name: %s\n", yamlString(name))

	if database != "" {
		fmt.Fprintf(&b, "  databaseRef:\n    name: %s\n", sanitizeName(database))
	}

	if schema != "" {
		fmt.Fprintf(&b, "  schemaRef:\n    name: %s\n", sanitizeName(database+"-"+schema))
	}

	indent := "  "

	emitField(&b, indent, "url", stringAttr(attrs, "url"))
	emitField(&b, indent, "storageIntegration", stringAttr(attrs, "storage_integration"))
	emitField(&b, indent, "fileFormat", stringAttr(attrs, "file_format"))

	// encryption block
	if v := stringAttr(attrs, "encryption"); v != nil && *v != "" {
		fmt.Fprintf(&b, "  encryption:\n    type: %s\n", yamlString(*v))
	}

	// directory
	if isTruthy(attrs, "directory") {
		fmt.Fprintf(&b, "  directory:\n    enable: true\n")
	}

	emitField(&b, indent, "comment", stringAttr(attrs, "comment"))

	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers for nested Terraform blocks
// ---------------------------------------------------------------------------

// firstBlock extracts the first element of a nested Terraform block (list of
// objects in JSON state).
func firstBlock(attrs map[string]any, key string) map[string]any {
	v, ok := attrs[key]
	if !ok || v == nil {
		return nil
	}

	switch bv := v.(type) {
	case []any:
		if len(bv) == 0 {
			return nil
		}

		m, ok := bv[0].(map[string]any)
		if !ok {
			return nil
		}

		return m
	case map[string]any:
		return bv
	default:
		return nil
	}
}

// sliceAttr extracts a list of strings from a Terraform state attribute.
func sliceAttr(attrs map[string]any, key string) []string {
	v, ok := attrs[key]
	if !ok || v == nil {
		return nil
	}

	list, ok := v.([]any)
	if !ok {
		return nil
	}

	var result []string

	for _, item := range list {
		s, ok := item.(string)
		if ok && s != "" {
			result = append(result, s)
		}
	}

	return result
}

// sanitizeName converts a Snowflake identifier to a Kubernetes-safe name.
// - Lowercase
// - Replace underscores and dots with hyphens
// - Strip characters not matching [a-z0-9-]
// - Collapse consecutive hyphens
// - Trim leading/trailing hyphens
func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, " ", "-")

	// Strip characters not allowed in K8s names.
	name = sanitizeNameRe.ReplaceAllString(name, "")

	// Collapse consecutive hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	name = strings.Trim(name, "-")

	if name == "" {
		name = "unnamed"
	}

	// K8s names must be max 253 chars
	if len(name) > 253 {
		name = name[:253]
	}

	// Trim trailing hyphens that may result from truncation.
	name = strings.TrimRight(name, "-")

	if name == "" {
		name = "unnamed"
	}

	return name
}

var sanitizeNameRe = regexp.MustCompile(`[^a-z0-9-]`)

// resourceTypeMap maps Terraform resource types to converter functions.
var resourceTypeMap = map[string]func(map[string]any, string, string) string{
	"snowflake_database":                          convertDatabase,
	"snowflake_schema":                            convertSchema,
	"snowflake_warehouse":                         convertWarehouse,
	"snowflake_user":                              convertUser,
	"snowflake_account_role":                      convertAccountRole,
	"snowflake_role":                              convertAccountRole, // legacy resource name
	"snowflake_database_role":                     convertDatabaseRole,
	"snowflake_grant_privileges_to_account_role":  convertGrantPrivilegesToAccountRole,
	"snowflake_grant_privileges_to_database_role": convertGrantPrivilegesToDatabaseRole,
	"snowflake_grant_privileges_to_share":         convertGrantPrivilegesToShare,
	"snowflake_table":                             convertTable,
	"snowflake_view":                              convertView,
	"snowflake_stage":                             convertStage,
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	statePath := flag.String("state", "terraform.tfstate", "Path to Terraform state file")
	provider := flag.String("provider", "default", "Name of the Snowplane ProviderConfig to reference")
	namespace := flag.String("namespace", "default", "Kubernetes namespace for generated manifests")
	removeScript := flag.String("remove-script", "", "Write a state-removal script to this path (use - for stderr)")

	flag.Parse()

	data, err := os.ReadFile(*statePath)
	if err != nil {
		slog.Error("Failed to read state file", "path", *statePath, "error", err)
		os.Exit(1)
	}

	var state TerraformState
	if err := json.Unmarshal(data, &state); err != nil {
		slog.Error("Failed to parse state file", "path", *statePath, "error", err)
		os.Exit(1)
	}

	var manifests []string
	var stateAddresses []string

	for _, res := range state.Resources {
		converter, ok := resourceTypeMap[res.Type]
		if !ok {
			continue
		}

		for _, inst := range res.Instances {
			if inst.Attributes == nil {
				continue
			}

			result := converter(inst.Attributes, *provider, *namespace)
			if result != "" {
				manifests = append(manifests, result)
				stateAddresses = append(stateAddresses, buildStateAddress(res, inst))
			}
		}
	}

	if len(manifests) == 0 {
		slog.Warn("No supported Snowflake resources found in state file", "path", *statePath)
		return
	}

	fmt.Print(strings.Join(manifests, "---\n"))

	slog.Info("Generated manifests", "count", len(manifests))

	if *removeScript != "" {
		script := generateRemoveScript(stateAddresses)

		if *removeScript == "-" {
			fmt.Fprint(os.Stderr, script)
		} else {
			if err := os.WriteFile(*removeScript, []byte(script), 0o755); err != nil { //nolint:gosec // G306: script needs execute permission
				slog.Error("Failed to write remove script", "path", *removeScript, "error", err)
				os.Exit(1)
			}

			slog.Info("Wrote state-removal script", "path", *removeScript)
		}
	}
}

// buildStateAddress constructs the full Terraform state address for a resource
// instance, e.g. "module.foo.snowflake_database.my_db[0]".
func buildStateAddress(res StateResource, inst StateInstance) string {
	addr := res.Type + "." + res.Name

	// Append index key for count / for_each resources.
	switch idx := inst.IndexKey.(type) {
	case float64:
		addr += fmt.Sprintf("[%d]", int(idx))
	case string:
		addr += fmt.Sprintf("[%q]", idx)
	}

	// Prepend module path if present.
	if res.Module != "" {
		addr = res.Module + "." + addr
	}

	return addr
}

// generateRemoveScript creates a shell script that removes all converted
// resources from the Terraform / OpenTofu state.
func generateRemoveScript(addresses []string) string {
	var b strings.Builder

	b.WriteString(`#!/usr/bin/env bash
# Remove migrated Snowflake resources from Terraform / OpenTofu state.
#
# Usage:
#   chmod +x remove-from-state.sh
#   ./remove-from-state.sh              # uses "terraform" by default
#   TF_CMD=tofu ./remove-from-state.sh  # uses OpenTofu
#
# Add --dry-run to preview without removing:
#   ./remove-from-state.sh --dry-run
set -euo pipefail

TF="${TF_CMD:-terraform}"
DRY_RUN=false

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
  esac
done

echo "Using CLI: ${TF}"
echo ""

`)

	for _, addr := range addresses {
		fmt.Fprintf(&b, "echo \"==> Removing %s\"\n", addr)
		fmt.Fprintf(&b, "if \"$DRY_RUN\"; then\n")
		fmt.Fprintf(&b, "  echo \"  [dry-run] ${TF} state rm '%s'\"\n", addr)
		fmt.Fprintf(&b, "else\n")
		fmt.Fprintf(&b, "  \"${TF}\" state rm '%s' || echo \"  (already removed or not found)\"\n", addr)
		fmt.Fprintf(&b, "fi\n\n")
	}

	fmt.Fprintf(&b, "echo \"\"\necho \"Done — removed %d resource(s) from state.\"\n", len(addresses))

	return b.String()
}
