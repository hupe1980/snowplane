# Snowflake Role Setup Guide

This guide explains how to create a least-privilege Snowflake role for the Snowplane operator. **Never use ACCOUNTADMIN in production.**

## Overview

Snowplane needs a dedicated Snowflake role with minimal privileges to manage resources declaratively. The operator should run with only the permissions it needs — no more.

## Step 1: Create a Dedicated Role

    USE ROLE ACCOUNTADMIN;

    CREATE ROLE IF NOT EXISTS SNOWPLANE_ROLE
        COMMENT = 'Role for Snowplane Kubernetes operator';

## Step 2: Create a Snowplane User

For key-pair authentication:

    CREATE USER IF NOT EXISTS SNOWPLANE_USER
        DEFAULT_ROLE = SNOWPLANE_ROLE
        MUST_CHANGE_PASSWORD = FALSE
        TYPE = SERVICE
        COMMENT = 'Service user for Snowplane operator';

For Workload Identity Federation (WIF), create a SERVICE-type user:

    CREATE USER IF NOT EXISTS SNOWPLANE_WIF_USER
        DEFAULT_ROLE = SNOWPLANE_ROLE
        TYPE = SERVICE
        COMMENT = 'Service user for Snowplane WIF';

## Step 3: Grant Role to User

    GRANT ROLE SNOWPLANE_ROLE TO USER SNOWPLANE_USER;

## Step 4: Grant Privileges per Resource Type

### Database Management

    GRANT CREATE DATABASE ON ACCOUNT TO ROLE SNOWPLANE_ROLE;

This allows the operator to create, alter, and drop databases. Ownership of created databases transfers to `SNOWPLANE_ROLE` automatically.

### Warehouse Management

    GRANT CREATE WAREHOUSE ON ACCOUNT TO ROLE SNOWPLANE_ROLE;

### User Management

    GRANT CREATE USER ON ACCOUNT TO ROLE SNOWPLANE_ROLE;

### Role Management

    GRANT CREATE ROLE ON ACCOUNT TO ROLE SNOWPLANE_ROLE;

### Grant Management

For the operator to manage grants (assign privileges and roles to other roles/users):

    GRANT MANAGE GRANTS ON ACCOUNT TO ROLE SNOWPLANE_ROLE;

**Warning:** `MANAGE GRANTS` is a powerful privilege. If your use case doesn't require the Grant CRD, omit this.

### Schema Management

Schema creation requires ownership of or `CREATE SCHEMA` privilege on the parent database. If the operator created the database, it already owns it. For pre-existing databases:

    GRANT CREATE SCHEMA ON DATABASE <database_name> TO ROLE SNOWPLANE_ROLE;

### Table and View Management

Table and view creation requires schema-level privileges:

    GRANT CREATE TABLE ON SCHEMA <database_name>.<schema_name> TO ROLE SNOWPLANE_ROLE;
    GRANT CREATE VIEW ON SCHEMA <database_name>.<schema_name> TO ROLE SNOWPLANE_ROLE;

### Stage Management

    GRANT CREATE STAGE ON SCHEMA <database_name>.<schema_name> TO ROLE SNOWPLANE_ROLE;

## Step 5: Set RSA Public Key (Key-Pair Auth)

    ALTER USER SNOWPLANE_USER SET RSA_PUBLIC_KEY = '<your-public-key>';

Generate a key pair:

    openssl genrsa 2048 | openssl pkcs8 -topk8 -inform PEM -out snowplane_key.p8 -nocrypt
    openssl rsa -in snowplane_key.p8 -pubout -out snowplane_key.pub

Store the private key in a Kubernetes Secret:

    kubectl create secret generic snowflake-credentials \
        --from-file=private-key=snowplane_key.p8 \
        -n snowplane-system

## Step 6: Workload Identity Federation Setup (Optional)

For WIF, create a security integration in Snowflake:

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

See [Workload Identity Guide](workload-identity.md) for cloud-specific setup.

## Summary Table

| Privilege | Required For | Optional? |
|-----------|-------------|-----------|
| `CREATE DATABASE ON ACCOUNT` | Database CRD | No (if using Database) |
| `CREATE WAREHOUSE ON ACCOUNT` | Warehouse CRD | No (if using Warehouse) |
| `CREATE USER ON ACCOUNT` | User CRD | No (if using User) |
| `CREATE ROLE ON ACCOUNT` | AccountRole CRD | No (if using AccountRole) |
| `MANAGE GRANTS ON ACCOUNT` | Grant CRD | Yes — powerful, only if needed |
| `CREATE SCHEMA ON DATABASE` | Schema CRD | Per-database |
| `CREATE TABLE ON SCHEMA` | Table CRD | Per-schema |
| `CREATE VIEW ON SCHEMA` | View CRD | Per-schema |
| `CREATE STAGE ON SCHEMA` | Stage CRD | Per-schema |

## Using `useRole` for Role Switching

By default, resources created by Snowplane use the role configured in the ProviderConfig (e.g. `SNOWPLANE_ROLE`). The `spec.useRole` field lets you specify a different Snowflake role for `USE ROLE` — without needing extra ProviderConfigs or MANAGE GRANTS. In Snowflake's DAC model, the role active at CREATE time becomes the object owner.

### How It Works

Under the hood, Snowplane executes `USE ROLE <useRole>` before creating the resource. In Snowflake, the active role at creation time becomes the owner. This is the same pattern that Terraform's multi-provider-alias solves — but with a single ProviderConfig.

### Setup

1. **Grant the target role to the service user** (not role-to-role, it must be user-level):

        GRANT ROLE ENGINEER TO USER SNOWPLANE_USER;

2. **Ensure the target role has the necessary privileges** to create the resource type:

        -- Example: let ENGINEER create databases
        GRANT CREATE DATABASE ON ACCOUNT TO ROLE ENGINEER;

3. **Set `useRole` on the resource spec**:

    ```yaml
    apiVersion: snowplane.io/v1alpha1
    kind: Database
    metadata:
      name: analytics-db
    spec:
      providerRef:
        name: default-pc  # connects as SYSADMIN / SNOWPLANE_ROLE
      useRole: ENGINEER  # database will be owned by ENGINEER
      name: ANALYTICS_DB
    ```

### Important Notes

- `useRole` is **immutable** after creation — it cannot be changed (ownership transfer is not supported).
- **No MANAGE GRANTS needed** — the operator uses `USE ROLE`, not `GRANT OWNERSHIP`.
- The service user must have the target role **directly granted** (`GRANT ROLE X TO USER Y`).
- If the role is not granted, the operator reports a terminal error with the exact `GRANT ROLE` command needed.
- `useRole` is optional — omit it to use the ProviderConfig's default role.

## Security Recommendations

1. **Never use ACCOUNTADMIN** — the operator should run with `SNOWPLANE_ROLE` only.
2. **Use SERVICE user type** — prevents interactive login and password requirements.
3. **Rotate keys regularly** — Snowplane supports key rotation via Secret updates (the operator auto-reconnects).
4. **Use separate roles per environment** — create `SNOWPLANE_ROLE_DEV`, `SNOWPLANE_ROLE_PROD` with different privileges.
5. **Audit regularly** — use `SNOWFLAKE.ACCOUNT_USAGE.QUERY_HISTORY` to monitor operator activity.
6. **Restrict with AllowedNamespaces** — use `spec.allowedNamespaces` on ProviderConfig to limit which Kubernetes namespaces can use each Snowflake account.
