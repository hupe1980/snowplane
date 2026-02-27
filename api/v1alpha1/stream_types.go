package v1alpha1

// StreamSourceType specifies the type of source object for a Snowflake Stream.
// +kubebuilder:validation:Enum=TABLE;VIEW;EXTERNAL_TABLE;STAGE;DYNAMIC_TABLE
type StreamSourceType string

// Valid StreamSourceType values.
const (
	StreamSourceTable         StreamSourceType = "TABLE"
	StreamSourceView          StreamSourceType = "VIEW"
	StreamSourceExternalTable StreamSourceType = "EXTERNAL_TABLE"
	StreamSourceStage         StreamSourceType = "STAGE"
	StreamSourceDynamicTable  StreamSourceType = "DYNAMIC_TABLE"
)

// StreamShowOutput mirrors the SHOW STREAMS output stored in status.
type StreamShowOutput struct {
	// CreatedOn is the timestamp when the stream was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the stream name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Owner is the role that owns the stream.
	Owner string `json:"owner,omitempty"`

	// Comment is the stream description.
	Comment string `json:"comment,omitempty"`

	// TableName is the fully qualified source object name.
	TableName string `json:"tableName,omitempty"`

	// SourceType is the type of source object (TABLE, VIEW, STAGE, etc.).
	SourceType string `json:"sourceType,omitempty"`

	// Mode is the stream mode (DEFAULT, APPEND_ONLY, INSERT_ONLY).
	Mode string `json:"mode,omitempty"`

	// Stale is whether the stream is stale.
	Stale bool `json:"stale,omitempty"`

	// StaleAfter is the timestamp after which the stream becomes stale.
	StaleAfter string `json:"staleAfter,omitempty"`
}
