// Package table implements the reconciler for Table resources.
package table

import (
	"context"
	"strings"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
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
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// defaultServiceFactory is the production ServiceFactory.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewTableClient(sfC), cleanup, nil
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

		table.Status.ShowOutput = &snowplanev1alpha1.TableShowOutput{
			CreatedOn:             obs.ShowOutput.CreatedOn,
			Name:                  obs.ShowOutput.Name,
			DatabaseName:          obs.ShowOutput.DatabaseName,
			SchemaName:            obs.ShowOutput.SchemaName,
			Kind:                  obs.ShowOutput.Kind,
			Comment:               obs.ShowOutput.Comment,
			Owner:                 obs.ShowOutput.Owner,
			RetentionTime:         obs.ShowOutput.RetentionTime,
			ClusterBy:             obs.ShowOutput.ClusterBy,
			ChangeTracking:        obs.ShowOutput.ChangeTracking,
			EnableSchemaEvolution: obs.ShowOutput.EnableSchemaEvolution,
		}
	}
}

func buildCreateOptions(table *snowplanev1alpha1.Table, id snowflake.SchemaObjectIdentifier) snowflake.CreateTableOptions {
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
				ct.ForeignKeyTable = c.ForeignKey.Table
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
	opts.UnsetFields = computeUnsetFields(table)

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
			action.SetNotNull = ptrBool(!desiredNullable)
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

func ptrBool(b bool) *bool { return &b }

func computeUnsetFields(table *snowplanev1alpha1.Table) []string {
	if len(table.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(table.Status.TrackedParameters))
	for _, f := range table.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if table.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	if table.Spec.DataRetentionTimeInDays == nil && managed["DATA_RETENTION_TIME_IN_DAYS"] {
		unset = append(unset, "DATA_RETENTION_TIME_IN_DAYS")
	}

	if table.Spec.MaxDataExtensionTimeInDays == nil && managed["MAX_DATA_EXTENSION_TIME_IN_DAYS"] {
		unset = append(unset, "MAX_DATA_EXTENSION_TIME_IN_DAYS")
	}

	if table.Spec.ChangeTracking == nil && managed["CHANGE_TRACKING"] {
		unset = append(unset, "CHANGE_TRACKING")
	}

	if table.Spec.DefaultDDLCollation == nil && managed["DEFAULT_DDL_COLLATION"] {
		unset = append(unset, "DEFAULT_DDL_COLLATION")
	}

	if table.Spec.EnableSchemaEvolution == nil && managed["ENABLE_SCHEMA_EVOLUTION"] {
		unset = append(unset, "ENABLE_SCHEMA_EVOLUTION")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.TableSpec) []string {
	var fields []string

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	if spec.DataRetentionTimeInDays != nil {
		fields = append(fields, "DATA_RETENTION_TIME_IN_DAYS")
	}

	if spec.MaxDataExtensionTimeInDays != nil {
		fields = append(fields, "MAX_DATA_EXTENSION_TIME_IN_DAYS")
	}

	if spec.ChangeTracking != nil {
		fields = append(fields, "CHANGE_TRACKING")
	}

	if spec.DefaultDDLCollation != nil {
		fields = append(fields, "DEFAULT_DDL_COLLATION")
	}

	if spec.EnableSchemaEvolution != nil {
		fields = append(fields, "ENABLE_SCHEMA_EVOLUTION")
	}

	return fields
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

// normaliseType strips whitespace around parentheses for column type comparison.
func normaliseType(t string) string {
	t = strings.TrimSpace(t)
	t = strings.ReplaceAll(t, " (", "(")
	t = strings.ReplaceAll(t, "( ", "(")
	t = strings.ReplaceAll(t, " )", ")")

	return t
}
