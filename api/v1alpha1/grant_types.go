package v1alpha1

import "fmt"

// GrantKind indicates the grant category, which determines SQL generation and observation.
type GrantKind string

const (
	// GrantKindRegular is a normal privilege grant on an existing object.
	GrantKindRegular GrantKind = "Regular"

	// GrantKindFuture is a future grant that applies to objects not yet created.
	GrantKindFuture GrantKind = "Future"

	// GrantKindAll is a bulk grant on all existing objects of a type.
	GrantKindAll GrantKind = "All"
)

// GrantOn defines the target object for the grant.
// Exactly one field must be set.
//
// +kubebuilder:validation:XValidation:rule="(has(self.account) && self.account ? 1 : 0) + (has(self.accountObject) ? 1 : 0) + (has(self.schema) ? 1 : 0) + (has(self.schemaObject) ? 1 : 0) == 1",message="exactly one of account, accountObject, schema, or schemaObject must be set"
type GrantOn struct {
	// Account grants a global account-level privilege (e.g. CREATE DATABASE, MANAGE GRANTS).
	// Set to true for ON ACCOUNT.
	// +optional
	Account bool `json:"account,omitempty"`

	// AccountObject grants a privilege on an account-level object
	// (DATABASE, WAREHOUSE, USER, RESOURCE MONITOR, etc.).
	// +optional
	AccountObject *GrantOnAccountObject `json:"accountObject,omitempty"`

	// Schema grants a privilege on a schema or on all/future schemas in a database.
	// +optional
	Schema *GrantOnSchema `json:"schema,omitempty"`

	// SchemaObject grants a privilege on a specific schema object, or on
	// all/future schema objects of a given type in a database or schema.
	// +optional
	SchemaObject *GrantOnSchemaObject `json:"schemaObject,omitempty"`
}

// GrantOnAccountObject specifies a grant on an account-level object.
type GrantOnAccountObject struct {
	// ObjectType is the type of the account object.
	// Valid values: USER, RESOURCE MONITOR, WAREHOUSE, COMPUTE POOL, DATABASE,
	// INTEGRATION, CONNECTION, FAILOVER GROUP, REPLICATION GROUP, EXTERNAL VOLUME.
	ObjectType string `json:"objectType"`

	// ObjectName is the identifier of the object.
	ObjectName string `json:"objectName"`
}

// GrantOnSchema specifies a grant on a schema or set of schemas.
// Exactly one field must be set.
//
// +kubebuilder:validation:XValidation:rule="(has(self.schemaName) ? 1 : 0) + (has(self.schemaRef) ? 1 : 0) + (has(self.allInDatabase) ? 1 : 0) + (has(self.allInDatabaseRef) ? 1 : 0) + (has(self.futureInDatabase) ? 1 : 0) + (has(self.futureInDatabaseRef) ? 1 : 0) == 1",message="exactly one of schemaName, schemaRef, allInDatabase, allInDatabaseRef, futureInDatabase, or futureInDatabaseRef must be set"
type GrantOnSchema struct {
	// SchemaName is the fully qualified schema name (e.g. "MY_DB"."PUBLIC").
	// For: ON SCHEMA <schema_name>
	// +optional
	SchemaName string `json:"schemaName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// When set, the schema FQN is resolved from the CR's fullyQualifiedName.
	// Mutually exclusive with SchemaName.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// AllInDatabase grants on all existing schemas in the specified database.
	// For: ON ALL SCHEMAS IN DATABASE <db_name>
	// +optional
	AllInDatabase string `json:"allInDatabase,omitempty"`

	// AllInDatabaseRef references a Database CR for the ALL SCHEMAS IN DATABASE grant.
	// Mutually exclusive with AllInDatabase.
	// +optional
	AllInDatabaseRef *LocalObjectReference `json:"allInDatabaseRef,omitempty"`

	// FutureInDatabase grants on future schemas in the specified database.
	// For: ON FUTURE SCHEMAS IN DATABASE <db_name>
	// +optional
	FutureInDatabase string `json:"futureInDatabase,omitempty"`

	// FutureInDatabaseRef references a Database CR for the FUTURE SCHEMAS IN DATABASE grant.
	// Mutually exclusive with FutureInDatabase.
	// +optional
	FutureInDatabaseRef *LocalObjectReference `json:"futureInDatabaseRef,omitempty"`
}

// GrantOnSchemaObject specifies a grant on schema-level objects.
// Exactly one of (ObjectType+ObjectName), All, or Future must be set.
//
// +kubebuilder:validation:XValidation:rule="(has(self.objectType) && has(self.objectName) ? 1 : 0) + (has(self.all) ? 1 : 0) + (has(self.future) ? 1 : 0) == 1",message="exactly one of (objectType+objectName), all, or future must be set"
type GrantOnSchemaObject struct {
	// ObjectType is the schema object type (e.g. TABLE, VIEW, STAGE, FUNCTION, PROCEDURE, STREAM, TASK, PIPE).
	// Required when granting on a specific object.
	// +optional
	ObjectType string `json:"objectType,omitempty"`

	// ObjectName is the fully qualified name of the specific object.
	// Required when granting on a specific object.
	// +optional
	ObjectName string `json:"objectName,omitempty"`

	// All specifies granting on all existing objects of a given type.
	// For: ON ALL <type_plural> IN {DATABASE <db> | SCHEMA <schema>}
	// +optional
	All *GrantOnBulk `json:"all,omitempty"`

	// Future specifies granting on future objects of a given type.
	// For: ON FUTURE <type_plural> IN {DATABASE <db> | SCHEMA <schema>}
	// +optional
	Future *GrantOnBulk `json:"future,omitempty"`
}

// GrantOnBulk specifies a bulk or future grant target scope.
//
// +kubebuilder:validation:XValidation:rule="!(has(self.inDatabase) && has(self.inDatabaseRef))",message="inDatabase and inDatabaseRef are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!(has(self.inSchema) && has(self.inSchemaRef))",message="inSchema and inSchemaRef are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="(has(self.inDatabase) || has(self.inDatabaseRef) ? 1 : 0) + (has(self.inSchema) || has(self.inSchemaRef) ? 1 : 0) == 1",message="exactly one scope (database or schema) must be set"
type GrantOnBulk struct {
	// ObjectTypePlural is the plural form of the object type (e.g. TABLES, VIEWS, STAGES).
	ObjectTypePlural string `json:"objectTypePlural"`

	// InDatabase scopes the grant to all objects of the type in the specified database.
	// +optional
	InDatabase string `json:"inDatabase,omitempty"`

	// InDatabaseRef references a Database CR.
	// Mutually exclusive with InDatabase.
	// +optional
	InDatabaseRef *LocalObjectReference `json:"inDatabaseRef,omitempty"`

	// InSchema scopes the grant to all objects of the type in the specified schema.
	// +optional
	InSchema string `json:"inSchema,omitempty"`

	// InSchemaRef references a Schema CR.
	// Mutually exclusive with InSchema.
	// +optional
	InSchemaRef *LocalObjectReference `json:"inSchemaRef,omitempty"`
}

// GrantShowOutput mirrors the SHOW GRANTS output stored in status.
type GrantShowOutput struct {
	// CreatedOn is the timestamp when the grant was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Privilege is the granted privilege name.
	Privilege string `json:"privilege,omitempty"`

	// GrantedOn is the object type (from SHOW GRANTS: "granted_on" or "grant_on" for future grants).
	GrantedOn string `json:"grantedOn,omitempty"`

	// Name is the object name.
	Name string `json:"name,omitempty"`

	// GrantedTo is the grantee category (ROLE, DATABASE_ROLE, SHARE).
	GrantedTo string `json:"grantedTo,omitempty"`

	// GranteeName is the role or share name.
	GranteeName string `json:"granteeName,omitempty"`

	// GrantOption indicates whether WITH GRANT OPTION was used.
	GrantOption bool `json:"grantOption,omitempty"`

	// GrantedBy is the role that performed the grant.
	GrantedBy string `json:"grantedBy,omitempty"`
}

// resolveGrantKind determines the GrantKind from the On structure.
func resolveGrantKind(on *GrantOn) GrantKind {
	if on.Schema != nil && (on.Schema.FutureInDatabase != "" || on.Schema.FutureInDatabaseRef != nil) {
		return GrantKindFuture
	}

	if on.SchemaObject != nil && on.SchemaObject.Future != nil {
		return GrantKindFuture
	}

	if on.Schema != nil && (on.Schema.AllInDatabase != "" || on.Schema.AllInDatabaseRef != nil) {
		return GrantKindAll
	}

	if on.SchemaObject != nil && on.SchemaObject.All != nil {
		return GrantKindAll
	}

	return GrantKindRegular
}

// Description returns a human-readable description of the GrantOn.
func (o *GrantOn) Description() string {
	if o.Account {
		return "ON ACCOUNT"
	}

	if o.AccountObject != nil {
		return fmt.Sprintf("ON %s %s", o.AccountObject.ObjectType, o.AccountObject.ObjectName)
	}

	if o.Schema != nil {
		if o.Schema.SchemaName != "" {
			return "ON SCHEMA " + o.Schema.SchemaName
		}

		if o.Schema.SchemaRef != nil {
			return "ON SCHEMA (ref: " + o.Schema.SchemaRef.Name + ")"
		}

		if o.Schema.AllInDatabase != "" {
			return "ON ALL SCHEMAS IN DATABASE " + o.Schema.AllInDatabase
		}

		if o.Schema.AllInDatabaseRef != nil {
			return "ON ALL SCHEMAS IN DATABASE (ref: " + o.Schema.AllInDatabaseRef.Name + ")"
		}

		if o.Schema.FutureInDatabase != "" {
			return "ON FUTURE SCHEMAS IN DATABASE " + o.Schema.FutureInDatabase
		}

		if o.Schema.FutureInDatabaseRef != nil {
			return "ON FUTURE SCHEMAS IN DATABASE (ref: " + o.Schema.FutureInDatabaseRef.Name + ")"
		}
	}

	if o.SchemaObject != nil {
		if o.SchemaObject.ObjectType != "" && o.SchemaObject.ObjectName != "" {
			return fmt.Sprintf("ON %s %s", o.SchemaObject.ObjectType, o.SchemaObject.ObjectName)
		}

		if o.SchemaObject.All != nil {
			return o.SchemaObject.All.description("ALL")
		}

		if o.SchemaObject.Future != nil {
			return o.SchemaObject.Future.description("FUTURE")
		}
	}

	return "ON <unknown>"
}

// description builds a descriptive ON clause for bulk grants.
func (b *GrantOnBulk) description(keyword string) string {
	if b.InSchema != "" {
		return fmt.Sprintf("ON %s %s IN SCHEMA %s", keyword, b.ObjectTypePlural, b.InSchema)
	}

	if b.InSchemaRef != nil {
		return fmt.Sprintf("ON %s %s IN SCHEMA (ref: %s)", keyword, b.ObjectTypePlural, b.InSchemaRef.Name)
	}

	if b.InDatabase != "" {
		return fmt.Sprintf("ON %s %s IN DATABASE %s", keyword, b.ObjectTypePlural, b.InDatabase)
	}

	if b.InDatabaseRef != nil {
		return fmt.Sprintf("ON %s %s IN DATABASE (ref: %s)", keyword, b.ObjectTypePlural, b.InDatabaseRef.Name)
	}

	return fmt.Sprintf("ON %s %s", keyword, b.ObjectTypePlural)
}
