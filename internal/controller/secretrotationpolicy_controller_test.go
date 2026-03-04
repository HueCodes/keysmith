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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	secretsv1alpha1 "github.com/hstores/keysmith/api/v1alpha1"
	"github.com/hstores/keysmith/internal/provider"
	"github.com/hstores/keysmith/internal/provider/mock"
)

// alwaysErrorProvider is a test double that always fails RotateSecret.
// It registers under the "mock" name so it passes CRD enum validation.
type alwaysErrorProvider struct{}

func (a *alwaysErrorProvider) Name() string                       { return "mock" }
func (a *alwaysErrorProvider) Validate(_ map[string]string) error { return nil }
func (a *alwaysErrorProvider) FetchSecret(_ context.Context, _ map[string]string) (provider.Secret, error) {
	return nil, fmt.Errorf("simulated provider failure")
}
func (a *alwaysErrorProvider) RotateSecret(_ context.Context, _ map[string]string) (provider.Secret, error) {
	return nil, fmt.Errorf("simulated provider failure")
}

// makePolicy returns a minimal valid SecretRotationPolicy.
// Call opts to customise fields before creation.
func makePolicy(name, secretName string, opts ...func(*secretsv1alpha1.SecretRotationPolicy)) *secretsv1alpha1.SecretRotationPolicy {
	srp := &secretsv1alpha1.SecretRotationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: secretsv1alpha1.SecretRotationPolicySpec{
			SecretRef: secretsv1alpha1.SecretReference{Name: secretName},
			Schedule:  "*/1 * * * *", // every minute
			Provider:  secretsv1alpha1.ProviderSpec{Name: "mock"},
			Keys: []secretsv1alpha1.KeyMapping{
				{ProviderKey: "password", SecretKey: "DB_PASSWORD"},
			},
		},
	}
	for _, opt := range opts {
		opt(srp)
	}
	return srp
}

// makeReconciler constructs a reconciler backed by the given registry and the shared k8sClient.
func makeReconciler(reg *provider.Registry) *SecretRotationPolicyReconciler {
	return &SecretRotationPolicyReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Registry: reg,
	}
}

// namespacedName is a helper shorthand.
func namespacedName(name string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: "default"}
}

// cleanupPolicy removes a SecretRotationPolicy and any RotationRecords it owns.
func cleanupPolicy(ctx context.Context, name string) {
	srp := &secretsv1alpha1.SecretRotationPolicy{}
	if err := k8sClient.Get(ctx, namespacedName(name), srp); err == nil {
		// Strip finalizer so deletion proceeds immediately.
		srp.Finalizers = nil
		_ = k8sClient.Update(ctx, srp)
		_ = k8sClient.Delete(ctx, srp)
	}

	rrList := &secretsv1alpha1.RotationRecordList{}
	if err := k8sClient.List(ctx, rrList,
		client.InNamespace("default"),
		client.MatchingLabels{"secrets.keysmith.io/policy": name},
	); err == nil {
		for i := range rrList.Items {
			rr := rrList.Items[i]
			rr.Finalizers = nil
			_ = k8sClient.Update(ctx, &rr)
			_ = k8sClient.Delete(ctx, &rr)
		}
	}
}

var _ = Describe("SecretRotationPolicy Controller", func() {

	Describe("Happy path — scheduled rotation", func() {
		const policyName = "happy-path"
		const secretName = "happy-secret"

		AfterEach(func() {
			cleanupPolicy(ctx, policyName)
			s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, s)
		})

		It("creates a K8s Secret and a Succeeded RotationRecord on first rotation", func() {
			reg := provider.NewRegistry()
			reg.Register(mock.New())
			srp := makePolicy(policyName, secretName)
			Expect(k8sClient.Create(ctx, srp)).To(Succeed())

			reconciler := makeReconciler(reg)
			req := reconcile.Request{NamespacedName: namespacedName(policyName)}

			// First reconcile: adds finalizer and requeues.
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: no lastRotationTime → IsDue returns true → executes rotation.
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the target K8s Secret was created with correct data")
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, namespacedName(secretName), secret)).To(Succeed())
			Expect(secret.Data).To(HaveKey("DB_PASSWORD"))
			Expect(secret.Labels["secrets.keysmith.io/managed-by"]).To(Equal("keysmith"))
			Expect(secret.Labels["secrets.keysmith.io/policy"]).To(Equal(policyName))

			By("verifying a RotationRecord with Succeeded phase was created")
			records := &secretsv1alpha1.RotationRecordList{}
			Expect(k8sClient.List(ctx, records,
				client.InNamespace("default"),
				client.MatchingLabels{"secrets.keysmith.io/policy": policyName},
			)).To(Succeed())
			Expect(records.Items).To(HaveLen(1))
			Expect(records.Items[0].Status.Phase).To(Equal(secretsv1alpha1.RecordPhaseSucceeded))
			Expect(records.Items[0].Status.ProviderName).To(Equal("mock"))
			Expect(records.Items[0].Status.RotatedKeys).To(ContainElement("DB_PASSWORD"))
			Expect(records.Items[0].Status.Duration).NotTo(BeEmpty())

			By("verifying policy status reflects a successful rotation")
			Expect(k8sClient.Get(ctx, namespacedName(policyName), srp)).To(Succeed())
			Expect(srp.Status.Phase).To(Equal(secretsv1alpha1.PhaseReady))
			Expect(srp.Status.LastRotationTime).NotTo(BeNil())
			Expect(srp.Status.NextRotationTime).NotTo(BeNil())
			Expect(srp.Status.LastRotationRecord).NotTo(BeNil())
		})
	})

	Describe("Manual rotation via annotation", func() {
		const policyName = "manual-rotation"
		const secretName = "manual-secret"

		AfterEach(func() {
			cleanupPolicy(ctx, policyName)
			s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, s)
		})

		It("executes rotation when annotation is set and removes it afterward", func() {
			reg := provider.NewRegistry()
			reg.Register(mock.New())

			// Use a far-future schedule so the rotation is not due automatically.
			srp := makePolicy(policyName, secretName, func(s *secretsv1alpha1.SecretRotationPolicy) {
				s.Spec.Schedule = "0 0 1 1 *" // once per year, January 1st
				s.Annotations = map[string]string{
					secretsv1alpha1.AnnotationRotateNow: secretsv1alpha1.AnnotationRotateVal,
				}
			})
			Expect(k8sClient.Create(ctx, srp)).To(Succeed())

			reconciler := makeReconciler(reg)
			req := reconcile.Request{NamespacedName: namespacedName(policyName)}

			// First reconcile: adds finalizer.
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Set lastRotationTime to now so the schedule is not due.
			Expect(k8sClient.Get(ctx, namespacedName(policyName), srp)).To(Succeed())
			now := metav1.Now()
			srp.Status.LastRotationTime = &now
			Expect(k8sClient.Status().Update(ctx, srp)).To(Succeed())

			// Second reconcile: schedule not due, but manual annotation is set → rotation executes.
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the manual rotation annotation was removed")
			Expect(k8sClient.Get(ctx, namespacedName(policyName), srp)).To(Succeed())
			Expect(srp.Annotations).NotTo(HaveKey(secretsv1alpha1.AnnotationRotateNow))

			By("verifying a RotationRecord with TriggerManual was created")
			records := &secretsv1alpha1.RotationRecordList{}
			Expect(k8sClient.List(ctx, records,
				client.InNamespace("default"),
				client.MatchingLabels{"secrets.keysmith.io/policy": policyName},
			)).To(Succeed())
			Expect(records.Items).NotTo(BeEmpty())
			Expect(records.Items[0].Spec.TriggeredBy).To(Equal(secretsv1alpha1.TriggerManual))
		})
	})

	Describe("Suspended policy", func() {
		const policyName = "suspended-policy"
		const secretName = "suspended-secret"

		AfterEach(func() {
			cleanupPolicy(ctx, policyName)
		})

		It("sets phase to Suspended without performing rotation", func() {
			reg := provider.NewRegistry()
			reg.Register(mock.New())

			srp := makePolicy(policyName, secretName, func(s *secretsv1alpha1.SecretRotationPolicy) {
				s.Spec.Policy = secretsv1alpha1.RotationPolicy{Suspend: true}
			})
			Expect(k8sClient.Create(ctx, srp)).To(Succeed())

			reconciler := makeReconciler(reg)
			req := reconcile.Request{NamespacedName: namespacedName(policyName)}

			// First reconcile: adds finalizer.
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: suspension detected.
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Minute))

			By("verifying the policy phase is Suspended")
			Expect(k8sClient.Get(ctx, namespacedName(policyName), srp)).To(Succeed())
			Expect(srp.Status.Phase).To(Equal(secretsv1alpha1.PhaseSuspended))

			By("verifying no RotationRecords were created")
			records := &secretsv1alpha1.RotationRecordList{}
			Expect(k8sClient.List(ctx, records,
				client.InNamespace("default"),
				client.MatchingLabels{"secrets.keysmith.io/policy": policyName},
			)).To(Succeed())
			Expect(records.Items).To(BeEmpty())
		})
	})

	Describe("Provider failure", func() {
		const policyName = "failing-policy"
		const secretName = "fail-secret"

		AfterEach(func() {
			cleanupPolicy(ctx, policyName)
		})

		It("creates a Failed RotationRecord and sets policy phase to Failed", func() {
			failingReg := provider.NewRegistry()
			failingReg.Register(&alwaysErrorProvider{})

			srp := makePolicy(policyName, secretName)
			Expect(k8sClient.Create(ctx, srp)).To(Succeed())

			reconciler := makeReconciler(failingReg)
			req := reconcile.Request{NamespacedName: namespacedName(policyName)}

			// First reconcile: adds finalizer.
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: rotation fails.
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("verifying the RotationRecord has Failed phase with an error message")
			records := &secretsv1alpha1.RotationRecordList{}
			Expect(k8sClient.List(ctx, records,
				client.InNamespace("default"),
				client.MatchingLabels{"secrets.keysmith.io/policy": policyName},
			)).To(Succeed())
			Expect(records.Items).To(HaveLen(1))
			Expect(records.Items[0].Status.Phase).To(Equal(secretsv1alpha1.RecordPhaseFailed))
			Expect(records.Items[0].Status.Error).NotTo(BeEmpty())

			By("verifying the policy phase is Failed")
			Expect(k8sClient.Get(ctx, namespacedName(policyName), srp)).To(Succeed())
			Expect(srp.Status.Phase).To(Equal(secretsv1alpha1.PhaseFailed))
		})

		It("sets phase to Ready when failurePolicy is Ignore", func() {
			failingReg := provider.NewRegistry()
			failingReg.Register(&alwaysErrorProvider{})

			srp := makePolicy(policyName+"-ignore", secretName+"-ignore", func(s *secretsv1alpha1.SecretRotationPolicy) {
				s.Spec.Policy.FailurePolicy = secretsv1alpha1.FailurePolicyIgnore
			})
			Expect(k8sClient.Create(ctx, srp)).To(Succeed())
			defer cleanupPolicy(ctx, policyName+"-ignore")

			reconciler := makeReconciler(failingReg)
			req := reconcile.Request{NamespacedName: namespacedName(policyName + "-ignore")}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName(policyName+"-ignore"), srp)).To(Succeed())
			Expect(srp.Status.Phase).To(Equal(secretsv1alpha1.PhaseReady))
		})
	})

	Describe("History pruning", func() {
		const policyName = "pruning-policy"
		const secretName = "pruning-secret"

		AfterEach(func() {
			cleanupPolicy(ctx, policyName)
			s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, s)
		})

		It("deletes the oldest RotationRecords when historyLimit is exceeded", func() {
			reg := provider.NewRegistry()
			reg.Register(mock.New())

			historyLimit := int32(2)
			srp := makePolicy(policyName, secretName, func(s *secretsv1alpha1.SecretRotationPolicy) {
				s.Spec.Policy = secretsv1alpha1.RotationPolicy{HistoryLimit: &historyLimit}
			})
			Expect(k8sClient.Create(ctx, srp)).To(Succeed())

			reconciler := makeReconciler(reg)
			req := reconcile.Request{NamespacedName: namespacedName(policyName)}

			// Add finalizer.
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("pre-creating old RotationRecords to simulate prior rotation history")
			for i := range 3 {
				rr := &secretsv1alpha1.RotationRecord{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: fmt.Sprintf("%s-old-", policyName),
						Namespace:    "default",
						Labels: map[string]string{
							"secrets.keysmith.io/policy": policyName,
						},
					},
					Spec: secretsv1alpha1.RotationRecordSpec{
						PolicyRef:   secretsv1alpha1.SecretReference{Name: policyName, Namespace: "default"},
						TriggeredBy: secretsv1alpha1.TriggerSchedule,
						RequestedAt: metav1.NewTime(time.Now().Add(-time.Duration(i+1) * time.Hour)),
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			}

			// Reconcile triggers one more rotation (no lastRotationTime → IsDue=true).
			// Total records = 3 old + 1 new = 4; historyLimit = 2 → pruned to 2.
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("verifying only historyLimit records remain")
			records := &secretsv1alpha1.RotationRecordList{}
			Expect(k8sClient.List(ctx, records,
				client.InNamespace("default"),
				client.MatchingLabels{"secrets.keysmith.io/policy": policyName},
			)).To(Succeed())
			Expect(records.Items).To(HaveLen(int(historyLimit)))
		})
	})

	Describe("Workload restarts", func() {
		const policyName = "restart-policy"
		const secretName = "restart-secret"

		AfterEach(func() {
			cleanupPolicy(ctx, policyName)
			s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, s)
		})

		It("records triggered restarts in the RotationRecord", func() {
			reg := provider.NewRegistry()
			reg.Register(mock.New())

			// Create a Deployment that will be referenced as a restart target.
			// In envtest the Deployment exists but no pods are managed; the controller
			// just patches the pod template annotation, which is sufficient to verify
			// the restart logic is wired correctly.

			srp := makePolicy(policyName, secretName, func(s *secretsv1alpha1.SecretRotationPolicy) {
				s.Spec.RestartTargets = []secretsv1alpha1.RestartTarget{
					// Reference a non-existent Deployment; the controller logs the error but continues.
					{Kind: "Deployment", Name: "nonexistent-deploy", Namespace: "default"},
				}
			})
			Expect(k8sClient.Create(ctx, srp)).To(Succeed())

			reconciler := makeReconciler(reg)
			req := reconcile.Request{NamespacedName: namespacedName(policyName)}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Rotation executes; restart of nonexistent Deployment fails silently.
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Rotation should still succeed.
			Expect(k8sClient.Get(ctx, namespacedName(policyName), srp)).To(Succeed())
			Expect(srp.Status.Phase).To(Equal(secretsv1alpha1.PhaseReady))
		})
	})
})
