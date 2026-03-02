package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildSetNetworkPolicySQL(t *testing.T) {
	t.Parallel()

	t.Run("AccountTarget", func(t *testing.T) {
		t.Parallel()
		opts := SetNetworkPolicyOptions{
			PolicyName: "MY_POLICY",
			TargetType: "ACCOUNT",
		}
		got := buildSetNetworkPolicySQL(opts)
		assert.Equal(t, `ALTER ACCOUNT SET NETWORK_POLICY = MY_POLICY`, got)
	})

	t.Run("UserTarget", func(t *testing.T) {
		t.Parallel()
		opts := SetNetworkPolicyOptions{
			PolicyName: "MY_POLICY",
			TargetType: "USER",
			TargetName: "JOHN_DOE",
		}
		got := buildSetNetworkPolicySQL(opts)
		assert.Equal(t, `ALTER USER JOHN_DOE SET NETWORK_POLICY = MY_POLICY`, got)
	})
}

func TestBuildUnsetNetworkPolicySQL(t *testing.T) {
	t.Parallel()

	t.Run("AccountTarget", func(t *testing.T) {
		t.Parallel()
		opts := UnsetNetworkPolicyOptions{
			TargetType: "ACCOUNT",
		}
		got := buildUnsetNetworkPolicySQL(opts)
		assert.Equal(t, `ALTER ACCOUNT UNSET NETWORK_POLICY`, got)
	})

	t.Run("UserTarget", func(t *testing.T) {
		t.Parallel()
		opts := UnsetNetworkPolicyOptions{
			TargetType: "USER",
			TargetName: "JOHN_DOE",
		}
		got := buildUnsetNetworkPolicySQL(opts)
		assert.Equal(t, `ALTER USER JOHN_DOE UNSET NETWORK_POLICY`, got)
	})
}

func TestBuildShowNetworkPolicyParameterSQL(t *testing.T) {
	t.Parallel()

	t.Run("AccountTarget", func(t *testing.T) {
		t.Parallel()
		id := NetworkPolicyAttachmentIdentifier{
			PolicyName: "MY_POLICY",
			TargetType: "ACCOUNT",
		}
		got := buildShowNetworkPolicyParameterSQL(id)
		assert.Equal(t, `SHOW PARAMETERS LIKE 'NETWORK_POLICY' IN ACCOUNT`, got)
	})

	t.Run("UserTarget", func(t *testing.T) {
		t.Parallel()
		id := NetworkPolicyAttachmentIdentifier{
			PolicyName: "MY_POLICY",
			TargetType: "USER",
			TargetName: "JOHN_DOE",
		}
		got := buildShowNetworkPolicyParameterSQL(id)
		assert.Equal(t, `SHOW PARAMETERS LIKE 'NETWORK_POLICY' FOR USER JOHN_DOE`, got)
	})
}

func TestNetworkPolicyAttachmentIdentifier_FullyQualifiedName(t *testing.T) {
	t.Parallel()

	t.Run("AccountTarget", func(t *testing.T) {
		t.Parallel()
		id := NetworkPolicyAttachmentIdentifier{
			PolicyName: "MY_POLICY",
			TargetType: "ACCOUNT",
		}
		assert.Equal(t, "NETWORK_POLICY MY_POLICY ON ACCOUNT", id.FullyQualifiedName())
	})

	t.Run("UserTarget", func(t *testing.T) {
		t.Parallel()
		id := NetworkPolicyAttachmentIdentifier{
			PolicyName: "MY_POLICY",
			TargetType: "USER",
			TargetName: "JOHN_DOE",
		}
		assert.Equal(t, "NETWORK_POLICY MY_POLICY ON USER JOHN_DOE", id.FullyQualifiedName())
	})
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestSetNetworkPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid_Account", func(t *testing.T) {
		t.Parallel()
		opts := &SetNetworkPolicyOptions{PolicyName: "MY_POLICY", TargetType: "ACCOUNT"}
		require.NoError(t, opts.Validate())
	})

	t.Run("Valid_User", func(t *testing.T) {
		t.Parallel()
		opts := &SetNetworkPolicyOptions{PolicyName: "MY_POLICY", TargetType: "USER", TargetName: "JOHN"}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingPolicyName", func(t *testing.T) {
		t.Parallel()
		opts := &SetNetworkPolicyOptions{TargetType: "ACCOUNT"}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingTargetType", func(t *testing.T) {
		t.Parallel()
		opts := &SetNetworkPolicyOptions{PolicyName: "P"}
		require.Error(t, opts.Validate())
	})

	t.Run("UserWithoutName", func(t *testing.T) {
		t.Parallel()
		opts := &SetNetworkPolicyOptions{PolicyName: "P", TargetType: "USER"}
		require.Error(t, opts.Validate())
	})
}

func TestUnsetNetworkPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid_Account", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetNetworkPolicyOptions{TargetType: "ACCOUNT"}
		require.NoError(t, opts.Validate())
	})

	t.Run("Valid_User", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetNetworkPolicyOptions{TargetType: "USER", TargetName: "JOHN"}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingTargetType", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetNetworkPolicyOptions{}
		require.Error(t, opts.Validate())
	})

	t.Run("UserWithoutName", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetNetworkPolicyOptions{TargetType: "USER"}
		require.Error(t, opts.Validate())
	})
}
