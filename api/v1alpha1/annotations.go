package v1alpha1

import "strings"

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

	// AnnotationForceDestroy controls whether the controller issues
	// DROP … CASCADE when deleting the resource. This cascading drop
	// removes all child Snowflake objects (e.g., all schemas, tables,
	// views inside a database). Use with extreme caution — child
	// Kubernetes CRs will become orphaned (pointing at non-existent
	// Snowflake objects). Only supported on resources whose Snowflake
	// DDL supports CASCADE (Database, Schema).
	AnnotationForceDestroy = "snowplane.hupe1980.github.io/force-destroy"
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

// IsForceDestroy returns true when the force-destroy annotation is set
// to "true", signaling the controller to issue DROP … CASCADE.
func IsForceDestroy(annotations map[string]string) bool {
	return annotations[AnnotationForceDestroy] == "true"
}

// boolAnnotationKeys lists all annotations that accept boolean "true"/"false" values.
var boolAnnotationKeys = []string{
	AnnotationForceNew,
	AnnotationAbandonOnDelete,
	AnnotationForceDestroy,
	AnnotationAllowDangerousGrant,
}

// ambiguousBoolValues are values that look like booleans but are not the
// canonical "true" that the annotation helpers check for.
var ambiguousBoolValues = map[string]bool{
	"True": true, "TRUE": true,
	"yes": true, "Yes": true, "YES": true,
	"1":  true,
	"on": true, "On": true, "ON": true,
	"false": true, "False": true, "FALSE": true,
	"no": true, "No": true, "NO": true,
	"0":   true,
	"off": true, "Off": true, "OFF": true,
}

// AmbiguousBoolAnnotations returns a list of human-readable warnings for any
// annotation that has a boolean-like value other than the canonical "true".
// The reconciler can emit these as Warning events to alert users about
// annotations that silently have no effect.
func AmbiguousBoolAnnotations(annotations map[string]string) []string {
	var warnings []string

	for _, key := range boolAnnotationKeys {
		v, ok := annotations[key]
		if !ok || v == "true" || v == "" {
			continue
		}

		if ambiguousBoolValues[v] {
			// Extract short annotation name for readability (e.g. "force-new").
			short := key
			if idx := strings.LastIndex(key, "/"); idx >= 0 {
				short = key[idx+1:]
			}

			warnings = append(warnings, "annotation "+short+" has value \""+v+"\" which is ignored; only \"true\" is recognized")
		}
	}

	return warnings
}
