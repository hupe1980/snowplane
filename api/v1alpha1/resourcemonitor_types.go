package v1alpha1

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceMonitorTriggerAction defines the action for a resource monitor trigger.
// +kubebuilder:validation:Enum=SUSPEND;SUSPEND_IMMEDIATE;NOTIFY
type ResourceMonitorTriggerAction string

const (
	// ResourceMonitorTriggerActionSuspend suspends warehouses after running queries finish.
	ResourceMonitorTriggerActionSuspend ResourceMonitorTriggerAction = "SUSPEND"
	// ResourceMonitorTriggerActionSuspendImmediate suspends warehouses and cancels running queries.
	ResourceMonitorTriggerActionSuspendImmediate ResourceMonitorTriggerAction = "SUSPEND_IMMEDIATE"
	// ResourceMonitorTriggerActionNotify sends a notification without suspending.
	ResourceMonitorTriggerActionNotify ResourceMonitorTriggerAction = "NOTIFY"
)

// ResourceMonitorFrequency defines the frequency interval at which credit usage resets.
// +kubebuilder:validation:Enum=MONTHLY;DAILY;WEEKLY;YEARLY;NEVER
type ResourceMonitorFrequency string

// ResourceMonitorFrequency constants define the allowed frequency intervals.
const (
	ResourceMonitorFrequencyMonthly ResourceMonitorFrequency = "MONTHLY"
	ResourceMonitorFrequencyDaily   ResourceMonitorFrequency = "DAILY"
	ResourceMonitorFrequencyWeekly  ResourceMonitorFrequency = "WEEKLY"
	ResourceMonitorFrequencyYearly  ResourceMonitorFrequency = "YEARLY"
	ResourceMonitorFrequencyNever   ResourceMonitorFrequency = "NEVER"
)

// ResourceMonitorTrigger defines a trigger for the resource monitor.
type ResourceMonitorTrigger struct {
	// Threshold is the percentage of the credit quota that triggers the action.
	// Values larger than 100 are supported.
	// +kubebuilder:validation:Minimum=1
	Threshold int32 `json:"threshold"`

	// Action is the action to perform when the threshold is reached.
	Action ResourceMonitorTriggerAction `json:"action"`
}

// ResourceMonitorSpec defines the desired state of a Snowflake Resource Monitor.
type ResourceMonitorSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake resource monitor name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// CreditQuota is the number of credits allocated per frequency interval.
	// +optional
	CreditQuota *int32 `json:"creditQuota,omitempty"`

	// Frequency is the interval at which credit usage resets to 0.
	// If set, startTimestamp must also be set.
	// +optional
	Frequency *ResourceMonitorFrequency `json:"frequency,omitempty"`

	// StartTimestamp is the date/time when monitoring begins. Use "IMMEDIATELY" for now.
	// If set, frequency must also be set.
	// +optional
	StartTimestamp *string `json:"startTimestamp,omitempty"`

	// EndTimestamp is the date/time when the monitor suspends assigned warehouses.
	// +optional
	EndTimestamp *string `json:"endTimestamp,omitempty"`

	// NotifyUsers is the list of users to receive email notifications.
	// +optional
	NotifyUsers []string `json:"notifyUsers,omitempty"`

	// Triggers defines the trigger thresholds and actions for the resource monitor.
	// Each resource monitor supports up to 5 NOTIFY triggers.
	// +optional
	Triggers []ResourceMonitorTrigger `json:"triggers,omitempty"`
}

// Validate checks the ResourceMonitorSpec for consistency.
func (s *ResourceMonitorSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, fmt.Errorf("spec.name is required"))
	}

	// frequency and startTimestamp must be set together.
	if (s.Frequency != nil) != (s.StartTimestamp != nil) {
		errs = append(errs, fmt.Errorf("spec.frequency and spec.startTimestamp must both be set or both be omitted"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// ResourceMonitorShowOutput mirrors the SHOW RESOURCE MONITORS output stored in status.
type ResourceMonitorShowOutput struct {
	// CreatedOn is the timestamp when the resource monitor was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the resource monitor name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// CreditQuota is the credit quota.
	CreditQuota string `json:"creditQuota,omitempty"`

	// UsedCredits is the credits used in the current interval.
	UsedCredits string `json:"usedCredits,omitempty"`

	// RemainingCredits is the credits remaining in the current interval.
	RemainingCredits string `json:"remainingCredits,omitempty"`

	// Level is the assignment level (ACCOUNT, WAREHOUSE, or null).
	Level string `json:"level,omitempty"`

	// Frequency is the reset frequency.
	Frequency string `json:"frequency,omitempty"`

	// StartTime is the monitoring start time.
	StartTime string `json:"startTime,omitempty"`

	// EndTime is the monitoring end time.
	EndTime string `json:"endTime,omitempty"`

	// NotifyAt is the notify trigger percentages (comma-separated).
	NotifyAt string `json:"notifyAt,omitempty"`

	// SuspendAt is the suspend trigger percentage.
	SuspendAt string `json:"suspendAt,omitempty"`

	// SuspendImmediatelyAt is the suspend-immediately trigger percentage.
	SuspendImmediatelyAt string `json:"suspendImmediatelyAt,omitempty"`

	// NotifyUsers is the comma-separated list of notification users.
	NotifyUsers string `json:"notifyUsers,omitempty"`
}

// ResourceMonitorStatus defines the observed state of a ResourceMonitor.
type ResourceMonitorStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW RESOURCE MONITORS output for this monitor.
	ShowOutput *ResourceMonitorShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// ResourceMonitor is the Schema for the resourcemonitors API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type ResourceMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceMonitorSpec   `json:"spec,omitempty"`
	Status ResourceMonitorStatus `json:"status,omitempty"`
}

// ResourceMonitorList contains a list of ResourceMonitor.
// +kubebuilder:object:root=true
type ResourceMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceMonitor `json:"items"`
}

// GetConditions returns the conditions of the ResourceMonitor.
func (rm *ResourceMonitor) GetConditions() []metav1.Condition { return rm.Status.Conditions }

// SetConditions sets the conditions of the ResourceMonitor.
func (rm *ResourceMonitor) SetConditions(c []metav1.Condition) { rm.Status.Conditions = c }

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (rm *ResourceMonitor) GetFullyQualifiedName() string { return rm.Status.FullyQualifiedName }

// GetSpecName returns the Snowflake resource name from the spec.
func (rm *ResourceMonitor) GetSpecName() string { return rm.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (rm *ResourceMonitor) GetProviderRef() ProviderReference { return rm.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (rm *ResourceMonitor) GetUseRole() *string { return rm.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (rm *ResourceMonitor) GetObservedGeneration() int64 { return rm.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (rm *ResourceMonitor) SetObservedGeneration(v int64) { rm.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (rm *ResourceMonitor) GetLastAppliedSpecHash() string { return rm.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (rm *ResourceMonitor) SetLastAppliedSpecHash(v string) { rm.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (rm *ResourceMonitor) GetTrackedParametersList() []string { return rm.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (rm *ResourceMonitor) SetTrackedParametersList(v []string) { rm.Status.TrackedParameters = v }

// ValidateSpec validates the resource spec.
func (rm *ResourceMonitor) ValidateSpec() error { return rm.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (rm *ResourceMonitor) ComputeSpecHash() (string, error) { return ComputeSpecHash(rm.Spec) }

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (rm *ResourceMonitor) GetDeletionPolicy() DeletionPolicy {
	if rm.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return rm.Spec.DeletionPolicy
}

// GetOwner returns the owner from status.
func (rm *ResourceMonitor) GetOwner() string {
	// SHOW RESOURCE MONITORS does not return an owner column.
	return ""
}

func init() {
	SchemeBuilder.Register(&ResourceMonitor{}, &ResourceMonitorList{})
}
