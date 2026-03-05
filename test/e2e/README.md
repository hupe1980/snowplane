# Snowplane E2E Tests

End-to-end tests that deploy the Snowplane operator into a self-contained
**k3s testcontainer** and test against a real Snowflake account.

The test harness (`TestMain`) automatically:

1. Builds the operator Docker image
2. Starts a k3s container via [testcontainers-go](https://golang.testcontainers.org/modules/k3s/)
3. Loads the image, installs CRDs, and deploys the Helm chart
4. Creates a ProviderConfig + Snowflake credentials Secret
5. Runs all E2E tests
6. Tears everything down (container auto-terminates)

No external `kind` cluster or setup scripts required.

## Prerequisites

| Tool    | Minimum Version | Install                                  |
|---------|-----------------|------------------------------------------|
| Go      | 1.25            | https://go.dev/dl/                       |
| Docker  | 24+             | https://docs.docker.com/install/         |
| kubectl | 1.30+           | https://kubernetes.io/docs/tasks/tools/  |
| Helm    | 3.x             | https://helm.sh/docs/intro/install/      |

> **Note:** `kind` is no longer required for automated E2E tests. A manual
> kind-based setup is available in `hack/setup-e2e-kind.sh` for interactive
> testing.

## Snowflake Configuration

Export the following environment variables (or `source .env`):

```bash
export SNOWFLAKE_ACCOUNT="<your-account-identifier>"
export SNOWFLAKE_USER="<your-user>"
# One of:
export SNOWFLAKE_PASSWORD="<your-password>"
export SNOWFLAKE_PRIVATE_KEY="<PEM-encoded-private-key>"

export SNOWFLAKE_ROLE="SYSADMIN"        # optional, default SYSADMIN
export SNOWFLAKE_WAREHOUSE="COMPUTE_WH" # optional
```

## Running

```bash
# Via just (recommended)
just test-e2e

# Or directly
source .env
go test -tags e2e -v -timeout 20m -count=1 ./test/e2e/
```

## Manual Kind-Based Testing

For interactive / ad-hoc testing you can still use the kind-based scripts:

```bash
just e2e-setup-kind    # hack/setup-e2e-kind.sh
# ... iterate manually ...
just e2e-teardown-kind # hack/teardown-e2e-kind.sh
```

## Test Matrix

| File                     | Coverage                                     |
|--------------------------|----------------------------------------------|
| `database_test.go`       | Database lifecycle, drift, orphan delete      |
| `schema_test.go`         | Schema lifecycle (depends on database)        |
| `warehouse_test.go`      | Warehouse lifecycle, drift, orphan            |
| `user_test.go`           | User lifecycle                                |
| `role_test.go`           | AccountRole + DatabaseRole lifecycle          |
| `table_test.go`          | Table lifecycle (database + schema deps)      |
| `view_test.go`           | View lifecycle (database + schema + table)    |
| `stage_test.go`          | Stage lifecycle (database + schema deps)      |
| `cross_resource_test.go` | Database -> Schema -> DatabaseRole chain      |
| `fieldexport_test.go`    | FieldExport -> ConfigMap / Secret             |
| `adoption_test.go`       | Pre-create in SF -> adopt via annotation      |
| `grant_test.go`          | AccountRole / DatabaseRole / future grants    |
| `policy_test.go`         | NetworkPolicy, PasswordPolicy, MaskingPolicy  |
| `task_test.go`           | Task lifecycle (database + schema deps)       |
| `stream_test.go`         | Stream lifecycle (database + schema + table)  |
| `apiintegration_test.go` | APIIntegration lifecycle, orphan delete        |
