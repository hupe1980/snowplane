# tfimport — Terraform → Snowplane Migration Tool

`tfimport` reads a Terraform (or OpenTofu) state file and generates Snowplane Kubernetes CRD manifests, making it easy to migrate Snowflake infrastructure from Terraform to the Snowplane operator.

## Supported Resources

| Terraform Resource | Snowplane CRD |
|--------------------|---------------|
| `snowflake_database` | Database |
| `snowflake_schema` | Schema |
| `snowflake_warehouse` | Warehouse |
| `snowflake_user` | User |
| `snowflake_account_role` / `snowflake_role` | AccountRole |
| `snowflake_database_role` | DatabaseRole |
| `snowflake_grant_privileges_to_account_role` | GrantPrivilegesToAccountRole |
| `snowflake_grant_privileges_to_database_role` | GrantPrivilegesToDatabaseRole |
| `snowflake_grant_privileges_to_share` | GrantPrivilegesToShare |
| `snowflake_table` | Table |
| `snowflake_view` | View |

Unsupported resource types are silently skipped.

## Usage

```bash
# Build
go build -o tfimport ./cmd/tfimport

# Generate manifests from state file
./tfimport -state terraform.tfstate > manifests.yaml

# Apply to cluster
kubectl apply -f manifests.yaml
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-state` | `terraform.tfstate` | Path to the Terraform / OpenTofu state file |
| `-provider` | `default` | Name of the Snowplane `ProviderConfig` to reference |
| `-namespace` | `default` | Kubernetes namespace for generated manifests |
| `-remove-script` | *(none)* | Write a state-removal script to this path (use `-` for stderr) |

### State-Removal Script

After migrating resources to Snowplane, you typically want to remove them
from Terraform state so Terraform no longer manages them. The `--remove-script`
flag generates a Bash script that does this:

```bash
# Generate manifests + removal script in one pass
./tfimport -state terraform.tfstate -remove-script remove-from-state.sh > manifests.yaml

# Review the generated script
cat remove-from-state.sh

# Dry run (echo commands without executing)
bash remove-from-state.sh --dry-run

# Execute the removal
bash remove-from-state.sh
```

The generated script uses `terraform state rm` by default. To use OpenTofu instead:

```bash
TF_CMD=tofu bash remove-from-state.sh
```

The script handles module-prefixed resources, `count` indices, and `for_each` keys
automatically.

## Examples

### Basic Migration

```bash
./tfimport -state prod.tfstate -namespace snowflake -provider prod > manifests.yaml
kubectl apply -f manifests.yaml
```

### Full Migration Workflow

```bash
# 1. Generate CRD manifests and removal script
./tfimport \
  -state terraform.tfstate \
  -namespace snowflake \
  -provider prod \
  -remove-script remove.sh \
  > manifests.yaml

# 2. Review generated manifests
less manifests.yaml

# 3. Apply manifests (resources will be adopted, not recreated)
kubectl apply -f manifests.yaml

# 4. Verify resources are synced
kubectl get databases,schemas,warehouses -n snowflake

# 5. Dry-run state removal
bash remove.sh --dry-run

# 6. Remove from Terraform state
bash remove.sh
```

## Running Tests

```bash
go test -v ./cmd/tfimport/
```
