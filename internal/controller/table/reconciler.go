// Package table implements the reconciler for Table resources.
package table

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/table"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake tables.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error)
	Create(ctx context.Context, opts snowflake.CreateTableOptions) error
	Alter(ctx context.Context, opts snowflake.AlterTableOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Table reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewTableClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for Table resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation]{
		ResourceNameVal:  "table",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.Table { return &snowplanev1alpha1.Table{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(table *snowplanev1alpha1.Table) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(table.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(table.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, table.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.TableObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.Table, id snowflake.SchemaObjectIdentifier) error {
			resolvedFKTables, err := resolveFKTableRefs(ctx, c, obj)
			if err != nil {
				return err
			}

			opts := buildCreateOptions(obj, id, resolvedFKTables)
			opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterTableOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.Table, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.TableObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.Table, obs *reconciler.Observation[*snowflake.TableObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.Table, obs *reconciler.Observation[*snowflake.TableObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		SupportsCoA:      true,
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, table *snowplanev1alpha1.Table) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, table,
				table.Namespace, table.Spec.DatabaseRef, table.Spec.DatabaseName, table.Status.DatabaseName)
			if err != nil {
				return err
			}

			table.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, table,
				table.Namespace, table.Spec.SchemaRef, table.Spec.SchemaName, table.Status.SchemaName)
			if err != nil {
				return err
			}

			table.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(table, table.Spec.DatabaseRef, table.Spec.DatabaseName, table.Spec.SchemaRef, table.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.Table{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					t, ok := o.(*snowplanev1alpha1.Table)
					if !ok || t.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{t.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.Table{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					t, ok := o.(*snowplanev1alpha1.Table)
					if !ok || t.Spec.SchemaRef == nil {
						return nil
					}

					return []string{t.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.TableList{} }, ".spec.databaseRef.name", "listing tables for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.TableList{} }, ".spec.schemaRef.name", "listing tables for schema watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, table *snowplanev1alpha1.Table) error {
	if reconciler.ShouldSkipImmutableValidation(table) {
		return nil
	}

	if table.Status.ShowOutput != nil {
		if table.Status.ShowOutput.Name != "" && !strings.EqualFold(table.Spec.Name, table.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", table.Status.ShowOutput.Name, table.Spec.Name)
		}

		isTransient := table.Status.ShowOutput.Kind == "TRANSIENT"
		if table.Spec.Transient != isTransient {
			return fmt.Errorf("spec.transient is immutable after creation (current: %v, desired: %v)", isTransient, table.Spec.Transient)
		}

		if table.Status.ShowOutput.DatabaseName != "" && table.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(table.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, table.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", table.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if table.Status.ShowOutput.SchemaName != "" && table.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(table.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, table.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", table.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

	}

	return nil
}

func applyObservation(table *snowplanev1alpha1.Table, obs *snowflake.TableObservation) {
	if obs.ShowOutput != nil {
		table.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		table.Status.DatabaseName = obs.ShowOutput.DatabaseName
		table.Status.SchemaName = obs.ShowOutput.SchemaName

		table.Status.ShowOutput = obs.ShowOutput
	}
}

// resolveFKTableRefs resolves all foreign key tableRef references in the
// table's constraints, returning a map of constraint index → resolved FQN.
func resolveFKTableRefs(ctx context.Context, c sigs.Client, table *snowplanev1alpha1.Table) (map[int]string, error) {
	resolved := make(map[int]string)

	for i, constraint := range table.Spec.Constraints {
		if constraint.Type != snowplanev1alpha1.TableConstraintForeignKey || constraint.ForeignKey == nil {
			continue
		}

		if constraint.ForeignKey.TableRef != nil {
			fqn, err := refresolver.ResolveTableRef(ctx, c, table.Namespace, *constraint.ForeignKey.TableRef)
			if err != nil {
				return nil, fmt.Errorf("resolving foreignKey.tableRef in constraint %d: %w", i, err)
			}

			resolved[i] = fqn
		}
	}

	return resolved, nil
}

func buildCreateOptions(table *snowplanev1alpha1.Table, id snowflake.SchemaObjectIdentifier, resolvedFKTables map[int]string) snowflake.CreateTableOptions {
	columns := make([]snowflake.CreateTableColumn, len(table.Spec.Columns))
	for i, col := range table.Spec.Columns {
		columns[i] = snowflake.CreateTableColumn{
			Name:     col.Name,
			Type:     col.Type,
			Nullable: col.Nullable,
			Default:  col.Default,
			Comment:  col.Comment,
		}
	}

	constraints := make([]snowflake.CreateTableConstraint, len(table.Spec.Constraints))
	for i, c := range table.Spec.Constraints {
		ct := snowflake.CreateTableConstraint{
			Name:    c.Name,
			Columns: c.Columns,
		}

		switch c.Type {
		case snowplanev1alpha1.TableConstraintPrimaryKey:
			ct.Type = "PRIMARY KEY"
		case snowplanev1alpha1.TableConstraintUnique:
			ct.Type = "UNIQUE"
		case snowplanev1alpha1.TableConstraintForeignKey:
			ct.Type = "FOREIGN KEY"
			if c.ForeignKey != nil {
				// Prefer resolved tableRef FQN; fall back to inline table name.
				if fqn, ok := resolvedFKTables[i]; ok {
					ct.ForeignKeyTable = fqn
				} else if c.ForeignKey.Table != nil {
					ct.ForeignKeyTable = *c.ForeignKey.Table
				}

				ct.ForeignKeyColumns = c.ForeignKey.Columns
			}
		}

		constraints[i] = ct
	}

	return snowflake.CreateTableOptions{
		Name:                       id,
		Columns:                    columns,
		Constraints:                constraints,
		Comment:                    table.Spec.Comment,
		Transient:                  table.Spec.Transient,
		DataRetentionTimeInDays:    table.Spec.DataRetentionTimeInDays,
		MaxDataExtensionTimeInDays: table.Spec.MaxDataExtensionTimeInDays,
		ChangeTracking:             table.Spec.ChangeTracking,
		DefaultDDLCollation:        table.Spec.DefaultDDLCollation,
		EnableSchemaEvolution:      table.Spec.EnableSchemaEvolution,
		ClusterBy:                  table.Spec.ClusterBy,
	}
}

func buildAlterOptions(table *snowplanev1alpha1.Table, id snowflake.SchemaObjectIdentifier, obs *snowflake.TableObservation) snowflake.AlterTableOptions {
	opts := snowflake.AlterTableOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&table.Spec, table.Status.TrackedParameters)

	if table.Spec.Comment != nil {
		if obs.ShowOutput == nil || *table.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = table.Spec.Comment
		}
	}

	if obs.ShowOutput != nil {
		// Change tracking: compare bool values.
		if table.Spec.ChangeTracking != nil {
			if *table.Spec.ChangeTracking != obs.ShowOutput.ChangeTracking {
				opts.ChangeTracking = table.Spec.ChangeTracking
			}
		}

		// Schema evolution: compare bool values.
		if table.Spec.EnableSchemaEvolution != nil {
			if *table.Spec.EnableSchemaEvolution != obs.ShowOutput.EnableSchemaEvolution {
				opts.EnableSchemaEvolution = table.Spec.EnableSchemaEvolution
			}
		}

		// Clustering key.
		desiredCluster := ""
		if len(table.Spec.ClusterBy) > 0 {
			desiredCluster = "(" + joinClusterBy(table.Spec.ClusterBy) + ")"
		}

		observedCluster := obs.ShowOutput.ClusterBy
		if desiredCluster != observedCluster {
			if desiredCluster == "" && observedCluster != "" {
				opts.DropClusteringKey = true
			} else if desiredCluster != "" {
				opts.ClusterBy = table.Spec.ClusterBy
			}
		}
	}

	if table.Spec.DataRetentionTimeInDays != nil {
		if obs.ShowOutput == nil || obs.ShowOutput.RetentionTime == 0 {
			opts.DataRetentionTimeInDays = table.Spec.DataRetentionTimeInDays
		}
	}

	if table.Spec.MaxDataExtensionTimeInDays != nil {
		opts.MaxDataExtensionTimeInDays = table.Spec.MaxDataExtensionTimeInDays
	}

	if table.Spec.DefaultDDLCollation != nil {
		opts.DefaultDDLCollation = table.Spec.DefaultDDLCollation
	}

	// Compute column changes (add/drop/alter).
	add, drop, alter := computeColumnChanges(table.Spec.Columns, obs.Columns)
	opts.AddColumns = add
	opts.DropColumns = drop
	opts.AlterColumns = alter

	return opts
}

func joinClusterBy(exprs []string) string {
	result := ""
	for i, e := range exprs {
		if i > 0 {
			result += ", "
		}

		result += e
	}

	return result
}

// computeColumnChanges compares spec columns against observed columns
// and returns the add, drop, and alter actions needed.
func computeColumnChanges(
	specCols []snowplanev1alpha1.ColumnDefinition,
	obsCols []snowflake.ColumnInfo,
) ([]snowflake.CreateTableColumn, []string, []snowflake.AlterColumnAction) {
	// Build observed lookup by uppercase name.
	obsMap := make(map[string]snowflake.ColumnInfo, len(obsCols))
	for _, c := range obsCols {
		obsMap[strings.ToUpper(c.Name)] = c
	}

	specNames := make(map[string]bool, len(specCols))

	var addCols []snowflake.CreateTableColumn
	var alterCols []snowflake.AlterColumnAction

	for _, sc := range specCols {
		upperName := strings.ToUpper(sc.Name)
		specNames[upperName] = true

		oc, found := obsMap[upperName]
		if !found {
			// New column — ADD.
			addCols = append(addCols, snowflake.CreateTableColumn{
				Name:     sc.Name,
				Type:     sc.Type,
				Nullable: sc.Nullable,
				Default:  sc.Default,
				Comment:  sc.Comment,
			})

			continue
		}

		// Existing column — check for modifications.
		action := snowflake.AlterColumnAction{Name: sc.Name}
		hasChange := false

		// Type change.
		if !strings.EqualFold(normaliseType(sc.Type), normaliseType(oc.Type)) {
			action.SetType = &sc.Type
			hasChange = true
		}

		// Nullable change.
		desiredNullable := sc.Nullable == nil || *sc.Nullable
		observedNullable := oc.Null == "Y"

		if desiredNullable != observedNullable {
			notNull := !desiredNullable
			action.SetNotNull = &notNull
			hasChange = true
		}

		// Comment change.
		desiredComment := ""
		if sc.Comment != nil {
			desiredComment = *sc.Comment
		}

		if desiredComment != oc.Comment {
			action.SetComment = &desiredComment
			hasChange = true
		}

		// Default change.
		desiredDefault := ""
		if sc.Default != nil {
			desiredDefault = *sc.Default
		}

		if desiredDefault != oc.Default {
			if sc.Default == nil && oc.Default != "" {
				action.DropDefault = true
				hasChange = true
			} else if sc.Default != nil && *sc.Default != oc.Default {
				action.SetDefault = sc.Default
				hasChange = true
			}
		}

		if hasChange {
			alterCols = append(alterCols, action)
		}
	}

	// Columns in Snowflake but not in spec — DROP.
	var dropCols []string

	for _, oc := range obsCols {
		if !specNames[strings.ToUpper(oc.Name)] {
			dropCols = append(dropCols, oc.Name)
		}
	}

	return addCols, dropCols, alterCols
}

func detectDrift(table *snowplanev1alpha1.Table, obs *snowflake.TableObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", table.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(table.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(table.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		isTransient := obs.ShowOutput.Kind == "TRANSIENT"
		d.CompareBoolValue("TRANSIENT", table.Spec.Transient, isTransient, true)

		// Mutable fields.
		d.CompareString("COMMENT", table.Spec.Comment, obs.ShowOutput.Comment, false)

		if table.Spec.ChangeTracking != nil {
			d.CompareBoolValue("CHANGE_TRACKING", *table.Spec.ChangeTracking, obs.ShowOutput.ChangeTracking, false)
		}

		if table.Spec.EnableSchemaEvolution != nil {
			d.CompareBoolValue("ENABLE_SCHEMA_EVOLUTION", *table.Spec.EnableSchemaEvolution, obs.ShowOutput.EnableSchemaEvolution, false)
		}
	}

	// Column drift detection.
	detectColumnDrift(d, table.Spec.Columns, obs.Columns)

	return d.Result()
}

// detectColumnDrift compares desired spec columns against observed DESCRIBE TABLE output.
func detectColumnDrift(d *drift.Detector, specCols []snowplanev1alpha1.ColumnDefinition, obsCols []snowflake.ColumnInfo) {
	// Build lookup from observed columns by uppercase name.
	obsMap := make(map[string]snowflake.ColumnInfo, len(obsCols))
	for _, c := range obsCols {
		obsMap[strings.ToUpper(c.Name)] = c
	}

	specNames := make(map[string]bool, len(specCols))

	for _, sc := range specCols {
		upperName := strings.ToUpper(sc.Name)
		specNames[upperName] = true

		oc, found := obsMap[upperName]
		if !found {
			// Column exists in spec but not in Snowflake — drift (will be added).
			d.CompareStringValue("COLUMN."+sc.Name, sc.Name, "<missing>", false)
			continue
		}

		// Compare type — Snowflake normalises types, so use case-insensitive.
		if !strings.EqualFold(normaliseType(sc.Type), normaliseType(oc.Type)) {
			d.CompareStringValueFold("COLUMN."+sc.Name+".TYPE", sc.Type, oc.Type, false)
		}

		// Compare nullable.
		desiredNullable := sc.Nullable == nil || *sc.Nullable // default true
		observedNullable := oc.Null == "Y"

		if desiredNullable != observedNullable {
			d.CompareBoolValue("COLUMN."+sc.Name+".NULLABLE", desiredNullable, observedNullable, false)
		}

		// Compare comment.
		desiredComment := ""
		if sc.Comment != nil {
			desiredComment = *sc.Comment
		}

		if desiredComment != oc.Comment {
			d.CompareStringValue("COLUMN."+sc.Name+".COMMENT", desiredComment, oc.Comment, false)
		}
	}

	// Columns that exist in Snowflake but not in the spec — drift (will be dropped).
	for _, oc := range obsCols {
		if !specNames[strings.ToUpper(oc.Name)] {
			d.CompareStringValue("COLUMN."+oc.Name, "<removed>", oc.Name, false)
		}
	}
}

// normaliseType canonicalises a Snowflake column type string for comparison.
// It strips whitespace around parentheses and expands common type aliases
// to their canonical Snowflake forms so that user-specified types like
// "VARCHAR" don't trigger false drift against Snowflake's "VARCHAR(16777216)".
func normaliseType(t string) string {
	t = strings.TrimSpace(t)
	t = strings.ReplaceAll(t, " (", "(")
	t = strings.ReplaceAll(t, "( ", "(")
	t = strings.ReplaceAll(t, " )", ")")
	t = strings.ToUpper(t)

	// Expand common Snowflake type aliases to their canonical forms.
	// Only expand types WITHOUT explicit parameters — if the user wrote
	// "VARCHAR(100)" we must keep it as-is.
	if !strings.Contains(t, "(") {
		switch t {
		case "VARCHAR", "STRING", "TEXT":
			return "VARCHAR(16777216)"
		case "CHAR", "CHARACTER":
			return "VARCHAR(1)"
		case "INT", "INTEGER", "BIGINT", "SMALLINT", "TINYINT", "BYTEINT":
			return "NUMBER(38,0)"
		case "FLOAT", "FLOAT4", "FLOAT8", "DOUBLE", "DOUBLE PRECISION", "REAL":
			return "FLOAT"
		case "BOOLEAN", "BOOL":
			return "BOOLEAN"
		case "TIMESTAMP", "DATETIME", "TIMESTAMP_NTZ":
			return "TIMESTAMP_NTZ(9)"
		case "TIMESTAMP_LTZ":
			return "TIMESTAMP_LTZ(9)"
		case "TIMESTAMP_TZ":
			return "TIMESTAMP_TZ(9)"
		case "DATE":
			return "DATE"
		case "TIME":
			return "TIME(9)"
		case "BINARY", "VARBINARY":
			return "BINARY(8388608)"
		case "NUMBER", "NUMERIC", "DECIMAL", "DEC":
			return "NUMBER(38,0)"
		}
	}

	return t
}
