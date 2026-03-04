# keysmith

**Automated secret rotation for Kubernetes — schedule it, forget it, audit it.**

Keysmith is a Kubernetes operator that reconciles `SecretRotationPolicy` custom resources to automatically rotate secrets on a configurable schedule. It fetches fresh credentials from a pluggable provider backend (AWS Secrets Manager, HashiCorp Vault, or a built-in static generator), writes them to a Kubernetes `Secret`, optionally triggers rolling restarts of dependent workloads, and creates an immutable `RotationRecord` audit log entry for every attempt.

---

## Why keysmith?

Secret rotation is a critical security practice mandated by standards like SOC 2, PCI-DSS, and ISO 27001, yet most teams rotate secrets manually — if at all. Keysmith brings rotation into your GitOps workflow as a first-class Kubernetes resource:

- **Declarative** — rotation policy lives next to your workload manifests
- **Auditable** — every rotation attempt — success or failure — creates an immutable `RotationRecord`
- **Resilient** — configurable retry/backoff and failure policies prevent a transient provider outage from cascading
- **Observable** — Prometheus metrics and structured logs out of the box
- **Zero-downtime** — optional rolling restart of Deployments/StatefulSets/DaemonSets after rotation

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                          Kubernetes Cluster                       │
│                                                                   │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │               keysmith Controller Manager                  │   │
│  │                                                           │   │
│  │  ┌─────────────────────────────────┐                     │   │
│  │  │   SecretRotationPolicy          │  reconcile loop      │   │
│  │  │   Reconciler                    │─────────────────┐   │   │
│  │  └─────────────────────────────────┘                 │   │   │
│  │              │ IsDue? Manual?                         │   │   │
│  │              ▼                                        │   │   │
│  │  ┌──────────────────┐  ┌─────────────────────────┐   │   │   │
│  │  │ Provider Registry│─▶│  Provider Interface      │   │   │   │
│  │  └──────────────────┘  │  RotateSecret() → Secret │   │   │   │
│  │   mock | static        └─────────────────────────┘   │   │   │
│  │   aws  | vault                  │                     │   │   │
│  │                                 ▼                     │   │   │
│  │                    ┌───────────────────────┐          │   │   │
│  │                    │  K8s Secret (upsert)  │          │   │   │
│  │                    └───────────────────────┘          │   │   │
│  │                                 │                     │   │   │
│  │                    ┌────────────┴──────────┐          │   │   │
│  │                    │  Rolling Restart       │          │   │   │
│  │                    │  Deployment/STS/DS     │          │   │   │
│  │                    └───────────────────────┘          │   │   │
│  │                                 │                     │   │   │
│  │                    ┌────────────┴──────────┐          │   │   │
│  │                    │  RotationRecord        │◀─────────┘   │   │
│  │                    │  (immutable audit log) │              │   │
│  │                    └───────────────────────┘              │   │
│  └───────────────────────────────────────────────────────────┘   │
│                                                                   │
│  ┌──────────────────┐   ┌──────────────────┐                     │
│  │ Prometheus        │   │ Webhook Notifs   │                     │
│  │ /metrics          │   │ (HTTP POST)      │                     │
│  └──────────────────┘   └──────────────────┘                     │
└──────────────────────────────────────────────────────────────────┘
```

---

## Features

| Feature | Details |
|---------|---------|
| **Cron scheduling** | Standard 5-field cron expressions plus `@hourly`, `@daily`, etc. |
| **Rotation window** | Rotate early if within N duration of next scheduled time |
| **Manual rotation** | Set annotation `secrets.keysmith.io/rotate=now` |
| **Suspend/resume** | Pause all rotations without deleting the policy |
| **Retry/backoff** | Configurable retry limit and backoff duration on transient failures |
| **Failure policies** | `Fail` (default) or `Ignore` — controls phase after provider error |
| **History pruning** | Auto-deletes oldest `RotationRecord`s beyond `historyLimit` |
| **Workload restarts** | Rolling restarts Deployments, StatefulSets, DaemonSets after rotation |
| **Webhook notifications** | HTTP POST on `RotationSucceeded`, `RotationFailed`, `RotationSkipped` |
| **Admission webhooks** | Defaulting (historyLimit=10, failurePolicy=Fail) + validation (cron parsing) |
| **Prometheus metrics** | `rotations_total`, `rotation_duration_seconds`, `next_rotation_timestamp`, `active_policies` |
| **Audit trail** | Immutable `RotationRecord` per attempt: phase, duration, provider, keys, error |
| **Provider abstraction** | Pluggable `Provider` interface — add new backends in ~50 lines |

---

## Quick Start (5 minutes, no external dependencies)

### Prerequisites

- [kind](https://kind.sigs.k8s.io/) or any Kubernetes ≥ 1.27 cluster
- `kubectl`
- `make`, `go 1.22+`

### 1. Create a cluster and install the operator

```bash
kind create cluster --name keysmith-demo

# Install CRDs into the cluster
make install

# Run the controller locally (uses current kubeconfig)
make run &
```

### 2. Apply the demo policy

```bash
kubectl apply -f config/samples/secrets_v1alpha1_secretrotationpolicy.yaml
```

This creates a `SecretRotationPolicy` that generates a new random password every 5 minutes using the **static** provider — no AWS account or Vault installation needed.

### 3. Watch it work

```bash
# Watch policy phase and rotation times
kubectl get srp -w

# NAME           PROVIDER   SCHEDULE      PHASE   LAST ROTATION          NEXT ROTATION
# demo-rotation  static     */5 * * * *   Ready   2026-01-15T10:05:00Z   2026-01-15T10:10:00Z

# Inspect the generated secret
kubectl get secret demo-secret -o jsonpath='{.data.DB_PASSWORD}' | base64 -d

# View the audit trail
kubectl get rr

# NAME                  POLICY         TRIGGER    PHASE       PROVIDER   DURATION   AGE
# demo-rotation-x9k2f   demo-rotation  Schedule   Succeeded   static     11ms       2m

# Trigger an immediate manual rotation
kubectl annotate srp demo-rotation secrets.keysmith.io/rotate=now
```

---

## Installation

### Option A: Single manifest

```bash
kubectl apply -f https://raw.githubusercontent.com/hstores/keysmith/main/dist/install.yaml
```

### Option B: Kustomize

```bash
kubectl apply -k config/default/
```

### Option C: From source

```bash
export IMG=ghcr.io/hstores/keysmith:latest
make docker-build docker-push IMG=$IMG
make deploy IMG=$IMG
```

---

## API Reference

### SecretRotationPolicy (`srp`)

```yaml
apiVersion: secrets.keysmith.io/v1alpha1
kind: SecretRotationPolicy
metadata:
  name: my-policy
  namespace: my-namespace
spec:
  # ── Required ──────────────────────────────────────────────────

  # secretRef identifies the Kubernetes Secret to create/update.
  secretRef:
    name: my-app-secret
    namespace: my-namespace    # optional; defaults to policy namespace

  # schedule is a 5-field cron expression or shorthand like @hourly.
  schedule: "0 */6 * * *"      # every 6 hours

  # provider configures the secret backend.
  provider:
    name: static               # mock | static | aws | vault
    params:
      length: "32"             # provider-specific parameters

  # keys maps provider-side keys to Kubernetes secret data keys.
  keys:
    - providerKey: password
      secretKey: DB_PASSWORD

  # ── Optional ──────────────────────────────────────────────────

  # rotationWindow rotates early if within this duration of the next scheduled time.
  rotationWindow: 5m

  # restartTargets lists workloads to rolling-restart after a successful rotation.
  restartTargets:
    - kind: Deployment         # Deployment | StatefulSet | DaemonSet
      name: my-app
      namespace: my-namespace  # optional; defaults to policy namespace

  # policy controls retry, history, and suspend behaviour.
  policy:
    historyLimit: 10           # max RotationRecords to retain (default: 10, set by webhook)
    suspend: false             # true pauses all scheduled rotations
    failurePolicy: Fail        # Fail | Ignore  (default: Fail, set by webhook)
    retryLimit: 3              # max retry attempts before giving up
    retryBackoff: 30s          # wait between retries

  # notifications sends HTTP webhooks on rotation events.
  notifications:
    - url: https://hooks.slack.com/services/XXX/YYY/ZZZ
      events: [RotationSucceeded, RotationFailed]
    - secretRef:               # read URL from a K8s Secret key
        name: my-webhook-secret
        key: url
      events: [RotationFailed]
```

**Status fields:**

| Field | Description |
|-------|-------------|
| `status.phase` | `Pending` → `Rotating` → `Ready` / `Failed` / `Suspended` |
| `status.lastRotationTime` | Timestamp of last successful rotation |
| `status.nextRotationTime` | Timestamp of next scheduled rotation |
| `status.lastRotationRecord` | ObjectReference to most recent `RotationRecord` |
| `status.currentRetryCount` | Retry attempt count for the current rotation |
| `status.conditions` | `Ready`, `Scheduled`, `ProviderHealthy`, `Degraded` |

**Annotations:**

| Annotation | Value | Effect |
|-----------|-------|--------|
| `secrets.keysmith.io/rotate` | `now` | Triggers immediate rotation on next reconcile |

---

### RotationRecord (`rr`)

Immutable audit log created for every rotation attempt. Cleaned up automatically per `historyLimit`.

```yaml
apiVersion: secrets.keysmith.io/v1alpha1
kind: RotationRecord
metadata:
  name: my-policy-x9k2f
  namespace: my-namespace
spec:
  policyRef:
    name: my-policy
    namespace: my-namespace
  triggeredBy: Schedule        # Schedule | Manual
  requestedAt: "2026-01-15T10:00:00Z"
status:
  phase: Succeeded             # Pending | Running | Succeeded | Failed
  startTime: "2026-01-15T10:00:00Z"
  completionTime: "2026-01-15T10:00:00Z"
  duration: "142ms"
  providerName: static
  rotatedKeys: [DB_PASSWORD]
  restartsTriggered:
    - kind: Deployment
      name: my-app
  error: ""                    # populated on failure
  retryCount: 0
```

---

## Providers

### `static` — Built-in random password generator

Generates cryptographically secure passwords using `crypto/rand`. No external dependencies.

```yaml
provider:
  name: static
  params:
    length: "32"    # 8–256, default 32
keys:
  - providerKey: password
    secretKey: MY_PASSWORD
```

### `mock` — Deterministic test/demo provider

Returns versioned fake values (`mock-password-v1`, `mock-password-v2`, …). Useful for demos and tests. No credentials needed.

```yaml
provider:
  name: mock
  params:
    prefix: myapp    # optional, default "mock"
keys:
  - { providerKey: password, secretKey: DB_PASSWORD }
  - { providerKey: username, secretKey: DB_USER }
  - { providerKey: apiKey,   secretKey: API_KEY }
```

### `aws` — AWS Secrets Manager (stub)

> **Status**: Stub — add `github.com/aws/aws-sdk-go-v2/service/secretsmanager` and implement `FetchSecret`/`RotateSecret` in `internal/provider/aws/aws.go`. Inline TODO comments document the required API calls.

```yaml
provider:
  name: aws
  params:
    secretId: prod/myapp/db      # required — ARN or name
    region: us-east-1            # optional; uses AWS_DEFAULT_REGION if unset
```

### `vault` — HashiCorp Vault KV v2 (stub)

> **Status**: Stub — add `github.com/hashicorp/vault-client-go` and implement `FetchSecret`/`RotateSecret` in `internal/provider/vault/vault.go`.

```yaml
provider:
  name: vault
  params:
    address: https://vault.example.com   # required
    path: secret/data/myapp              # required — KV v2 path
```

### Adding a custom provider

Implement the `Provider` interface (≈ 50 lines), then register in `cmd/main.go`:

```go
// internal/provider/mybackend/mybackend.go
package mybackend

import (
    "context"
    "github.com/hstores/keysmith/internal/provider"
)

type MyBackend struct{}

func New() *MyBackend                     { return &MyBackend{} }
func (b *MyBackend) Name() string         { return "mybackend" }
func (b *MyBackend) Validate(params map[string]string) error { ... }
func (b *MyBackend) FetchSecret(ctx context.Context, params map[string]string) (provider.Secret, error) { ... }
func (b *MyBackend) RotateSecret(ctx context.Context, params map[string]string) (provider.Secret, error) { ... }
```

```go
// cmd/main.go
registry.Register(mybackend.New())
```

Also add `"mybackend"` to the `+kubebuilder:validation:Enum` on `ProviderSpec.Name`, then run `make manifests`.

---

## Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `keysmith_rotations_total` | Counter | `policy`, `namespace`, `provider`, `result` | Total rotation attempts |
| `keysmith_rotation_duration_seconds` | Histogram | `policy`, `namespace`, `provider` | Rotation execution latency |
| `keysmith_next_rotation_timestamp` | Gauge | `policy`, `namespace` | Unix timestamp of next scheduled rotation |
| `keysmith_active_policies` | Gauge | — | Count of non-suspended `SecretRotationPolicy` objects |

**Stale-rotation alert example:**

```yaml
groups:
  - name: keysmith
    rules:
      - alert: KeysmithRotationOverdue
        expr: time() - keysmith_next_rotation_timestamp > 3600
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Keysmith policy {{ $labels.policy }}/{{ $labels.namespace }} is overdue"
```

---

## Development

```bash
make test          # unit + envtest integration tests
make lint          # golangci-lint
make lint-fix      # auto-fix lint issues
make fmt           # go fmt
make vet           # go vet

# After editing *_types.go
make manifests generate

# Run against current kubeconfig context
make run
```

### Running tests

Tests use [envtest](https://book.kubebuilder.io/reference/envtest.html) — a real Kubernetes API server + etcd in-process. `make test` auto-downloads the binaries on first run.

```bash
make test
# coverage: 56%+ on controller, 87-100% on providers, 91% on scheduler
```

---

## Project structure

```
api/v1alpha1/
  secretrotationpolicy_types.go   SecretRotationPolicy CRD schema
  rotationrecord_types.go         RotationRecord CRD schema
  zz_generated.deepcopy.go        (auto-generated — do not edit)

cmd/main.go                       Manager entry point; registers controllers + webhooks

config/
  crd/bases/                      Generated CRD manifests (do not edit)
  rbac/                           Generated RBAC manifests (do not edit)
  samples/                        Example custom resources

internal/
  controller/
    secretrotationpolicy_controller.go   Core reconciliation loop
    notifications.go                     Webhook notification dispatch
  metrics/                         Prometheus metric registration
  provider/
    provider.go                    Provider interface + Registry
    mock/                          Deterministic fake provider
    static/                        Crypto-random password provider
    aws/                           AWS Secrets Manager stub
    vault/                         HashiCorp Vault stub
  rotation/                        Cron scheduling utilities
  webhook/                         Admission webhooks (defaulting + validation)
```

---

## License

Apache License 2.0 — see [LICENSE](./LICENSE).
