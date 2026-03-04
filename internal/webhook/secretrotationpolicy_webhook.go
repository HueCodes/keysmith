/*
Copyright 2026 keysmith contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package webhook implements admission webhooks for Keysmith CRDs.
package webhook

import (
	"context"
	"fmt"

	"github.com/robfig/cron/v3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	secretsv1alpha1 "github.com/hstores/keysmith/api/v1alpha1"
)

// SecretRotationPolicyWebhook implements defaulting and validation webhooks
// for SecretRotationPolicy resources.
//
// It satisfies admission.Defaulter[*secretsv1alpha1.SecretRotationPolicy] and
// admission.Validator[*secretsv1alpha1.SecretRotationPolicy] so that
// controller-runtime v0.23's generics-based builder can wire them automatically.
type SecretRotationPolicyWebhook struct{}

// +kubebuilder:webhook:path=/mutate-secrets-keysmith-io-v1alpha1-secretrotationpolicy,mutating=true,failurePolicy=fail,sideEffects=None,groups=secrets.keysmith.io,resources=secretrotationpolicies,verbs=create;update,versions=v1alpha1,name=msecretrotationpolicy.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-secrets-keysmith-io-v1alpha1-secretrotationpolicy,mutating=false,failurePolicy=fail,sideEffects=None,groups=secrets.keysmith.io,resources=secretrotationpolicies,verbs=create;update,versions=v1alpha1,name=vsecretrotationpolicy.kb.io,admissionReviewVersions=v1

// SetupWebhookWithManager registers the defaulting and validation webhooks with the manager.
func (w *SecretRotationPolicyWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return builder.WebhookManagedBy[*secretsv1alpha1.SecretRotationPolicy](mgr, &secretsv1alpha1.SecretRotationPolicy{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

// Default applies default values to a SecretRotationPolicy on creation and update.
// Implements admission.Defaulter[*SecretRotationPolicy].
func (w *SecretRotationPolicyWebhook) Default(_ context.Context, srp *secretsv1alpha1.SecretRotationPolicy) error {
	if srp.Spec.Policy.HistoryLimit == nil {
		limit := int32(10)
		srp.Spec.Policy.HistoryLimit = &limit
	}
	if srp.Spec.Policy.FailurePolicy == "" {
		srp.Spec.Policy.FailurePolicy = secretsv1alpha1.FailurePolicyFail
	}
	return nil
}

// ValidateCreate validates a new SecretRotationPolicy.
// Implements admission.Validator[*SecretRotationPolicy].
func (w *SecretRotationPolicyWebhook) ValidateCreate(_ context.Context, srp *secretsv1alpha1.SecretRotationPolicy) (admission.Warnings, error) {
	return nil, w.validate(srp)
}

// ValidateUpdate validates an updated SecretRotationPolicy.
// Implements admission.Validator[*SecretRotationPolicy].
func (w *SecretRotationPolicyWebhook) ValidateUpdate(_ context.Context, _ *secretsv1alpha1.SecretRotationPolicy, newSRP *secretsv1alpha1.SecretRotationPolicy) (admission.Warnings, error) {
	return nil, w.validate(newSRP)
}

// ValidateDelete always permits deletion.
// Implements admission.Validator[*SecretRotationPolicy].
func (w *SecretRotationPolicyWebhook) ValidateDelete(_ context.Context, _ *secretsv1alpha1.SecretRotationPolicy) (admission.Warnings, error) {
	return nil, nil
}

// validate applies semantic validation beyond what CRD OpenAPI constraints can express.
func (w *SecretRotationPolicyWebhook) validate(srp *secretsv1alpha1.SecretRotationPolicy) error {
	// Verify the cron expression is parseable by the scheduler library.
	// The CRD regex catches structural issues; this catches semantic ones (e.g., "60 * * * *").
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(srp.Spec.Schedule); err != nil {
		return fmt.Errorf("spec.schedule %q is not a valid cron expression: %w", srp.Spec.Schedule, err)
	}

	// Require secretRef.name.
	if srp.Spec.SecretRef.Name == "" {
		return fmt.Errorf("spec.secretRef.name is required")
	}

	// Require at least one key mapping (belt-and-suspenders: CRD also enforces this).
	if len(srp.Spec.Keys) == 0 {
		return fmt.Errorf("spec.keys must contain at least one entry")
	}

	return nil
}
