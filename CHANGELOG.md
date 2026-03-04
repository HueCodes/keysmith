# Changelog

All notable changes to keysmith are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Keysmith uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

---

## [0.1.0] — 2026-03-03

### Added

#### Core Operator

- `SecretRotationPolicy` CRD — declarative secret rotation with cron scheduling, provider
  selection, key mapping, workload restarts, retry/backoff, and notification webhooks
- `RotationRecord` CRD — immutable per-attempt audit log with phase, duration, provider name,
  rotated keys, triggered restarts, and error detail
- Finalizer pattern for graceful cleanup on policy deletion
- Manual rotation via `secrets.keysmith.io/rotate=now` annotation
- Suspend/resume via `spec.policy.suspend`
- `historyLimit` enforcement — prunes oldest `RotationRecord`s beyond the configured limit
- Rolling restart of `Deployment`, `StatefulSet`, and `DaemonSet` workloads after successful
  rotation, triggered via pod template annotation patch

#### Provider System

- Pluggable `Provider` interface with `FetchSecret`, `RotateSecret`, and `Validate` methods
- `provider.Registry` — thread-safe name-based provider registry with duplicate detection
- `provider.MapKeys` — maps provider-side keys to Kubernetes secret data keys per `KeyMapping`
- **`static` provider** — generates cryptographically random passwords using `crypto/rand`;
  configurable length (8–256 bytes, default 32)
- **`mock` provider** — deterministic versioned values for testing and demos; incrementing
  counter produces observable rotation (`mock-password-v1`, `mock-password-v2`, …)
- **`aws` provider** stub — validates `secretId` parameter; documents AWS SDK integration path
- **`vault` provider** stub — validates `address` and `path` parameters; documents Vault client
  integration path

#### Notifications

- Asynchronous HTTP webhook dispatch on `RotationSucceeded` and `RotationFailed` events
- URL resolution: inline `url` field or K8s Secret lookup via `secretRef`
- JSON payload includes event type, policy name, namespace, timestamp, record name, and error

#### Admission Webhooks

- **Defaulting webhook** — sets `historyLimit=10` and `failurePolicy=Fail` if unset
- **Validation webhook** — semantic cron expression validation (beyond CRD regex), required
  field checks for `secretRef.name` and `keys`

#### Observability

- Prometheus metrics registered via `init()`:
  - `keysmith_rotations_total` — counter by policy, namespace, provider, result
  - `keysmith_rotation_duration_seconds` — histogram by policy, namespace, provider
  - `keysmith_next_rotation_timestamp` — gauge for alerting on stale policies
  - `keysmith_active_policies` — gauge for non-suspended policy count

#### Scheduling

- Cron-based scheduling using `robfig/cron v3` with 5-field parser
- `rotation.IsDue` — supports rotation window (rotate early before scheduled time)
- `rotation.RequeueDelay` — computes efficient requeue delay to avoid polling

#### CI/CD

- GitHub Actions: `ci.yml`, `lint.yml`, `test.yml`, `test-e2e.yml`, `release.yml`
- `golangci-lint` configuration with `errcheck`, `staticcheck`, `revive`, `ginkgolinter`, and more
- Devcontainer configuration for reproducible development environments

#### Testing

- `envtest`-based integration tests for the controller reconcile loop:
  - Happy path — scheduled rotation creates K8s Secret and Succeeded RotationRecord
  - Manual rotation — annotation triggers rotation and is removed afterward
  - Suspended policy — no rotation, phase set to Suspended
  - Provider failure — Failed RotationRecord created; policy phase set to Failed or Ready
    depending on `failurePolicy`
  - History pruning — oldest records deleted when `historyLimit` is exceeded
  - Workload restarts — graceful handling of missing restart targets
- Unit tests for `mock` provider (100% coverage), `static` provider (87%), `rotation`
  scheduler (91%), and provider registry

#### Configuration Samples

- `config/samples/secrets_v1alpha1_secretrotationpolicy.yaml` — minimal working example
  (static provider, no external dependencies)
- `config/samples/secrets_v1alpha1_full_example.yaml` — fully-annotated example showing all
  available fields with inline documentation

[Unreleased]: https://github.com/hstores/keysmith/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/hstores/keysmith/releases/tag/v0.1.0
