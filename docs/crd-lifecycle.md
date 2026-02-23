# CRD Lifecycle and Maturity Classification

This document describes the maturity lifecycle for Snowplane Custom Resource Definitions (CRDs), including graduation criteria, deprecation policy, and version management strategy.

## Maturity Levels

Every Snowplane CRD carries a maturity label (`LabelMaturity` constant, key `snowplane.hupe1980.github.io/maturity`) on its YAML metadata:

    metadata:
      labels:
        snowplane.hupe1980.github.io/maturity: alpha

| Level | API Version | Stability Guarantees | Gating |
|-------|-------------|---------------------|--------|
| **alpha** | `v1alpha1` | None — API may change or be removed without notice | Requires `--enable-alpha-resources` flag (default: `true`) |
| **beta** | `v1beta1` | Backwards-compatible changes only; schema may add fields | Always enabled |
| **stable** | `v1` | Full backwards-compatibility; no breaking changes | Always enabled |

## Current Classification

| CRD | Maturity | Notes |
|-----|----------|-------|
| Database | alpha | Full lifecycle, adoption, drift detection |
| Schema | alpha | Parent ref resolution, cascading delete |
| Warehouse | alpha | Account-level resource, adoption support |
| User | alpha | Secret-referenced credentials |
| AccountRole | alpha | Simplest resource type |
| DatabaseRole | alpha | Two-part identifier, parent dependency |
| AccountRoleGrant | alpha | Grant/Revoke to account roles (no ALTER) |
| DatabaseRoleGrant | alpha | Grant/Revoke to database roles (no ALTER) |
| ShareGrant | alpha | Grant/Revoke to shares (no ALTER) |
| Table | alpha | Column management, schema-level resource |
| View | alpha | AS query management, secure views |
| Stage | alpha | Internal/external stages, file format |
| ProviderConfig | stable | Core infrastructure, always required |

## Graduation Criteria

### Alpha to Beta

A CRD graduates from alpha to beta when **all** of the following are met:

1. **API Stability** — No breaking spec changes for at least 2 minor releases.
2. **Test Coverage** — Integration tests cover create, update, delete, drift detection, adoption, and edge cases.
3. **Documentation** — Complete user-facing documentation with examples.
4. **Production Usage** — At least one production deployment has validated the resource.
5. **Validation** — CEL validation rules and admission webhooks are complete.
6. **Error Handling** — All error paths produce meaningful conditions and events.

### Beta to Stable

A CRD graduates from beta to stable when **all** of the following are met:

1. **API Freeze** — No schema changes for at least 3 minor releases (additive fields allowed).
2. **Conversion Webhook** — A conversion webhook handles `v1beta1` → `v1` transparently.
3. **Migration Path** — Documented migration guide from beta to stable.
4. **Performance** — Reconciliation performance validated under load (100+ resources).
5. **Multi-tenant** — Tested with multiple ProviderConfigs and namespaces.
6. **Observability** — Metrics, events, and conditions provide complete operational visibility.

## Deprecation Policy

Snowplane follows a conservative deprecation policy aligned with Kubernetes conventions:

### Version Overlap

- **Minimum 2 minor releases** of overlap between deprecated and replacement API versions.
- During the overlap period, both versions are served and stored.
- The deprecated version emits a warning header on API responses.

### Deprecation Timeline

    Release N     — v1beta1 introduced alongside v1alpha1
    Release N+1   — v1alpha1 deprecated (warning header)
    Release N+2   — v1alpha1 removed from served versions
    Release N+3   — v1alpha1 storage version migrated (if any resources remain)

### Communication

- Deprecation announced in release notes and CHANGELOG.
- `kubectl get <resource>` shows a warning when using a deprecated API version.
- Controller logs emit a warning on first reconciliation of a deprecated-version resource.

## Conversion Webhooks

When a CRD has multiple served API versions, a conversion webhook translates between them:

### Strategy

- **Hub-and-spoke model**: The latest stable version is the "hub"; all other versions convert to/from the hub.
- Conversion logic is generated from structural schema diffs where possible.
- Manual conversion code is written for semantic changes (field renames, type changes).

### Implementation

    // ConvertTo converts this Database (v1alpha1) to the hub version (v1beta1).
    func (src *Database) ConvertTo(dstRaw conversion.Hub) error {
        dst := dstRaw.(*v1beta1.Database)
        // Copy common fields...
        return nil
    }

    // ConvertFrom converts from the hub version to this Database (v1alpha1).
    func (dst *Database) ConvertFrom(srcRaw conversion.Hub) error {
        src := srcRaw.(*v1beta1.Database)
        // Copy common fields...
        return nil
    }

### Testing

- Round-trip fuzz tests validate that `ConvertTo → ConvertFrom` produces the original object.
- Property-based tests ensure no data loss during conversion.

## Storage Version Management

### Single Storage Version

At any point, only one API version is the storage version. The storage version determines the format persisted in etcd.

### Migration Strategy

1. **Before promotion**: Ensure all clusters have the conversion webhook deployed.
2. **Promote storage version**: Update CRD to mark the new version as `storage: true`.
3. **Migrate existing objects**: Run `kubectl get <resource> --all-namespaces -o yaml | kubectl apply -f -` or use the [Storage Version Migrator](https://github.com/kubernetes-sigs/kube-storage-version-migrator).
4. **Remove old version**: After migration, remove the old version from `spec.versions`.

### Safety Checks

- The operator validates at startup that the storage version matches expectations.
- If a storage version mismatch is detected, the operator logs an error and refuses to start until manual intervention.

## Controller Gating

The `--enable-alpha-resources` flag (default: `true`) gates alpha-maturity controllers:

    # Disable alpha resources (only stable controllers run)
    manager --enable-alpha-resources=false

When disabled:
- Alpha controllers log a skip message and do not register with the manager.
- Existing alpha CRD instances are not reconciled (they remain in their last-known state).
- The CRD definitions remain in the cluster (they are not removed).

When re-enabled:
- Alpha controllers resume reconciliation on the next manager restart.
- Resources are reconciled from their current state (no data loss).

## Version Transition Checklist

When promoting a CRD from one maturity level to the next, follow this checklist:

- [ ] Update `snowplane.hupe1980.github.io/maturity` label in CRD YAML
- [ ] Add new API version directory (e.g., `api/v1beta1/`)
- [ ] Implement conversion webhook (hub-and-spoke)
- [ ] Add round-trip fuzz tests for conversion
- [ ] Update `WithMaturity()` call in controller registration
- [ ] Update controller flag documentation
- [ ] Add migration guide to release notes
- [ ] Update this document with new classification
