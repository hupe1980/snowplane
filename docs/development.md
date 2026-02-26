---
layout: default
title: Development Guide
parent: Development
nav_order: 1
description: "Project layout, architecture principles, conventions, and workflow for contributing to Snowplane."
---

# Development Guide
{: .fs-8 }

Project layout, architecture principles, conventions, and workflow for contributing to Snowplane.
{: .fs-5 .fw-300 }

---

## Prerequisites

| Tool | Min Version |
|:-----|:------------|
| Go | 1.25+ |
| kubectl | 1.27+ |
| [just](https://github.com/casey/just) | 1.0+ |
| golangci-lint | 2.0+ |

---

## Project Layout

```
.
├── api/v1alpha1/              # CRD type definitions
│   ├── common_types.go        # Shared types (CommonSpec, CommonStatus)
│   ├── conditions.go          # Condition type & reason constants
│   ├── annotations.go         # Annotation & label constants
│   ├── validation.go          # Validate() methods (errors.Join)
│   ├── *_types.go             # Per-resource spec, status, show output
│   ├── zz_generated.deepcopy.go    # Auto-generated DeepCopy
│   └── zz_generated_accessors.go   # Auto-generated ManagedResource accessors
├── cmd/manager/               # Controller manager entrypoint
├── config/
│   ├── crd/bases/             # CRD YAML manifests (31 types)
│   ├── manager/               # Deployment, PDB, NetworkPolicy
│   ├── rbac/                  # RBAC roles and bindings
│   └── samples/               # Example CR YAML files
├── internal/
│   ├── clients/
│   │   ├── clientfactory/     # Snowflake client cache with LRU eviction & singleflight dedup
│   │   └── snowflake/         # Snowflake SDK wrapper
│   ├── controller/
│   │   ├── reconciler/        # Generic reconciler framework
│   │   └── <resource>/        # 29 resource-specific controllers
│   ├── drift/                 # Generic field-level drift engine
│   ├── metrics/               # Custom Prometheus metrics
│   ├── provider/              # Config builder & hash
│   ├── ratelimit/             # Per-provider token-bucket rate limiter
│   ├── circuitbreaker/        # Per-provider circuit breaker
│   ├── sfretry/               # Retry wrapper for transient errors
│   └── utils/                 # Sanitization, conditions, finalizers
├── test/
│   ├── e2e/                   # E2E tests (k3s + real Snowflake)
│   └── integration/           # envtest integration tests
└── docs/                      # Documentation (this site)
```

---

## Day-to-day Workflow

```bash
just generate          # Regenerate DeepCopy + CRD manifests + accessors
just test              # Run tests (with race detection)
just test-integration  # Integration tests (requires setup-envtest)
just lint              # Run linter
just vet               # Run go vet
just build             # Build binary
just ci                # Full pipeline (lint + vet + test + build)
just fuzz              # Run fuzz tests (default: 10s)
```

### Code Generation

After modifying any type in `api/v1alpha1/`:

```bash
just generate     # DeepCopy + CRD YAMLs + ManagedResource accessors
just sync-crds    # Copy CRDs into Helm chart
just verify-crds  # CI check: fails if out-of-sync
```

**Generated files (never edit manually):**

| File | Generator | Purpose |
|:-----|:----------|:--------|
| `zz_generated.deepcopy.go` | `controller-gen` | DeepCopy methods |
| `zz_generated_accessors.go` | `hack/gen-accessors` | 458 ManagedResource methods across 29 types |
| `config/crd/bases/*.yaml` | `controller-gen` | CRD manifests with CEL rules |

---

## Architecture Principles

### Observe-Diff-Apply

Every reconciler follows the same lifecycle:

1. **Observe** — Query Snowflake for the current state
2. **Diff** — Compare observed state with desired spec
3. **Apply** — Execute only the SQL statements needed

This minimises Snowflake API calls and avoids unnecessary mutations.

### Generic Reconciler Framework

All 29 resource reconcilers share the same state machine via `internal/controller/reconciler/`:

- **`GenericReconciler[T, S, D]`** — Type-parameterised reconciler handling finalizers, ProviderConfig resolution, client caching, SSA status patching, conditions, rate limiting, retry, metrics, and drift detection
- **`ResourceAdapter[T, S, D]`** — Interface each resource implements for resource-specific behaviour
- **`Observation[D]`** — Typed observation struct (`Exists bool`, `Detail D`) with compile-time safety
- **`ManagedResource`** — Constraint interface with 16 code-generated accessor methods

Each resource package provides:
1. **`adapter.go`** — Implements `ResourceAdapter` with resource-specific logic
2. **`reconciler.go`** — Service interface, constructor, helper functions

This reduces each reconciler from ~800 lines to ~200-400 lines of resource-specific code.

### Drift Correction

The reconciler computes diffs on every loop — not just when `metadata.generation` changes. Out-of-band changes in Snowflake are automatically corrected.

The generic drift engine (`internal/drift/`) provides a fluent builder:

```go
result := drift.New().
    CompareString("COMMENT", spec.Comment, obs.Comment, false).
    CompareInt32("RETENTION", spec.Retention, obs.Retention, false).
    Result()
```

### Cross-Resource Reference Resolution

Schema-level resources reference parents via `databaseRef`. The `ReferenceResolver` in `internal/controller/refresolver/` handles lookup, Ready checks, and watch-based requeue.

```go
dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, obj,
    obj.Namespace, obj.Spec.DatabaseRef, obj.Spec.DatabaseName, obj.Status.DatabaseName)
```

### Interface-Driven Testing

All Snowflake operations are behind interfaces. Unit tests inject mocks via `ServiceFactory`.

---

## Advanced Patterns

### UNSET Support & TrackedParameters

Pointer fields use nil-means-unmanaged. When a user removes a field, `computeUnsetFields()` compares spec against `status.trackedParameters` and generates `ALTER ... UNSET` statements.

### Recoverable vs Terminal Conditions

| Condition | Meaning | User Action |
|:----------|:--------|:------------|
| `Recoverable=True` | Transient error (timeout, rate limit) | None — auto-retry |
| `Terminal=True` | Invalid config, immutable field violation | Fix the spec |

### Deletion Resilience

- **Schema → Database deleted:** Reconciler falls back to `status.databaseName`
- **Any → ProviderConfig deleted:** Finalizer removed, Kubernetes garbage-collects
- **ProviderConfig in-use guard:** Blocks deletion while resources reference it

### Defense-in-Depth Validation

Every reconciler calls `spec.Validate()` before resolving the client — a safety net complementing CEL validation.

### Rate Limiting

Per-provider `rate.Limiter` from `golang.org/x/time/rate`. Configurable via `--rate-limit-qps` and `--rate-limit-burst`.

### Retry for Transient Errors

`sfretry.Do()` retries up to 3 times with 2s backoff. Classifies errors as retryable (timeouts, connections) or terminal (permissions, SQL compilation). All retried operations are idempotent.

### Circuit Breaker

Per-provider failure isolation. Opens after 5 consecutive failures, probes after 60s. Resources using other ProviderConfigs are unaffected.

### CEL Validation Rules

All 32 CRD types include `x-kubernetes-validations` rules — evaluated server-side on UPDATE, no webhook required. Covers immutable fields, schema defaults, policy body blocklists, mutual exclusion, and auth validation.

### Resource Adoption

Adopt pre-existing Snowflake resources via annotation:

| Annotation Value | Behaviour |
|:-----------------|:----------|
| *(absent)* / `fail-if-exists` | Terminal error — prevents accidental takeover |
| `adopt` | Populates status from current state, sets `LateInitialized` |

### Terraform Migration Tool

`cmd/tfimport` reads Terraform state and generates Kubernetes manifests with `adopt` annotation:

```bash
./tfimport -state terraform.tfstate -namespace snowflake > manifests.yaml
```

---

## Testing Strategy

| Layer | What to Test | Tool |
|:------|:-------------|:-----|
| Snowflake client | SQL generation, options, errors | `testing` + `testify` |
| Reconciler | Full loop, conditions, status, refs | `testing` + fake `client.Client` |
| Fuzz | Denylist validators, escaping functions | Go built-in `testing.F` |
| CEL validation | Immutable fields, defaults, blocklists | envtest |
| Integration | Full CRD → reconciler pipeline | envtest (59 tests, 11 resources) |
| E2E | Real Snowflake + k3s testcontainer | 27 tests |

### Integration Tests

```bash
# Install setup-envtest (one-time)
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
setup-envtest use

# Run
just test-integration
```

### E2E Tests

```bash
# Requires .env with SNOWFLAKE_ACCOUNT, SNOWFLAKE_USER, etc.
just test-e2e
```

---

## Code Style

- `golangci-lint` enforces style (`.golangci.yml`) — includes `gosec` (security), `errorlint` (error wrapping), `gofmt`, `goimports`
- All exported functions have doc comments
- All tests are parallelised (`t.Parallel()`)
- Constants from `api/v1alpha1/conditions.go` — no magic strings
