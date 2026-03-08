// helpers_extended_test.go provides test helper constructors and observation
// factories for the additional resource types covered by integration tests.
//
//go:build integration

package integration

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	sqlstmtclient "github.com/hupe1980/snowplane/internal/clients/snowflake/sqlstatement"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ===================================================================
// CR constructors — Functions (schema-scoped, callable)
// ===================================================================

func newTestFunctionJava(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.FunctionJava {
	return &snowplanev1alpha1.FunctionJava{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.FunctionJavaSpec{
			CommonSpec:      snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:            sfName,
			DatabaseRef:     &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:       &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Returns:         "VARCHAR",
			Handler:         "MyClass.myMethod",
			RuntimeVersion:  "11",
			SnowparkPackage: "com.snowflake:snowpark:latest",
		},
	}
}

func newTestFunctionJavascript(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.FunctionJavascript {
	return &snowplanev1alpha1.FunctionJavascript{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.FunctionJavascriptSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Returns:     "VARCHAR",
			Body:        "return 'hello';",
		},
	}
}

func newTestFunctionPython(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.FunctionPython {
	return &snowplanev1alpha1.FunctionPython{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.FunctionPythonSpec{
			CommonSpec:      snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:            sfName,
			DatabaseRef:     &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:       &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Returns:         "VARCHAR",
			Handler:         "my_func",
			RuntimeVersion:  "3.8",
			SnowparkPackage: "snowflake-snowpark-python",
		},
	}
}

func newTestFunctionScala(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.FunctionScala {
	return &snowplanev1alpha1.FunctionScala{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.FunctionScalaSpec{
			CommonSpec:      snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:            sfName,
			DatabaseRef:     &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:       &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Returns:         "VARCHAR",
			Handler:         "MyClass.myMethod",
			RuntimeVersion:  "2.12",
			SnowparkPackage: "com.snowflake:snowpark:latest",
		},
	}
}

func newTestFunctionSQL(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.FunctionSQL {
	return &snowplanev1alpha1.FunctionSQL{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.FunctionSQLSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Returns:     "VARCHAR",
			Body:        "SELECT 'hello'",
		},
	}
}

// ===================================================================
// CR constructors — Procedures (schema-scoped, callable)
// ===================================================================

func newTestProcedureJava(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.ProcedureJava {
	return &snowplanev1alpha1.ProcedureJava{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.ProcedureJavaSpec{
			CommonSpec:      snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:            sfName,
			DatabaseRef:     &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:       &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Returns:         "VARCHAR",
			Handler:         "MyClass.myMethod",
			RuntimeVersion:  "11",
			SnowparkPackage: "com.snowflake:snowpark:latest",
		},
	}
}

func newTestProcedureJavascript(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.ProcedureJavascript {
	return &snowplanev1alpha1.ProcedureJavascript{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.ProcedureJavascriptSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Returns:     "VARCHAR",
			Body:        "return 'hello';",
		},
	}
}

func newTestProcedurePython(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.ProcedurePython {
	return &snowplanev1alpha1.ProcedurePython{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.ProcedurePythonSpec{
			CommonSpec:      snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:            sfName,
			DatabaseRef:     &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:       &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Returns:         "VARCHAR",
			Handler:         "my_proc",
			RuntimeVersion:  "3.8",
			SnowparkPackage: "snowflake-snowpark-python",
		},
	}
}

func newTestProcedureScala(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.ProcedureScala {
	return &snowplanev1alpha1.ProcedureScala{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.ProcedureScalaSpec{
			CommonSpec:      snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:            sfName,
			DatabaseRef:     &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:       &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Returns:         "VARCHAR",
			Handler:         "MyClass.myMethod",
			RuntimeVersion:  "2.12",
			SnowparkPackage: "com.snowflake:snowpark:latest",
		},
	}
}

func newTestProcedureSQL(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.ProcedureSQL {
	return &snowplanev1alpha1.ProcedureSQL{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.ProcedureSQLSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Returns:     "VARCHAR",
			Body:        "BEGIN RETURN 'hello'; END;",
		},
	}
}

// ===================================================================
// CR constructors — Secrets (schema-scoped)
// ===================================================================

func newTestSecretWithAuthCodeGrant(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.SecretWithAuthorizationCodeGrant {
	return &snowplanev1alpha1.SecretWithAuthorizationCodeGrant{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.SecretWithAuthorizationCodeGrantSpec{
			CommonSpec:                  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:                        sfName,
			DatabaseRef:                 &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:                   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			APIAuthentication:           "MY_OAUTH_INT",
			OAuthRefreshToken:           "token123",
			OAuthRefreshTokenExpiryTime: "2099-01-01 00:00:00",
		},
	}
}

func newTestSecretWithBasicAuth(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.SecretWithBasicAuthentication {
	return &snowplanev1alpha1.SecretWithBasicAuthentication{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.SecretWithBasicAuthenticationSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Username:    "myuser",
			Password:    "mypass",
		},
	}
}

func newTestSecretWithClientCreds(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.SecretWithClientCredentials {
	return &snowplanev1alpha1.SecretWithClientCredentials{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.SecretWithClientCredentialsSpec{
			CommonSpec:        snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:              sfName,
			DatabaseRef:       &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:         &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			APIAuthentication: "MY_OAUTH_INT",
			OAuthScopes:       []string{"session:role:analyst"},
		},
	}
}

func newTestSecretWithGenericString(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.SecretWithGenericString {
	return &snowplanev1alpha1.SecretWithGenericString{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.SecretWithGenericStringSpec{
			CommonSpec:   snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:         sfName,
			DatabaseRef:  &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:    &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			SecretString: "my-secret-value",
		},
	}
}

// ===================================================================
// CR constructors — Streams (schema-scoped, source-ref required)
// ===================================================================

func newTestStreamOnDirectoryTable(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.StreamOnDirectoryTable {
	return &snowplanev1alpha1.StreamOnDirectoryTable{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.StreamOnDirectoryTableSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			StageName:   ptr("MY_STAGE"),
		},
	}
}

func newTestStreamOnDynamicTable(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.StreamOnDynamicTable {
	return &snowplanev1alpha1.StreamOnDynamicTable{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.StreamOnDynamicTableSpec{
			CommonSpec:       snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:             sfName,
			DatabaseRef:      &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:        &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			DynamicTableName: ptr("MY_DYN_TABLE"),
		},
	}
}

func newTestStreamOnExternalTable(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.StreamOnExternalTable {
	return &snowplanev1alpha1.StreamOnExternalTable{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.StreamOnExternalTableSpec{
			CommonSpec:        snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:              sfName,
			DatabaseRef:       &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:         &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			ExternalTableName: ptr("MY_EXT_TABLE"),
		},
	}
}

func newTestStreamOnTable(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.StreamOnTable {
	return &snowplanev1alpha1.StreamOnTable{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.StreamOnTableSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			TableName:   ptr("MY_TABLE"),
		},
	}
}

func newTestStreamOnView(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.StreamOnView {
	return &snowplanev1alpha1.StreamOnView{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.StreamOnViewSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			ViewName:    ptr("MY_VIEW"),
		},
	}
}

// ===================================================================
// CR constructors — API Authentication Integrations (account-scoped)
// ===================================================================

func newTestAPIAuthCodeGrant(name, sfName string) *snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant {
	return &snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrantSpec{
			CommonSpec:           snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:                 sfName,
			Enabled:              ptr(true),
			OAuthClientID:        "client-id",
			OAuthClientSecretRef: snowplanev1alpha1.SecretKeyReference{Name: "test-secret", Key: "password"},
		},
	}
}

func newTestAPIAuthClientCreds(name, sfName string) *snowplanev1alpha1.APIAuthenticationIntegrationWithClientCredentials {
	return &snowplanev1alpha1.APIAuthenticationIntegrationWithClientCredentials{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.APIAuthenticationIntegrationWithClientCredentialsSpec{
			CommonSpec:           snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:                 sfName,
			Enabled:              ptr(true),
			OAuthClientID:        "client-id",
			OAuthClientSecretRef: snowplanev1alpha1.SecretKeyReference{Name: "test-secret", Key: "password"},
		},
	}
}

func newTestAPIAuthJWTBearer(name, sfName string) *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer {
	return &snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearerSpec{
			CommonSpec:           snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:                 sfName,
			Enabled:              ptr(true),
			OAuthClientID:        "client-id",
			OAuthClientSecretRef: snowplanev1alpha1.SecretKeyReference{Name: "test-secret", Key: "password"},
			OAuthAssertionIssuer: "https://issuer.example.com",
		},
	}
}

// ===================================================================
// CR constructors — Security Integrations (account-scoped)
// ===================================================================

func newTestExternalOAuthIntegration(name, sfName string) *snowplanev1alpha1.ExternalOAuthIntegration {
	return &snowplanev1alpha1.ExternalOAuthIntegration{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.ExternalOAuthIntegrationSpec{
			CommonSpec:                    snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:                          sfName,
			ExternalOAuthType:             "CUSTOM",
			Issuer:                        "https://issuer.example.com",
			TokenUserMappingClaim:         "sub",
			SnowflakeUserMappingAttribute: "LOGIN_NAME",
		},
	}
}

func newTestSAML2Integration(name, sfName string) *snowplanev1alpha1.SAML2Integration {
	return &snowplanev1alpha1.SAML2Integration{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.SAML2IntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:       sfName,
			Issuer:     "https://issuer.example.com",
			SSOURL:     "https://sso.example.com",
			Provider:   "CUSTOM",
			X509Cert:   "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQ",
		},
	}
}

// ===================================================================
// CR constructors — Schema-scoped standard resources
// ===================================================================

func newTestExternalTable(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.ExternalTable {
	return &snowplanev1alpha1.ExternalTable{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.ExternalTableSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Location:    "@MY_DB.MY_SCHEMA.MY_STAGE/path/",
			FileFormat:  "TYPE = PARQUET",
		},
	}
}

func newTestMaterializedView(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.MaterializedView {
	return &snowplanev1alpha1.MaterializedView{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.MaterializedViewSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Statement:   "SELECT 1 AS id",
		},
	}
}

func newTestNetworkRule(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.NetworkRule {
	return &snowplanev1alpha1.NetworkRule{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.NetworkRuleSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Type:        snowplanev1alpha1.NetworkRuleTypeIPV4,
			Mode:        snowplanev1alpha1.NetworkRuleModeIngress,
			ValueList:   []string{"192.168.1.0/24"},
		},
	}
}

func newTestSequence(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.Sequence {
	return &snowplanev1alpha1.Sequence{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.SequenceSpec{
			CommonSpec:  snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
		},
	}
}

// ===================================================================
// CR constructors — Account-scoped resources
// ===================================================================

func newTestFailoverGroup(name, sfName string) *snowplanev1alpha1.FailoverGroup {
	return &snowplanev1alpha1.FailoverGroup{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.FailoverGroupSpec{
			CommonSpec:      snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:            sfName,
			ObjectTypes:     []string{"DATABASES"},
			AllowedAccounts: []string{"myorg.myaccount2"},
		},
	}
}

// ===================================================================
// CR constructors — Policy attachments
// ===================================================================

func newTestMaskingPolicyApplication(name, policyName, tableName, columnName string) *snowplanev1alpha1.MaskingPolicyApplication {
	return &snowplanev1alpha1.MaskingPolicyApplication{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.MaskingPolicyApplicationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			PolicyName: &policyName,
			TableName:  tableName,
			ColumnName: columnName,
		},
	}
}

func newTestNetworkPolicyAttachment(name, policyName string) *snowplanev1alpha1.NetworkPolicyAttachment {
	return &snowplanev1alpha1.NetworkPolicyAttachment{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.NetworkPolicyAttachmentSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			PolicyName: &policyName,
			TargetType: "ACCOUNT",
		},
	}
}

func newTestPasswordPolicyAttachment(name, policyName string) *snowplanev1alpha1.PasswordPolicyAttachment {
	return &snowplanev1alpha1.PasswordPolicyAttachment{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.PasswordPolicyAttachmentSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			PolicyName: &policyName,
			TargetType: "ACCOUNT",
		},
	}
}

// ===================================================================
// CR constructors — Special resources
// ===================================================================

func newTestTagAssociation(name, tagName, objectType, objectName, tagValue string) *snowplanev1alpha1.TagAssociation {
	return &snowplanev1alpha1.TagAssociation{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.TagAssociationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			TagName:    &tagName,
			TagValue:   tagValue,
			ObjectType: objectType,
			ObjectName: objectName,
		},
	}
}

func newTestTableConstraint(name, constraintName, constraintType, tableName string, columns []string) *snowplanev1alpha1.TableConstraint {
	return &snowplanev1alpha1.TableConstraint{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.TableConstraintSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:       constraintName,
			Type:       snowplanev1alpha1.ConstraintType(constraintType),
			TableName:  tableName,
			Columns:    columns,
		},
	}
}

func newTestSQLStatement(name, executeSQL string) *snowplanev1alpha1.SQLStatement {
	return &snowplanev1alpha1.SQLStatement{
		ObjectMeta: ctrl.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: snowplanev1alpha1.SQLStatementSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Execute:    executeSQL,
		},
	}
}

// ===================================================================
// Observation factories
// ===================================================================

func functionObservation(name, dbName, schemaName string) *snowflake.FunctionObservation {
	return &snowflake.FunctionObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.FunctionShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Language:     "JAVA",
			Owner:        "SYSADMIN",
		},
	}
}

func procedureObservation(name, dbName, schemaName string) *snowflake.ProcedureObservation {
	return &snowflake.ProcedureObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.ProcedureShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        "SYSADMIN",
		},
	}
}

func secretObservation(name, dbName, schemaName string) *snowflake.SecretObservation {
	return &snowflake.SecretObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.SecretShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        "SYSADMIN",
		},
	}
}

func streamObservation(name, dbName, schemaName string) *snowflake.StreamObservation {
	return &snowflake.StreamObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.StreamShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        "SYSADMIN",
		},
	}
}

func apiAuthIntegrationObservation(name string) *snowflake.APIAuthenticationIntegrationObservation {
	return &snowflake.APIAuthenticationIntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.SecurityIntegrationShowOutput{
			CreatedOn: "2024-01-01",
			Name:      name,
			Type:      "API_AUTHENTICATION",
			Category:  "SECURITY",
			Enabled:   true,
		},
	}
}

func externalOAuthObservation(name string) *snowflake.ExternalOAuthIntegrationObservation {
	return &snowflake.ExternalOAuthIntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.ExternalOAuthIntegrationShowOutput{
			CreatedOn: "2024-01-01",
			Name:      name,
			Type:      "EXTERNAL_OAUTH",
			Category:  "SECURITY",
			Enabled:   true,
		},
	}
}

func saml2Observation(name string) *snowflake.SAML2IntegrationObservation {
	return &snowflake.SAML2IntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.SAML2IntegrationShowOutput{
			CreatedOn: "2024-01-01",
			Name:      name,
			Type:      "SAML2",
			Category:  "SECURITY",
			Enabled:   true,
		},
	}
}

func externalTableObservation(name, dbName, schemaName string) *snowflake.ExternalTableObservation {
	return &snowflake.ExternalTableObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.ExternalTableShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        "SYSADMIN",
		},
	}
}

func materializedViewObservation(name, dbName, schemaName string) *snowflake.MaterializedViewObservation {
	return &snowflake.MaterializedViewObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.MaterializedViewShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        "SYSADMIN",
			Text:         "SELECT 1 AS id",
		},
	}
}

func networkRuleObservation(name, dbName, schemaName string) *snowflake.NetworkRuleObservation {
	return &snowflake.NetworkRuleObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.NetworkRuleShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        "SYSADMIN",
			Type:         "IPV4",
			Mode:         "INGRESS",
		},
	}
}

func sequenceObservation(name, dbName, schemaName string) *snowflake.SequenceObservation {
	return &snowflake.SequenceObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.SequenceShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        "SYSADMIN",
			NextValue:    "1",
			Interval:     "1",
		},
	}
}

func failoverGroupObservation(name string) *snowflake.FailoverGroupObservation {
	return &snowflake.FailoverGroupObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.FailoverGroupShowOutput{
			CreatedOn:   "2024-01-01",
			Name:        name,
			Type:        "FAILOVER",
			IsPrimary:   true,
			ObjectTypes: "DATABASES",
		},
	}
}

func maskingPolicyApplicationObservation(policyName string) *snowflake.MaskingPolicyApplicationObservation {
	return &snowflake.MaskingPolicyApplicationObservation{
		Exists:     true,
		PolicyName: policyName,
	}
}

func networkPolicyAttachmentObservation(policyName string) *snowflake.NetworkPolicyAttachmentObservation {
	return &snowflake.NetworkPolicyAttachmentObservation{
		Exists:     true,
		PolicyName: policyName,
	}
}

func passwordPolicyAttachmentObservation(policyName string) *snowflake.PasswordPolicyAttachmentObservation {
	return &snowflake.PasswordPolicyAttachmentObservation{
		Exists:     true,
		PolicyName: policyName,
	}
}

func tagAssociationObservation(tagValue string) *snowflake.TagAssociationObservation {
	return &snowflake.TagAssociationObservation{
		Exists:   true,
		TagValue: tagValue,
	}
}

func tableConstraintObservation(constraintName, constraintType string, columns []string) *snowflake.TableConstraintObservation {
	return &snowflake.TableConstraintObservation{
		Exists:         true,
		ConstraintName: constraintName,
		ConstraintType: constraintType,
		Columns:        columns,
	}
}

func sqlStatementObservation() *sqlstmtclient.Observation {
	return &sqlstmtclient.Observation{
		Exists:  true,
		Matched: true,
	}
}
