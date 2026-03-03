package sanitize

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

// stubObject satisfies runtime.Object for testing the SafeRecorder.
type stubObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (s *stubObject) DeepCopyObject() runtime.Object { return s }

func TestForEvent_PlainMessage(t *testing.T) {
	t.Parallel()
	got := ForEvent("Database \"mydb\" created")
	assert.Equal(t, `Database "mydb" created`, got)
}

func TestForEvent_SQLCreate(t *testing.T) {
	t.Parallel()
	msg := `Failed to create database "mydb": SQL compilation error: CREATE DATABASE IF NOT EXISTS "mydb" COMMENT = 'test'`
	got := ForEvent(msg)
	assert.NotContains(t, got, "CREATE DATABASE")
	assert.NotContains(t, got, "COMMENT")
	assert.Contains(t, got, "[SQL redacted]")
}

func TestForEvent_SQLAlter(t *testing.T) {
	t.Parallel()
	msg := `Failed to alter warehouse "wh1": ALTER WAREHOUSE "wh1" SET WAREHOUSE_SIZE = 'XLARGE' AUTO_SUSPEND = 600`
	got := ForEvent(msg)
	assert.NotContains(t, got, "ALTER WAREHOUSE")
	assert.NotContains(t, got, "WAREHOUSE_SIZE")
	assert.Contains(t, got, "[SQL redacted]")
}

func TestForEvent_SQLDrop(t *testing.T) {
	t.Parallel()
	msg := `Failed to drop user "bob": DROP USER IF EXISTS "bob": 390318 (08001): connection error`
	got := ForEvent(msg)
	assert.NotContains(t, got, `DROP USER`)
	assert.Contains(t, got, "[SQL redacted]")
}

func TestForEvent_SQLShow(t *testing.T) {
	t.Parallel()
	msg := `Failed to observe: SHOW DATABASES LIKE 'prod%' IN ACCOUNT`
	got := ForEvent(msg)
	assert.NotContains(t, got, "SHOW DATABASES")
	assert.Contains(t, got, "[SQL redacted]")
}

func TestForEvent_UseRole(t *testing.T) {
	t.Parallel()
	msg := `role switch failed: USE ROLE "SYSADMIN" returned error 390189`
	got := ForEvent(msg)
	assert.NotContains(t, got, `USE ROLE "SYSADMIN"`)
	assert.Contains(t, got, "[SQL redacted]")
}

func TestForEvent_DSN(t *testing.T) {
	t.Parallel()
	msg := `connection error: user_svc@xy12345.us-east-1.snowflakecomputing.com:443/mydb failed`
	got := ForEvent(msg)
	assert.NotContains(t, got, "xy12345")
	assert.NotContains(t, got, "user_svc@")
	assert.Contains(t, got, "[connection redacted]")
}

func TestForEvent_BareHost(t *testing.T) {
	t.Parallel()
	msg := `dial tcp xy12345.us-east-1.snowflakecomputing.com:443: i/o timeout`
	got := ForEvent(msg)
	assert.NotContains(t, got, "xy12345")
	assert.Contains(t, got, "[host redacted]")
}

func TestForEvent_MultiplePatterns(t *testing.T) {
	t.Parallel()
	msg := `ALTER USER "bob" SET PASSWORD = 'secret': user_svc@xy12345.snowflakecomputing.com timed out`
	got := ForEvent(msg)
	assert.NotContains(t, got, "ALTER USER")
	assert.NotContains(t, got, "PASSWORD")
	assert.NotContains(t, got, "secret")
	assert.NotContains(t, got, "xy12345")
}

func TestForEvent_Truncation(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 2000)
	got := ForEvent(long)
	require.LessOrEqual(t, len(got), maxEventMessageLength)
	assert.True(t, strings.HasSuffix(got, "..."))
}

func TestForEvent_EmptyString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", ForEvent(""))
}

func TestForEvent_WhitespaceCollapse(t *testing.T) {
	t.Parallel()
	msg := "error:   too  many   spaces"
	got := ForEvent(msg)
	assert.Equal(t, "error: too many spaces", got)
}

func TestForEvent_SelectStatement(t *testing.T) {
	t.Parallel()
	msg := `query failed: SELECT * FROM information_schema.tables WHERE table_name = 'users'`
	got := ForEvent(msg)
	assert.NotContains(t, got, "SELECT *")
	assert.NotContains(t, got, "information_schema")
	assert.Contains(t, got, "[SQL redacted]")
}

func TestForEvent_GrantRevoke(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, msg string
	}{
		{"grant", `GRANT OWNERSHIP ON DATABASE "prod" TO ROLE "DBA_ROLE"`},
		{"revoke", `REVOKE ALL PRIVILEGES ON SCHEMA "public" FROM ROLE "read_only"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ForEvent(tc.msg)
			assert.Contains(t, got, "[SQL redacted]")
		})
	}
}

func TestForEvent_DescribeTable(t *testing.T) {
	t.Parallel()
	msg := `failed: DESCRIBE USER "alice" returned unexpected columns`
	got := ForEvent(msg)
	assert.NotContains(t, got, `DESCRIBE USER`)
	assert.Contains(t, got, "[SQL redacted]")
}

// ── SafeRecorder tests ──────────────────────────────────────────────────

func TestSafeRecorder_Event(t *testing.T) {
	t.Parallel()
	fake := record.NewFakeRecorder(10)
	rec := newSafeRecorder(fake)

	obj := &stubObject{}
	rec.Event(obj, corev1.EventTypeWarning, "TestReason",
		`Failed: ALTER DATABASE "x" SET COMMENT = 'pwned'`)

	select {
	case ev := <-fake.Events:
		assert.NotContains(t, ev, "ALTER DATABASE")
		assert.NotContains(t, ev, "pwned")
		assert.Contains(t, ev, "[SQL redacted]")
	default:
		t.Fatal("expected one event")
	}
}

func TestSafeRecorder_Eventf(t *testing.T) {
	t.Parallel()
	fake := record.NewFakeRecorder(10)
	rec := newSafeRecorder(fake)

	obj := &stubObject{}
	rec.Eventf(obj, corev1.EventTypeWarning, "TestReason",
		"Failed to create %s: %v", "db",
		"CREATE DATABASE IF NOT EXISTS \"db\" COMMENT = 'test'")

	select {
	case ev := <-fake.Events:
		assert.NotContains(t, ev, "CREATE DATABASE")
		assert.Contains(t, ev, "[SQL redacted]")
	default:
		t.Fatal("expected one event")
	}
}

func TestSafeRecorder_ImplementsInterface(t *testing.T) {
	t.Parallel()
	// Compile-time check: SafeRecorder satisfies record.EventRecorder.
	var _ record.EventRecorder = (*SafeRecorder)(nil)
}

// ── ForLog tests ────────────────────────────────────────────────────────

func TestForLog_PlainMessage(t *testing.T) {
	t.Parallel()
	got := ForLog("Database created successfully")
	assert.Equal(t, "Database created successfully", got)
}

func TestForLog_PEMKeyRedaction(t *testing.T) {
	t.Parallel()
	msg := `failed to parse key: -----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA1a2b3c...
-----END RSA PRIVATE KEY-----`
	got := ForLog(msg)
	assert.NotContains(t, got, "-----BEGIN")
	assert.NotContains(t, got, "MIIEpAIBAAK")
	assert.Contains(t, got, "[REDACTED]")
}

func TestForLog_PasswordFieldRedaction(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, input string
	}{
		{"password=", `error setting password=MySecret123`},
		{"password:", `config: password: "hunter2"`},
		{"rsaPublicKey=", `rsaPublicKey=MIIBIjAN...`},
		{"rsaPublicKey2=", `rsaPublicKey2=MIIBIjAN...`},
		{"token=", `token=eyJhbGciOiJIUzI1NiJ9.secret`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ForLog(tc.input)
			assert.Contains(t, got, "[REDACTED]")
			assert.NotContains(t, got, "MySecret")
			assert.NotContains(t, got, "hunter2")
			assert.NotContains(t, got, "MIIBIjAN")
			assert.NotContains(t, got, "eyJhbGci")
		})
	}
}

func TestForLog_DSNRedaction(t *testing.T) {
	t.Parallel()
	msg := `dialing user@account.snowflakecomputing.com:443`
	got := ForLog(msg)
	assert.NotContains(t, got, "user@account")
	assert.Contains(t, got, "[connection redacted]")
}

func TestForLog_PreservesSQL(t *testing.T) {
	t.Parallel()
	// Unlike ForEvent, ForLog should keep SQL for debugging.
	msg := `ALTER DATABASE "mydb" SET COMMENT = 'hello'`
	got := ForLog(msg)
	assert.Contains(t, got, "ALTER DATABASE")
}

func TestForLog_NoTruncation(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 2000)
	got := ForLog(long)
	assert.Len(t, got, 2000)
}

func TestForEvent_PEMKeyRedaction(t *testing.T) {
	t.Parallel()
	msg := `error: -----BEGIN PUBLIC KEY-----\nMIIBIjAN\n-----END PUBLIC KEY-----`
	got := ForEvent(msg)
	assert.NotContains(t, got, "-----BEGIN")
	assert.Contains(t, got, "[REDACTED]")
}

func TestForEvent_PasswordFieldRedaction(t *testing.T) {
	t.Parallel()
	msg := `password=SuperSecret123 in user spec`
	got := ForEvent(msg)
	assert.NotContains(t, got, "SuperSecret")
	assert.Contains(t, got, "[REDACTED]")
}

// --------------------------------------------------------------------------
// Tests: eventsAdapter + NewSafeRecorderFromEvents
// --------------------------------------------------------------------------

type fakeEventsRecorder struct {
	calls []fakeEventsCall
}

type fakeEventsCall struct {
	regarding interface{}
	related   interface{}
	eventtype string
	reason    string
	action    string
	note      string
}

func (f *fakeEventsRecorder) Eventf(regarding, related runtime.Object, eventtype, reason, action, note string, args ...interface{}) {
	f.calls = append(f.calls, fakeEventsCall{
		regarding: regarding,
		related:   related,
		eventtype: eventtype,
		reason:    reason,
		action:    action,
		note:      fmt.Sprintf(note, args...),
	})
}

func TestEventsAdapter_Event(t *testing.T) {
	t.Parallel()
	fake := &fakeEventsRecorder{}
	rec := NewSafeRecorderFromEvents(fake)

	pod := &corev1.Pod{}
	rec.Event(pod, corev1.EventTypeNormal, "Created", "resource created")

	require.Len(t, fake.calls, 1)
	assert.Equal(t, corev1.EventTypeNormal, fake.calls[0].eventtype)
	assert.Equal(t, "Created", fake.calls[0].reason)
	assert.Equal(t, "Reconcile", fake.calls[0].action)
	assert.Equal(t, "resource created", fake.calls[0].note)
}

func TestEventsAdapter_Event_SanitizesSQL(t *testing.T) {
	t.Parallel()
	fake := &fakeEventsRecorder{}
	rec := NewSafeRecorderFromEvents(fake)

	pod := &corev1.Pod{}
	rec.Event(pod, corev1.EventTypeWarning, "Error", `CREATE TABLE "foo" failed: duplicate`)

	require.Len(t, fake.calls, 1)
	assert.Contains(t, fake.calls[0].note, "[SQL redacted]")
	assert.NotContains(t, fake.calls[0].note, "CREATE TABLE")
}

func TestEventsAdapter_Eventf(t *testing.T) {
	t.Parallel()
	fake := &fakeEventsRecorder{}
	rec := NewSafeRecorderFromEvents(fake)

	pod := &corev1.Pod{}
	rec.Eventf(pod, corev1.EventTypeNormal, "Synced", "synced %d resources", 5)

	require.Len(t, fake.calls, 1)
	assert.Contains(t, fake.calls[0].note, "synced 5 resources")
}

// ── ForCondition tests ──────────────────────────────────────────────────

func TestForCondition_PlainMessage(t *testing.T) {
	t.Parallel()
	got := ForCondition("validation failed: name is required")
	assert.Equal(t, "validation failed: name is required", got)
}

func TestForCondition_StripsSQLFragments(t *testing.T) {
	t.Parallel()
	msg := `failed to execute: CREATE TABLE "secret_data" (id INT) — insufficient privileges`
	got := ForCondition(msg)
	assert.NotContains(t, got, "CREATE TABLE")
	assert.NotContains(t, got, "secret_data")
	assert.Contains(t, got, "[SQL redacted]")
}

func TestForCondition_StripsDSN(t *testing.T) {
	t.Parallel()
	msg := `failed connecting to user@acme.snowflakecomputing.com: timeout`
	got := ForCondition(msg)
	assert.NotContains(t, got, "user@acme")
	assert.Contains(t, got, "[connection redacted]")
}

func TestForCondition_StripsPEMKey(t *testing.T) {
	t.Parallel()
	msg := `error parsing key: -----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAK\n-----END RSA PRIVATE KEY-----`
	got := ForCondition(msg)
	assert.NotContains(t, got, "-----BEGIN")
	assert.NotContains(t, got, "MIIEpAIBAAK")
	assert.Contains(t, got, "[REDACTED]")
}

func TestForCondition_StripsPassword(t *testing.T) {
	t.Parallel()
	msg := `error: password=SuperSecret123 in connection config`
	got := ForCondition(msg)
	assert.NotContains(t, got, "SuperSecret123")
	assert.Contains(t, got, "[REDACTED]")
}

func TestForCondition_TruncatesLongMessages(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 40000)
	got := ForCondition(long)
	assert.LessOrEqual(t, len([]rune(got)), 32768)
}

func TestForCondition_DoesNotTruncateShortMessages(t *testing.T) {
	t.Parallel()
	msg := strings.Repeat("a", 1000)
	got := ForCondition(msg)
	assert.Len(t, got, 1000)
}
