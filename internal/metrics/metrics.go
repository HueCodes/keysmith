// Package metrics registers Prometheus metrics for the keysmith operator.
// Import this package with a blank import in cmd/main.go to activate registration.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// RotationsTotal counts rotation attempts by policy, namespace, provider, and result.
	RotationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "keysmith",
			Name:      "rotations_total",
			Help:      "Total number of secret rotation attempts.",
		},
		[]string{"policy", "namespace", "provider", "result"},
	)

	// RotationDuration measures how long each rotation takes.
	RotationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "keysmith",
			Name:      "rotation_duration_seconds",
			Help:      "Duration of secret rotation operations in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"policy", "namespace", "provider"},
	)

	// NextRotationTimestamp tracks when each policy is next scheduled to rotate.
	// Useful for alerting on stale policies (value stops updating).
	NextRotationTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "keysmith",
			Name:      "next_rotation_timestamp_seconds",
			Help:      "Unix timestamp of the next scheduled rotation for each policy.",
		},
		[]string{"policy", "namespace"},
	)

	// ActivePolicies tracks the number of non-suspended policies being managed.
	ActivePolicies = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "keysmith",
		Name:      "active_policies_total",
		Help:      "Number of active (non-suspended) SecretRotationPolicy objects.",
	})
)

func init() {
	metrics.Registry.MustRegister(
		RotationsTotal,
		RotationDuration,
		NextRotationTimestamp,
		ActivePolicies,
	)
}
