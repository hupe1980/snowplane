package v1alpha1

// CallableArgument defines an argument in a stored procedure or function signature.
type CallableArgument struct {
	// Name is the argument name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Type is the Snowflake data type (e.g. VARCHAR, NUMBER, TIMESTAMP_NTZ).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Type string `json:"type"`

	// DefaultValue is the optional default value for the argument.
	// For text values use single quotes. Numeric values can be unquoted.
	// +optional
	DefaultValue *string `json:"defaultValue,omitempty"`
}

// ProcedureShowOutput mirrors the SHOW PROCEDURES output stored in status.
// Shared by all procedure language variants.
type ProcedureShowOutput struct {
	// CreatedOn is the timestamp when the procedure was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the procedure name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Arguments is the procedure signature string (e.g. "MY_PROC(VARCHAR, NUMBER) RETURN VARCHAR").
	Arguments string `json:"arguments,omitempty"`

	// Description is the procedure comment/description.
	Description string `json:"description,omitempty" snowflake:"COMMENT"`

	// IsSecure indicates whether the procedure is a secure procedure.
	IsSecure bool `json:"isSecure,omitempty"`

	// Owner is the role that owns the procedure.
	Owner string `json:"owner,omitempty"`
}

// FunctionShowOutput mirrors the SHOW USER FUNCTIONS output stored in status.
// Shared by all function language variants.
type FunctionShowOutput struct {
	// CreatedOn is the timestamp when the function was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the function name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Arguments is the function signature string (e.g. "MY_FUNC(VARCHAR, NUMBER) RETURN VARCHAR").
	Arguments string `json:"arguments,omitempty"`

	// Description is the function comment/description.
	Description string `json:"description,omitempty" snowflake:"COMMENT"`

	// Language is the implementation language.
	Language string `json:"language,omitempty"`

	// IsSecure indicates whether the function is a secure function.
	IsSecure bool `json:"isSecure,omitempty"`

	// Owner is the role that owns the function.
	Owner string `json:"owner,omitempty"`
}
