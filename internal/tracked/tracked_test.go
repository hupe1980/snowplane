package tracked_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hupe1980/snowplane/internal/tracked"
)

func ptr[T any](v T) *T { return &v }

type SimpleSpec struct {
	Name    string  `json:"name"` // no tag → not tracked
	Comment *string `json:"comment" snowflake:"COMMENT"`
	Size    *string `json:"size" snowflake:"SIZE"`
}

func TestComputeTracked_Simple(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		spec   SimpleSpec
		expect []string
	}{
		{
			name:   "AllNil",
			spec:   SimpleSpec{Name: "test"},
			expect: nil,
		},
		{
			name:   "OneSet",
			spec:   SimpleSpec{Name: "test", Comment: ptr("hello")},
			expect: []string{"COMMENT"},
		},
		{
			name:   "BothSet",
			spec:   SimpleSpec{Name: "test", Comment: ptr("hello"), Size: ptr("LARGE")},
			expect: []string{"COMMENT", "SIZE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tracked.ComputeTracked(tt.spec)
			assert.Equal(t, tt.expect, got)
		})
	}
}

type SliceSpec struct {
	Comment *string  `snowflake:"COMMENT"`
	IPs     []string `snowflake:"ALLOWED_IP_LIST"`
}

func TestComputeTracked_Slices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		spec   SliceSpec
		expect []string
	}{
		{
			name:   "EmptySlice",
			spec:   SliceSpec{},
			expect: nil,
		},
		{
			name:   "NonEmptySlice",
			spec:   SliceSpec{IPs: []string{"1.2.3.4"}},
			expect: []string{"ALLOWED_IP_LIST"},
		},
		{
			name:   "Both",
			spec:   SliceSpec{Comment: ptr("x"), IPs: []string{"1.2.3.4"}},
			expect: []string{"COMMENT", "ALLOWED_IP_LIST"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tracked.ComputeTracked(tt.spec)
			assert.Equal(t, tt.expect, got)
		})
	}
}

type AlwaysSpec struct {
	Body    string  `snowflake:"BODY,always"`
	Comment *string `snowflake:"COMMENT"`
}

func TestComputeTracked_Always(t *testing.T) {
	t.Parallel()

	got := tracked.ComputeTracked(AlwaysSpec{Body: "return val", Comment: nil})
	assert.Equal(t, []string{"BODY"}, got)

	got = tracked.ComputeTracked(AlwaysSpec{Body: "return val", Comment: ptr("x")})
	assert.Equal(t, []string{"BODY", "COMMENT"}, got)
}

type NestedEmailConfig struct {
	Recipients []string `snowflake:"ALLOWED_RECIPIENTS"`
	Subject    *string  `snowflake:"DEFAULT_SUBJECT"`
}

type NestedQueueConfig struct {
	Provider string  `snowflake:"NOTIFICATION_PROVIDER,always"`
	TopicARN *string `snowflake:"AWS_SNS_TOPIC_ARN"`
}

type UnionSpec struct {
	Comment *string            `snowflake:"COMMENT"`
	Email   *NestedEmailConfig `json:"email"`
	Queue   *NestedQueueConfig `json:"queue"`
}

func TestComputeTracked_NestedUnion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		spec   UnionSpec
		expect []string
	}{
		{
			name:   "NilBranches",
			spec:   UnionSpec{Comment: ptr("x")},
			expect: []string{"COMMENT"},
		},
		{
			name: "EmailBranch",
			spec: UnionSpec{
				Email: &NestedEmailConfig{
					Recipients: []string{"a@b.com"},
					Subject:    ptr("Alert"),
				},
			},
			expect: []string{"ALLOWED_RECIPIENTS", "DEFAULT_SUBJECT"},
		},
		{
			name: "QueueBranch",
			spec: UnionSpec{
				Queue: &NestedQueueConfig{
					Provider: "AWS_SNS",
					TopicARN: ptr("arn:aws:sns:..."),
				},
			},
			expect: []string{"NOTIFICATION_PROVIDER", "AWS_SNS_TOPIC_ARN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tracked.ComputeTracked(tt.spec)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestComputeTracked_PointerToStruct(t *testing.T) {
	t.Parallel()

	spec := &SimpleSpec{Name: "test", Comment: ptr("hello")}
	got := tracked.ComputeTracked(spec)
	assert.Equal(t, []string{"COMMENT"}, got)
}

type SkippedSpec struct {
	Internal string  `snowflake:"-"`
	Comment  *string `snowflake:"COMMENT"`
}

func TestComputeTracked_SkipDash(t *testing.T) {
	t.Parallel()

	got := tracked.ComputeTracked(SkippedSpec{Internal: "x", Comment: ptr("y")})
	assert.Equal(t, []string{"COMMENT"}, got)
}

// --- ComputeUnset tests ---

func TestComputeUnset_NoPrevious(t *testing.T) {
	t.Parallel()

	got := tracked.ComputeUnset(SimpleSpec{}, nil)
	assert.Nil(t, got)
}

func TestComputeUnset_NothingRemoved(t *testing.T) {
	t.Parallel()

	spec := SimpleSpec{Comment: ptr("x"), Size: ptr("L")}
	got := tracked.ComputeUnset(spec, []string{"COMMENT", "SIZE"})
	assert.Nil(t, got)
}

func TestComputeUnset_FieldRemoved(t *testing.T) {
	t.Parallel()

	// Was: COMMENT + SIZE, now only COMMENT
	spec := SimpleSpec{Comment: ptr("x")}
	got := tracked.ComputeUnset(spec, []string{"COMMENT", "SIZE"})
	assert.Equal(t, []string{"SIZE"}, got)
}

func TestComputeUnset_AllRemoved(t *testing.T) {
	t.Parallel()

	spec := SimpleSpec{}
	got := tracked.ComputeUnset(spec, []string{"COMMENT", "SIZE"})
	assert.Equal(t, []string{"COMMENT", "SIZE"}, got)
}

type EmbeddedBase struct {
	Comment *string `snowflake:"COMMENT"`
}

type EmbeddedSpec struct {
	EmbeddedBase
	Size *string `snowflake:"SIZE"`
}

func TestComputeTracked_Embedded(t *testing.T) {
	t.Parallel()

	spec := EmbeddedSpec{
		EmbeddedBase: EmbeddedBase{Comment: ptr("hello")},
		Size:         ptr("LARGE"),
	}
	got := tracked.ComputeTracked(spec)
	assert.Equal(t, []string{"COMMENT", "SIZE"}, got)
}

type MapSpec struct {
	Headers map[string]string `snowflake:"WEBHOOK_HEADERS"`
}

func TestComputeTracked_Map(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		spec   MapSpec
		expect []string
	}{
		{
			name:   "NilMap",
			spec:   MapSpec{},
			expect: nil,
		},
		{
			name:   "NonEmptyMap",
			spec:   MapSpec{Headers: map[string]string{"X-Key": "val"}},
			expect: []string{"WEBHOOK_HEADERS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tracked.ComputeTracked(tt.spec)
			assert.Equal(t, tt.expect, got)
		})
	}
}

// --- prefix map tests ---

type PrefixMapSpec struct {
	Comment *string           `snowflake:"COMMENT"`
	Headers map[string]string `snowflake:"WEBHOOK_HEADER_,prefix"`
}

func TestComputeTracked_PrefixMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		spec   PrefixMapSpec
		expect []string
	}{
		{
			name:   "Empty",
			spec:   PrefixMapSpec{},
			expect: nil,
		},
		{
			name:   "OneKey",
			spec:   PrefixMapSpec{Headers: map[string]string{"foo": "bar"}},
			expect: []string{"WEBHOOK_HEADER_foo"},
		},
		{
			name: "MultipleKeysSorted",
			spec: PrefixMapSpec{Headers: map[string]string{
				"zeta": "z", "alpha": "a", "beta": "b",
			}},
			expect: []string{"WEBHOOK_HEADER_alpha", "WEBHOOK_HEADER_beta", "WEBHOOK_HEADER_zeta"},
		},
		{
			name:   "WithComment",
			spec:   PrefixMapSpec{Comment: ptr("hi"), Headers: map[string]string{"k": "v"}},
			expect: []string{"COMMENT", "WEBHOOK_HEADER_k"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tracked.ComputeTracked(tt.spec)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestComputeUnset_PrefixMap(t *testing.T) {
	t.Parallel()

	t.Run("KeyRemoved", func(t *testing.T) {
		t.Parallel()
		spec := PrefixMapSpec{Headers: map[string]string{"foo": "bar"}}
		got := tracked.ComputeUnset(spec, []string{"WEBHOOK_HEADER_foo", "WEBHOOK_HEADER_old"})
		assert.Equal(t, []string{"WEBHOOK_HEADER_old"}, got)
	})

	t.Run("AllRemoved", func(t *testing.T) {
		t.Parallel()
		spec := PrefixMapSpec{}
		got := tracked.ComputeUnset(spec, []string{"WEBHOOK_HEADER_x"})
		assert.Equal(t, []string{"WEBHOOK_HEADER_x"}, got)
	})
}

// --- nounset tests ---

type NounsetSpec struct {
	Enabled *bool   `snowflake:"ENABLED,nounset"`
	Comment *string `snowflake:"COMMENT"`
}

func TestComputeUnset_Nounset(t *testing.T) {
	t.Parallel()

	t.Run("NounsetExcluded", func(t *testing.T) {
		t.Parallel()
		// ENABLED was tracked, now nil — should NOT be in unset.
		spec := NounsetSpec{}
		got := tracked.ComputeUnset(spec, []string{"ENABLED", "COMMENT"})
		assert.Equal(t, []string{"COMMENT"}, got)
	})

	t.Run("NounsetStillTracked", func(t *testing.T) {
		t.Parallel()
		// Both set → tracked, neither in unset.
		spec := NounsetSpec{Enabled: ptr(true), Comment: ptr("hi")}
		got := tracked.ComputeTracked(spec)
		assert.Equal(t, []string{"ENABLED", "COMMENT"}, got)
		unset := tracked.ComputeUnset(spec, []string{"ENABLED", "COMMENT"})
		assert.Nil(t, unset)
	})
}

// --- nounset in nested struct (even when nil) ---

type NestedNounsetChild struct {
	Provider string  `snowflake:"NOTIFICATION_PROVIDER,always,nounset"`
	TopicARN *string `snowflake:"AWS_SNS_TOPIC_ARN"`
}

type NestedNounsetSpec struct {
	Comment *string             `snowflake:"COMMENT"`
	Queue   *NestedNounsetChild `json:"queue"`
}

func TestComputeUnset_NounsetInNilNestedStruct(t *testing.T) {
	t.Parallel()

	// Queue was non-nil before → tracked NOTIFICATION_PROVIDER and AWS_SNS_TOPIC_ARN.
	// Now Queue is nil → both disappear from current tracked list.
	// But NOTIFICATION_PROVIDER has nounset → should NOT be in unset.
	// AWS_SNS_TOPIC_ARN should be in unset.
	spec := NestedNounsetSpec{Comment: ptr("x")}
	got := tracked.ComputeUnset(spec, []string{"COMMENT", "NOTIFICATION_PROVIDER", "AWS_SNS_TOPIC_ARN"})
	assert.Equal(t, []string{"AWS_SNS_TOPIC_ARN"}, got)
}
