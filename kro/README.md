# kro ResourceGraphDefinitions

Ready-to-use [kro](https://kro.run) compositions for Snowplane. Each file is a `kubectl apply`-ready ResourceGraphDefinition (RGD) that bundles multiple Snowplane CRDs into a single, self-service custom API.

## Prerequisites

1. **Snowplane** operator running with a valid `ProviderConfig`
2. **kro** installed in your cluster:

```bash
helm install kro oci://registry.k8s.io/kro/charts/kro \
  --namespace kro-system \
  --create-namespace
```

## Available Compositions

| RGD | Custom Kind | Resources Created | Key Feature |
|:----|:------------|:------------------|:------------|
| [snowflake-project.yaml](snowflake-project.yaml) | `SnowflakeProject` | Database, Schema, Warehouse, AccountRole, AccountRoleGrant | Project bootstrap |
| [tagged-table.yaml](tagged-table.yaml) | `TaggedTable` | Table, N × TagAssociation | `forEach` collections |
| [data-pipeline.yaml](data-pipeline.yaml) | `SnowflakePipeline` | Table, StreamOnTable, Task | Conditional resources |
| [rbac-stack.yaml](rbac-stack.yaml) | `SnowflakeRBAC` | AccountRole, N × AccountRoleGrant, AccountRoleAssignment | `forEach` grants |

## Quick Start

```bash
# 1. Install all RGDs
kubectl apply -f kro/

# 2. Verify they are Active
kubectl get rgd -owide

# 3. Create an instance (example: project)
cat <<EOF | kubectl apply -f -
apiVersion: kro.run/v1alpha1
kind: SnowflakeProject
metadata:
  name: analytics
  namespace: data-team
spec:
  name: analytics
  environment: prod
  warehouseSize: MEDIUM
EOF

# 4. Watch all resources come up
kubectl get databases,schemas,warehouses,accountroles,accountrolegrants -n data-team

# 5. Clean up — deletes all managed resources in correct order
kubectl delete snowflakeproject analytics -n data-team
```

## Customisation

These RGDs are designed as starting points. Fork and adapt them for your organisation:

- **Add resources** — append entries to `spec.resources` (e.g., add a MaskingPolicy to the tagged table)
- **Change defaults** — modify `| default=` values in `spec.schema`
- **Add fields** — extend the schema to expose more Snowplane spec fields
- **Restrict scope** — remove resources you don't need

See the [kro documentation](https://kro.run/docs/overview) for the full RGD authoring guide.

## Documentation

Full walkthrough with architecture diagrams: [docs/kro.md](../docs/kro.md)
