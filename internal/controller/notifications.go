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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	secretsv1alpha1 "github.com/hstores/keysmith/api/v1alpha1"
)

const notificationTimeout = 10 * time.Second

// NotificationPayload is the JSON body posted to webhook endpoints on rotation events.
type NotificationPayload struct {
	Event     secretsv1alpha1.NotificationEvent `json:"event"`
	Policy    string                            `json:"policy"`
	Namespace string                            `json:"namespace"`
	Timestamp string                            `json:"timestamp"`
	Record    string                            `json:"record,omitempty"`
	Error     string                            `json:"error,omitempty"`
}

// dispatchNotifications fires asynchronous HTTP webhook notifications for the given event.
// URL resolution precedence: NotificationSpec.URL → K8s Secret via NotificationSpec.SecretRef.
// Each notification is dispatched in its own goroutine so it never blocks the reconcile loop.
func (r *SecretRotationPolicyReconciler) dispatchNotifications(
	ctx context.Context,
	policy *secretsv1alpha1.SecretRotationPolicy,
	event secretsv1alpha1.NotificationEvent,
	payload NotificationPayload,
) {
	logger := log.FromContext(ctx)

	for i := range policy.Spec.Notifications {
		notif := &policy.Spec.Notifications[i]
		if !slices.Contains(notif.Events, event) {
			continue
		}

		url := notif.URL
		if url == "" && notif.SecretRef != nil {
			secret := &corev1.Secret{}
			key := types.NamespacedName{Name: notif.SecretRef.Name, Namespace: policy.Namespace}
			if err := r.Get(ctx, key, secret); err != nil {
				logger.Error(err, "Failed to read notification secret",
					"secret", notif.SecretRef.Name, "event", event)
				continue
			}
			url = string(secret.Data[notif.SecretRef.Key])
		}

		if url == "" {
			logger.Info("Skipping notification with no resolved URL", "event", event)
			continue
		}

		endpoint := url
		go func() {
			dispatchCtx, cancel := context.WithTimeout(context.Background(), notificationTimeout)
			defer cancel()
			if err := postNotification(dispatchCtx, endpoint, payload); err != nil {
				logger.Error(err, "Failed to deliver notification", "url", endpoint, "event", event)
			}
		}()
	}
}

func postNotification(ctx context.Context, url string, payload NotificationPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling notification payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "keysmith-operator/v1alpha1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending notification to %q: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= 400 {
		return fmt.Errorf("notification endpoint %q returned HTTP %d", url, resp.StatusCode)
	}
	return nil
}
