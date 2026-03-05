package v1alpha1

// Condition type constants for Snowplane CRD status conditions.
const (
	// TypeReady indicates whether the resource is provisioned and synchronized.
	// Ready=False with terminal reasons (TerminalError, ValidationFailed,
	// ImmutableField, ResourceAlreadyExists) replaces the former TypeTerminal.
	// Ready=False with other reasons (ReconcileError, ClientCreationFailed, etc.)
	// replaces the former TypeRecoverable.
	TypeReady = "Ready"

	// TypeSynced indicates that the last reconciliation was successful.
	TypeSynced = "Synced"

	// TypeReferencesResolved indicates all cross-resource references have been resolved.
	TypeReferencesResolved = "ReferencesResolved"

	// TypeDriftDetected indicates that the observed Snowflake state differs from
	// the desired spec. The condition message contains a summary of drifted fields.
	TypeDriftDetected = "DriftDetected"
)

// Condition reason constants.
const (
	ReasonAvailable          = "Available"
	ReasonReconcileSuccess   = "ReconcileSuccess"
	ReasonReconcileError     = "ReconcileError"
	ReasonCreating           = "Creating"
	ReasonDeleting           = "Deleting"
	ReasonDriftDetected      = "DriftDetected"
	ReasonDependencyWait     = "DependencyWait"
	ReasonDependencyNotReady = "DependencyNotReady"
	ReasonCredentialsError   = "CredentialsError"
	ReasonTerminalError      = "TerminalError"
	ReasonImmutableField     = "ImmutableField"
	ReasonValidationFailed   = "ValidationFailed"
	ReasonSecretNotFound     = "SecretNotFound"
	ReasonInvalidConfig      = "InvalidConfig"
	ReasonClientFailed       = "ClientCreationFailed"
	ReasonPingFailed         = "PingFailed"
	ReasonDriftCorrected     = "DriftCorrected"

	ReasonAdopted               = "Adopted"
	ReasonResourceExists        = "ResourceAlreadyExists"
	ReasonRecoverableError      = "RecoverableError"
	ReasonNamespaceNotAllowed   = "NamespaceNotAllowed"
	ReasonDatabaseNotAllowed    = "DatabaseNotAllowed"
	ReasonSchemaNotAllowed      = "SchemaNotAllowed"
	ReasonOrphanedResource      = "OrphanedResource"
	ReasonConflictDetected      = "ConflictDetected"
	ReasonForceNewActive        = "ForceNewActive"
	ReasonCreateOrAlterFallback = "CreateOrAlterFallback"
	ReasonUnsupportedAnnotation = "UnsupportedAnnotation"
	ReasonRefResolutionFailed   = "RefResolutionFailed"
	ReasonFinalizerRemoved      = "FinalizerRemoved"
	ReasonCredentialsRotated    = "CredentialsRotated"
	ReasonInUse                 = "InUse"
	ReasonDeleteBlocked         = "DeleteBlocked"
	ReasonRoleNotAllowed        = "RoleNotAllowed"
	ReasonReconcilePaused       = "ReconcilePaused"
)
