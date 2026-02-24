package user

import (
	"context"
	"fmt"
	"strings"

	sigs "sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
)

// adapter implements reconciler.ResourceAdapter for User.
type adapter struct {
	client     sigs.Client
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "user" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.User {
	return &snowplanev1alpha1.User{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(_ context.Context, _ *snowplanev1alpha1.User) error {
	return nil
}

func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.User) reconciler.Identifier {
	return snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc { return nil }

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.UserObservation], error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, aid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.UserObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.User, id reconciler.Identifier) error {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts, err := buildCreateOptions(ctx, a.client, obj, aid)
	if err != nil {
		return err
	}

	if err := svc.Create(ctx, opts); err != nil {
		return err
	}

	// Track secret hashes on status for change-detection on future reconciles.
	// Safe here because Create() only returns nil on success.
	uid := string(obj.UID)
	if opts.Password != nil {
		obj.Status.LastAppliedPasswordHash = hashSecret(*opts.Password, uid)
	}

	if opts.RSAPublicKey != nil {
		obj.Status.LastAppliedRSAPublicKeyHash = hashSecret(*opts.RSAPublicKey, uid)
	}

	if opts.RSAPublicKey2 != nil {
		obj.Status.LastAppliedRSAPublicKey2Hash = hashSecret(*opts.RSAPublicKey2, uid)
	}

	return nil
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterUserOptions](opts)
	if err != nil {
		return err
	}

	return svc.Alter(ctx, *ao)
}

func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return err
	}

	return svc.Drop(ctx, aid)
}

func (a *adapter) ValidateImmutableFields(_ context.Context, user *snowplanev1alpha1.User) error {
	if reconciler.ShouldSkipImmutableValidation(user) {
		return nil
	}

	if user.Status.ShowOutput != nil {
		if user.Status.ShowOutput.Name != "" && !strings.EqualFold(user.Spec.Name, user.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", user.Status.ShowOutput.Name, user.Spec.Name)
		}

		if user.Status.ShowOutput.Type != "" && user.Spec.Type != nil {
			if !strings.EqualFold(string(*user.Spec.Type), user.Status.ShowOutput.Type) {
				return fmt.Errorf("spec.type is immutable after creation (current: %q, desired: %q)", user.Status.ShowOutput.Type, string(*user.Spec.Type))
			}
		}

	}

	return nil
}

func (a *adapter) BuildAlterOptions(ctx context.Context, obj *snowplanev1alpha1.User, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.UserObservation]) (reconciler.AlterOptions, error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts, err := buildAlterOptions(ctx, a.client, obj, aid, detail)
	if err != nil {
		return nil, err
	}

	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.User, obs *reconciler.Observation[*snowflake.UserObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.User) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.User, obs *reconciler.Observation[*snowflake.UserObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) PostCreate(_ *snowplanev1alpha1.User) {}

func (a *adapter) PostUpdate(user *snowplanev1alpha1.User, altered bool, alterOpts reconciler.AlterOptions) {
	// Commit secret hashes only after a successful ALTER, reading from
	// the per-reconciliation AlterOptions (no shared mutable state).
	uid := string(user.UID)

	if altered {
		if opts, ok := alterOpts.(*snowflake.AlterUserOptions); ok {
			if opts.Password != nil {
				user.Status.LastAppliedPasswordHash = hashSecret(*opts.Password, uid)
			}

			if opts.RSAPublicKey != nil {
				user.Status.LastAppliedRSAPublicKeyHash = hashSecret(*opts.RSAPublicKey, uid)
			}

			if opts.RSAPublicKey2 != nil {
				user.Status.LastAppliedRSAPublicKey2Hash = hashSecret(*opts.RSAPublicKey2, uid)
			}
		}
	}

	// Clear hashes when secret refs are removed from spec.
	if user.Spec.Password == nil {
		user.Status.LastAppliedPasswordHash = ""
	}

	if user.Spec.RSAPublicKey == nil {
		user.Status.LastAppliedRSAPublicKeyHash = ""
	}

	if user.Spec.RSAPublicKey2 == nil {
		user.Status.LastAppliedRSAPublicKey2Hash = ""
	}
}

func (a *adapter) SupportsCreateOrAlter() bool { return false }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.User, Service, *snowflake.UserObservation] = (*adapter)(nil)
