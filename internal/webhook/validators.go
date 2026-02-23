// Package webhook provides validating and mutating admission webhooks for Snowplane CRDs.
package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// denyAll converts an errors.Join result into a single Denied response.
// Returns Allowed when errs is nil.
func denyAll(errs error) admission.Response {
	if errs != nil {
		return admission.Denied(errs.Error())
	}

	return admission.Allowed("")
}

// hasForceNew returns true if the object carries the force-new annotation.
func hasForceNew(annotations map[string]string) bool {
	return annotations[snowplanev1alpha1.AnnotationForceNew] == "true"
}

// --------------------------------------------------------------------------
// Generic resource validator
// --------------------------------------------------------------------------

// resourceValidator is the generic implementation shared by all CRD validators.
// It handles the common decode → validate → immutable-check → denyAll scaffold,
// letting each resource type provide only its immutable-field comparison via a closure.
//
// Type parameters:
//   - T: the CRD struct type (e.g. snowplanev1alpha1.Database)
//   - PT: pointer to T satisfying the required object interfaces
type resourceValidator[T any, PT interface {
	*T
	client.Object
	ValidateSpec() error
	GetObservedGeneration() int64
}] struct {
	decoder        admission.Decoder
	immutableCheck func(old, cur PT) []error
	extraChecks    []func(PT) []error
}

// newResourceValidator creates a validating admission handler for a CRD type.
// immutableCheck is called on UPDATE when ObservedGeneration > 0 and force-new
// is not set. extraChecks run on every request (CREATE and UPDATE) after
// ValidateSpec.
func newResourceValidator[T any, PT interface {
	*T
	client.Object
	ValidateSpec() error
	GetObservedGeneration() int64
}](
	scheme *runtime.Scheme,
	immutableCheck func(old, cur PT) []error,
	extraChecks ...func(PT) []error,
) admission.Handler {
	return &resourceValidator[T, PT]{
		decoder:        admission.NewDecoder(scheme),
		immutableCheck: immutableCheck,
		extraChecks:    extraChecks,
	}
}

// Handle implements admission.Handler.
func (v *resourceValidator[T, PT]) Handle(_ context.Context, req admission.Request) admission.Response {
	newObj := PT(new(T))
	if err := v.decoder.Decode(req, newObj); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding new object: %w", err))
	}

	var errs []error

	if err := newObj.ValidateSpec(); err != nil {
		errs = append(errs, err)
	}

	for _, check := range v.extraChecks {
		errs = append(errs, check(newObj)...)
	}

	if req.Operation == admissionv1.Update && v.immutableCheck != nil {
		oldObj := PT(new(T))
		if err := v.decoder.DecodeRaw(req.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding old object: %w", err))
		}

		if oldObj.GetObservedGeneration() > 0 && !hasForceNew(newObj.GetAnnotations()) {
			errs = append(errs, v.immutableCheck(oldObj, newObj)...)
		}
	}

	return denyAll(errors.Join(errs...))
}

// --------------------------------------------------------------------------
// Resource validator constructors
// --------------------------------------------------------------------------

// NewDatabaseValidator creates a validating admission handler for Database resources.
func NewDatabaseValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.Database](scheme,
		func(old, cur *snowplanev1alpha1.Database) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			if old.Spec.Transient != cur.Spec.Transient {
				errs = append(errs, fmt.Errorf(
					"spec.transient is immutable after creation (current: %v, desired: %v)",
					old.Spec.Transient, cur.Spec.Transient,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewSchemaValidator creates a validating admission handler for Schema resources.
func NewSchemaValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.Schema](scheme,
		func(old, cur *snowplanev1alpha1.Schema) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			if old.Spec.DatabaseRef.Name != cur.Spec.DatabaseRef.Name {
				errs = append(errs, fmt.Errorf(
					"spec.databaseRef.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.DatabaseRef.Name, cur.Spec.DatabaseRef.Name,
				))
			}

			if old.Spec.Transient != cur.Spec.Transient {
				errs = append(errs, fmt.Errorf(
					"spec.transient is immutable after creation (current: %v, desired: %v)",
					old.Spec.Transient, cur.Spec.Transient,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewWarehouseValidator creates a validating admission handler for Warehouse resources.
func NewWarehouseValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.Warehouse](scheme,
		func(old, cur *snowplanev1alpha1.Warehouse) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewAccountRoleValidator creates a validating admission handler for AccountRole resources.
func NewAccountRoleValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.AccountRole](scheme,
		func(old, cur *snowplanev1alpha1.AccountRole) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewDatabaseRoleValidator creates a validating admission handler for DatabaseRole resources.
func NewDatabaseRoleValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.DatabaseRole](scheme,
		func(old, cur *snowplanev1alpha1.DatabaseRole) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			if old.Spec.DatabaseRef.Name != cur.Spec.DatabaseRef.Name {
				errs = append(errs, fmt.Errorf(
					"spec.databaseRef.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.DatabaseRef.Name, cur.Spec.DatabaseRef.Name,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewUserValidator creates a validating admission handler for User resources.
func NewUserValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.User](scheme,
		func(old, cur *snowplanev1alpha1.User) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.type", old.Spec.Type, cur.Spec.Type,
			)...)

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewAccountRoleGrantValidator creates a validating admission handler for AccountRoleGrant resources.
func NewAccountRoleGrantValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.AccountRoleGrant](scheme,
		func(old, cur *snowplanev1alpha1.AccountRoleGrant) []error {
			var errs []error

			if old.Spec.Privilege != cur.Spec.Privilege {
				errs = append(errs, fmt.Errorf(
					"spec.privilege is immutable after creation (current: %q, desired: %q)",
					old.Spec.Privilege, cur.Spec.Privilege,
				))
			}

			if old.Spec.On.Description() != cur.Spec.On.Description() {
				errs = append(errs, fmt.Errorf(
					"spec.on is immutable after creation (current: %q, desired: %q)",
					old.Spec.On.Description(), cur.Spec.On.Description(),
				))
			}

			if old.Spec.AccountRole != cur.Spec.AccountRole {
				errs = append(errs, fmt.Errorf(
					"spec.accountRole is immutable after creation (current: %q, desired: %q)",
					old.Spec.AccountRole, cur.Spec.AccountRole,
				))
			}

			if old.Spec.WithGrantOption != cur.Spec.WithGrantOption {
				errs = append(errs, fmt.Errorf(
					"spec.withGrantOption is immutable after creation (current: %v, desired: %v)",
					old.Spec.WithGrantOption, cur.Spec.WithGrantOption,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
		// Extra check: dangerous privilege/target combinations unless opted in.
		func(grant *snowplanev1alpha1.AccountRoleGrant) []error {
			if grant.Annotations[snowplanev1alpha1.AnnotationAllowDangerousGrant] != "true" {
				if err := snowplanev1alpha1.ValidateDangerousAccountRoleGrant(&grant.Spec); err != nil {
					return []error{err}
				}
			}

			return nil
		},
	)
}

// NewDatabaseRoleGrantValidator creates a validating admission handler for DatabaseRoleGrant resources.
func NewDatabaseRoleGrantValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.DatabaseRoleGrant](scheme,
		func(old, cur *snowplanev1alpha1.DatabaseRoleGrant) []error {
			var errs []error

			if old.Spec.Privilege != cur.Spec.Privilege {
				errs = append(errs, fmt.Errorf(
					"spec.privilege is immutable after creation (current: %q, desired: %q)",
					old.Spec.Privilege, cur.Spec.Privilege,
				))
			}

			if old.Spec.On.Description() != cur.Spec.On.Description() {
				errs = append(errs, fmt.Errorf(
					"spec.on is immutable after creation (current: %q, desired: %q)",
					old.Spec.On.Description(), cur.Spec.On.Description(),
				))
			}

			if old.Spec.DatabaseRole != cur.Spec.DatabaseRole {
				errs = append(errs, fmt.Errorf(
					"spec.databaseRole is immutable after creation (current: %q, desired: %q)",
					old.Spec.DatabaseRole, cur.Spec.DatabaseRole,
				))
			}

			if old.Spec.WithGrantOption != cur.Spec.WithGrantOption {
				errs = append(errs, fmt.Errorf(
					"spec.withGrantOption is immutable after creation (current: %v, desired: %v)",
					old.Spec.WithGrantOption, cur.Spec.WithGrantOption,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewShareGrantValidator creates a validating admission handler for ShareGrant resources.
func NewShareGrantValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.ShareGrant](scheme,
		func(old, cur *snowplanev1alpha1.ShareGrant) []error {
			var errs []error

			if old.Spec.Privilege != cur.Spec.Privilege {
				errs = append(errs, fmt.Errorf(
					"spec.privilege is immutable after creation (current: %q, desired: %q)",
					old.Spec.Privilege, cur.Spec.Privilege,
				))
			}

			if old.Spec.ObjectType != cur.Spec.ObjectType {
				errs = append(errs, fmt.Errorf(
					"spec.objectType is immutable after creation (current: %q, desired: %q)",
					old.Spec.ObjectType, cur.Spec.ObjectType,
				))
			}

			if old.Spec.ObjectName != cur.Spec.ObjectName {
				errs = append(errs, fmt.Errorf(
					"spec.objectName is immutable after creation (current: %q, desired: %q)",
					old.Spec.ObjectName, cur.Spec.ObjectName,
				))
			}

			if old.Spec.Share != cur.Spec.Share {
				errs = append(errs, fmt.Errorf(
					"spec.share is immutable after creation (current: %q, desired: %q)",
					old.Spec.Share, cur.Spec.Share,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewTableValidator creates a validating admission handler for Table resources.
func NewTableValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.Table](scheme,
		func(old, cur *snowplanev1alpha1.Table) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			if old.Spec.DatabaseRef.Name != cur.Spec.DatabaseRef.Name {
				errs = append(errs, fmt.Errorf(
					"spec.databaseRef.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.DatabaseRef.Name, cur.Spec.DatabaseRef.Name,
				))
			}

			if old.Spec.SchemaRef.Name != cur.Spec.SchemaRef.Name {
				errs = append(errs, fmt.Errorf(
					"spec.schemaRef.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.SchemaRef.Name, cur.Spec.SchemaRef.Name,
				))
			}

			if old.Spec.Transient != cur.Spec.Transient {
				errs = append(errs, fmt.Errorf(
					"spec.transient is immutable after creation (current: %v, desired: %v)",
					old.Spec.Transient, cur.Spec.Transient,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewViewValidator creates a validating admission handler for View resources.
func NewViewValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.View](scheme,
		func(old, cur *snowplanev1alpha1.View) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			if old.Spec.DatabaseRef.Name != cur.Spec.DatabaseRef.Name {
				errs = append(errs, fmt.Errorf(
					"spec.databaseRef.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.DatabaseRef.Name, cur.Spec.DatabaseRef.Name,
				))
			}

			if old.Spec.SchemaRef.Name != cur.Spec.SchemaRef.Name {
				errs = append(errs, fmt.Errorf(
					"spec.schemaRef.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.SchemaRef.Name, cur.Spec.SchemaRef.Name,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewStageValidator creates a validating admission handler for Stage resources.
func NewStageValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.Stage](scheme,
		func(old, cur *snowplanev1alpha1.Stage) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			if old.Spec.DatabaseRef.Name != cur.Spec.DatabaseRef.Name {
				errs = append(errs, fmt.Errorf(
					"spec.databaseRef.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.DatabaseRef.Name, cur.Spec.DatabaseRef.Name,
				))
			}

			if old.Spec.SchemaRef.Name != cur.Spec.SchemaRef.Name {
				errs = append(errs, fmt.Errorf(
					"spec.schemaRef.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.SchemaRef.Name, cur.Spec.SchemaRef.Name,
				))
			}

			// Stage type (internal/external) is immutable.
			if old.IsExternal() != cur.IsExternal() {
				errs = append(errs, fmt.Errorf(
					"stage type is immutable: cannot convert between internal and external (was external: %v, desired external: %v)",
					old.IsExternal(), cur.IsExternal(),
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewTaskValidator creates a validating admission handler for Task resources.
func NewTaskValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.Task](scheme,
		func(old, cur *snowplanev1alpha1.Task) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			errs = append(errs, validateImmutableRef(
				"spec.databaseRef", old.Spec.DatabaseRef, cur.Spec.DatabaseRef,
			)...)
			errs = append(errs, validateImmutableStringPtr(
				"spec.databaseName", old.Spec.DatabaseName, cur.Spec.DatabaseName,
			)...)
			errs = append(errs, validateImmutableRef(
				"spec.schemaRef", old.Spec.SchemaRef, cur.Spec.SchemaRef,
			)...)
			errs = append(errs, validateImmutableStringPtr(
				"spec.schemaName", old.Spec.SchemaName, cur.Spec.SchemaName,
			)...)

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewStreamValidator creates a validating admission handler for Stream resources.
func NewStreamValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.Stream](scheme,
		func(old, cur *snowplanev1alpha1.Stream) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			errs = append(errs, validateImmutableRef(
				"spec.databaseRef", old.Spec.DatabaseRef, cur.Spec.DatabaseRef,
			)...)
			errs = append(errs, validateImmutableStringPtr(
				"spec.databaseName", old.Spec.DatabaseName, cur.Spec.DatabaseName,
			)...)
			errs = append(errs, validateImmutableRef(
				"spec.schemaRef", old.Spec.SchemaRef, cur.Spec.SchemaRef,
			)...)
			errs = append(errs, validateImmutableStringPtr(
				"spec.schemaName", old.Spec.SchemaName, cur.Spec.SchemaName,
			)...)

			if string(old.Spec.SourceType) != string(cur.Spec.SourceType) {
				errs = append(errs, fmt.Errorf(
					"spec.sourceType is immutable after creation (current: %q, desired: %q)",
					old.Spec.SourceType, cur.Spec.SourceType,
				))
			}

			if old.Spec.SourceName != cur.Spec.SourceName {
				errs = append(errs, fmt.Errorf(
					"spec.sourceName is immutable after creation (current: %q, desired: %q)",
					old.Spec.SourceName, cur.Spec.SourceName,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewTagValidator creates a validating admission handler for Tag resources.
func NewTagValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.Tag](scheme,
		func(old, cur *snowplanev1alpha1.Tag) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			errs = append(errs, validateImmutableRef(
				"spec.databaseRef", old.Spec.DatabaseRef, cur.Spec.DatabaseRef,
			)...)
			errs = append(errs, validateImmutableStringPtr(
				"spec.databaseName", old.Spec.DatabaseName, cur.Spec.DatabaseName,
			)...)
			errs = append(errs, validateImmutableRef(
				"spec.schemaRef", old.Spec.SchemaRef, cur.Spec.SchemaRef,
			)...)
			errs = append(errs, validateImmutableStringPtr(
				"spec.schemaName", old.Spec.SchemaName, cur.Spec.SchemaName,
			)...)

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewNetworkPolicyValidator creates a validating admission handler for NetworkPolicy resources.
func NewNetworkPolicyValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.NetworkPolicy](scheme,
		func(old, cur *snowplanev1alpha1.NetworkPolicy) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewResourceMonitorValidator creates a validating admission handler for ResourceMonitor resources.
func NewResourceMonitorValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.ResourceMonitor](scheme,
		func(old, cur *snowplanev1alpha1.ResourceMonitor) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewMaskingPolicyValidator creates a validating admission handler for MaskingPolicy resources.
func NewMaskingPolicyValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.MaskingPolicy](scheme,
		func(old, cur *snowplanev1alpha1.MaskingPolicy) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			errs = append(errs, validateImmutableRef(
				"spec.databaseRef", old.Spec.DatabaseRef, cur.Spec.DatabaseRef,
			)...)

			errs = append(errs, validateImmutableStringPtr(
				"spec.databaseName", old.Spec.DatabaseName, cur.Spec.DatabaseName,
			)...)

			errs = append(errs, validateImmutableRef(
				"spec.schemaRef", old.Spec.SchemaRef, cur.Spec.SchemaRef,
			)...)

			errs = append(errs, validateImmutableStringPtr(
				"spec.schemaName", old.Spec.SchemaName, cur.Spec.SchemaName,
			)...)

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewRowAccessPolicyValidator creates a validating admission handler for RowAccessPolicy resources.
func NewRowAccessPolicyValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.RowAccessPolicy](scheme,
		func(old, cur *snowplanev1alpha1.RowAccessPolicy) []error {
			var errs []error

			if old.Spec.Name != cur.Spec.Name {
				errs = append(errs, fmt.Errorf(
					"spec.name is immutable after creation (current: %q, desired: %q)",
					old.Spec.Name, cur.Spec.Name,
				))
			}

			errs = append(errs, validateImmutableRef(
				"spec.databaseRef", old.Spec.DatabaseRef, cur.Spec.DatabaseRef,
			)...)

			errs = append(errs, validateImmutableStringPtr(
				"spec.databaseName", old.Spec.DatabaseName, cur.Spec.DatabaseName,
			)...)

			errs = append(errs, validateImmutableRef(
				"spec.schemaRef", old.Spec.SchemaRef, cur.Spec.SchemaRef,
			)...)

			errs = append(errs, validateImmutableStringPtr(
				"spec.schemaName", old.Spec.SchemaName, cur.Spec.SchemaName,
			)...)

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// NewGrantOwnershipValidator creates a validating admission handler for GrantOwnership resources.
// All fields are immutable after creation — ownership transfers cannot be modified.
func NewGrantOwnershipValidator(scheme *runtime.Scheme) admission.Handler {
	return newResourceValidator[snowplanev1alpha1.GrantOwnership](scheme,
		func(old, cur *snowplanev1alpha1.GrantOwnership) []error {
			var errs []error

			if old.Spec.ObjectType != cur.Spec.ObjectType {
				errs = append(errs, fmt.Errorf(
					"spec.objectType is immutable after creation (current: %q, desired: %q)",
					old.Spec.ObjectType, cur.Spec.ObjectType,
				))
			}

			if old.Spec.ObjectName != cur.Spec.ObjectName {
				errs = append(errs, fmt.Errorf(
					"spec.objectName is immutable after creation (current: %q, desired: %q)",
					old.Spec.ObjectName, cur.Spec.ObjectName,
				))
			}

			if old.Spec.AccountRole != cur.Spec.AccountRole {
				errs = append(errs, fmt.Errorf(
					"spec.accountRole is immutable after creation (current: %q, desired: %q)",
					old.Spec.AccountRole, cur.Spec.AccountRole,
				))
			}

			errs = append(errs, validateImmutableRef(
				"spec.accountRoleRef", old.Spec.AccountRoleRef, cur.Spec.AccountRoleRef,
			)...)

			if old.Spec.DatabaseRole != cur.Spec.DatabaseRole {
				errs = append(errs, fmt.Errorf(
					"spec.databaseRole is immutable after creation (current: %q, desired: %q)",
					old.Spec.DatabaseRole, cur.Spec.DatabaseRole,
				))
			}

			errs = append(errs, validateImmutableRef(
				"spec.databaseRoleRef", old.Spec.DatabaseRoleRef, cur.Spec.DatabaseRoleRef,
			)...)

			errs = append(errs, validateImmutablePointer(
				"spec.useRole", old.Spec.UseRole, cur.Spec.UseRole,
			)...)

			return errs
		},
	)
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// validateImmutablePointer reports errors when a pointer-typed immutable field
// is mutated (nil to non-nil, non-nil to nil, or value change).
func validateImmutablePointer[T comparable](field string, oldVal, newVal *T) []error {
	var errs []error

	switch {
	case oldVal == nil && newVal != nil:
		errs = append(errs, fmt.Errorf(
			"%s is immutable after creation (was unset, desired: %v)",
			field, *newVal,
		))
	case oldVal != nil && newVal == nil:
		errs = append(errs, fmt.Errorf(
			"%s is immutable after creation (current: %v, cannot unset)",
			field, *oldVal,
		))
	case oldVal != nil && newVal != nil && *oldVal != *newVal:
		errs = append(errs, fmt.Errorf(
			"%s is immutable after creation (current: %v, desired: %v)",
			field, *oldVal, *newVal,
		))
	}

	return errs
}

// validateImmutableRef checks that a LocalObjectReference pointer has not changed.
func validateImmutableRef(field string, oldVal, newVal *snowplanev1alpha1.LocalObjectReference) []error {
	var errs []error

	switch {
	case oldVal == nil && newVal != nil:
		errs = append(errs, fmt.Errorf(
			"%s is immutable after creation (was unset, desired: %q)",
			field, newVal.Name,
		))
	case oldVal != nil && newVal == nil:
		errs = append(errs, fmt.Errorf(
			"%s is immutable after creation (current: %q, cannot unset)",
			field, oldVal.Name,
		))
	case oldVal != nil && newVal != nil && oldVal.Name != newVal.Name:
		errs = append(errs, fmt.Errorf(
			"%s is immutable after creation (current: %q, desired: %q)",
			field, oldVal.Name, newVal.Name,
		))
	}

	return errs
}

// validateImmutableStringPtr checks that a string pointer has not changed.
func validateImmutableStringPtr(field string, oldVal, newVal *string) []error {
	var errs []error

	switch {
	case oldVal == nil && newVal != nil:
		errs = append(errs, fmt.Errorf(
			"%s is immutable after creation (was unset, desired: %q)",
			field, *newVal,
		))
	case oldVal != nil && newVal == nil:
		errs = append(errs, fmt.Errorf(
			"%s is immutable after creation (current: %q, cannot unset)",
			field, *oldVal,
		))
	case oldVal != nil && newVal != nil && *oldVal != *newVal:
		errs = append(errs, fmt.Errorf(
			"%s is immutable after creation (current: %q, desired: %q)",
			field, *oldVal, *newVal,
		))
	}

	return errs
}

// --------------------------------------------------------------------------
// ProviderConfigValidator (standalone — no force-new, unique semantics)
// --------------------------------------------------------------------------

// ProviderConfigValidator validates ProviderConfig admission requests.
type ProviderConfigValidator struct {
	decoder admission.Decoder
}

// NewProviderConfigValidator creates a new ProviderConfigValidator.
func NewProviderConfigValidator(scheme *runtime.Scheme) *ProviderConfigValidator {
	return &ProviderConfigValidator{decoder: admission.NewDecoder(scheme)}
}

// Handle validates ProviderConfig admission requests.
// Note: ProviderConfig intentionally does NOT support the force-new annotation
// because changing the account or user would silently redirect the operator to a
// different Snowflake account, risking catastrophic state corruption.
func (v *ProviderConfigValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	pc := &snowplanev1alpha1.ProviderConfig{}
	if err := v.decoder.Decode(req, pc); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding new object: %w", err))
	}

	var errs []error

	if err := pc.Spec.Validate(); err != nil {
		errs = append(errs, err)
	}

	// Immutable-field checks on UPDATE — changing account or user would
	// silently redirect the operator to a different Snowflake account while
	// managing resources that belong to the original one.
	if req.Operation == admissionv1.Update {
		oldPC := &snowplanev1alpha1.ProviderConfig{}
		if err := v.decoder.DecodeRaw(req.OldObject, oldPC); err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding old object: %w", err))
		}

		// Use ObservedGeneration > 0 to determine if the resource has been
		// reconciled at least once, consistent with all other validators (L-16).
		if oldPC.Status.ObservedGeneration > 0 && oldPC.Spec.Account != pc.Spec.Account {
			errs = append(errs, fmt.Errorf(
				"spec.account is immutable after creation (current: %q, desired: %q)",
				oldPC.Spec.Account, pc.Spec.Account,
			))
		}

		if oldPC.Status.ObservedGeneration > 0 && oldPC.Spec.User != pc.Spec.User {
			errs = append(errs, fmt.Errorf(
				"spec.user is immutable after creation (current: %q, desired: %q)",
				oldPC.Spec.User, pc.Spec.User,
			))
		}
	}

	return denyAll(errors.Join(errs...))
}

// --------------------------------------------------------------------------
// FieldExportValidator (standalone — unique validation, no CommonSpec)
// --------------------------------------------------------------------------

// FieldExportValidator validates FieldExport admission requests.
type FieldExportValidator struct {
	decoder admission.Decoder
}

// NewFieldExportValidator creates a new FieldExportValidator.
func NewFieldExportValidator(scheme *runtime.Scheme) *FieldExportValidator {
	return &FieldExportValidator{decoder: admission.NewDecoder(scheme)}
}

// Handle validates FieldExport admission requests.
func (v *FieldExportValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	newFE := &snowplanev1alpha1.FieldExport{}
	if err := v.decoder.Decode(req, newFE); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding new object: %w", err))
	}

	var errs []error

	// Validate source kind is a known Snowplane resource.
	if _, ok := snowplanev1alpha1.ValidFieldExportSourceKinds[newFE.Spec.From.Resource.Kind]; !ok {
		errs = append(errs, fmt.Errorf("spec.from.resource.kind %q is not a known Snowplane resource kind", newFE.Spec.From.Resource.Kind))
	}

	// Validate path starts with ".status."
	if !strings.HasPrefix(newFE.Spec.From.Path, ".status.") {
		errs = append(errs, fmt.Errorf("spec.from.path must start with \".status.\" (got %q)", newFE.Spec.From.Path))
	}

	// Validate path does not use array indexing.
	if strings.Contains(newFE.Spec.From.Path, "[") {
		errs = append(errs, fmt.Errorf("spec.from.path does not support array indexing (got %q)", newFE.Spec.From.Path))
	}

	// Validate target kind (defense-in-depth, also enforced by CRD enum).
	if newFE.Spec.To.Kind != snowplanev1alpha1.FieldExportTargetConfigMap &&
		newFE.Spec.To.Kind != snowplanev1alpha1.FieldExportTargetSecret {
		errs = append(errs, fmt.Errorf("spec.to.kind must be ConfigMap or Secret (got %q)", newFE.Spec.To.Kind))
	}

	// Immutable fields on UPDATE.
	if req.Operation == admissionv1.Update {
		oldFE := &snowplanev1alpha1.FieldExport{}
		if err := v.decoder.DecodeRaw(req.OldObject, oldFE); err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding old object: %w", err))
		}

		if oldFE.Spec.From.Resource.Kind != newFE.Spec.From.Resource.Kind {
			errs = append(errs, fmt.Errorf("spec.from.resource.kind is immutable (current: %q, desired: %q)",
				oldFE.Spec.From.Resource.Kind, newFE.Spec.From.Resource.Kind))
		}

		if oldFE.Spec.From.Resource.Name != newFE.Spec.From.Resource.Name {
			errs = append(errs, fmt.Errorf("spec.from.resource.name is immutable (current: %q, desired: %q)",
				oldFE.Spec.From.Resource.Name, newFE.Spec.From.Resource.Name))
		}

		if oldFE.Spec.To.Kind != newFE.Spec.To.Kind {
			errs = append(errs, fmt.Errorf("spec.to.kind is immutable (current: %q, desired: %q)",
				oldFE.Spec.To.Kind, newFE.Spec.To.Kind))
		}

		if oldFE.Spec.To.Name != newFE.Spec.To.Name {
			errs = append(errs, fmt.Errorf("spec.to.name is immutable (current: %q, desired: %q)",
				oldFE.Spec.To.Name, newFE.Spec.To.Name))
		}

		if oldFE.Spec.To.Key != newFE.Spec.To.Key {
			errs = append(errs, fmt.Errorf("spec.to.key is immutable (current: %q, desired: %q)",
				oldFE.Spec.To.Key, newFE.Spec.To.Key))
		}
	}

	return denyAll(errors.Join(errs...))
}
