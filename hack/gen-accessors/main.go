//go:build ignore

// This program generates ManagedResource accessor methods for all CRD types.
// It reads type definitions from this file and produces zz_generated_accessors.go
// in the api/v1alpha1 package.
//
// Usage:
//
//	go run hack/gen-accessors/main.go
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"strings"
	"text/template"
)

// OwnerSource describes where GetOwner reads from.
type OwnerSource int

const (
	// OwnerFromShowOutputOwner reads Status.ShowOutput.Owner.
	OwnerFromShowOutputOwner OwnerSource = iota
	// OwnerFromShowOutputGrantedBy reads Status.ShowOutput.GrantedBy.
	OwnerFromShowOutputGrantedBy
	// OwnerEmpty always returns "".
	OwnerEmpty
)

// TrackedParamsMode describes how TrackedParameters is handled.
type TrackedParamsMode int

const (
	// TrackedParamsFromStatus reads/writes Status.TrackedParameters.
	TrackedParamsFromStatus TrackedParamsMode = iota
	// TrackedParamsNil returns nil and no-ops the setter.
	TrackedParamsNil
)

// TypeDef describes a CRD type for accessor generation.
type TypeDef struct {
	// TypeName is the Go type name (e.g. "Database").
	TypeName string
	// Receiver is the short variable name (e.g. "d" for Database).
	Receiver string
	// SkipGetSpecName skips generating GetSpecName (stays hand-written).
	SkipGetSpecName bool
	// Owner source.
	Owner OwnerSource
	// OwnerComment is an optional comment for GetOwner when it returns "".
	OwnerComment string
	// TrackedParams mode.
	TrackedParams TrackedParamsMode
}

// OwnerString returns a stable string key for template comparison,
// decoupling the template from iota ordering.
func (td TypeDef) OwnerString() string {
	switch td.Owner {
	case OwnerFromShowOutputOwner:
		return "ShowOutputOwner"
	case OwnerFromShowOutputGrantedBy:
		return "ShowOutputGrantedBy"
	default:
		return "Empty"
	}
}

// TrackedParamsString returns a stable string key for template comparison.
func (td TypeDef) TrackedParamsString() string {
	switch td.TrackedParams {
	case TrackedParamsFromStatus:
		return "FromStatus"
	default:
		return "Nil"
	}
}

var types = []TypeDef{
	// Pattern A1 — Standard resources with ShowOutput.Owner
	{TypeName: "Database", Receiver: "d", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "Schema", Receiver: "s", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "Warehouse", Receiver: "w", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "User", Receiver: "u", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "AccountRole", Receiver: "r", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "DatabaseRole", Receiver: "r", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "Tag", Receiver: "t", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "MaskingPolicy", Receiver: "mp", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "RowAccessPolicy", Receiver: "rap", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "Stage", Receiver: "s", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "StreamOnTable", Receiver: "s", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "StreamOnView", Receiver: "s", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "StreamOnExternalTable", Receiver: "s", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "StreamOnDirectoryTable", Receiver: "s", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "StreamOnDynamicTable", Receiver: "s", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "Task", Receiver: "t", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "Alert", Receiver: "a", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "View", Receiver: "v", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "Table", Receiver: "t", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},

	// Pattern A2 — Standard resources without owner column
	{TypeName: "NetworkPolicy", Receiver: "np", Owner: OwnerEmpty, OwnerComment: "SHOW NETWORK POLICIES does not return an owner column.", TrackedParams: TrackedParamsFromStatus},
	{TypeName: "ResourceMonitor", Receiver: "rm", Owner: OwnerEmpty, OwnerComment: "SHOW RESOURCE MONITORS does not return an owner column.", TrackedParams: TrackedParamsFromStatus},
	{TypeName: "StorageIntegration", Receiver: "si", Owner: OwnerEmpty, OwnerComment: "SHOW STORAGE INTEGRATIONS does not return an owner column.", TrackedParams: TrackedParamsFromStatus},

	// Pattern A3 — Schema-level resources with ShowOutput.Owner (new Phase 5 resources)
	{TypeName: "FileFormat", Receiver: "ff", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "Pipe", Receiver: "p", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "DynamicTable", Receiver: "dt", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},

	// Phase 6 — Wave 2 resources
	{TypeName: "NotificationIntegration", Receiver: "ni", Owner: OwnerEmpty, OwnerComment: "SHOW NOTIFICATION INTEGRATIONS does not return an owner column.", TrackedParams: TrackedParamsFromStatus},
	{TypeName: "SecurityIntegration", Receiver: "si", Owner: OwnerEmpty, OwnerComment: "SHOW SECURITY INTEGRATIONS does not return an owner column.", TrackedParams: TrackedParamsFromStatus},
	{TypeName: "PasswordPolicy", Receiver: "pp", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},
	{TypeName: "NetworkRule", Receiver: "nr", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},

	// Pattern B — Grant resources (custom GetSpecName)
	{TypeName: "AccountRoleGrant", Receiver: "r", SkipGetSpecName: true, Owner: OwnerFromShowOutputGrantedBy, TrackedParams: TrackedParamsNil},
	{TypeName: "DatabaseRoleGrant", Receiver: "r", SkipGetSpecName: true, Owner: OwnerFromShowOutputGrantedBy, TrackedParams: TrackedParamsNil},
	{TypeName: "ShareGrant", Receiver: "r", SkipGetSpecName: true, Owner: OwnerFromShowOutputGrantedBy, TrackedParams: TrackedParamsNil},

	// Pattern C — GrantOwnership (custom GetSpecName, nil tracked params)
	{TypeName: "GrantOwnership", Receiver: "g", SkipGetSpecName: true, Owner: OwnerEmpty, TrackedParams: TrackedParamsNil},

	// Pattern D — Role assignment resources (custom GetSpecName, GrantedBy owner)
	{TypeName: "AccountRoleAssignment", Receiver: "a", SkipGetSpecName: true, Owner: OwnerFromShowOutputGrantedBy, TrackedParams: TrackedParamsNil},
	{TypeName: "DatabaseRoleAssignment", Receiver: "d", SkipGetSpecName: true, Owner: OwnerFromShowOutputGrantedBy, TrackedParams: TrackedParamsNil},
}

const accessorTmpl = `// Code generated by hack/gen-accessors/main.go; DO NOT EDIT.

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

{{ range .Types }}
// ---------------------------------------------------------------------------
// {{ .TypeName }}
// ---------------------------------------------------------------------------

func ({{ .Receiver }} *{{ .TypeName }}) GetConditions() []metav1.Condition {
	return {{ .Receiver }}.Status.Conditions
}

func ({{ .Receiver }} *{{ .TypeName }}) SetConditions(conditions []metav1.Condition) {
	{{ .Receiver }}.Status.Conditions = conditions
}

func ({{ .Receiver }} *{{ .TypeName }}) GetDeletionPolicy() DeletionPolicy {
	if {{ .Receiver }}.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return {{ .Receiver }}.Spec.DeletionPolicy
}

func ({{ .Receiver }} *{{ .TypeName }}) GetFullyQualifiedName() string {
	return {{ .Receiver }}.Status.FullyQualifiedName
}

func ({{ .Receiver }} *{{ .TypeName }}) GetProviderRef() ProviderReference {
	return {{ .Receiver }}.Spec.ProviderRef
}
{{ if not .SkipGetSpecName }}
func ({{ .Receiver }} *{{ .TypeName }}) GetSpecName() string {
	return {{ .Receiver }}.Spec.Name
}
{{ end }}

func ({{ .Receiver }} *{{ .TypeName }}) GetUseRole() *string {
	return {{ .Receiver }}.Spec.UseRole
}

func ({{ .Receiver }} *{{ .TypeName }}) GetPaused() bool {
	return {{ .Receiver }}.Spec.Paused
}

func ({{ .Receiver }} *{{ .TypeName }}) GetManagementPolicies() ManagementPolicies {
	return {{ .Receiver }}.Spec.ManagementPolicies
}

func ({{ .Receiver }} *{{ .TypeName }}) SetCreateOrAlter(val *bool) {
	{{ .Receiver }}.Spec.ManagementPolicies.CreateOrAlter = val
}

func ({{ .Receiver }} *{{ .TypeName }}) GetObservedGeneration() int64 {
	return {{ .Receiver }}.Status.ObservedGeneration
}

func ({{ .Receiver }} *{{ .TypeName }}) SetObservedGeneration(val int64) {
	{{ .Receiver }}.Status.ObservedGeneration = val
}

func ({{ .Receiver }} *{{ .TypeName }}) GetLastAppliedSpecHash() string {
	return {{ .Receiver }}.Status.LastAppliedSpecHash
}

func ({{ .Receiver }} *{{ .TypeName }}) SetLastAppliedSpecHash(val string) {
	{{ .Receiver }}.Status.LastAppliedSpecHash = val
}

func ({{ .Receiver }} *{{ .TypeName }}) ValidateSpec() error {
	return {{ .Receiver }}.Spec.Validate()
}

func ({{ .Receiver }} *{{ .TypeName }}) ComputeSpecHash() (string, error) {
	return ComputeSpecHash({{ .Receiver }}.Spec)
}

{{ if eq .OwnerString "ShowOutputOwner" }}
func ({{ .Receiver }} *{{ .TypeName }}) GetOwner() string {
	if {{ .Receiver }}.Status.ShowOutput != nil {
		return {{ .Receiver }}.Status.ShowOutput.Owner
	}

	return ""
}
{{ else if eq .OwnerString "ShowOutputGrantedBy" }}
func ({{ .Receiver }} *{{ .TypeName }}) GetOwner() string {
	if {{ .Receiver }}.Status.ShowOutput != nil {
		return {{ .Receiver }}.Status.ShowOutput.GrantedBy
	}

	return ""
}
{{ else }}{{/* Empty */}}
func ({{ .Receiver }} *{{ .TypeName }}) GetOwner() string {
{{- if .OwnerComment }}
	// {{ .OwnerComment }}
{{- end }}
	return ""
}
{{ end }}

{{ if eq .TrackedParamsString "FromStatus" }}
func ({{ .Receiver }} *{{ .TypeName }}) GetTrackedParametersList() []string {
	return {{ .Receiver }}.Status.TrackedParameters
}

func ({{ .Receiver }} *{{ .TypeName }}) SetTrackedParametersList(val []string) {
	{{ .Receiver }}.Status.TrackedParameters = val
}
{{ else }}{{/* Nil */}}
func ({{ .Receiver }} *{{ .TypeName }}) GetTrackedParametersList() []string { return nil }

func ({{ .Receiver }} *{{ .TypeName }}) SetTrackedParametersList(_ []string) {}
{{ end }}
{{ end }}`

func main() {
	tmpl, err := template.New("accessors").Parse(accessorTmpl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "template parse error: %v\n", err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	data := struct {
		Types []TypeDef
	}{
		Types: types,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		fmt.Fprintf(os.Stderr, "template execute error: %v\n", err)
		os.Exit(1)
	}

	// gofmt the output
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Write unformatted for debugging
		fmt.Fprintf(os.Stderr, "gofmt error: %v\n", err)
		fmt.Fprintf(os.Stderr, "unformatted output:\n%s\n", buf.String())
		os.Exit(1)
	}

	outPath := "zz_generated_accessors.go"

	// If running from the repo root (e.g. via justfile), write to api/v1alpha1/.
	if _, err := os.Stat("api/v1alpha1"); err == nil {
		outPath = "api/v1alpha1/zz_generated_accessors.go"
	}
	if err := os.WriteFile(outPath, formatted, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}

	// Count methods and lines
	lineCount := strings.Count(string(formatted), "\n")
	methodCount := strings.Count(string(formatted), "func (")
	fmt.Printf("Generated %s: %d methods across %d types (%d lines)\n", outPath, methodCount, len(types), lineCount)
}
