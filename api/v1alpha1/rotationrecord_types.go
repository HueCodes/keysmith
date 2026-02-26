package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TriggerReason identifies what initiated a rotation.
// +kubebuilder:validation:Enum=Schedule;Manual
type TriggerReason string

const (
	TriggerSchedule TriggerReason = "Schedule"
	TriggerManual   TriggerReason = "Manual"
)

// RecordPhase is the lifecycle state of a RotationRecord.
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed
type RecordPhase string

const (
	RecordPhasePending   RecordPhase = "Pending"
	RecordPhaseRunning   RecordPhase = "Running"
	RecordPhaseSucceeded RecordPhase = "Succeeded"
	RecordPhaseFailed    RecordPhase = "Failed"
)

// RotationRecordSpec defines the immutable intent of a rotation attempt.
type RotationRecordSpec struct {
	// PolicyRef references the SecretRotationPolicy that owns this record.
	PolicyRef SecretReference `json:"policyRef"`

	// TriggeredBy identifies what initiated this rotation.
	TriggeredBy TriggerReason `json:"triggeredBy"`

	// RequestedAt is the timestamp when the rotation was requested.
	RequestedAt metav1.Time `json:"requestedAt"`
}

// RotationRecordStatus records the outcome of a rotation attempt.
type RotationRecordStatus struct {
	// Phase is the current state of this rotation record.
	// +optional
	Phase RecordPhase `json:"phase,omitempty"`

	// StartTime is when the rotation execution began.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the rotation execution finished.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Duration is the human-readable duration of the rotation.
	// +optional
	Duration string `json:"duration,omitempty"`

	// ProviderName is the provider used for this rotation.
	// +optional
	ProviderName string `json:"providerName,omitempty"`

	// RotatedKeys lists the Kubernetes secret keys that were updated.
	// +optional
	RotatedKeys []string `json:"rotatedKeys,omitempty"`

	// RestartsTriggered lists the workloads that were rolling-restarted.
	// +optional
	RestartsTriggered []RestartTarget `json:"restartsTriggered,omitempty"`

	// Error contains the error message if the rotation failed.
	// +optional
	Error string `json:"error,omitempty"`

	// RetryCount is the number of retry attempts made for this rotation.
	// +optional
	RetryCount int32 `json:"retryCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=rr,categories=keysmith
// +kubebuilder:printcolumn:name="Policy",type=string,JSONPath=`.spec.policyRef.name`
// +kubebuilder:printcolumn:name="Trigger",type=string,JSONPath=`.spec.triggeredBy`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.status.providerName`
// +kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.status.duration`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RotationRecord is an immutable audit log entry for a single rotation attempt.
// Records are created by the SecretRotationPolicy controller and cleaned up
// according to spec.policy.historyLimit.
type RotationRecord struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RotationRecordSpec   `json:"spec,omitempty"`
	Status RotationRecordStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RotationRecordList contains a list of RotationRecord.
type RotationRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RotationRecord `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RotationRecord{}, &RotationRecordList{})
}
