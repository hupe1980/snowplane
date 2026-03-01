package v1alpha1

// Annotation and label keys used by Snowplane controllers and CRDs.
const (
	// AnnotationForceNew signals the controller to delete and recreate the
	// Snowflake resource when an immutable field changes. Without this
	// annotation, immutable-field changes are rejected as terminal errors.
	// Set the value to "true" to opt-in.
	AnnotationForceNew = "snowplane.hupe1980.github.io/force-new"

	// AnnotationAllowDangerousGrant opts in to granting privileges that are
	// normally blocked by the privilege safety checks. Set to "true" to allow
	// granting MANAGE GRANTS, OWNERSHIP, or targeting system roles
	// (ACCOUNTADMIN, SECURITYADMIN, ORGADMIN). This annotation acts as an
	// explicit acknowledgement of the security implications.
	AnnotationAllowDangerousGrant = "snowplane.hupe1980.github.io/allow-dangerous-grant"

	// AnnotationCreationInitiated is set by the reconciler just before
	// issuing a Snowflake CREATE. If the controller crashes after CREATE
	// succeeds but before the status patches, the presence of this
	// annotation tells the next reconcile that *we* created the resource
	// and it should be treated as a post-crash continuation rather than
	// triggering the adoption-or-reject path.
	AnnotationCreationInitiated = "internal.snowplane.hupe1980.github.io/creation-initiated"

	// AnnotationLateInitialized indicates that status.atProvider fields were
	// populated from an existing Snowflake resource during adoption. Set to
	// "true" for adopted resources.
	AnnotationLateInitialized = "internal.snowplane.hupe1980.github.io/late-initialized"

	// AnnotationAbandonOnDelete controls whether the controller removes the
	// finalizer without attempting to drop the Snowflake resource. Set to
	// "true" on a resource that is pending deletion to unblock garbage
	// collection when the DROP is permanently blocked (e.g., insufficient
	// privileges). The Snowflake resource will remain and may require
	// manual cleanup.
	AnnotationAbandonOnDelete = "snowplane.hupe1980.github.io/abandon-on-delete"
)

// Label keys applied to CRD metadata (not CR annotations).
const (
	// LabelMaturity is the CRD label key for maturity classification.
	// Applied to CRD metadata.labels to indicate stability guarantees.
	// Valid values are "alpha", "beta", and "stable".
	LabelMaturity = "snowplane.hupe1980.github.io/maturity"

	// LabelExternalNameHash is a label applied to managed CRs containing a
	// truncated SHA-256 hex digest of the Snowflake fully-qualified name.
	// Used for same-cluster ownership conflict detection during adoption:
	// before adopting, the reconciler lists all CRs of the same GVK with
	// the same label value and rejects if another CR (different UID) already
	// claims the same Snowflake resource.
	LabelExternalNameHash = "snowplane.hupe1980.github.io/external-name-hash"
)

// Maturity classification values.
const (
	// MaturityAlpha indicates an alpha-quality CRD with no stability guarantees.
	MaturityAlpha = "alpha"

	// MaturityBeta indicates a beta-quality CRD with backwards-compatible changes expected.
	MaturityBeta = "beta"

	// MaturityStable indicates a stable CRD with full backwards-compatibility guarantees.
	MaturityStable = "stable"
)

// IsForceNew returns true when the force-new annotation is set to "true",
// signaling the controller to delete and recreate the resource on immutable
// field changes.
func IsForceNew(annotations map[string]string) bool {
	return annotations[AnnotationForceNew] == "true"
}

// IsAbandonOnDelete returns true when the abandon-on-delete annotation is set
// to "true", signaling the controller to remove the finalizer without
// attempting to drop the Snowflake resource.
func IsAbandonOnDelete(annotations map[string]string) bool {
	return annotations[AnnotationAbandonOnDelete] == "true"
}
