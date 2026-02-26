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

package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	secretsv1alpha1 "github.com/hstores/keysmith/api/v1alpha1"
	ksmetrics "github.com/hstores/keysmith/internal/metrics"
	"github.com/hstores/keysmith/internal/provider"
	"github.com/hstores/keysmith/internal/rotation"
)

const (
	defaultHistoryLimit = int32(10)
	minRequeueDelay     = 30 * time.Second
	defaultRetryBackoff = 30 * time.Second
)

// SecretRotationPolicyReconciler reconciles SecretRotationPolicy objects.
//
// +kubebuilder:rbac:groups=secrets.keysmith.io,resources=secretrotationpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=secrets.keysmith.io,resources=secretrotationpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=secrets.keysmith.io,resources=secretrotationpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=secrets.keysmith.io,resources=rotationrecords,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=secrets.keysmith.io,resources=rotationrecords/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;daemonsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
type SecretRotationPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Registry *provider.Registry
}

// Reconcile is the main reconcile loop for SecretRotationPolicy.
func (r *SecretRotationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	policy := &secretsv1alpha1.SecretRotationPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetching SecretRotationPolicy: %w", err)
	}

	// Handle deletion via finalizer.
	if !policy.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, policy)
	}

	// Ensure finalizer is present.
	if !controllerutil.ContainsFinalizer(policy, secretsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(policy, secretsv1alpha1.FinalizerName)
		if err := r.Update(ctx, policy); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Suspended: update status and requeue slowly.
	if policy.Spec.Policy.Suspend {
		logger.Info("policy is suspended, skipping rotation")
		return r.setSuspended(ctx, policy)
	}

	// Resolve and validate provider.
	prov, err := r.Registry.Get(policy.Spec.Provider.Name)
	if err != nil {
		return r.setProviderUnhealthy(ctx, policy, err)
	}
	if err := prov.Validate(policy.Spec.Provider.Params); err != nil {
		return r.setProviderUnhealthy(ctx, policy, fmt.Errorf("provider validation: %w", err))
	}

	// Check for manual rotation annotation.
	manualRotation := policy.Annotations[secretsv1alpha1.AnnotationRotateNow] == secretsv1alpha1.AnnotationRotateVal

	// Determine whether a scheduled rotation is due.
	var lastRotation *time.Time
	if policy.Status.LastRotationTime != nil {
		t := policy.Status.LastRotationTime.Time
		lastRotation = &t
	}
	rotationWindow := time.Duration(0)
	if policy.Spec.RotationWindow != nil {
		rotationWindow = policy.Spec.RotationWindow.Duration
	}

	due, err := rotation.IsDue(policy.Spec.Schedule, lastRotation, rotationWindow, time.Now())
	if err != nil {
		return r.markDegraded(ctx, policy, "InvalidSchedule", err.Error())
	}

	if !due && !manualRotation {
		delay, err := rotation.RequeueDelay(policy.Spec.Schedule, lastRotation, rotationWindow, time.Now(), minRequeueDelay)
		if err != nil {
			return r.markDegraded(ctx, policy, "InvalidSchedule", err.Error())
		}

		// Update nextRotationTime in status so it shows in kubectl.
		if lastRotation != nil {
			if next, err := rotation.NextScheduled(policy.Spec.Schedule, *lastRotation); err == nil {
				nextTime := metav1.NewTime(next)
				policy.Status.NextRotationTime = &nextTime
				_ = r.Status().Update(ctx, policy)
			}
		}

		logger.V(1).Info("rotation not due yet, requeueing", "delay", delay)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	return r.executeRotation(ctx, policy, prov, manualRotation)
}

// executeRotation runs the full rotation lifecycle:
// fetch new secret -> update k8s secret -> restart workloads -> record audit trail.
func (r *SecretRotationPolicyReconciler) executeRotation(
	ctx context.Context,
	policy *secretsv1alpha1.SecretRotationPolicy,
	prov provider.Provider,
	manual bool,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	start := time.Now()

	// Mark as Rotating.
	policy.Status.Phase = secretsv1alpha1.PhaseRotating
	_ = r.Status().Update(ctx, policy)

	trigger := secretsv1alpha1.TriggerSchedule
	if manual {
		trigger = secretsv1alpha1.TriggerManual
	}

	// Create the RotationRecord before attempting rotation (audit trail).
	record := r.newRotationRecord(policy, trigger)
	if err := ctrl.SetControllerReference(policy, record, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting owner reference on RotationRecord: %w", err)
	}
	if err := r.Create(ctx, record); err != nil {
		logger.Error(err, "failed to create RotationRecord")
	}

	// Call the provider.
	secret, provErr := prov.RotateSecret(ctx, policy.Spec.Provider.Params)
	duration := time.Since(start)

	result := "success"
	if provErr != nil {
		result = "failure"
	}
	ksmetrics.RotationsTotal.WithLabelValues(policy.Name, policy.Namespace, prov.Name(), result).Inc()
	ksmetrics.RotationDuration.WithLabelValues(policy.Name, policy.Namespace, prov.Name()).Observe(duration.Seconds())

	if provErr != nil {
		logger.Error(provErr, "provider rotation failed")
		r.finalizeRecord(ctx, record, secretsv1alpha1.RecordPhaseFailed, duration, prov.Name(), nil, nil, provErr.Error())

		if policy.Spec.Policy.FailurePolicy == secretsv1alpha1.FailurePolicyIgnore {
			policy.Status.Phase = secretsv1alpha1.PhaseReady
		} else {
			policy.Status.Phase = secretsv1alpha1.PhaseFailed
		}
		meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
			Type:               secretsv1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "RotationFailed",
			Message:            provErr.Error(),
			LastTransitionTime: metav1.Now(),
		})
		_ = r.Status().Update(ctx, policy)

		retryBackoff := defaultRetryBackoff
		if policy.Spec.Policy.RetryBackoff != nil {
			retryBackoff = policy.Spec.Policy.RetryBackoff.Duration
		}
		return ctrl.Result{RequeueAfter: retryBackoff}, nil
	}

	// Map provider keys to Kubernetes secret keys.
	data, err := provider.MapKeys(secret, policy.Spec.Keys)
	if err != nil {
		r.finalizeRecord(ctx, record, secretsv1alpha1.RecordPhaseFailed, duration, prov.Name(), nil, nil, err.Error())
		return ctrl.Result{}, fmt.Errorf("mapping provider keys: %w", err)
	}

	// Create or update the target Kubernetes secret.
	targetNS := policy.Namespace
	if policy.Spec.SecretRef.Namespace != "" {
		targetNS = policy.Spec.SecretRef.Namespace
	}
	k8sSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policy.Spec.SecretRef.Name,
			Namespace: targetNS,
		},
	}
	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, k8sSecret, func() error {
		k8sSecret.Data = data
		if k8sSecret.Labels == nil {
			k8sSecret.Labels = make(map[string]string)
		}
		k8sSecret.Labels["secrets.keysmith.io/managed-by"] = "keysmith"
		k8sSecret.Labels["secrets.keysmith.io/policy"] = policy.Name
		return nil
	}); err != nil {
		r.finalizeRecord(ctx, record, secretsv1alpha1.RecordPhaseFailed, duration, prov.Name(), nil, nil, err.Error())
		return ctrl.Result{}, fmt.Errorf("updating kubernetes secret: %w", err)
	}

	// Remove manual rotation annotation after successful rotation.
	if manual {
		delete(policy.Annotations, secretsv1alpha1.AnnotationRotateNow)
		if err := r.Update(ctx, policy); err != nil {
			logger.Error(err, "failed to remove manual rotation annotation")
		}
	}

	// Trigger rolling restarts of dependent workloads.
	rotatedKeys := make([]string, 0, len(policy.Spec.Keys))
	for _, k := range policy.Spec.Keys {
		rotatedKeys = append(rotatedKeys, k.SecretKey)
	}
	restarted := r.triggerRestarts(ctx, policy)

	// Finalize the RotationRecord.
	r.finalizeRecord(ctx, record, secretsv1alpha1.RecordPhaseSucceeded, duration, prov.Name(), rotatedKeys, restarted, "")

	// Update policy status.
	now := metav1.Now()
	policy.Status.LastRotationTime = &now
	policy.Status.Phase = secretsv1alpha1.PhaseReady
	policy.Status.CurrentRetryCount = 0
	policy.Status.LastRotationRecord = &corev1.ObjectReference{
		APIVersion: secretsv1alpha1.GroupVersion.String(),
		Kind:       "RotationRecord",
		Name:       record.Name,
		Namespace:  record.Namespace,
	}

	next, _ := rotation.NextScheduled(policy.Spec.Schedule, now.Time)
	nextTime := metav1.NewTime(next)
	policy.Status.NextRotationTime = &nextTime
	ksmetrics.NextRotationTimestamp.WithLabelValues(policy.Name, policy.Namespace).Set(float64(next.Unix()))

	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               secretsv1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "RotationComplete",
		Message:            fmt.Sprintf("Secret rotated successfully in %s via %s provider", duration.Round(time.Millisecond), prov.Name()),
		LastTransitionTime: metav1.Now(),
	})
	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               secretsv1alpha1.ConditionScheduled,
		Status:             metav1.ConditionTrue,
		Reason:             "NextRotationScheduled",
		Message:            fmt.Sprintf("Next rotation scheduled for %s", next.Format(time.RFC3339)),
		LastTransitionTime: metav1.Now(),
	})
	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               secretsv1alpha1.ConditionProviderHealthy,
		Status:             metav1.ConditionTrue,
		Reason:             "ProviderReachable",
		Message:            fmt.Sprintf("Provider %q responded successfully", prov.Name()),
		LastTransitionTime: metav1.Now(),
	})

	if err := r.Status().Update(ctx, policy); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating policy status: %w", err)
	}

	r.pruneHistory(ctx, policy)

	logger.Info("rotation completed successfully",
		"provider", prov.Name(),
		"duration", duration.Round(time.Millisecond),
		"nextRotation", next.Format(time.RFC3339),
	)

	return ctrl.Result{RequeueAfter: time.Until(next)}, nil
}

// triggerRestarts patches workload pod template annotations to trigger a rolling restart.
func (r *SecretRotationPolicyReconciler) triggerRestarts(
	ctx context.Context,
	policy *secretsv1alpha1.SecretRotationPolicy,
) []secretsv1alpha1.RestartTarget {
	logger := log.FromContext(ctx)
	restarted := make([]secretsv1alpha1.RestartTarget, 0, len(policy.Spec.RestartTargets))
	restartAt := time.Now().Format(time.RFC3339)

	for _, target := range policy.Spec.RestartTargets {
		ns := target.Namespace
		if ns == "" {
			ns = policy.Namespace
		}
		key := types.NamespacedName{Name: target.Name, Namespace: ns}

		var patchErr error
		switch target.Kind {
		case "Deployment":
			obj := &appsv1.Deployment{}
			if patchErr = r.Get(ctx, key, obj); patchErr == nil {
				if obj.Spec.Template.Annotations == nil {
					obj.Spec.Template.Annotations = make(map[string]string)
				}
				obj.Spec.Template.Annotations["secrets.keysmith.io/restartedAt"] = restartAt
				patchErr = r.Update(ctx, obj)
			}
		case "StatefulSet":
			obj := &appsv1.StatefulSet{}
			if patchErr = r.Get(ctx, key, obj); patchErr == nil {
				if obj.Spec.Template.Annotations == nil {
					obj.Spec.Template.Annotations = make(map[string]string)
				}
				obj.Spec.Template.Annotations["secrets.keysmith.io/restartedAt"] = restartAt
				patchErr = r.Update(ctx, obj)
			}
		case "DaemonSet":
			obj := &appsv1.DaemonSet{}
			if patchErr = r.Get(ctx, key, obj); patchErr == nil {
				if obj.Spec.Template.Annotations == nil {
					obj.Spec.Template.Annotations = make(map[string]string)
				}
				obj.Spec.Template.Annotations["secrets.keysmith.io/restartedAt"] = restartAt
				patchErr = r.Update(ctx, obj)
			}
		default:
			logger.Info("unsupported restart target kind, skipping", "kind", target.Kind, "name", target.Name)
			continue
		}

		if patchErr != nil {
			logger.Error(patchErr, "failed to trigger rolling restart",
				"kind", target.Kind, "name", target.Name, "namespace", ns)
		} else {
			restarted = append(restarted, target)
		}
	}
	return restarted
}

// pruneHistory deletes the oldest RotationRecords exceeding the historyLimit.
func (r *SecretRotationPolicyReconciler) pruneHistory(ctx context.Context, policy *secretsv1alpha1.SecretRotationPolicy) {
	limit := defaultHistoryLimit
	if policy.Spec.Policy.HistoryLimit != nil {
		limit = *policy.Spec.Policy.HistoryLimit
	}

	list := &secretsv1alpha1.RotationRecordList{}
	if err := r.List(ctx, list,
		client.InNamespace(policy.Namespace),
		client.MatchingLabels{"secrets.keysmith.io/policy": policy.Name},
	); err != nil {
		return
	}

	if int32(len(list.Items)) <= limit {
		return
	}

	// Sort ascending by creation time so we delete the oldest first.
	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].CreationTimestamp.Before(&list.Items[j].CreationTimestamp)
	})

	toDelete := len(list.Items) - int(limit)
	for i := 0; i < toDelete; i++ {
		_ = r.Delete(ctx, &list.Items[i])
	}
}

func (r *SecretRotationPolicyReconciler) newRotationRecord(
	policy *secretsv1alpha1.SecretRotationPolicy,
	trigger secretsv1alpha1.TriggerReason,
) *secretsv1alpha1.RotationRecord {
	return &secretsv1alpha1.RotationRecord{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", policy.Name),
			Namespace:    policy.Namespace,
			Labels: map[string]string{
				"secrets.keysmith.io/policy": policy.Name,
			},
		},
		Spec: secretsv1alpha1.RotationRecordSpec{
			PolicyRef:   secretsv1alpha1.SecretReference{Name: policy.Name, Namespace: policy.Namespace},
			TriggeredBy: trigger,
			RequestedAt: metav1.Now(),
		},
		Status: secretsv1alpha1.RotationRecordStatus{
			Phase: secretsv1alpha1.RecordPhaseRunning,
		},
	}
}

func (r *SecretRotationPolicyReconciler) finalizeRecord(
	ctx context.Context,
	record *secretsv1alpha1.RotationRecord,
	phase secretsv1alpha1.RecordPhase,
	duration time.Duration,
	providerName string,
	keys []string,
	restarted []secretsv1alpha1.RestartTarget,
	errMsg string,
) {
	now := metav1.Now()
	record.Status.Phase = phase
	record.Status.CompletionTime = &now
	record.Status.Duration = duration.Round(time.Millisecond).String()
	record.Status.ProviderName = providerName
	record.Status.RotatedKeys = keys
	record.Status.RestartsTriggered = restarted
	record.Status.Error = errMsg
	_ = r.Status().Update(ctx, record)
}

func (r *SecretRotationPolicyReconciler) handleDeletion(
	ctx context.Context,
	policy *secretsv1alpha1.SecretRotationPolicy,
) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(policy, secretsv1alpha1.FinalizerName) {
		controllerutil.RemoveFinalizer(policy, secretsv1alpha1.FinalizerName)
		if err := r.Update(ctx, policy); err != nil {
			return ctrl.Result{}, err
		}
	}
	ksmetrics.ActivePolicies.Dec()
	return ctrl.Result{}, nil
}

func (r *SecretRotationPolicyReconciler) setSuspended(
	ctx context.Context,
	policy *secretsv1alpha1.SecretRotationPolicy,
) (ctrl.Result, error) {
	policy.Status.Phase = secretsv1alpha1.PhaseSuspended
	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               secretsv1alpha1.ConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             "Suspended",
		Message:            "Rotation is suspended via spec.policy.suspend=true",
		LastTransitionTime: metav1.Now(),
	})
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, r.Status().Update(ctx, policy)
}

func (r *SecretRotationPolicyReconciler) setProviderUnhealthy(
	ctx context.Context,
	policy *secretsv1alpha1.SecretRotationPolicy,
	err error,
) (ctrl.Result, error) {
	policy.Status.Phase = secretsv1alpha1.PhaseFailed
	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               secretsv1alpha1.ConditionProviderHealthy,
		Status:             metav1.ConditionFalse,
		Reason:             "ProviderError",
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})
	_ = r.Status().Update(ctx, policy)
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

func (r *SecretRotationPolicyReconciler) markDegraded(
	ctx context.Context,
	policy *secretsv1alpha1.SecretRotationPolicy,
	reason, msg string,
) (ctrl.Result, error) {
	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               secretsv1alpha1.ConditionDegraded,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		LastTransitionTime: metav1.Now(),
	})
	_ = r.Status().Update(ctx, policy)
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

// SetupWithManager registers the controller and its owned resources with the manager.
func (r *SecretRotationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&secretsv1alpha1.SecretRotationPolicy{}).
		Owns(&secretsv1alpha1.RotationRecord{}).
		Complete(r)
}
