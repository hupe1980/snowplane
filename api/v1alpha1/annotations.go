package v1alpha1

// Annotation and label keys used by Snowplane controllers, webhooks, and CRDs.
const (
	// AnnotationForceNew signals the controller to delete and recreate the
	// Snowflake resource when an immutable field changes. Without this
	// annotation, immutable-field changes are rejected as terminal errors.
	// Set the value to "true" to opt-in.
	AnnotationForceNew = "snowplane.hupe1980.github.io/force-new"

	// AnnotationAdoptionPolicy controls how the reconciler handles pre-existing
	// Snowflake resources on first reconciliation. When set to "adopt", the
	// reconciler takes over management of the existing resource. When absent
	// or set to "fail-if-exists", the reconciler returns a Terminal error if
	// the resource already exists.
	AnnotationAdoptionPolicy = "snowplane.hupe1980.github.io/adoption-policy"

	// AnnotationDriftPolicy controls whether drift is corrected or only reported.
	// Set to "detect-only" to report drift via conditions and events without
	// issuing ALTER statements.
	AnnotationDriftPolicy = "snowplane.hupe1980.github.io/drift-policy"

	// AnnotationAllowDangerousGrant opts in to granting privileges that are
	// normally blocked by the privilege safety checks. Set to "true" to allow
	// granting MANAGE GRANTS, OWNERSHIP, or targeting system roles
	// (ACCOUNTADMIN, SECURITYADMIN, ORGADMIN). This annotation acts as an
	// explicit acknowledgement of the security implications.
	AnnotationAllowDangerousGrant = "snowplane.hupe1980.github.io/allow-dangerous-grant"

	// AnnotationUseCreateOrAlter controls whether CREATE OR ALTER is used
	// instead of the default CREATE IF NOT EXISTS + ALTER two-step flow.
	// CREATE OR ALTER is enabled by default for supported types (Database,
	// Schema, Table, Warehouse). Set to "false" to opt out and use the
	// legacy two-step flow. For unsupported types the annotation is ignored.
	AnnotationUseCreateOrAlter = "snowplane.hupe1980.github.io/use-create-or-alter"

	// AnnotationCreationInitiated is set by the reconciler just before
	// issuing a Snowflake CREATE. If the controller crashes after CREATE
	// succeeds but before the status patches, the presence of this
	// annotation tells the next reconcile that *we* created the resource
	// and it should be treated as a post-crash continuation rather than
	// triggering the adoption-or-reject path.
	AnnotationCreationInitiated = "snowplane.hupe1980.github.io/creation-initiated"

	// AnnotationLateInitialized indicates that status.atProvider fields were
	// populated from an existing Snowflake resource during adoption. Set to
	// "true" for adopted resources.
	AnnotationLateInitialized = "snowplane.hupe1980.github.io/late-initialized"
)

// Label keys applied to CRD metadata (not CR annotations).
const (
	// LabelMaturity is the CRD label key for maturity classification.
	// Applied to CRD metadata.labels to indicate stability guarantees.
	// Valid values are "alpha", "beta", and "stable".
	LabelMaturity = "snowplane.hupe1980.github.io/maturity"
)

// Adoption policy values.
const (
	// AdoptionPolicyAdopt allows the reconciler to adopt an existing Snowflake
	// resource, populating status from the current state.
	AdoptionPolicyAdopt = "adopt"

	// AdoptionPolicyFailIfExists (the default) causes the reconciler to return
	// a Terminal error when the Snowflake resource already exists.
	AdoptionPolicyFailIfExists = "fail-if-exists"
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

// IsCreateOrAlter returns true when CREATE OR ALTER should be used.
// CREATE OR ALTER is enabled by default; set the annotation to "false" to opt out.
func IsCreateOrAlter(annotations map[string]string) bool {
	return annotations[AnnotationUseCreateOrAlter] != "false"
}

// IsForceNew returns true when the force-new annotation is set to "true",
// signaling the controller to delete and recreate the resource on immutable
// field changes.
func IsForceNew(annotations map[string]string) bool {
	return annotations[AnnotationForceNew] == "true"
}
