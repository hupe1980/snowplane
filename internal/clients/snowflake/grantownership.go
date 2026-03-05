package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// GrantOwnershipObservation holds the result of observing ownership of an object.
type GrantOwnershipObservation struct {
	// Exists indicates whether the expected ownership was found.
	Exists bool

	// ShowOutput contains the SHOW GRANTS row for the OWNERSHIP privilege.
	ShowOutput *v1alpha1.GrantOwnershipShowOutput
}

// GrantOwnershipIdentifier identifies the ownership grant to observe.
type GrantOwnershipIdentifier struct {
	// ObjectType is the Snowflake object type (e.g. DATABASE, TABLE).
	ObjectType string

	// ObjectName is the fully qualified object name.
	ObjectName string

	// GranteeName is the expected owner role name.
	GranteeName string
}

// FullyQualifiedName returns a composite identifier string.
func (id GrantOwnershipIdentifier) FullyQualifiedName() string {
	return fmt.Sprintf("OWNERSHIP ON %s %s -> %s", id.ObjectType, id.ObjectName, id.GranteeName)
}

// String returns the identifier as a human-readable string.
func (id GrantOwnershipIdentifier) String() string {
	return id.FullyQualifiedName()
}

// CreateGrantOwnershipOptions holds the parameters for transferring ownership.
type CreateGrantOwnershipOptions struct {
	ObjectType            string
	ObjectName            string
	ToRole                string // "ROLE <name>" or "DATABASE ROLE <db>.<name>"
	CurrentGrantsBehavior string // "COPY" or "REVOKE" or ""
}

// Validate checks the CreateGrantOwnershipOptions for validity.
func (o *CreateGrantOwnershipOptions) Validate() error {
	if o.ObjectType == "" {
		return fmt.Errorf("object type is required")
	}

	// Defense-in-depth: validate object type against allowlist even though CRD validation fires first.
	if err := sqlbuilder.ValidateObjectType(o.ObjectType); err != nil {
		return fmt.Errorf("invalid object type: %w", err)
	}

	if o.ObjectName == "" {
		return fmt.Errorf("object name is required")
	}

	// Defense-in-depth: reject dangerous SQL metacharacters in the pre-quoted object name.
	if err := sqlbuilder.ValidateDollarQuotedValue(o.ObjectName); err != nil {
		return fmt.Errorf("invalid object name: %w", err)
	}

	if o.ToRole == "" {
		return fmt.Errorf("target role is required")
	}

	// Validate CurrentGrantsBehavior if set (must be a keyword-safe value).
	if o.CurrentGrantsBehavior != "" {
		if err := sqlbuilder.ValidateKeywordValue(o.CurrentGrantsBehavior); err != nil {
			return fmt.Errorf("invalid current grants behavior: %w", err)
		}
	}

	return nil
}

// GrantOwnershipClient provides operations against Snowflake ownership transfers.
type GrantOwnershipClient struct {
	client SQLExecutor
}

// NewGrantOwnershipClient creates a new GrantOwnershipClient.
func NewGrantOwnershipClient(c SQLExecutor) *GrantOwnershipClient {
	return &GrantOwnershipClient{client: c}
}

// buildGrantOwnershipSQL builds the GRANT OWNERSHIP SQL statement.
// Callers must call Validate() first to ensure ObjectType is a known keyword.
func buildGrantOwnershipSQL(opts CreateGrantOwnershipOptions) string {
	var b sqlbuilder.Builder
	b.WriteString("GRANT OWNERSHIP ON ")
	b.WriteString(opts.ObjectType)
	b.WriteString(" ")
	b.WriteString(opts.ObjectName) // pre-quoted from CRD; validated in Validate()
	b.WriteString(" TO ")
	b.WriteString(opts.ToRole)

	if opts.CurrentGrantsBehavior != "" {
		b.WriteString(" ")
		b.WriteString(opts.CurrentGrantsBehavior)
		b.WriteString(" CURRENT GRANTS")
	}

	return b.String()
}

// Create executes a GRANT OWNERSHIP statement.
func (g *GrantOwnershipClient) Create(ctx context.Context, opts CreateGrantOwnershipOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid grant ownership options: %w", err))
	}

	stmt := buildGrantOwnershipSQL(opts)

	if _, err := g.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("granting ownership on %s %s: %w", opts.ObjectType, opts.ObjectName, err)
	}

	return nil
}

// Drop is a no-op for ownership transfers — ownership cannot be revoked,
// only transferred to another role.
func (g *GrantOwnershipClient) Drop(_ context.Context, _ GrantOwnershipIdentifier) error {
	return nil
}

// Observe checks whether the expected role owns the specified object.
func (g *GrantOwnershipClient) Observe(ctx context.Context, id GrantOwnershipIdentifier) (*GrantOwnershipObservation, error) {
	if id.ObjectType == "" || id.ObjectName == "" {
		return nil, NewTerminalError(fmt.Errorf("object type and name are required"))
	}

	// Defense-in-depth: validate object type even though CRD validates it.
	if err := sqlbuilder.ValidateObjectType(id.ObjectType); err != nil {
		return nil, NewTerminalError(fmt.Errorf("invalid object type: %w", err))
	}

	query := fmt.Sprintf("SHOW GRANTS ON %s %s", id.ObjectType, id.ObjectName) // ObjectName pre-quoted

	rows, err := g.client.Query(ctx, query)
	if err != nil {
		if IsObjectNotFound(err) {
			return &GrantOwnershipObservation{Exists: false}, nil
		}

		return nil, fmt.Errorf("showing grants on %s %s: %w", id.ObjectType, id.ObjectName, err)
	}
	defer closeRows(rows)

	return scanOwnershipGrant(rows, id.GranteeName)
}

// scanOwnershipGrant scans SHOW GRANTS results for the OWNERSHIP privilege.
func scanOwnershipGrant(rows *sql.Rows, expectedGrantee string) (*GrantOwnershipObservation, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading columns: %w", err)
	}

	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))

		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		colMap := make(map[string]string, len(cols))
		for i, col := range cols {
			if values[i].Valid {
				colMap[col] = values[i].String
			}
		}

		if !strings.EqualFold(colMap["privilege"], "OWNERSHIP") {
			continue
		}

		out := &v1alpha1.GrantOwnershipShowOutput{
			CreatedOn:   colMap["created_on"],
			Privilege:   colMap["privilege"],
			GrantedOn:   colMap["granted_on"],
			Name:        colMap["name"],
			GrantedTo:   colMap["granted_to"],
			GranteeName: colMap["grantee_name"],
		}

		if strings.EqualFold(out.GranteeName, expectedGrantee) {
			return &GrantOwnershipObservation{
				Exists:     true,
				ShowOutput: out,
			}, nil
		}

		// OWNERSHIP found but owned by a different role.
		return &GrantOwnershipObservation{
			Exists:     false,
			ShowOutput: out,
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return &GrantOwnershipObservation{Exists: false}, nil
}
