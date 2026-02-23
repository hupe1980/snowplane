# Workload Identity Federation Guide

This guide explains how to configure Snowplane to use **Workload Identity Federation (WIF)** for passwordless authentication to Snowflake. WIF eliminates the need to manage Snowflake credentials as Kubernetes Secrets.

## How It Works

1. Kubernetes projects a short-lived OIDC token from the cluster's ServiceAccount into the operator pod.
2. The gosnowflake driver reads the OIDC token from the projected volume path (`TokenFilePath`) on each new connection.
3. The driver performs native Workload Identity Federation attestation exchange with Snowflake using `AuthTypeWorkloadIdentityFederation`.
4. Snowflake validates the token against the configured OIDC issuer and grants access.
5. Kubelet automatically refreshes the projected token before expiry — the driver re-reads the latest token on each connection.

No passwords, private keys, or static credentials are stored anywhere. The driver handles token reading, refresh, and attestation natively.

## Prerequisites

- Kubernetes cluster with OIDC issuer configured (EKS, GKE, and AKS have this by default).
- A Snowflake account with External OAuth support.
- A SERVICE-type user in Snowflake (no password required).
- Access to create Security Integrations in Snowflake.

## Step 1: Create a SERVICE User in Snowflake

WIF requires a SERVICE-type user (not a regular user):

    USE ROLE ACCOUNTADMIN;

    CREATE USER IF NOT EXISTS SVC_SNOWPLANE
        DEFAULT_ROLE = SNOWPLANE_ROLE
        TYPE = SERVICE
        COMMENT = 'Snowplane WIF service user';

    GRANT ROLE SNOWPLANE_ROLE TO USER SVC_SNOWPLANE;

See [Snowflake Role Setup Guide](snowflake-role-setup.md) for configuring the required role privileges.

## Step 2: Create a Security Integration

### EKS (IRSA)

Discover your EKS OIDC issuer URL:

    aws eks describe-cluster --name <cluster-name> --query "cluster.identity.oidc.issuer" --output text

Example output: `https://oidc.eks.us-east-1.amazonaws.com/id/EXAMPLE1234567890`

Create the Snowflake security integration:

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

### GKE (Workload Identity)

Discover your GKE OIDC issuer:

    gcloud container clusters describe <cluster-name> --zone <zone> --format="value(selfLink)"

GKE OIDC issuer format: `https://container.googleapis.com/v1/projects/<project>/locations/<zone>/clusters/<cluster>`

Create the Snowflake security integration:

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

### AKS (Workload Identity)

For AKS clusters with OIDC issuer enabled:

    az aks show -g <resource-group> -n <cluster-name> --query "oidcIssuerProfile.issuerUrl" -o tsv

Create the Snowflake security integration:

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

## Step 3: Map Kubernetes ServiceAccount to Snowflake User

The projected token's `sub` claim is typically `system:serviceaccount:<namespace>:<serviceaccount>`.

Set the Snowflake user's `LOGIN_NAME` to match:

    ALTER USER SVC_SNOWPLANE SET LOGIN_NAME = 'system:serviceaccount:snowplane-system:snowplane';

Or set a custom mapping depending on your OIDC provider configuration.

## Step 4: Deploy Snowplane with WIF

### Helm Values

Configure the Helm chart with WIF enabled:

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

Install:

    helm upgrade --install snowplane charts/snowplane \
      -n snowplane-system --create-namespace \
      -f values-wif.yaml

### ProviderConfig

Create a ProviderConfig using WorkloadIdentity authentication:

    apiVersion: snowplane.hupe1980.github.io/v1alpha1
    kind: ProviderConfig
    metadata:
      name: production
      namespace: snowplane-system
    spec:
      account: "orgname-accountname"
      user: "SVC_SNOWPLANE"
      role: "SNOWPLANE_ROLE"
      warehouse: "COMPUTE_WH"
      authenticationType: WorkloadIdentity
      credentials: {}
      workloadIdentity:
        audience: "https://orgname-accountname.snowflakecomputing.com"
        # tokenFilePath defaults to /var/run/secrets/snowflake/token

## Troubleshooting

### Token file not found

The operator logs: `reading WIF token file "/var/run/secrets/snowflake/token": open ... no such file`

**Fix:** Ensure `workloadIdentity.enabled: true` is set in Helm values so the projected volume is mounted.

### Token audience mismatch

Snowflake rejects the token with an audience error.

**Fix:** Ensure the `workloadIdentity.audience` in Helm values and ProviderConfig matches the `EXTERNAL_OAUTH_AUDIENCE_LIST` in the Snowflake security integration.

### User mapping failure

Snowflake cannot map the token to a user.

**Fix:** Check the `sub` claim in the projected token and ensure the Snowflake user's `LOGIN_NAME` matches. Decode the token to inspect claims:

    cat /var/run/secrets/snowflake/token | cut -d. -f2 | base64 -d | jq .

### ProviderConfig shows NotReady

Check the ProviderConfig status conditions:

    kubectl describe providerconfig production -n snowplane-system

Look for the `Ready` condition's `Reason` and `Message` for specific error details.

## Security Considerations

1. **Token lifetime** — Projected tokens are short-lived (default: 1 hour). Kubelet refreshes them automatically before expiry.
2. **No static credentials** — No passwords or private keys are stored in Kubernetes Secrets.
3. **Automatic rotation** — Token refresh is handled by kubelet. The gosnowflake driver re-reads the latest token from `TokenFilePath` on each new connection.
4. **Multi-cloud providers** — Native support for OIDC (any K8s cluster), AWS (IRSA/Pod Identity), GCP (Workload Identity), and Azure (AKS IMDS) via the `provider` field.
5. **Least privilege** — Combine WIF with a restricted `SNOWPLANE_ROLE` (see [Role Setup Guide](snowflake-role-setup.md)).
6. **Namespace isolation** — Use `spec.allowedNamespaces` on ProviderConfig to restrict which teams can use each Snowflake account.
