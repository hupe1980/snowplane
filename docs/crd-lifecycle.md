---
layout: default
title: CRD Lifecycle
parent: Concepts
nav_order: 3
description: "Maturity lifecycle, graduation criteria, deprecation policy, and version management for Snowplane CRDs."
---

# CRD Lifecycle
{: .fs-8 }

Maturity lifecycle, graduation criteria, and version management for Snowplane Custom Resource Definitions.
{: .fs-5 .fw-300 }

---

## Maturity Levels

Every Snowplane CRD carries a maturity label (`snowplane.hupe1980.github.io/maturity`):

| Level | API Version | Stability | Gating |
|:------|:------------|:----------|:-------|
| **alpha** | `v1alpha1` | None — API may change or be removed | `--enable-alpha-resources` (default: `true`) |
| **beta** | `v1beta1` | Backwards-compatible changes only | Always enabled |
| **stable** | `v1` | Full backwards-compatibility | Always enabled |

---

## Current Classification

| CRD | Maturity | Notes |
|:----|:---------|:------|
| Database | alpha | Full lifecycle, adoption, drift detection |
| Schema | alpha | Parent ref resolution, cascading delete |
| Warehouse | alpha | Account-level resource, adoption support |
| User | alpha | Secret-referenced credentials |
| AccountRole | alpha | Simplest resource type |
| DatabaseRole | alpha | Two-part identifier, parent dependency |
| GrantPrivilegesToAccountRole | alpha | Grant/Revoke (no ALTER) |
| GrantPrivilegesToDatabaseRole | alpha | Grant/Revoke (no ALTER) |
| AccountRoleAssignment | alpha | GRANT ROLE TO ROLE/USER (immutable, no ALTER) |
| DatabaseRoleAssignment | alpha | GRANT DATABASE ROLE TO ROLE/DATABASE ROLE (immutable, no ALTER) |
| GrantPrivilegesToShare | alpha | Grant/Revoke (no ALTER) |
| GrantOwnership | alpha | Ownership transfer, no-op on delete |
| Table | alpha | Column management, schema-level |
| View | alpha | AS query management, secure views |
| MaterializedView | alpha | Materialized query results, secure, cluster by |
| InternalStage | alpha | Internal stages, file format |
| ExternalStage | alpha | External stages (S3, GCS, Azure) |
| Task | alpha | Scheduled SQL, DAG, serverless |
| Alert | alpha | Condition-based monitoring & notification |
| StreamOnTable | alpha | CDC on tables |
| StreamOnView | alpha | CDC on views |
| StreamOnExternalTable | alpha | CDC on external tables |
| StreamOnDirectoryTable | alpha | CDC on stage directory tables |
| StreamOnDynamicTable | alpha | CDC on dynamic tables |
| Tag | alpha | Data governance with allowed values |
| NetworkPolicy | alpha | IP allow/block lists |
| ResourceMonitor | alpha | Credit quota monitoring |
| MaskingPolicy | alpha | Dynamic masking for PII/PCI |
| RowAccessPolicy | alpha | Row-level security |
| StorageIntegrationAWS | alpha | S3 storage integration |
| StorageIntegrationGCS | alpha | GCS storage integration |
| StorageIntegrationAzure | alpha | Azure storage integration |
| ExternalOAuthIntegration | alpha | External OAuth integration |
| SAML2Integration | alpha | SAML2 integration |
| SCIMIntegration | alpha | SCIM provisioning integration |
| EmailNotificationIntegration | alpha | Email notification integration |
| QueueNotificationIntegration | alpha | Queue notification integration (AWS SNS, Azure Event Grid, GCP Pub/Sub) |
| WebhookNotificationIntegration | alpha | Webhook notification integration |
| PasswordPolicy | alpha | Password compliance rules |
| NetworkRule | alpha | Network identifier groups |
| FileFormat | alpha | CSV, JSON, Parquet, etc. |
| Pipe | alpha | Snowpipe continuous loading |
| DynamicTable | alpha | Auto-refresh materialisation |
| FieldExport | alpha | ConfigMap/Secret export |
| Share | alpha | Outbound data sharing, account management |
| ExternalFunction | alpha | HTTPS external functions, create/drop only |
| ComputePool | alpha | SPCS compute infrastructure, immutable instanceFamily |
| Service | alpha | SPCS container workloads, schema-scoped |
| ImageRepository | alpha | SPCS container image registry, create/drop only |
| ProviderConfig | **stable** | Core infrastructure, always required |

---

## Graduation Criteria

### Alpha → Beta

1. **API Stability** — No breaking spec changes for 2+ minor releases
2. **Test Coverage** — Integration tests for all lifecycle operations
3. **Documentation** — Complete user-facing docs with examples
4. **Production Usage** — At least one production deployment
5. **Validation** — CEL rules complete
6. **Error Handling** — All error paths produce meaningful conditions

### Beta → Stable

1. **API Freeze** — No schema changes for 3+ minor releases (additive OK)
2. **Conversion Webhook** — `v1beta1` → `v1` transparency
3. **Migration Path** — Documented migration guide
4. **Performance** — Validated under load (100+ resources)
5. **Multi-tenant** — Tested with multiple ProviderConfigs
6. **Observability** — Complete metrics, events, and conditions

---

## Deprecation Policy

### Version Overlap

- **Minimum 2 minor releases** of overlap between deprecated and replacement versions
- Both versions are served and stored during overlap
- Deprecated version emits a warning header

### Timeline

```
Release N     — v1beta1 introduced alongside v1alpha1
Release N+1   — v1alpha1 deprecated (warning header)
Release N+2   — v1alpha1 removed from served versions
Release N+3   — v1alpha1 storage version migrated
```

---

## Conversion Webhooks

When a CRD has multiple served API versions:

- **Hub-and-spoke model**: Latest stable version is the "hub"
- All other versions convert to/from the hub
- Round-trip fuzz tests validate no data loss

```go
func (src *Database) ConvertTo(dstRaw conversion.Hub) error {
    dst := dstRaw.(*v1beta1.Database)
    // Copy common fields...
    return nil
}
```

---

## Controller Gating

```bash
# Disable alpha resources
manager --enable-alpha-resources=false

# Disable specific controllers
manager --disable-controllers=grantprivilegestoaccountrole,internalstage,view
```

When disabled:
- Controllers log a skip message and do not register
- Existing CRD instances remain in their last-known state
- CRD definitions remain in the cluster

---

## Version Transition Checklist

- [ ] Update maturity label in CRD YAML
- [ ] Add new API version directory
- [ ] Implement conversion webhook (hub-and-spoke)
- [ ] Add round-trip fuzz tests
- [ ] Update `WithMaturity()` in controller registration
- [ ] Add migration guide to release notes
