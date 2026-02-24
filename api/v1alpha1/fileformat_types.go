package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FileFormatType specifies the type of file format.
// +kubebuilder:validation:Enum=CSV;JSON;AVRO;ORC;PARQUET;XML
type FileFormatType string

// Valid FileFormatType values.
const (
	FileFormatTypeCSV     FileFormatType = "CSV"
	FileFormatTypeJSON    FileFormatType = "JSON"
	FileFormatTypeAVRO    FileFormatType = "AVRO"
	FileFormatTypeORC     FileFormatType = "ORC"
	FileFormatTypePARQUET FileFormatType = "PARQUET"
	FileFormatTypeXML     FileFormatType = "XML"
)

// FileFormatSpec defines the desired state of a Snowflake File Format.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.type == oldSelf.type",message="spec.type is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
type FileFormatSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake file format name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the raw Snowflake database identifier.
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the raw Snowflake schema FQN.
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SchemaName *string `json:"schemaName,omitempty"`

	// Type specifies the file format type. Immutable after creation.
	Type FileFormatType `json:"type"`

	// -- CSV-specific options --

	// FieldDelimiter specifies the character used to separate fields (CSV only).
	// +optional
	FieldDelimiter *string `json:"fieldDelimiter,omitempty"`

	// RecordDelimiter specifies the character used to separate records (CSV only).
	// +optional
	RecordDelimiter *string `json:"recordDelimiter,omitempty"`

	// SkipHeader specifies the number of lines to skip at the start (CSV only).
	// +optional
	SkipHeader *int32 `json:"skipHeader,omitempty"`

	// FieldOptionallyEnclosedBy specifies the character used to enclose strings (CSV only).
	// +optional
	FieldOptionallyEnclosedBy *string `json:"fieldOptionallyEnclosedBy,omitempty"`

	// Escape specifies the escape character for enclosed fields (CSV only).
	// +optional
	Escape *string `json:"escape,omitempty"`

	// EscapeUnenclosedField specifies the escape character for unenclosed fields (CSV only).
	// +optional
	EscapeUnenclosedField *string `json:"escapeUnenclosedField,omitempty"`

	// EmptyFieldAsNull specifies whether to treat empty fields as NULL (CSV only).
	// +optional
	EmptyFieldAsNull *bool `json:"emptyFieldAsNull,omitempty"`

	// ErrorOnColumnCountMismatch aborts if the number of columns doesn't match (CSV only).
	// +optional
	ErrorOnColumnCountMismatch *bool `json:"errorOnColumnCountMismatch,omitempty"`

	// SkipBlankLines skips blank lines in CSV data (CSV only).
	// +optional
	SkipBlankLines *bool `json:"skipBlankLines,omitempty"`

	// ParseHeader determines whether to use the first row as column names (CSV only).
	// +optional
	ParseHeader *bool `json:"parseHeader,omitempty"`

	// Encoding specifies the character set encoding of the source data (CSV only).
	// Examples: "UTF-8", "UTF-16", "WINDOWS-1252"
	// +optional
	Encoding *string `json:"encoding,omitempty"`

	// NullIf specifies strings that represent NULL values (CSV/JSON).
	// +optional
	NullIf []string `json:"nullIf,omitempty"`

	// -- JSON-specific options --

	// StripOuterArray removes outer brackets from JSON arrays (JSON only).
	// +optional
	StripOuterArray *bool `json:"stripOuterArray,omitempty"`

	// StripNullValues removes key-value pairs with null values (JSON only).
	// +optional
	StripNullValues *bool `json:"stripNullValues,omitempty"`

	// EnableOctal enables parsing of octal number strings (JSON only).
	// +optional
	EnableOctal *bool `json:"enableOctal,omitempty"`

	// AllowDuplicate allows duplicate object field names in JSON (JSON only).
	// +optional
	AllowDuplicate *bool `json:"allowDuplicate,omitempty"`

	// -- Parquet-specific options --

	// BinaryAsText interprets Parquet BINARY columns as text (Parquet only).
	// +optional
	BinaryAsText *bool `json:"binaryAsText,omitempty"`

	// UseLogicalType instructs use of Parquet logical types (Parquet only).
	// +optional
	UseLogicalType *bool `json:"useLogicalType,omitempty"`

	// SnappyCompression uses Snappy compression for Parquet output (Parquet only).
	// +optional
	SnappyCompression *bool `json:"snappyCompression,omitempty"`

	// -- XML-specific options --

	// PreserveSpace preserves leading/trailing whitespace in XML (XML only).
	// +optional
	PreserveSpace *bool `json:"preserveSpace,omitempty"`

	// StripOuterElement strips the outer XML element (XML only).
	// +optional
	StripOuterElement *bool `json:"stripOuterElement,omitempty"`

	// DisableAutoConvert disables automatic conversion of numeric/boolean strings (XML only).
	// +optional
	DisableAutoConvert *bool `json:"disableAutoConvert,omitempty"`

	// DisableSnowflakeData disables Snowflake semi-structured data parsing (JSON only).
	// +optional
	DisableSnowflakeData *bool `json:"disableSnowflakeData,omitempty"`

	// -- Cross-format options --

	// ReplaceInvalidCharacters replaces invalid UTF-8 characters with the
	// Unicode replacement character (all formats).
	// +optional
	ReplaceInvalidCharacters *bool `json:"replaceInvalidCharacters,omitempty"`

	// SkipByteOrderMark skips the BOM character at the start (CSV/JSON).
	// +optional
	SkipByteOrderMark *bool `json:"skipByteOrderMark,omitempty"`

	// IgnoreUtf8Errors ignores UTF-8 encoding errors (CSV/JSON/XML).
	// +optional
	IgnoreUtf8Errors *bool `json:"ignoreUtf8Errors,omitempty"`

	// DateFormat specifies the format for date values (CSV/JSON).
	// +optional
	DateFormat *string `json:"dateFormat,omitempty"`

	// TimeFormat specifies the format for time values (CSV/JSON).
	// +optional
	TimeFormat *string `json:"timeFormat,omitempty"`

	// TimestampFormat specifies the format for timestamp values (CSV/JSON).
	// +optional
	TimestampFormat *string `json:"timestampFormat,omitempty"`

	// BinaryFormat specifies the encoding format for binary input/output (CSV/JSON).
	// +optional
	// +kubebuilder:validation:Enum=HEX;BASE64;UTF8
	BinaryFormat *string `json:"binaryFormat,omitempty"`

	// Compression specifies the compression algorithm.
	// +optional
	// +kubebuilder:validation:Enum=AUTO;GZIP;BZ2;BROTLI;ZSTD;DEFLATE;RAW_DEFLATE;NONE
	Compression *string `json:"compression,omitempty"`

	// -- Common options --

	// TrimSpace removes leading/trailing whitespace from string values.
	// +optional
	TrimSpace *bool `json:"trimSpace,omitempty"`

	// Comment is an optional description for the file format.
	// +optional
	Comment *string `json:"comment,omitempty"`
}

// FileFormatShowOutput mirrors the SHOW FILE FORMATS output stored in status.
type FileFormatShowOutput struct {
	// CreatedOn is the timestamp when the file format was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the file format name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Owner is the role that owns the file format.
	Owner string `json:"owner,omitempty"`

	// Comment is the file format description.
	Comment string `json:"comment,omitempty"`

	// Type is the file format type (CSV, JSON, etc.).
	Type string `json:"type,omitempty"`
}

// FileFormatStatus defines the observed state of a FileFormat.
type FileFormatStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW FILE FORMATS output for this file format.
	ShowOutput *FileFormatShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// FileFormat is the Schema for the fileformats API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type FileFormat struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FileFormatSpec   `json:"spec,omitempty"`
	Status FileFormatStatus `json:"status,omitempty"`
}

// FileFormatList contains a list of FileFormat.
// +kubebuilder:object:root=true
type FileFormatList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FileFormat `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FileFormat{}, &FileFormatList{})
}
