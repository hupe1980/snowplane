// Package snowflake provides Snowflake client implementations.
package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// ShareObservation is the observation result for a Snowflake share.
type ShareObservation struct {
	Exists     bool
	ShowOutput *snowplanev1alpha1.ShareShowOutput
}

// CreateShareOptions defines options for CREATE SHARE.
type CreateShareOptions struct {
	Name    AccountObjectIdentifier
	Comment *string
}

// Validate checks the CreateShareOptions for validity.
func (o *CreateShareOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("share name is required")
	}

	return nil
}

// AlterShareOptions defines options for ALTER SHARE.
type AlterShareOptions struct {
	Name        AccountObjectIdentifier
	Comment     *string
	AddAccounts []string
	RemAccounts []string
	UnsetFields []string
}

// Validate checks the AlterShareOptions for validity.
func (o *AlterShareOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("share name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterShareOptions) HasChanges() bool {
	return o.Comment != nil || len(o.AddAccounts) > 0 || len(o.RemAccounts) > 0 || len(o.UnsetFields) > 0
}

// ShareClient talks to Snowflake for Share CRUD.
type ShareClient struct {
	client SQLExecutor
}

// NewShareClient creates a new ShareClient.
func NewShareClient(c SQLExecutor) *ShareClient {
	return &ShareClient{client: c}
}

// buildShowShareByIDSQL builds a SHOW SHARES LIKE query.
func buildShowShareByIDSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowLike("SHARES", name.Name())
}

// ShowByID queries SHOW SHARES for a specific share.
func (c *ShareClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*snowplanev1alpha1.ShareShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("share name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowShareByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing share %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanShareShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a ShareObservation.
func (c *ShareClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*ShareObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) || IsObjectNotExistOrNotAuthorized(err) {
			return &ShareObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &ShareObservation{Exists: true, ShowOutput: show}, nil
}

// scanShareShowOutput scans SHOW SHARES results for a matching row.
func scanShareShowOutput(rows *sql.Rows, name string) (*snowplanev1alpha1.ShareShowOutput, error) {
	return ScanShowOutput(rows, name, mapShareShowOutput)
}

func mapShareShowOutput(cols map[string]string) (*snowplanev1alpha1.ShareShowOutput, error) {
	return &snowplanev1alpha1.ShareShowOutput{
		CreatedOn:    cols["created_on"],
		Kind:         cols["kind"],
		Name:         cols["name"],
		DatabaseName: cols["database_name"],
		To:           cols["to"],
		Owner:        cols["owner"],
		Comment:      cols["comment"],
		ListingType:  cols["listing_type"],
	}, nil
}

// Create creates a new share.
func (c *ShareClient) Create(ctx context.Context, opts CreateShareOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "CREATE SHARE %s", opts.Name.FullyQualifiedName())

	if opts.Comment != nil {
		fmt.Fprintf(&b, " COMMENT = '%s'", sqlbuilder.EscapeString(*opts.Comment))
	}

	if _, err := c.client.Exec(ctx, b.String()); err != nil {
		return fmt.Errorf("creating share %s: %w", opts.Name, err)
	}

	return nil
}

// validateAccountIdentifier checks that a Snowflake account identifier
// (ORG.ACCOUNT format) contains only safe characters: letters, digits,
// underscores, hyphens, and dots.
func validateAccountIdentifier(s string) error {
	if s == "" {
		return fmt.Errorf("account identifier must not be empty")
	}

	for _, c := range s {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '-' && c != '.' {
			return fmt.Errorf("invalid character %q in account identifier %q", string(c), s)
		}
	}

	return nil
}

// validateAccountIdentifiers validates each account identifier for safe SQL use.
func validateAccountIdentifiers(accounts []string) error {
	for _, a := range accounts {
		if err := validateAccountIdentifier(a); err != nil {
			return err
		}
	}

	return nil
}

// Alter modifies an existing share.
func (c *ShareClient) Alter(ctx context.Context, opts AlterShareOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(err)
	}

	fqn := opts.Name.FullyQualifiedName()

	if opts.Comment != nil {
		q := fmt.Sprintf("ALTER SHARE %s SET COMMENT = '%s'",
			fqn, sqlbuilder.EscapeString(*opts.Comment))
		if _, err := c.client.Exec(ctx, q); err != nil {
			return fmt.Errorf("altering share %s SET COMMENT: %w", opts.Name, err)
		}
	}

	if len(opts.AddAccounts) > 0 {
		if err := validateAccountIdentifiers(opts.AddAccounts); err != nil {
			return NewTerminalError(err)
		}

		q := fmt.Sprintf("ALTER SHARE %s ADD ACCOUNTS = %s",
			fqn, strings.Join(opts.AddAccounts, ", "))
		if _, err := c.client.Exec(ctx, q); err != nil {
			return fmt.Errorf("altering share %s ADD ACCOUNTS: %w", opts.Name, err)
		}
	}

	if len(opts.RemAccounts) > 0 {
		if err := validateAccountIdentifiers(opts.RemAccounts); err != nil {
			return NewTerminalError(err)
		}

		q := fmt.Sprintf("ALTER SHARE %s REMOVE ACCOUNTS = %s",
			fqn, strings.Join(opts.RemAccounts, ", "))
		if _, err := c.client.Exec(ctx, q); err != nil {
			return fmt.Errorf("altering share %s REMOVE ACCOUNTS: %w", opts.Name, err)
		}
	}

	if len(opts.UnsetFields) > 0 {
		q, err := sqlbuilder.BuildUnset("SHARE", fqn, opts.UnsetFields)
		if err != nil {
			return NewTerminalError(err)
		}

		if q != "" {
			if _, err := c.client.Exec(ctx, q); err != nil {
				return fmt.Errorf("altering share %s UNSET: %w", opts.Name, err)
			}
		}
	}

	return nil
}

// Drop removes a share.
func (c *ShareClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("share name is required"))
	}

	q := sqlbuilder.DropIfExists("SHARE", name.FullyQualifiedName())

	if _, err := c.client.Exec(ctx, q); err != nil {
		return fmt.Errorf("dropping share %s: %w", name, err)
	}

	return nil
}
