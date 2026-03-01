package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// TagAssociationObservation holds the result of observing a Snowflake tag association.
type TagAssociationObservation struct {
	// Exists indicates whether the tag is currently set on the object.
	Exists bool

	// TagValue is the current tag value, as returned by SYSTEM$GET_TAG.
	TagValue string
}

// TagAssociationIdentifier uniquely identifies a tag association.
type TagAssociationIdentifier struct {
	// TagName is the fully qualified tag name (e.g. "DB"."SCHEMA"."TAG").
	TagName string

	// ObjectType is the Snowflake object type (e.g. "TABLE", "WAREHOUSE").
	ObjectType string

	// ObjectName is the fully qualified object name.
	ObjectName string
}

// FullyQualifiedName returns a human-readable representation of the tag association.
func (id TagAssociationIdentifier) FullyQualifiedName() string {
	return fmt.Sprintf("TAG %s ON %s %s", id.TagName, id.ObjectType, id.ObjectName)
}

// String returns the fully qualified name.
func (id TagAssociationIdentifier) String() string { return id.FullyQualifiedName() }

// objectDomain maps spec-level object types to the domain string required by
// SYSTEM$GET_TAG. Snowflake overloads "TABLE" for views & materialized views,
// and uses "ROLE" for account- and database-level roles.
func objectDomain(objectType string) string {
	switch objectType {
	case "VIEW":
		return "TABLE"
	case "DATABASE ROLE":
		return "DATABASE ROLE"
	default:
		return objectType
	}
}

// SetTagOptions holds the parameters for setting a tag on an object.
type SetTagOptions struct {
	TagName    string // fully qualified tag name
	TagValue   string
	ObjectType string // e.g. "TABLE", "WAREHOUSE"
	ObjectName string // fully qualified object name
}

// Validate checks the SetTagOptions.
func (o *SetTagOptions) Validate() error {
	if o.TagName == "" {
		return fmt.Errorf("tag name is required")
	}

	if o.ObjectType == "" {
		return fmt.Errorf("object type is required")
	}

	if o.ObjectName == "" {
		return fmt.Errorf("object name is required")
	}

	return nil
}

// UnsetTagOptions holds the parameters for unsetting a tag from an object.
type UnsetTagOptions struct {
	TagName    string // fully qualified tag name
	ObjectType string
	ObjectName string
}

// Validate checks the UnsetTagOptions.
func (o *UnsetTagOptions) Validate() error {
	if o.TagName == "" {
		return fmt.Errorf("tag name is required")
	}

	if o.ObjectType == "" {
		return fmt.Errorf("object type is required")
	}

	if o.ObjectName == "" {
		return fmt.Errorf("object name is required")
	}

	return nil
}

// TagAssociationClient provides operations against Snowflake tag associations.
type TagAssociationClient struct {
	client SQLExecutor
}

// NewTagAssociationClient creates a new TagAssociationClient.
func NewTagAssociationClient(c SQLExecutor) *TagAssociationClient {
	return &TagAssociationClient{client: c}
}

// buildSetTagSQL builds the ALTER ... SET TAG SQL statement.
// Example: ALTER TABLE "DB"."SCH"."TBL" SET TAG "DB"."SCH"."MY_TAG" = 'value'
func buildSetTagSQL(opts SetTagOptions) string {
	return fmt.Sprintf("ALTER %s %s SET TAG %s = '%s'",
		opts.ObjectType,
		opts.ObjectName,
		opts.TagName,
		sqlbuilder.EscapeString(opts.TagValue),
	)
}

// buildUnsetTagSQL builds the ALTER ... UNSET TAG SQL statement.
// Example: ALTER TABLE "DB"."SCH"."TBL" UNSET TAG "DB"."SCH"."MY_TAG"
func buildUnsetTagSQL(opts UnsetTagOptions) string {
	return fmt.Sprintf("ALTER %s %s UNSET TAG %s",
		opts.ObjectType,
		opts.ObjectName,
		opts.TagName,
	)
}

// buildGetTagSQL builds a SELECT SYSTEM$GET_TAG query.
// The function returns the tag value as a string, or NULL if the tag is not set.
func buildGetTagSQL(id TagAssociationIdentifier) string {
	return fmt.Sprintf("SELECT SYSTEM$GET_TAG('%s', '%s', '%s')",
		sqlbuilder.EscapeString(id.TagName),
		sqlbuilder.EscapeString(id.ObjectName),
		sqlbuilder.EscapeString(objectDomain(id.ObjectType)),
	)
}

// Observe queries Snowflake to check if a tag is set on an object and its current value.
func (c *TagAssociationClient) Observe(ctx context.Context, id TagAssociationIdentifier) (*TagAssociationObservation, error) {
	var tagValue sql.NullString

	if err := c.client.QueryRow(ctx, buildGetTagSQL(id)).Scan(&tagValue); err != nil {
		// SYSTEM$GET_TAG may return a SQL compilation error if the object
		// does not exist or the tag does not exist. Treat these as "not found".
		if IsObjectNotExistOrNotAuthorized(err) || errors.Is(err, ErrSQLCompilation) {
			return &TagAssociationObservation{Exists: false}, nil
		}

		return nil, fmt.Errorf("observing tag association %s: %w", id, err)
	}

	if !tagValue.Valid {
		return &TagAssociationObservation{Exists: false}, nil
	}

	return &TagAssociationObservation{
		Exists:   true,
		TagValue: tagValue.String,
	}, nil
}

// SetTag sets or updates a tag value on a Snowflake object.
func (c *TagAssociationClient) SetTag(ctx context.Context, opts SetTagOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid set tag options: %w", err))
	}

	if _, err := c.client.Exec(ctx, buildSetTagSQL(opts)); err != nil {
		return fmt.Errorf("setting tag %s on %s %s: %w", opts.TagName, opts.ObjectType, opts.ObjectName, err)
	}

	return nil
}

// UnsetTag removes a tag from a Snowflake object.
func (c *TagAssociationClient) UnsetTag(ctx context.Context, opts UnsetTagOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid unset tag options: %w", err))
	}

	if _, err := c.client.Exec(ctx, buildUnsetTagSQL(opts)); err != nil {
		// If the tag or object doesn't exist, treat as already gone.
		if IsObjectNotExistOrNotAuthorized(err) {
			return nil
		}

		return fmt.Errorf("unsetting tag %s from %s %s: %w", opts.TagName, opts.ObjectType, opts.ObjectName, err)
	}

	return nil
}
