// Package user implements the reconciler for User resources.
package user

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/user"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake users.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error)
	Create(ctx context.Context, opts snowflake.CreateUserOptions) error
	Alter(ctx context.Context, opts snowflake.AlterUserOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new User reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.User, Service, *snowflake.UserObservation] {
	a := &adapter{client: c, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.User, Service, *snowflake.UserObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory. This is intended for integration tests that
// inject mock Snowflake services while still going through SetupWithManager.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.User, Service, *snowflake.UserObservation] {
	a := &adapter{client: c, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.User, Service, *snowflake.UserObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// defaultServiceFactory is the production ServiceFactory used by NewReconciler.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewUserClient(sfC), cleanup, nil
}

// resolveSecretValue reads a secret key reference and returns the plaintext value.
func resolveSecretValue(ctx context.Context, c client.Client, namespace string, ref *snowplanev1alpha1.SecretKeyReference) (string, error) {
	if ref == nil {
		return "", nil
	}

	return refresolver.ResolveSecretKeyRef(ctx, c, namespace, *ref)
}

// hashSecret returns the hex-encoded HMAC-SHA256 of a secret value keyed by
// the resource UID. Using HMAC instead of a plain hash prevents an attacker
// who can read status from pre-computing rainbow tables: the UID acts as a
// per-resource key that makes each hash unique even for identical passwords
// across different User resources.
func hashSecret(value, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(value))

	return hex.EncodeToString(mac.Sum(nil))
}

func buildCreateOptions(ctx context.Context, c client.Client, user *snowplanev1alpha1.User, id snowflake.AccountObjectIdentifier) (snowflake.CreateUserOptions, error) {
	opts := snowflake.CreateUserOptions{
		Name:                  id,
		LoginName:             user.Spec.LoginName,
		DisplayName:           user.Spec.DisplayName,
		Email:                 user.Spec.Email,
		FirstName:             user.Spec.FirstName,
		LastName:              user.Spec.LastName,
		Comment:               user.Spec.Comment,
		DefaultRole:           user.Spec.DefaultRole,
		DefaultSecondaryRoles: user.Spec.DefaultSecondaryRoles,
		DefaultWarehouse:      user.Spec.DefaultWarehouse,
		DefaultNamespace:      user.Spec.DefaultNamespace,
		MustChangePassword:    user.Spec.MustChangePassword,
		Disabled:              user.Spec.Disabled,
	}

	if user.Spec.Type != nil {
		t := string(*user.Spec.Type)
		opts.Type = &t
	}

	// Resolve secrets for sensitive fields.
	if user.Spec.Password != nil {
		v, err := resolveSecretValue(ctx, c, user.Namespace, user.Spec.Password)
		if err != nil {
			return opts, fmt.Errorf("resolving password: %w", err)
		}

		opts.Password = &v
	}

	if user.Spec.RSAPublicKey != nil {
		v, err := resolveSecretValue(ctx, c, user.Namespace, user.Spec.RSAPublicKey)
		if err != nil {
			return opts, fmt.Errorf("resolving rsaPublicKey: %w", err)
		}

		opts.RSAPublicKey = &v
	}

	if user.Spec.RSAPublicKey2 != nil {
		v, err := resolveSecretValue(ctx, c, user.Namespace, user.Spec.RSAPublicKey2)
		if err != nil {
			return opts, fmt.Errorf("resolving rsaPublicKey2: %w", err)
		}

		opts.RSAPublicKey2 = &v
	}

	return opts, nil
}

func buildAlterOptions(ctx context.Context, c client.Client, user *snowplanev1alpha1.User, id snowflake.AccountObjectIdentifier, obs *snowflake.UserObservation) (snowflake.AlterUserOptions, error) {
	opts := snowflake.AlterUserOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(user)

	show := obs.ShowOutput

	if user.Spec.LoginName != nil && (show == nil || !strings.EqualFold(*user.Spec.LoginName, show.LoginName)) {
		opts.LoginName = user.Spec.LoginName
	}

	if user.Spec.DisplayName != nil && (show == nil || *user.Spec.DisplayName != show.DisplayName) {
		opts.DisplayName = user.Spec.DisplayName
	}

	if user.Spec.Email != nil && (show == nil || *user.Spec.Email != show.Email) {
		opts.Email = user.Spec.Email
	}

	if user.Spec.FirstName != nil && (show == nil || *user.Spec.FirstName != show.FirstName) {
		opts.FirstName = user.Spec.FirstName
	}

	if user.Spec.LastName != nil && (show == nil || *user.Spec.LastName != show.LastName) {
		opts.LastName = user.Spec.LastName
	}

	if user.Spec.Comment != nil && (show == nil || *user.Spec.Comment != show.Comment) {
		opts.Comment = user.Spec.Comment
	}

	if user.Spec.DefaultRole != nil && (show == nil || !strings.EqualFold(*user.Spec.DefaultRole, show.DefaultRole)) {
		opts.DefaultRole = user.Spec.DefaultRole
	}

	if user.Spec.DefaultSecondaryRoles != nil && (show == nil || *user.Spec.DefaultSecondaryRoles != show.DefaultSecondaryRoles) {
		opts.DefaultSecondaryRoles = user.Spec.DefaultSecondaryRoles
	}

	if user.Spec.DefaultWarehouse != nil && (show == nil || !strings.EqualFold(*user.Spec.DefaultWarehouse, show.DefaultWarehouse)) {
		opts.DefaultWarehouse = user.Spec.DefaultWarehouse
	}

	if user.Spec.DefaultNamespace != nil && (show == nil || !strings.EqualFold(*user.Spec.DefaultNamespace, show.DefaultNamespace)) {
		opts.DefaultNamespace = user.Spec.DefaultNamespace
	}

	if user.Spec.MustChangePassword != nil && (show == nil || *user.Spec.MustChangePassword != show.MustChangePassword) {
		opts.MustChangePassword = user.Spec.MustChangePassword
	}

	if user.Spec.Disabled != nil && (show == nil || *user.Spec.Disabled != show.Disabled) {
		opts.Disabled = user.Spec.Disabled
	}

	// Resolve secrets — password is only re-applied if its hash differs from
	// the last-applied hash stored in status. This avoids unnecessary ALTER
	// USER SET PASSWORD on every reconcile cycle.
	if user.Spec.Password != nil {
		v, err := resolveSecretValue(ctx, c, user.Namespace, user.Spec.Password)
		if err != nil {
			return opts, fmt.Errorf("resolving password: %w", err)
		}

		if hashSecret(v, string(user.UID)) != user.Status.LastAppliedPasswordHash {
			opts.Password = &v
		}
	}

	// RSA keys use the same hash-and-compare strategy so unchanged keys are
	// not needlessly re-applied via ALTER USER, avoiding audit log noise.
	if user.Spec.RSAPublicKey != nil {
		v, err := resolveSecretValue(ctx, c, user.Namespace, user.Spec.RSAPublicKey)
		if err != nil {
			return opts, fmt.Errorf("resolving rsaPublicKey: %w", err)
		}

		if hashSecret(v, string(user.UID)) != user.Status.LastAppliedRSAPublicKeyHash {
			opts.RSAPublicKey = &v
		}
	}

	if user.Spec.RSAPublicKey2 != nil {
		v, err := resolveSecretValue(ctx, c, user.Namespace, user.Spec.RSAPublicKey2)
		if err != nil {
			return opts, fmt.Errorf("resolving rsaPublicKey2: %w", err)
		}

		if hashSecret(v, string(user.UID)) != user.Status.LastAppliedRSAPublicKey2Hash {
			opts.RSAPublicKey2 = &v
		}
	}

	return opts, nil
}

func applyObservation(user *snowplanev1alpha1.User, obs *snowflake.UserObservation) {
	if obs.ShowOutput != nil {
		user.Status.FullyQualifiedName = snowflake.NewAccountObjectIdentifier(obs.ShowOutput.Name).FullyQualifiedName()

		user.Status.ShowOutput = &snowplanev1alpha1.UserShowOutput{
			CreatedOn:             obs.ShowOutput.CreatedOn,
			Name:                  obs.ShowOutput.Name,
			LoginName:             obs.ShowOutput.LoginName,
			DisplayName:           obs.ShowOutput.DisplayName,
			Email:                 obs.ShowOutput.Email,
			FirstName:             obs.ShowOutput.FirstName,
			LastName:              obs.ShowOutput.LastName,
			Comment:               obs.ShowOutput.Comment,
			DefaultRole:           obs.ShowOutput.DefaultRole,
			DefaultSecondaryRoles: obs.ShowOutput.DefaultSecondaryRoles,
			DefaultWarehouse:      obs.ShowOutput.DefaultWarehouse,
			DefaultNamespace:      obs.ShowOutput.DefaultNamespace,
			Owner:                 obs.ShowOutput.Owner,
			Disabled:              obs.ShowOutput.Disabled,
			MustChangePassword:    obs.ShowOutput.MustChangePassword,
			HasRSAPublicKey:       obs.ShowOutput.HasRSAPublicKey,
			Type:                  obs.ShowOutput.Type,
		}
	}

	if obs.DescribeOutput != nil {
		user.Status.DescribeOutput = &snowplanev1alpha1.UserDescribeOutput{
			RSAPublicKeyFP:  obs.DescribeOutput.RSAPublicKeyFP,
			RSAPublicKey2FP: obs.DescribeOutput.RSAPublicKey2FP,
		}
	}
}

// computeUnsetFields returns the Snowflake parameter names that were previously
// SET (tracked in status.TrackedParameters) but are now nil in the spec.
func computeUnsetFields(user *snowplanev1alpha1.User) []string {
	if len(user.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(user.Status.TrackedParameters))
	for _, f := range user.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if user.Spec.LoginName == nil && managed["LOGIN_NAME"] {
		unset = append(unset, "LOGIN_NAME")
	}
	if user.Spec.DisplayName == nil && managed["DISPLAY_NAME"] {
		unset = append(unset, "DISPLAY_NAME")
	}
	if user.Spec.Email == nil && managed["EMAIL"] {
		unset = append(unset, "EMAIL")
	}
	if user.Spec.FirstName == nil && managed["FIRST_NAME"] {
		unset = append(unset, "FIRST_NAME")
	}
	if user.Spec.LastName == nil && managed["LAST_NAME"] {
		unset = append(unset, "LAST_NAME")
	}
	if user.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}
	if user.Spec.DefaultRole == nil && managed["DEFAULT_ROLE"] {
		unset = append(unset, "DEFAULT_ROLE")
	}
	if user.Spec.DefaultSecondaryRoles == nil && managed["DEFAULT_SECONDARY_ROLES"] {
		unset = append(unset, "DEFAULT_SECONDARY_ROLES")
	}
	if user.Spec.DefaultWarehouse == nil && managed["DEFAULT_WAREHOUSE"] {
		unset = append(unset, "DEFAULT_WAREHOUSE")
	}
	if user.Spec.DefaultNamespace == nil && managed["DEFAULT_NAMESPACE"] {
		unset = append(unset, "DEFAULT_NAMESPACE")
	}
	if user.Spec.MustChangePassword == nil && managed["MUST_CHANGE_PASSWORD"] {
		unset = append(unset, "MUST_CHANGE_PASSWORD")
	}
	if user.Spec.Disabled == nil && managed["DISABLED"] {
		unset = append(unset, "DISABLED")
	}
	if user.Spec.Password == nil && managed["PASSWORD"] {
		unset = append(unset, "PASSWORD")
	}
	if user.Spec.RSAPublicKey == nil && managed["RSA_PUBLIC_KEY"] {
		unset = append(unset, "RSA_PUBLIC_KEY")
	}
	if user.Spec.RSAPublicKey2 == nil && managed["RSA_PUBLIC_KEY_2"] {
		unset = append(unset, "RSA_PUBLIC_KEY_2")
	}

	return unset
}

// computeTrackedParameters returns the Snowflake parameter names that are
// actively managed (non-nil) in the user spec.
func computeTrackedParameters(spec *snowplanev1alpha1.UserSpec) []string {
	var fields []string

	if spec.LoginName != nil {
		fields = append(fields, "LOGIN_NAME")
	}
	if spec.DisplayName != nil {
		fields = append(fields, "DISPLAY_NAME")
	}
	if spec.Email != nil {
		fields = append(fields, "EMAIL")
	}
	if spec.FirstName != nil {
		fields = append(fields, "FIRST_NAME")
	}
	if spec.LastName != nil {
		fields = append(fields, "LAST_NAME")
	}
	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}
	if spec.DefaultRole != nil {
		fields = append(fields, "DEFAULT_ROLE")
	}
	if spec.DefaultSecondaryRoles != nil {
		fields = append(fields, "DEFAULT_SECONDARY_ROLES")
	}
	if spec.DefaultWarehouse != nil {
		fields = append(fields, "DEFAULT_WAREHOUSE")
	}
	if spec.DefaultNamespace != nil {
		fields = append(fields, "DEFAULT_NAMESPACE")
	}
	if spec.MustChangePassword != nil {
		fields = append(fields, "MUST_CHANGE_PASSWORD")
	}
	if spec.Disabled != nil {
		fields = append(fields, "DISABLED")
	}
	if spec.Password != nil {
		fields = append(fields, "PASSWORD")
	}
	if spec.RSAPublicKey != nil {
		fields = append(fields, "RSA_PUBLIC_KEY")
	}
	if spec.RSAPublicKey2 != nil {
		fields = append(fields, "RSA_PUBLIC_KEY_2")
	}

	return fields
}

// detectDrift compares desired spec against the observed state.
func detectDrift(user *snowplanev1alpha1.User, obs *snowflake.UserObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", user.Spec.Name, obs.ShowOutput.Name, true)
		if user.Spec.Type != nil {
			d.CompareStringValueFold("TYPE", string(*user.Spec.Type), obs.ShowOutput.Type, true)
		}

		// Mutable fields.
		d.CompareStringFold("LOGIN_NAME", user.Spec.LoginName, obs.ShowOutput.LoginName, false)
		d.CompareString("DISPLAY_NAME", user.Spec.DisplayName, obs.ShowOutput.DisplayName, false)
		d.CompareString("EMAIL", user.Spec.Email, obs.ShowOutput.Email, false)
		d.CompareString("FIRST_NAME", user.Spec.FirstName, obs.ShowOutput.FirstName, false)
		d.CompareString("LAST_NAME", user.Spec.LastName, obs.ShowOutput.LastName, false)
		d.CompareString("COMMENT", user.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareStringFold("DEFAULT_ROLE", user.Spec.DefaultRole, obs.ShowOutput.DefaultRole, false)
		d.CompareString("DEFAULT_SECONDARY_ROLES", user.Spec.DefaultSecondaryRoles, obs.ShowOutput.DefaultSecondaryRoles, false)
		d.CompareStringFold("DEFAULT_WAREHOUSE", user.Spec.DefaultWarehouse, obs.ShowOutput.DefaultWarehouse, false)
		d.CompareStringFold("DEFAULT_NAMESPACE", user.Spec.DefaultNamespace, obs.ShowOutput.DefaultNamespace, false)
		if user.Spec.Disabled != nil {
			d.CompareBoolValue("DISABLED", *user.Spec.Disabled, obs.ShowOutput.Disabled, false)
		}

		if user.Spec.MustChangePassword != nil {
			d.CompareBoolValue("MUST_CHANGE_PASSWORD", *user.Spec.MustChangePassword, obs.ShowOutput.MustChangePassword, false)
		}
	}

	return d.Result()
}
