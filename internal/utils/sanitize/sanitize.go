// Package sanitize provides helpers for stripping sensitive details
// (SQL fragments, connection strings) from error messages before they
// are emitted as Kubernetes Events visible to end-users.
package sanitize

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
)

const maxEventMessageLength = 1024
const maxConditionMessageLength = 32768

// sqlStmtRe matches SQL statement keywords followed by any content.
// It is intentionally broad — the goal is to remove SQL fragments that
// may be embedded in Snowflake driver error strings.
var sqlStmtRe = regexp.MustCompile(
	`(?i)\b(CREATE|ALTER|DROP|SHOW|DESCRIBE|SELECT|INSERT|UPDATE|DELETE|USE ROLE|GRANT|REVOKE)\s+[^:]+`,
)

// dsnRe matches connection-string patterns (DSN, JDBC URLs, etc.).
var dsnRe = regexp.MustCompile(
	`(?i)[a-z0-9_.+-]+@[a-z0-9_.-]+\.snowflakecomputing\.com[^\s]*`,
)

// hostRe matches bare Snowflake hostnames that may appear in errors.
var hostRe = regexp.MustCompile(
	`(?i)[a-z0-9_.-]+\.snowflakecomputing\.com`,
)

// secretValueRe matches PEM-encoded keys and other secret patterns.
var secretValueRe = regexp.MustCompile(
	`(?i)-----BEGIN [A-Z ]+-----[\s\S]*?-----END [A-Z ]+-----`,
)

// passwordFieldRe matches password values in structured output (e.g.
// password='...', password: "...").
var passwordFieldRe = regexp.MustCompile(
	`(?i)(password|rsaPublicKey|rsaPublicKey2|token)[=:\s]+['"]?[^\s'",:}]+['"]?`,
)

// ForEvent sanitises msg so it is safe to include in a Kubernetes Event.
// It removes embedded SQL statements, connection strings, Snowflake
// hostnames, PEM keys and password-like fields, then truncates the result
// to maxEventMessageLength.
func ForEvent(msg string) string {
	msg = secretValueRe.ReplaceAllString(msg, "[REDACTED]")
	msg = passwordFieldRe.ReplaceAllString(msg, "${1}=[REDACTED]")
	msg = sqlStmtRe.ReplaceAllString(msg, "[SQL redacted]")
	msg = dsnRe.ReplaceAllString(msg, "[connection redacted]")
	msg = hostRe.ReplaceAllString(msg, "[host redacted]")
	// Collapse runs of whitespace that may result from stripping.
	msg = strings.Join(strings.Fields(msg), " ")

	// Truncate using rune-aware slicing to avoid splitting multi-byte UTF-8.
	runes := []rune(msg)
	if len(runes) > maxEventMessageLength {
		msg = string(runes[:maxEventMessageLength-3]) + "..."
	}

	return msg
}

// ForLog sanitises msg for use in structured log output. It strips
// PEM-encoded keys, password/token field values, connection strings
// and Snowflake hostnames. Unlike ForEvent, it does NOT strip SQL
// statements (these are useful for debugging) and does NOT truncate.
func ForLog(msg string) string {
	msg = secretValueRe.ReplaceAllString(msg, "[REDACTED]")
	msg = passwordFieldRe.ReplaceAllString(msg, "${1}=[REDACTED]")
	msg = dsnRe.ReplaceAllString(msg, "[connection redacted]")
	msg = hostRe.ReplaceAllString(msg, "[host redacted]")
	return msg
}

// ForCondition sanitises msg so it is safe to include in a Kubernetes
// status condition message. Conditions are stored in etcd and readable
// by anyone with GET access to the CRD, so they must not contain SQL
// fragments, connection strings, or credentials.
//
// It applies the same sanitisation as ForEvent (SQL stripping, DSN
// removal, PEM key redaction) but uses a larger truncation limit
// because condition messages are not displayed in event streams.
func ForCondition(msg string) string {
	msg = secretValueRe.ReplaceAllString(msg, "[REDACTED]")
	msg = passwordFieldRe.ReplaceAllString(msg, "${1}=[REDACTED]")
	msg = sqlStmtRe.ReplaceAllString(msg, "[SQL redacted]")
	msg = dsnRe.ReplaceAllString(msg, "[connection redacted]")
	msg = hostRe.ReplaceAllString(msg, "[host redacted]")
	// Collapse runs of whitespace that may result from stripping.
	msg = strings.Join(strings.Fields(msg), " ")

	runes := []rune(msg)
	if len(runes) > maxConditionMessageLength {
		msg = string(runes[:maxConditionMessageLength-3]) + "..."
	}

	return msg
}

// SafeRecorder wraps a record.EventRecorder and sanitises every message
// through ForEvent before delegating to the inner recorder.  It implements
// the full record.EventRecorder interface so it can be used as a drop-in
// replacement everywhere a recorder is accepted.
type SafeRecorder struct {
	inner record.EventRecorder
}

// NewSafeRecorder returns a record.EventRecorder that sanitises messages.
func NewSafeRecorder(inner record.EventRecorder) record.EventRecorder {
	return &SafeRecorder{inner: inner}
}

// Event records an event with a sanitised message.
func (r *SafeRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	r.inner.Event(object, eventtype, reason, ForEvent(message))
}

// Eventf records a formatted event with a sanitised message.
func (r *SafeRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	msg := fmt.Sprintf(messageFmt, args...)
	r.inner.Event(object, eventtype, reason, ForEvent(msg))
}

// AnnotatedEventf records an annotated event with a sanitised message.
func (r *SafeRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	msg := fmt.Sprintf(messageFmt, args...)
	r.inner.AnnotatedEventf(object, annotations, eventtype, reason, "%s", ForEvent(msg))
}

// --------------------------------------------------------------------------
// Events API bridge: events.EventRecorder → record.EventRecorder
// --------------------------------------------------------------------------

// eventsAdapter bridges the new events.EventRecorder API to the legacy
// record.EventRecorder interface. This allows callers to use
// mgr.GetEventRecorder (non-deprecated) while the rest of the codebase
// continues to accept record.EventRecorder.
type eventsAdapter struct {
	inner events.EventRecorder
}

func (a *eventsAdapter) Event(object runtime.Object, eventtype, reason, message string) {
	a.inner.Eventf(object, nil, eventtype, reason, "Reconcile", "%s", message)
}

func (a *eventsAdapter) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	a.inner.Eventf(object, nil, eventtype, reason, "Reconcile", messageFmt, args...)
}

// AnnotatedEventf records an annotated event. The annotations parameter is
// not forwarded because the events.EventRecorder API uses structured fields
// (action, regarding, related) instead of free-form annotations.
func (a *eventsAdapter) AnnotatedEventf(object runtime.Object, _ map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	a.inner.Eventf(object, nil, eventtype, reason, "Reconcile", messageFmt, args...)
}

// NewSafeRecorderFromEvents creates a sanitised record.EventRecorder from the
// non-deprecated events.EventRecorder returned by mgr.GetEventRecorder().
func NewSafeRecorderFromEvents(rec events.EventRecorder) record.EventRecorder {
	return NewSafeRecorder(&eventsAdapter{inner: rec})
}
