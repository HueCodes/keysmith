# keysmith

**Automated secret rotation for Kubernetes — schedule it, forget it, audit it.**

Keysmith is a Kubernetes operator that reconciles `SecretRotationPolicy` custom resources to automatically rotate secrets on a configurable schedule. It fetches fresh credentials from a pluggable provider backend (AWS Secrets Manager, HashiCorp Vault, or a built-in static generator), writes them to a Kubernetes `Secret`, optionally triggers rolling restarts of dependent workloads, and creates an immutable `RotationRecord` audit log entry for every attempt.

---

