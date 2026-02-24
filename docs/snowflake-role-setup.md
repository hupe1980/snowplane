---
layout: default
title: Snowflake Role Setup
parent: Guides
nav_order: 2
description: "Create a least-privilege Snowflake role for the Snowplane operator."
---

# Snowflake Role Setup
{: .fs-8 }

Create a least-privilege Snowflake role for the Snowplane operator. **Never use ACCOUNTADMIN in production.**
{: .fs-5 .fw-300 }

---

## Create a Dedicated Role

```sql
USE ROLE ACCOUNTADMIN;

CREATE ROLE IF NOT EXISTS SNOWPLANE_ROLE
    COMMENT = 'Role for Snowplane Kubernetes operator';
```

## Create a Snowplane User

For key-pair authentication:

```sql
CREATE USER IF NOT EXISTS SNOWPLANE_USER
    DEFAULT_ROLE = SNOWPLANE_ROLE
    MUST_CHANGE_PASSWORD = FALSE
    TYPE = SERVICE
    COMMENT = 'Service user for Snowplane operator';
```

For Workload Identity Federation (WIF):

```sql
CREATE USER IF NOT EXISTS SNOWPLANE_WIF_USER
    DEFAULT_ROLE = SNOWPLANE_ROLE
    TYPE = SERVICE
    COMMENT = 'Service user for Snowplane WIF';
```

## Grant Role to User

```sql
GRANT ROLE SNOWPLANE_ROLE TO USER SNOWPLANE_USER;
```

---

## Grant Privileges per Resource Type

### Account-Level Privileges

```sql
GRANT CREATE DATABASE ON ACCOUNT TO ROLE SNOWPLANE_ROLE;
GRANT CREATE WAREHOUSE ON ACCOUNT TO ROLE SNOWPLANE_ROLE;
GRANT CREATE USER ON ACCOUNT TO ROLE SNOWPLANE_ROLE;
GRANT CREATE ROLE ON ACCOUNT TO ROLE SNOWPLANE_ROLE;
GRANT CREATE NETWORK POLICY ON ACCOUNT TO ROLE SNOWPLANE_ROLE;
GRANT CREATE RESOURCE MONITOR ON ACCOUNT TO ROLE SNOWPLANE_ROLE;
GRANT CREATE INTEGRATION ON ACCOUNT TO ROLE SNOWPLANE_ROLE;
```

{: .warning }
> `MANAGE GRANTS` is a powerful privilege. Only grant it if using the Grant or GrantOwnership CRDs:
> ```sql
> GRANT MANAGE GRANTS ON ACCOUNT TO ROLE SNOWPLANE_ROLE;
> ```

### Database-Level Privileges

For pre-existing databases not managed by Snowplane:

```sql
GRANT CREATE SCHEMA ON DATABASE <database_name> TO ROLE SNOWPLANE_ROLE;
```

### Schema-Level Privileges

```sql
GRANT CREATE TABLE ON SCHEMA <db>.<schema> TO ROLE SNOWPLANE_ROLE;
GRANT CREATE VIEW ON SCHEMA <db>.<schema> TO ROLE SNOWPLANE_ROLE;
GRANT CREATE STAGE ON SCHEMA <db>.<schema> TO ROLE SNOWPLANE_ROLE;
GRANT CREATE TASK ON SCHEMA <db>.<schema> TO ROLE SNOWPLANE_ROLE;
GRANT EXECUTE TASK ON ACCOUNT TO ROLE SNOWPLANE_ROLE;
GRANT CREATE STREAM ON SCHEMA <db>.<schema> TO ROLE SNOWPLANE_ROLE;
GRANT CREATE TAG ON SCHEMA <db>.<schema> TO ROLE SNOWPLANE_ROLE;
GRANT CREATE MASKING POLICY ON SCHEMA <db>.<schema> TO ROLE SNOWPLANE_ROLE;
GRANT CREATE ROW ACCESS POLICY ON SCHEMA <db>.<schema> TO ROLE SNOWPLANE_ROLE;
GRANT CREATE FILE FORMAT ON SCHEMA <db>.<schema> TO ROLE SNOWPLANE_ROLE;
GRANT CREATE PIPE ON SCHEMA <db>.<schema> TO ROLE SNOWPLANE_ROLE;
GRANT CREATE DYNAMIC TABLE ON SCHEMA <db>.<schema> TO ROLE SNOWPLANE_ROLE;
```

---

## Privilege Summary

| Privilege | Required For | Scope |
|:----------|:-------------|:------|
| `CREATE DATABASE ON ACCOUNT` | Database CRD | Account |
| `CREATE WAREHOUSE ON ACCOUNT` | Warehouse CRD | Account |
| `CREATE USER ON ACCOUNT` | User CRD | Account |
| `CREATE ROLE ON ACCOUNT` | AccountRole CRD | Account |
| `MANAGE GRANTS ON ACCOUNT` | Grant / GrantOwnership CRDs | Account (optional) |
| `CREATE SCHEMA ON DATABASE` | Schema CRD | Per-database |
| `CREATE TABLE ON SCHEMA` | Table CRD | Per-schema |
| `CREATE VIEW ON SCHEMA` | View CRD | Per-schema |
| `CREATE STAGE ON SCHEMA` | Stage CRD | Per-schema |
| `CREATE TASK ON SCHEMA` | Task CRD | Per-schema |
| `EXECUTE TASK ON ACCOUNT` | Task CRD (resume) | Account |
| `CREATE STREAM ON SCHEMA` | Stream CRD | Per-schema |
| `CREATE TAG ON SCHEMA` | Tag CRD | Per-schema |
| `CREATE NETWORK POLICY ON ACCOUNT` | NetworkPolicy CRD | Account |
| `CREATE RESOURCE MONITOR ON ACCOUNT` | ResourceMonitor CRD | Account |
| `CREATE MASKING POLICY ON SCHEMA` | MaskingPolicy CRD | Per-schema |
| `CREATE ROW ACCESS POLICY ON SCHEMA` | RowAccessPolicy CRD | Per-schema |
| `CREATE INTEGRATION ON ACCOUNT` | StorageIntegration CRD | Account |
| `CREATE FILE FORMAT ON SCHEMA` | FileFormat CRD | Per-schema |
| `CREATE PIPE ON SCHEMA` | Pipe CRD | Per-schema |
| `CREATE DYNAMIC TABLE ON SCHEMA` | DynamicTable CRD | Per-schema |

---

## RSA Key Setup (Key-Pair Auth)

```bash
openssl genrsa 2048 | openssl pkcs8 -topk8 -inform PEM -out snowplane_key.p8 -nocrypt
openssl rsa -in snowplane_key.p8 -pubout -out snowplane_key.pub
```

```sql
ALTER USER SNOWPLANE_USER SET RSA_PUBLIC_KEY = '<your-public-key>';
```

```bash
kubectl create secret generic snowflake-credentials \
    --from-file=private-key=snowplane_key.p8 \
    -n snowplane-system
```

---

## Workload Identity Federation Setup

For WIF, create a security integration in Snowflake:

```sql
CREATE SECURITY INTEGRATION IF NOT EXISTS SNOWPLANE_WIF_INTEGRATION
    TYPE = EXTERNAL_OAUTH
    ENABLED = TRUE
    EXTERNAL_OAUTH_TYPE = CUSTOM
    EXTERNAL_OAUTH_ISSUER = '<your-oidc-issuer-url>'
    EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM = 'sub'
    EXTERNAL_OAUTH_SNOWFLAKE_USER_MAPPING_ATTRIBUTE = 'LOGIN_NAME'
    EXTERNAL_OAUTH_ANY_ROLE_MODE = 'ENABLE'
    EXTERNAL_OAUTH_AUDIENCE_LIST = ('<your-audience>')
    EXTERNAL_OAUTH_JWS_KEYS_URL = '<your-oidc-jwks-url>';
```

See [Workload Identity Guide]({% link workload-identity.md %}) for cloud-specific setup.

---

## Using `useRole` for Role Switching

The `spec.useRole` field lets you specify a different Snowflake role for `USE ROLE` — without extra ProviderConfigs or MANAGE GRANTS.

### Setup

1. Grant the target role to the service user:

    ```sql
    GRANT ROLE ENGINEER TO USER SNOWPLANE_USER;
    ```

2. Ensure the target role has the necessary privileges:

    ```sql
    GRANT CREATE DATABASE ON ACCOUNT TO ROLE ENGINEER;
    ```

3. Set `useRole` on the resource spec:

    ```yaml
    apiVersion: snowplane.hupe1980.github.io/v1alpha1
    kind: Database
    metadata:
      name: analytics-db
    spec:
      providerRef:
        name: default-pc
      useRole: ENGINEER
      name: ANALYTICS_DB
    ```

{: .important }
> `useRole` is **immutable** after creation. No MANAGE GRANTS needed. The service user must have the target role directly granted.

---

## Security Recommendations

1. **Never use ACCOUNTADMIN** — run with `SNOWPLANE_ROLE` only
2. **Use SERVICE user type** — prevents interactive login
3. **Rotate keys regularly** — Snowplane auto-reconnects on Secret updates
4. **Separate roles per environment** — `SNOWPLANE_ROLE_DEV`, `SNOWPLANE_ROLE_PROD`
5. **Audit regularly** — use `SNOWFLAKE.ACCOUNT_USAGE.QUERY_HISTORY`
6. **Restrict with AllowedNamespaces** — `spec.allowedNamespaces` on ProviderConfig
