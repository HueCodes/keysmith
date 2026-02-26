package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition type constants
const (
	ConditionReady           = "Ready"
	ConditionScheduled       = "Scheduled"
	ConditionProviderHealthy = "ProviderHealthy"
	ConditionDegraded        = "Degraded"

	// AnnotationRotateNow triggers an immediate manual rotation when set to AnnotationRotateVal.
	AnnotationRotateNow = "secrets.keysmith.io/rotate"
	AnnotationRotateVal = "now"

	FinalizerName = "secrets.keysmith.io/finalizer"
)

// SecretRotationPolicySpec defines the desired state of SecretRotationPolicy.
type SecretRotationPolicySpec struct {
	// SecretRef is the Kubernetes secret to create or update with rotated values.
	SecretRef SecretReference `json:"secretRef"`

	// Schedule is a standard cron expression defining when to rotate.
	// +kubebuilder:validation:Pattern=`^(@(annually|yearly|monthly|weekly|daily|hourly)|((\*|[0-9,\-\*\/]+)\s){4}(\*|[0-9,\-\*\/]+))$`
	Schedule string `json:"schedule"`

	// RotationWindow is how early before the scheduled time to trigger rotation.
	// +optional
	RotationWindow *metav1.Duration `json:"rotationWindow,omitempty"`

	// Provider configures the secret backend to fetch rotated values from.
	Provider ProviderSpec `json:"provider"`

	// Keys maps provider secret keys to Kubernetes secret data keys.
	// +kubebuilder:validation:MinItems=1
	Keys []KeyMapping `json:"keys"`

	// RestartTargets lists workloads to rolling-restart after a successful rotation.
	// +optional
	RestartTargets []RestartTarget `json:"restartTargets,omitempty"`

	// Policy controls retry, history, and suspend behavior.
	// +optional
	Policy RotationPolicy `json:"policy,omitempty"`

	// Notifications configures webhook alerts on rotation events.
	// +optional
	Notifications []NotificationSpec `json:"notifications,omitempty"`
}

// SecretReference identifies a Kubernetes secret.
type SecretReference struct {
	Name string `json:"name"`
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ProviderSpec configures the secret backend.
type ProviderSpec struct {
	// Name is the provider type: mock, static, aws, vault.
	// +kubebuilder:validation:Enum=mock;static;aws;vault
	Name string `json:"name"`

	// ConfigRef optionally references a ConfigMap or Secret with provider configuration.
	// +optional
	ConfigRef *corev1.LocalObjectReference `json:"configRef,omitempty"`

	// Params contains non-sensitive, provider-specific configuration parameters.
	// +optional
	Params map[string]string `json:"params,omitempty"`
}

// KeyMapping maps a key from the provider secret to a Kubernetes secret data key.
type KeyMapping struct {
	ProviderKey string `json:"providerKey"`
	SecretKey   string `json:"secretKey"`
}

// RestartTarget identifies a workload to rolling-restart after rotation.
type RestartTarget struct {
	// Kind is one of: Deployment, StatefulSet, DaemonSet.
	// +kubebuilder:validation:Enum=Deployment;StatefulSet;DaemonSet
	Kind string `json:"kind"`
	Name string `json:"name"`
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// RotationPolicy controls retry, history, and suspend behavior.
type RotationPolicy struct {
	// HistoryLimit is the maximum number of RotationRecords to retain per policy.
	// Defaults to 10.
	// +optional
	// +kubebuilder:validation:Minimum=1
	HistoryLimit *int32 `json:"historyLimit,omitempty"`

	// Suspend pauses all scheduled rotations without deleting the policy.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// FailurePolicy controls behavior when the provider returns an error.
	// Fail marks the policy as Failed; Ignore logs the error and continues.
	// +optional
	// +kubebuilder:validation:Enum=Fail;Ignore
	FailurePolicy FailurePolicy `json:"failurePolicy,omitempty"`

	// RetryLimit is the maximum number of retry attempts on transient failure.
	// +optional
	// +kubebuilder:validation:Minimum=0
	RetryLimit *int32 `json:"retryLimit,omitempty"`

	// RetryBackoff is the delay between retry attempts.
	// +optional
	RetryBackoff *metav1.Duration `json:"retryBackoff,omitempty"`
}

// FailurePolicy controls behavior on rotation failure.
// +kubebuilder:validation:Enum=Fail;Ignore
type FailurePolicy string

const (
	FailurePolicyFail   FailurePolicy = "Fail"
	FailurePolicyIgnore FailurePolicy = "Ignore"
)

// NotificationSpec configures a webhook notification for rotation events.
type NotificationSpec struct {
	// URL is the webhook endpoint. Mutually exclusive with SecretRef.
	// +optional
	URL string `json:"url,omitempty"`

	// Events lists which rotation events trigger this notification.
	// +kubebuilder:validation:MinItems=1
	Events []NotificationEvent `json:"events"`

	// SecretRef reads the webhook URL from a Kubernetes secret key.
	// +optional
	SecretRef *SecretKeySelector `json:"secretRef,omitempty"`
}

// NotificationEvent is a rotation lifecycle event that can trigger a notification.
// +kubebuilder:validation:Enum=RotationSucceeded;RotationFailed;RotationSkipped
type NotificationEvent string

const (
	EventRotationSucceeded NotificationEvent = "RotationSucceeded"
	EventRotationFailed    NotificationEvent = "RotationFailed"
	EventRotationSkipped   NotificationEvent = "RotationSkipped"
)

// SecretKeySelector references a specific key within a Kubernetes secret.
type SecretKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// Phase represents the current lifecycle state of a SecretRotationPolicy.
// +kubebuilder:validation:Enum=Pending;Rotating;Ready;Failed;Suspended
type Phase string

const (
	PhasePending   Phase = "Pending"
	PhaseRotating  Phase = "Rotating"
	PhaseReady     Phase = "Ready"
	PhaseFailed    Phase = "Failed"
	PhaseSuspended Phase = "Suspended"
)

// SecretRotationPolicyStatus defines the observed state of SecretRotationPolicy.
type SecretRotationPolicyStatus struct {
	// LastRotationTime is the timestamp of the most recent successful rotation.
	// +optional
	LastRotationTime *metav1.Time `json:"lastRotationTime,omitempty"`

	// NextRotationTime is the timestamp of the next scheduled rotation.
	// +optional
	NextRotationTime *metav1.Time `json:"nextRotationTime,omitempty"`

	// Phase is the current lifecycle phase of this policy.
	// +optional
	Phase Phase `json:"phase,omitempty"`

	// CurrentRetryCount is the number of retry attempts for the current rotation.
	// +optional
	CurrentRetryCount int32 `json:"currentRetryCount,omitempty"`

	// LastRotationRecord references the most recent RotationRecord.
	// +optional
	LastRotationRecord *corev1.ObjectReference `json:"lastRotationRecord,omitempty"`

	// Conditions represents the latest observations of the policy's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=srp,categories=keysmith
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.provider.name`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Last Rotation",type=string,JSONPath=`.status.lastRotationTime`
// +kubebuilder:printcolumn:name="Next Rotation",type=string,JSONPath=`.status.nextRotationTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SecretRotationPolicy is the Schema for the secretrotationpolicies API.
// It defines a schedule and provider for automatically rotating a Kubernetes secret.
type SecretRotationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecretRotationPolicySpec   `json:"spec,omitempty"`
	Status SecretRotationPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SecretRotationPolicyList contains a list of SecretRotationPolicy.
type SecretRotationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecretRotationPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecretRotationPolicy{}, &SecretRotationPolicyList{})
}
