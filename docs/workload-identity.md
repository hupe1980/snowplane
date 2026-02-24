---
layout: default
title: Workload Identity
parent: Guides
nav_order: 3
description: "Configure passwordless authentication to Snowflake using Workload Identity Federation."
---

# Workload Identity Federation
{: .fs-8 }

Passwordless authentication to Snowflake — no passwords, private keys, or static credentials stored anywhere.
{: .fs-5 .fw-300 }

---

## How It Works

1. Kubernetes projects a short-lived OIDC token from the cluster's ServiceAccount into the operator pod
2. The gosnowflake driver reads the token from the projected volume path on each new connection
3. The driver performs native WIF attestation exchange with Snowflake
4. Snowflake validates the token against the configured OIDC issuer
5. Kubelet automatically refreshes the token before expiry

---

## Prerequisites

- Kubernetes cluster with OIDC issuer (EKS, GKE, AKS have this by default)
- Snowflake account with External OAuth support
- A SERVICE-type user in Snowflake
- Access to create Security Integrations

---

## Create a SERVICE User

```sql
USE ROLE ACCOUNTADMIN;

CREATE USER IF NOT EXISTS SVC_SNOWPLANE
    DEFAULT_ROLE = SNOWPLANE_ROLE
    TYPE = SERVICE
    COMMENT = 'Snowplane WIF service user';

GRANT ROLE SNOWPLANE_ROLE TO USER SVC_SNOWPLANE;
```

See [Snowflake Role Setup]({% link snowflake-role-setup.md %}) for configuring role privileges.

---

## Create a Security Integration

### EKS (IRSA)

```bash
aws eks describe-cluster --name <cluster-name> \
  --query "cluster.identity.oidc.issuer" --output text
```

```sql
CREATE SECURITY INTEGRATION IF NOT EXISTS SNOWPLANE_EKS_WIF
    TYPE = EXTERNAL_OAUTH
    ENABLED = TRUE
    EXTERNAL_OAUTH_TYPE = CUSTOM
    EXTERNAL_OAUTH_ISSUER = 'https://oidc.eks.us-east-1.amazonaws.com/id/EXAMPLE1234567890'
    EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM = 'sub'
    EXTERNAL_OAUTH_SNOWFLAKE_USER_MAPPING_ATTRIBUTE = 'LOGIN_NAME'
    EXTERNAL_OAUTH_ANY_ROLE_MODE = 'ENABLE'
    EXTERNAL_OAUTH_AUDIENCE_LIST = ('https://<orgname>-<accountname>.snowflakecomputing.com')
    EXTERNAL_OAUTH_JWS_KEYS_URL = 'https://oidc.eks.us-east-1.amazonaws.com/id/EXAMPLE1234567890/.well-known/openid-configuration';
```

### GKE (Workload Identity)

```bash
gcloud container clusters describe <cluster-name> --zone <zone> \
  --format="value(selfLink)"
```

```sql
CREATE SECURITY INTEGRATION IF NOT EXISTS SNOWPLANE_GKE_WIF
    TYPE = EXTERNAL_OAUTH
    ENABLED = TRUE
    EXTERNAL_OAUTH_TYPE = CUSTOM
    EXTERNAL_OAUTH_ISSUER = 'https://container.googleapis.com/v1/projects/<project>/locations/<zone>/clusters/<cluster>'
    EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM = 'sub'
    EXTERNAL_OAUTH_SNOWFLAKE_USER_MAPPING_ATTRIBUTE = 'LOGIN_NAME'
    EXTERNAL_OAUTH_ANY_ROLE_MODE = 'ENABLE'
    EXTERNAL_OAUTH_AUDIENCE_LIST = ('https://<orgname>-<accountname>.snowflakecomputing.com')
    EXTERNAL_OAUTH_JWS_KEYS_URL = 'https://container.googleapis.com/v1/projects/<project>/locations/<zone>/clusters/<cluster>/jwks';
```

### AKS (Workload Identity)

```bash
az aks show -g <resource-group> -n <cluster-name> \
  --query "oidcIssuerProfile.issuerUrl" -o tsv
```

```sql
CREATE SECURITY INTEGRATION IF NOT EXISTS SNOWPLANE_AKS_WIF
    TYPE = EXTERNAL_OAUTH
    ENABLED = TRUE
    EXTERNAL_OAUTH_TYPE = CUSTOM
    EXTERNAL_OAUTH_ISSUER = 'https://oidc.prod-aks.azure.com/<tenant-id>/'
    EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM = 'sub'
    EXTERNAL_OAUTH_SNOWFLAKE_USER_MAPPING_ATTRIBUTE = 'LOGIN_NAME'
    EXTERNAL_OAUTH_ANY_ROLE_MODE = 'ENABLE'
    EXTERNAL_OAUTH_AUDIENCE_LIST = ('https://<orgname>-<accountname>.snowflakecomputing.com')
    EXTERNAL_OAUTH_JWS_KEYS_URL = 'https://oidc.prod-aks.azure.com/<tenant-id>/discovery/v2.0/keys';
```

---

## Map ServiceAccount to Snowflake User

The projected token's `sub` claim is typically `system:serviceaccount:<namespace>:<serviceaccount>`.

```sql
ALTER USER SVC_SNOWPLANE SET LOGIN_NAME = 'system:serviceaccount:snowplane-system:snowplane';
```

---

## Deploy Snowplane with WIF

### Helm Values

```yaml
# values-wif.yaml
serviceAccount:
  create: true
  annotations:
    # EKS: eks.amazonaws.com/role-arn: arn:aws:iam::<account-id>:role/<role-name>
    # GKE: iam.gke.io/gcp-service-account: <gsa>@<project>.iam.gserviceaccount.com
    # AKS: azure.workload.identity/client-id: <client-id>

workloadIdentity:
  enabled: true
  audience: "https://<orgname>-<accountname>.snowflakecomputing.com"
  expirationSeconds: 3600
  mountPath: /var/run/secrets/snowflake
  tokenFileName: token
```

```bash
helm upgrade --install snowplane charts/snowplane \
  -n snowplane-system --create-namespace \
  -f values-wif.yaml
```

### ProviderConfig

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: production
spec:
  account: "orgname-accountname"
  user: "SVC_SNOWPLANE"
  role: "SNOWPLANE_ROLE"
  warehouse: "COMPUTE_WH"
  authenticationType: WorkloadIdentity
  credentials: {}
  workloadIdentity:
    audience: "https://orgname-accountname.snowflakecomputing.com"
```

---

## Troubleshooting

| Problem | Fix |
|:--------|:----|
| **Token file not found** | Ensure `workloadIdentity.enabled: true` in Helm values |
| **Token audience mismatch** | Match `audience` in Helm values, ProviderConfig, and Snowflake integration |
| **User mapping failure** | Check `sub` claim matches Snowflake user's `LOGIN_NAME`. Decode: `cat /var/run/secrets/snowflake/token \| cut -d. -f2 \| base64 -d \| jq .` |
| **ProviderConfig NotReady** | `kubectl describe providerconfig production` |

---

## Security Considerations

1. **Token lifetime** — Projected tokens are short-lived (default: 1 hour), kubelet refreshes automatically
2. **No static credentials** — No passwords or private keys in Kubernetes Secrets
3. **Automatic rotation** — Token refresh handled by kubelet, driver re-reads on each connection
4. **Multi-cloud providers** — Native support for OIDC, AWS, GCP, Azure via the `provider` field
5. **Least privilege** — Combine WIF with a restricted `SNOWPLANE_ROLE`
6. **Namespace isolation** — Use `spec.allowedNamespaces` on ProviderConfig
