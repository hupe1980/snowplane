// helpers_extra_test.go provides test helper constructors and observation
// factories for the 17 additional resource types covered by integration tests.
//
//go:build integration

package integration

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ---------------------------------------------------------------------------
// Test helper constructors — schema-scoped resources
// ---------------------------------------------------------------------------

func newTestAlert(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.Alert {
	return &snowplanev1alpha1.Alert{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.AlertSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Condition:   "SELECT 1",
			Action:      "SELECT 1",
		},
	}
}

func newTestTask(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.Task {
	return &snowplanev1alpha1.Task{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.TaskSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:         sfName,
			DatabaseRef:  &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:    &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			SQLStatement: "SELECT 1",
		},
	}
}

func newTestDynamicTable(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.DynamicTable {
	return &snowplanev1alpha1.DynamicTable{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.DynamicTableSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:          sfName,
			DatabaseRef:   &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:     &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Query:         "SELECT 1 AS id",
			TargetLag:     "1 minute",
			WarehouseName: ptr("MY_WH"),
		},
	}
}

func newTestMaskingPolicy(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.MaskingPolicy {
	return &snowplanev1alpha1.MaskingPolicy{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.MaskingPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Signature:   []snowplanev1alpha1.MaskingPolicyArgument{{Name: "val", Type: "VARCHAR"}},
			Body:        "CASE WHEN current_role() IN ('ANALYST') THEN val ELSE '***' END",
		},
	}
}

func newTestPasswordPolicy(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.PasswordPolicy {
	return &snowplanev1alpha1.PasswordPolicy{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.PasswordPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
		},
	}
}

func newTestPipe(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.Pipe {
	return &snowplanev1alpha1.Pipe{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.PipeSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:          sfName,
			DatabaseRef:   &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:     &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			CopyStatement: "COPY INTO my_table FROM @my_stage",
		},
	}
}

func newTestFileFormat(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.FileFormat {
	return &snowplanev1alpha1.FileFormat{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.FileFormatSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Type:        snowplanev1alpha1.FileFormatTypeCSV,
		},
	}
}

func newTestTag(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.Tag {
	return &snowplanev1alpha1.Tag{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.TagSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
		},
	}
}

func newTestRowAccessPolicy(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.RowAccessPolicy {
	return &snowplanev1alpha1.RowAccessPolicy{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.RowAccessPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			Signature:   []snowplanev1alpha1.RowAccessPolicyArgument{{Name: "val", Type: "VARCHAR"}},
			Body:        "CASE WHEN current_role() IN ('ANALYST') THEN true ELSE false END",
		},
	}
}

// ---------------------------------------------------------------------------
// Test helper constructors — account-level resources
// ---------------------------------------------------------------------------

func newTestNetworkPolicy(name, sfName string) *snowplanev1alpha1.NetworkPolicy {
	return &snowplanev1alpha1.NetworkPolicy{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.NetworkPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:          sfName,
			AllowedIPList: []string{"192.168.1.0/24"},
		},
	}
}

func newTestResourceMonitor(name, sfName string) *snowplanev1alpha1.ResourceMonitor {
	return &snowplanev1alpha1.ResourceMonitor{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.ResourceMonitorSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: sfName,
		},
	}
}

// ---------------------------------------------------------------------------
// Test helper constructors — special resources
// ---------------------------------------------------------------------------

func newTestGrantOwnership(name, objectType, objectName, accountRole string) *snowplanev1alpha1.GrantOwnership {
	return &snowplanev1alpha1.GrantOwnership{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.GrantOwnershipSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			ObjectType:  objectType,
			ObjectName:  objectName,
			AccountRole: &accountRole,
		},
	}
}

func newTestAccountRoleAssignment(name, roleName, toRole string) *snowplanev1alpha1.AccountRoleAssignment {
	return &snowplanev1alpha1.AccountRoleAssignment{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.AccountRoleAssignmentSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			RoleName: &roleName,
			ToRole:   &toRole,
		},
	}
}

func newTestDatabaseRoleAssignment(name, databaseRoleName, toRole string) *snowplanev1alpha1.DatabaseRoleAssignment {
	return &snowplanev1alpha1.DatabaseRoleAssignment{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.DatabaseRoleAssignmentSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			DatabaseRoleName: &databaseRoleName,
			ToRole:           &toRole,
		},
	}
}

// ---------------------------------------------------------------------------
// Observation factories
// ---------------------------------------------------------------------------

func alertObservation(name, dbName, schemaName, owner string) *snowflake.AlertObservation {
	return &snowflake.AlertObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.AlertShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        owner,
			State:        "suspended",
			Condition:    "SELECT 1",
			Action:       "SELECT 1",
		},
	}
}

func taskObservation(name, dbName, schemaName, owner string) *snowflake.TaskObservation {
	return &snowflake.TaskObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.TaskShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        owner,
			State:        "suspended",
			Definition:   "SELECT 1",
		},
	}
}

func dynamicTableObservation(name, dbName, schemaName, owner string) *snowflake.DynamicTableObservation {
	return &snowflake.DynamicTableObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.DynamicTableShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        owner,
			TargetLag:    "1 minute",
			Warehouse:    "MY_WH",
			Text:         "SELECT 1 AS id",
		},
	}
}

func networkPolicyObservation(name string) *snowflake.NetworkPolicyObservation {
	return &snowflake.NetworkPolicyObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.NetworkPolicyShowOutput{
			CreatedOn:              "2024-01-01",
			Name:                   name,
			EntriesInAllowedIPList: "1",
		},
	}
}

func maskingPolicyObservation(name, dbName, schemaName, owner string) *snowflake.MaskingPolicyObservation {
	return &snowflake.MaskingPolicyObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.MaskingPolicyShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        owner,
			Kind:         "MASKING_POLICY",
		},
	}
}

func passwordPolicyObservation(name, dbName, schemaName, owner string) *snowflake.PasswordPolicyObservation {
	return &snowflake.PasswordPolicyObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.PasswordPolicyShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        owner,
		},
	}
}

func resourceMonitorObservation(name string) *snowflake.ResourceMonitorObservation {
	return &snowflake.ResourceMonitorObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.ResourceMonitorShowOutput{
			CreatedOn: "2024-01-01",
			Name:      name,
		},
	}
}

func pipeObservation(name, dbName, schemaName, owner string) *snowflake.PipeObservation {
	return &snowflake.PipeObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.PipeShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        owner,
			Definition:   "COPY INTO my_table FROM @my_stage",
		},
	}
}

func fileFormatObservation(name, dbName, schemaName, owner string) *snowflake.FileFormatObservation {
	return &snowflake.FileFormatObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.FileFormatShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        owner,
			Type:         "CSV",
		},
	}
}

func tagObservation(name, dbName, schemaName, owner string) *snowflake.TagObservation {
	return &snowflake.TagObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.TagShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        owner,
		},
	}
}

func rowAccessPolicyObservation(name, dbName, schemaName, owner string) *snowflake.RowAccessPolicyObservation {
	return &snowflake.RowAccessPolicyObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.RowAccessPolicyShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        owner,
			Kind:         "ROW_ACCESS_POLICY",
		},
	}
}

func grantOwnershipObservation(objectType, objectName, grantedTo, granteeName string) *snowflake.GrantOwnershipObservation {
	return &snowflake.GrantOwnershipObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.GrantOwnershipShowOutput{
			CreatedOn:   "2024-01-01",
			Privilege:   "OWNERSHIP",
			GrantedOn:   objectType,
			Name:        objectName,
			GrantedTo:   grantedTo,
			GranteeName: granteeName,
		},
	}
}

func roleAssignmentObservation(role, grantedTo, granteeName string) *snowflake.RoleAssignmentObservation {
	return &snowflake.RoleAssignmentObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.RoleAssignmentShowOutput{
			CreatedOn:   "2024-01-01",
			Role:        role,
			GrantedTo:   grantedTo,
			GranteeName: granteeName,
		},
	}
}

func newTestAuthenticationPolicy(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.AuthenticationPolicy {
	return &snowplanev1alpha1.AuthenticationPolicy{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:                  sfName,
			DatabaseRef:           &snowplanev1alpha1.ObjectReference{Name: dbRefName},
			SchemaRef:             &snowplanev1alpha1.ObjectReference{Name: schemaRefName},
			AuthenticationMethods: []string{"PASSWORD", "SAML"},
			ClientTypes:           []string{"SNOWFLAKE_UI", "DRIVERS"},
		},
	}
}

func authenticationPolicyObservation(name, dbName, schemaName, owner string) *snowflake.AuthenticationPolicyObservation {
	return &snowflake.AuthenticationPolicyObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.AuthenticationPolicyShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			SchemaName:   schemaName,
			Owner:        owner,
		},
		DescribeOutput: map[string]string{
			"AUTHENTICATION_METHODS": "[PASSWORD, SAML]",
			"CLIENT_TYPES":           "[SNOWFLAKE_UI, DRIVERS]",
		},
	}
}

// ---------------------------------------------------------------------------
// Test helper constructors — APIIntegration, SecondaryDatabase, SharedDatabase
// ---------------------------------------------------------------------------

func newTestAPIIntegration(name, sfName string) *snowplanev1alpha1.APIIntegration {
	return &snowplanev1alpha1.APIIntegration{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.APIIntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:               sfName,
			APIProvider:        "aws_api_gateway",
			APIAllowedPrefixes: []string{"https://example.com/"},
		},
	}
}

func newTestSecondaryDatabase(name, sfName string) *snowplanev1alpha1.SecondaryDatabase {
	return &snowplanev1alpha1.SecondaryDatabase{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.SecondaryDatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			AsReplicaOf: "myorg.myaccount.mydb",
		},
	}
}

func newTestSharedDatabase(name, sfName string) *snowplanev1alpha1.SharedDatabase {
	return &snowplanev1alpha1.SharedDatabase{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.SharedDatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:      sfName,
			FromShare: "provider_account.my_share",
		},
	}
}

// ---------------------------------------------------------------------------
// Observation factories — APIIntegration, SecondaryDatabase, SharedDatabase
// ---------------------------------------------------------------------------

func apiIntegrationObservation(name string) *snowflake.APIIntegrationObservation {
	return &snowflake.APIIntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.APIIntegrationShowOutput{
			CreatedOn: "2024-01-01",
			Name:      name,
			Type:      "EXTERNAL_API",
			Category:  "API",
			Enabled:   true,
		},
		DescribeOutput: map[string]string{
			"API_ALLOWED_PREFIXES": "https://example.com/",
		},
	}
}

func secondaryDatabaseObservation(name string) *snowflake.SecondaryDatabaseObservation {
	return &snowflake.SecondaryDatabaseObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.SecondaryDatabaseShowOutput{
			CreatedOn:     "2024-01-01",
			Name:          name,
			Kind:          "SECONDARY",
			Owner:         "SYSADMIN",
			RetentionTime: 1,
			Origin:        "myorg.myaccount.mydb",
		},
		Parameters: &snowflake.DatabaseParameters{},
	}
}

func sharedDatabaseObservation(name string) *snowflake.SharedDatabaseObservation {
	return &snowflake.SharedDatabaseObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{
			CreatedOn:     "2024-01-01",
			Name:          name,
			Kind:          "IMPORTED DATABASE",
			Owner:         "SYSADMIN",
			RetentionTime: 1,
			Origin:        "provider_account.my_share",
		},
		Parameters: &snowflake.DatabaseParameters{},
	}
}
