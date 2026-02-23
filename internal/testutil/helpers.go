// Package testutil provides shared test helper functions used across
// controller reconciler test packages.
package testutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// PtrString returns a pointer to the given string.
func PtrString(s string) *string { return &s }

// PtrInt32 returns a pointer to the given int32.
func PtrInt32(i int32) *int32 { return &i }

// PtrBool returns a pointer to the given bool.
func PtrBool(b bool) *bool { return &b }

// TestScheme returns a runtime.Scheme with core and Snowplane types registered.
// Panics on registration failure — test setup errors must be loud.
func TestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(snowplanev1alpha1.AddToScheme(s))

	return s
}

// NewTestPC returns a ProviderConfig in Ready state for testing.
func NewTestPC(namespace string) *snowplanev1alpha1.ProviderConfig {
	pc := &snowplanev1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-pc",
			Namespace: namespace,
		},
		Spec: snowplanev1alpha1.ProviderConfigSpec{
			Account:            "acct",
			User:               "user",
			Region:             "us-east-1",
			Role:               "SYSADMIN",
			Warehouse:          "WH",
			AuthenticationType: snowplanev1alpha1.AuthenticationTypeUsernamePassword,
			Credentials: snowplanev1alpha1.ProviderCredentials{
				SecretRef: &snowplanev1alpha1.SecretKeyReference{
					Name:      "snowflake-creds",
					Namespace: namespace,
					Key:       "password",
				},
			},
		},
	}
	conditions.SetReady(pc, "ok")

	return pc
}

// NewTestSecret returns a Secret with Snowflake credentials for testing.
func NewTestSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "snowflake-creds",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"password": []byte("s3cret"),
		},
	}
}

// ReconcileReq builds a ctrl.Request for the given name and namespace.
func ReconcileReq(name, ns string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: ns}}
}

// DrainEvents reads all buffered events from a FakeRecorder and returns them.
func DrainEvents(rec *record.FakeRecorder) []string {
	var events []string

	for {
		select {
		case e := <-rec.Events:
			events = append(events, e)
		default:
			return events
		}
	}
}

// ContainsEvent checks if any event string contains the given substring.
func ContainsEvent(events []string, substring string) bool {
	for _, e := range events {
		if strings.Contains(e, substring) {
			return true
		}
	}

	return false
}

// AssertCondition asserts that the given condition exists with the expected status and reason.
func AssertCondition(t *testing.T, obj conditions.ConditionedObject, condType string, status metav1.ConditionStatus, reason string) {
	t.Helper()

	c := conditions.Get(obj, condType)
	if !assert.NotNilf(t, c, "expected condition %q to exist", condType) {
		return
	}

	assert.Equalf(t, status, c.Status, "condition %q status mismatch", condType)
	assert.Equalf(t, reason, c.Reason, "condition %q reason mismatch", condType)
}

// AssertReady asserts that the Ready condition is True.
func AssertReady(t *testing.T, obj conditions.ConditionedObject) {
	t.Helper()
	AssertCondition(t, obj, snowplanev1alpha1.TypeReady, metav1.ConditionTrue, snowplanev1alpha1.ReasonAvailable)
}

// AssertNotReady asserts that the Ready condition is False.
func AssertNotReady(t *testing.T, obj conditions.ConditionedObject, reason string) {
	t.Helper()
	AssertCondition(t, obj, snowplanev1alpha1.TypeReady, metav1.ConditionFalse, reason)
}

// AssertSynced asserts that the Synced condition is True.
func AssertSynced(t *testing.T, obj conditions.ConditionedObject) {
	t.Helper()
	AssertCondition(t, obj, snowplanev1alpha1.TypeSynced, metav1.ConditionTrue, snowplanev1alpha1.ReasonReconcileSuccess)
}

// AssertNotSynced asserts that the Synced condition is False.
func AssertNotSynced(t *testing.T, obj conditions.ConditionedObject, reason string) {
	t.Helper()
	AssertCondition(t, obj, snowplanev1alpha1.TypeSynced, metav1.ConditionFalse, reason)
}

// AssertTerminal asserts that the resource is in a terminal state
// (Ready=False with the given terminal reason).
func AssertTerminal(t *testing.T, obj conditions.ConditionedObject, reason string) {
	t.Helper()
	AssertCondition(t, obj, snowplanev1alpha1.TypeReady, metav1.ConditionFalse, reason)
}

// AssertNoCondition asserts that the given condition does not exist.
func AssertNoCondition(t *testing.T, obj conditions.ConditionedObject, condType string) {
	t.Helper()
	assert.Nilf(t, conditions.Get(obj, condType), "expected condition %q to not exist", condType)
}

// NewTestRecorder returns a FakeRecorder with a 100-event buffer.
func NewTestRecorder() *record.FakeRecorder {
	return record.NewFakeRecorder(100)
}
