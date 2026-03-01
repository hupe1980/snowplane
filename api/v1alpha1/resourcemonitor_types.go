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
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type ResourceMonitorSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake resource monitor name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// CreditQuota is the number of credits allocated per frequency interval.
	// +optional
	CreditQuota *int32 `json:"creditQuota,omitempty" snowflake:"CREDIT_QUOTA,nounset"`

	// Frequency is the interval at which credit usage resets to 0.
	// If set, startTimestamp must also be set.
	// +optional
	Frequency *ResourceMonitorFrequency `json:"frequency,omitempty" snowflake:"FREQUENCY,nounset"`

	// StartTimestamp is the date/time when monitoring begins. Use "IMMEDIATELY" for now.
	// If set, frequency must also be set.
	// +optional
	StartTimestamp *string `json:"startTimestamp,omitempty" snowflake:"START_TIMESTAMP,nounset"`

	// EndTimestamp is the date/time when the monitor suspends assigned warehouses.
	// +optional
	EndTimestamp *string `json:"endTimestamp,omitempty" snowflake:"END_TIMESTAMP,nounset"`

	// NotifyUsers is the list of users to receive email notifications.
	// +optional
	NotifyUsers []string `json:"notifyUsers,omitempty" snowflake:"NOTIFY_USERS,nounset"`

	// Triggers defines the trigger thresholds and actions for the resource monitor.
	// Each resource monitor supports up to 5 NOTIFY triggers.
	// +optional
	Triggers []ResourceMonitorTrigger `json:"triggers,omitempty" snowflake:"TRIGGERS,nounset"`
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
	CreditQuota string `json:"creditQuota,omitempty" snowflake:"CREDIT_QUOTA,nounset"`

	// UsedCredits is the credits used in the current interval.
	UsedCredits string `json:"usedCredits,omitempty"`

	// RemainingCredits is the credits remaining in the current interval.
	RemainingCredits string `json:"remainingCredits,omitempty"`

	// Level is the assignment level (ACCOUNT, WAREHOUSE, or null).
	Level string `json:"level,omitempty"`

	// Frequency is the reset frequency.
	Frequency string `json:"frequency,omitempty" snowflake:"FREQUENCY,nounset"`

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
	NotifyUsers string `json:"notifyUsers,omitempty" snowflake:"NOTIFY_USERS,nounset"`
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
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
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

func init() {
	SchemeBuilder.Register(&ResourceMonitor{}, &ResourceMonitorList{})
}
