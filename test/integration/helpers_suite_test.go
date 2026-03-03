//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Generic pointer helper
// --------------------------------------------------------------------------

func ptr[T any](v T) *T { return &v }

// --------------------------------------------------------------------------
// CR factory helpers — construct CRs with sensible defaults
// --------------------------------------------------------------------------

// newTestDatabase creates a Database CR with sensible defaults for integration tests.
func newTestDatabase(name, sfName string) *snowplanev1alpha1.Database {
	return &snowplanev1alpha1.Database{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: sfName,
		},
	}
}

// newTestSchema creates a Schema CR referencing a Database for integration tests.
func newTestSchema(name, sfName, dbRefName string) *snowplanev1alpha1.Schema {
	return &snowplanev1alpha1.Schema{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRefName},
		},
	}
}

// newTestTable creates a Table CR referencing a Database and Schema for integration tests.
func newTestTable(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.Table {
	return &snowplanev1alpha1.Table{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.TableSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: schemaRefName},
			Columns: []snowplanev1alpha1.ColumnDefinition{
				{Name: "ID", Type: "NUMBER(38,0)"},
				{Name: "NAME", Type: "VARCHAR(256)"},
			},
		},
	}
}

// newTestView creates a View CR referencing a Database and Schema for integration tests.
func newTestView(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.View {
	return &snowplanev1alpha1.View{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.ViewSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: schemaRefName},
			Statement:   "SELECT 1",
		},
	}
}

// newTestStage creates a Stage CR referencing a Database and Schema for integration tests.
func newTestStage(name, sfName, dbRefName, schemaRefName string) *snowplanev1alpha1.Stage {
	return &snowplanev1alpha1.Stage{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.StageSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRefName},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: schemaRefName},
		},
	}
}

// newTestWarehouse creates a Warehouse CR with sensible defaults for integration tests.
func newTestWarehouse(name, sfName string) *snowplanev1alpha1.Warehouse {
	return &snowplanev1alpha1.Warehouse{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.WarehouseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: sfName,
		},
	}
}

// newTestUser creates a User CR with sensible defaults for integration tests.
func newTestUser(name, sfName string) *snowplanev1alpha1.User {
	return &snowplanev1alpha1.User{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.UserSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: sfName,
		},
	}
}

// newTestAccountRole creates an AccountRole CR with sensible defaults for integration tests.
func newTestAccountRole(name, sfName string) *snowplanev1alpha1.AccountRole {
	return &snowplanev1alpha1.AccountRole{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.AccountRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: sfName,
		},
	}
}

// newTestDatabaseRole creates a DatabaseRole CR referencing a Database for integration tests.
func newTestDatabaseRole(name, sfName, dbRefName string) *snowplanev1alpha1.DatabaseRole {
	return &snowplanev1alpha1.DatabaseRole{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.DatabaseRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        sfName,
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: dbRefName},
		},
	}
}

// newTestGrant creates an AccountRoleGrant CR for a database-level privilege.
func newTestGrant(name, privilege, objectType, objectName, toRole string) *snowplanev1alpha1.AccountRoleGrant {
	return &snowplanev1alpha1.AccountRoleGrant{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.AccountRoleGrantSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Privilege: privilege,
			On: snowplanev1alpha1.GrantOn{
				AccountObject: &snowplanev1alpha1.GrantOnAccountObject{
					ObjectType: objectType,
					ObjectName: objectName,
				},
			},
			AccountRole: &toRole,
		},
	}
}

// --------------------------------------------------------------------------
// Observation factory helpers — construct mock observations
// --------------------------------------------------------------------------

// databaseObservation returns a standard existing-database observation.
func databaseObservation(name, comment, owner string) *snowflake.DatabaseObservation {
	return &snowflake.DatabaseObservation{
		Exists: true,
		ShowOutput: &snowflake.DatabaseShowOutput{
			CreatedOn:     "2024-01-01",
			Name:          name,
			Kind:          "STANDARD",
			Comment:       comment,
			Owner:         owner,
			RetentionTime: 1,
		},
		Parameters: &snowflake.DatabaseParameters{
			DataRetentionTimeInDays:    ptr(int32(1)),
			MaxDataExtensionTimeInDays: ptr(int32(14)),
			DefaultDDLCollation:        "",
			ReplaceInvalidCharacters:   ptr(false),
			StorageSerializationPolicy: "COMPATIBLE",
			LogLevel:                   "OFF",
			MetricLevel:                "NONE",
			TraceLevel:                 "OFF",
		},
	}
}

// schemaObservation returns a standard existing-schema observation.
func schemaObservation(name, dbName, comment, owner string) *snowflake.SchemaObservation {
	return &snowflake.SchemaObservation{
		Exists: true,
		ShowOutput: &snowflake.SchemaShowOutput{
			CreatedOn:     "2024-01-01",
			Name:          name,
			DatabaseName:  dbName,
			Kind:          "STANDARD",
			Comment:       comment,
			Owner:         owner,
			RetentionTime: 1,
		},
		Parameters: &snowflake.SchemaParameters{
			DataRetentionTimeInDays:    ptr(int32(1)),
			MaxDataExtensionTimeInDays: ptr(int32(14)),
			DefaultDDLCollation:        "",
			ReplaceInvalidCharacters:   ptr(false),
			StorageSerializationPolicy: "COMPATIBLE",
			LogLevel:                   "OFF",
			MetricLevel:                "NONE",
			TraceLevel:                 "OFF",
		},
	}
}

// tableObservation returns a standard existing-table observation.
func tableObservation(name, dbName, schemaName, comment, owner string) *snowflake.TableObservation {
	return &snowflake.TableObservation{
		Exists: true,
		ShowOutput: &snowflake.TableShowOutput{
			CreatedOn:             "2024-01-01",
			Name:                  name,
			DatabaseName:          dbName,
			SchemaName:            schemaName,
			Kind:                  "TABLE",
			Comment:               comment,
			Owner:                 owner,
			RetentionTime:         1,
			ChangeTracking:        false,
			EnableSchemaEvolution: false,
		},
	}
}

// viewObservation returns a standard existing-view observation.
func viewObservation(name, dbName, schemaName, comment, owner, statement string, secure bool) *snowflake.ViewObservation {
	return &snowflake.ViewObservation{
		Exists: true,
		ShowOutput: &snowflake.ViewShowOutput{
			CreatedOn:      "2024-01-01",
			Name:           name,
			DatabaseName:   dbName,
			SchemaName:     schemaName,
			Comment:        comment,
			Owner:          owner,
			IsSecure:       secure,
			Text:           statement,
			ChangeTracking: false,
		},
	}
}

// stageObservation returns a standard existing-stage observation.
func stageObservation(name, dbName, schemaName, comment, owner, stageType string) *snowflake.StageObservation {
	return &snowflake.StageObservation{
		Exists: true,
		ShowOutput: &snowflake.StageShowOutput{
			CreatedOn:        "2024-01-01",
			Name:             name,
			DatabaseName:     dbName,
			SchemaName:       schemaName,
			URL:              "",
			Owner:            owner,
			Comment:          comment,
			Type:             stageType,
			DirectoryEnabled: false,
		},
	}
}

// warehouseObservation returns a standard existing-warehouse observation.
func warehouseObservation(name, comment, owner string) *snowflake.WarehouseObservation {
	return &snowflake.WarehouseObservation{
		Exists: true,
		ShowOutput: &snowflake.WarehouseShowOutput{
			CreatedOn:       "2024-01-01",
			Name:            name,
			State:           "STARTED",
			Type:            "STANDARD",
			Size:            "X-Small",
			Comment:         comment,
			Owner:           owner,
			AutoSuspend:     600,
			AutoResume:      true,
			MinClusterCount: 1,
			MaxClusterCount: 1,
			ScalingPolicy:   "STANDARD",
		},
		Parameters: &snowflake.WarehouseParameters{
			MaxConcurrencyLevel:             ptr(int32(8)),
			StatementQueuedTimeoutInSeconds: ptr(int32(0)),
			StatementTimeoutInSeconds:       ptr(int32(172800)),
			EnableQueryAcceleration:         ptr(false),
			QueryAccelerationMaxScaleFactor: ptr(int32(8)),
		},
	}
}

// userObservation returns a standard existing-user observation.
func userObservation(name, comment, owner string) *snowflake.UserObservation {
	return &snowflake.UserObservation{
		Exists: true,
		ShowOutput: &snowflake.UserShowOutput{
			CreatedOn:   "2024-01-01",
			Name:        name,
			LoginName:   name,
			DisplayName: name,
			Comment:     comment,
			Owner:       owner,
			Type:        "PERSON",
			DefaultRole: "PUBLIC",
		},
		DescribeOutput: &snowflake.UserDescribeOutput{},
	}
}

// accountRoleObservation returns a standard existing-account-role observation.
func accountRoleObservation(name, comment, owner string) *snowflake.AccountRoleObservation {
	return &snowflake.AccountRoleObservation{
		Exists: true,
		ShowOutput: &snowflake.AccountRoleShowOutput{
			CreatedOn: "2024-01-01",
			Name:      name,
			Comment:   comment,
			Owner:     owner,
		},
	}
}

// databaseRoleObservation returns a standard existing-database-role observation.
func databaseRoleObservation(name, dbName, comment, owner string) *snowflake.DatabaseRoleObservation {
	return &snowflake.DatabaseRoleObservation{
		Exists: true,
		ShowOutput: &snowflake.DatabaseRoleShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         name,
			DatabaseName: dbName,
			Comment:      comment,
			Owner:        owner,
		},
	}
}

// grantObservation returns a standard existing-grant observation.
func grantObservation(privilege, grantedOn, objectName, grantedTo, granteeName string, grantOption bool) *snowflake.GrantObservation {
	return &snowflake.GrantObservation{
		Exists: true,
		ShowOutput: &snowflake.GrantShowOutput{
			CreatedOn:   "2024-01-01",
			Privilege:   privilege,
			GrantedOn:   grantedOn,
			Name:        objectName,
			GrantedTo:   grantedTo,
			GranteeName: granteeName,
			GrantOption: grantOption,
		},
	}
}

// --------------------------------------------------------------------------
// Setup helpers — create prerequisite resources and wait for Ready
// --------------------------------------------------------------------------

// setupReadyDatabase creates a Database that is Ready, returning a cleanup function.
// This is a prerequisite for DatabaseRole integration tests.
func setupReadyDatabase(t *testing.T, dbK8sName, sfDBName string) (dbKey types.NamespacedName, cleanup func()) {
	t.Helper()

	var dbCreated atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if id.Name() == sfDBName && dbCreated.Load() {
			return databaseObservation(sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		if opts.Name.Name() == sfDBName {
			dbCreated.Store(true)
		}

		return nil
	})

	db := newTestDatabase(dbK8sName, sfDBName)
	require.NoError(t, k8sClient.Create(ctx, db))

	dbKey = types.NamespacedName{Name: dbK8sName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "parent database should become Ready")

	cleanup = func() {
		dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var d snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &d); err == nil {
			_ = k8sClient.Delete(ctx, &d)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, dbKey, &snowplanev1alpha1.Database{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	}

	return dbKey, cleanup
}

// setupReadyDatabaseAndSchema creates a Database and Schema that are both Ready,
// returning cleanup functions and the keys. This is a common prerequisite for
// Table, View, and Stage integration tests.
func setupReadyDatabaseAndSchema(t *testing.T, dbK8sName, sfDBName, schemaK8sName, sfSchemaName string) (
	dbKey types.NamespacedName, schemaKey types.NamespacedName, cleanup func(),
) {
	t.Helper()

	var dbCreated atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if id.Name() == sfDBName && dbCreated.Load() {
			return databaseObservation(sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		if opts.Name.Name() == sfDBName {
			dbCreated.Store(true)
		}

		return nil
	})

	db := newTestDatabase(dbK8sName, sfDBName)
	require.NoError(t, k8sClient.Create(ctx, db))

	dbKey = types.NamespacedName{Name: dbK8sName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "parent database should become Ready")

	var schemaCreated atomic.Bool

	schemaMockSvc.SetObserve(func(_ context.Context, id snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
		if schemaCreated.Load() {
			return schemaObservation(sfSchemaName, sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.SchemaObservation{Exists: false}, nil
	})

	schemaMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSchemaOptions) error {
		schemaCreated.Store(true)

		return nil
	})

	schema := newTestSchema(schemaK8sName, sfSchemaName, dbK8sName)
	require.NoError(t, k8sClient.Create(ctx, schema))

	schemaKey = types.NamespacedName{Name: schemaK8sName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema
		if err := k8sClient.Get(ctx, schemaKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "parent schema should become Ready")

	cleanup = func() {
		schemaMockSvc.SetDrop(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error { return nil })

		var s snowplanev1alpha1.Schema
		if err := k8sClient.Get(ctx, schemaKey, &s); err == nil {
			_ = k8sClient.Delete(ctx, &s)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, schemaKey, &snowplanev1alpha1.Schema{}) != nil
			}, defaultTimeout, defaultInterval)
		}

		dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var d snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &d); err == nil {
			_ = k8sClient.Delete(ctx, &d)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, dbKey, &snowplanev1alpha1.Database{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	}

	return dbKey, schemaKey, cleanup
}
