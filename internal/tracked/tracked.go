// Package tracked provides generic computation of Snowflake tracked-parameter
// lists from spec structs annotated with `snowflake:"PARAM_NAME"` struct tags.
//
// Instead of hand-writing computeTrackedParameters and computeUnsetFields per
// resource (~30 adapters × ~2 functions × ~10-40 lines each), adapters annotate
// their spec fields and call the generic functions.
//
// Supported tag syntax:
//
//	`snowflake:"PARAM_NAME"`                — tracked when pointer is non-nil or slice is non-empty
//	`snowflake:"PARAM_NAME,always"`         — always included in the tracked list
//	`snowflake:"PARAM_NAME,nounset"`        — tracked but excluded from ComputeUnset
//	`snowflake:"PARAM_NAME,always,nounset"` — always tracked and excluded from unset
//	`snowflake:"PREFIX_,prefix"`            — map keys: each key becomes PREFIX_<key>
//	`snowflake:"-"`                         — explicitly skipped (for documentation)
//
// Nested struct-pointer fields (e.g. spec.Email *EmailConfig) are recursed
// into only when non-nil, enabling union-type patterns.
package tracked

import (
	"reflect"
	"sort"
	"strings"
)

// TagKey is the struct tag key used to annotate Snowflake parameter fields.
const TagKey = "snowflake"

// ComputeTracked returns the Snowflake parameter names for all spec-struct
// fields whose current value indicates "user set this". Pointer fields are
// checked for non-nil, slices for non-empty, and "always" tagged fields are
// unconditionally included. Nested struct pointers are recursed.
func ComputeTracked(spec any) []string {
	var out []string
	computeTrackedValue(reflect.ValueOf(spec), &out)

	return out
}

// ComputeUnset returns Snowflake parameter names that were previously tracked
// but are now nil/empty in the spec. Fields tagged with "nounset" or "always"
// are excluded — they are never candidates for UNSET.
func ComputeUnset(spec any, previouslyTracked []string) []string {
	if len(previouslyTracked) == 0 {
		return nil
	}

	current := ComputeTracked(spec)
	currentSet := make(map[string]struct{}, len(current))

	for _, f := range current {
		currentSet[f] = struct{}{}
	}

	// Walk the TYPE hierarchy to find all nounset param names —
	// this works even when nested structs are nil.
	nounset := make(map[string]struct{})
	collectNounsetType(reflect.TypeOf(spec), nounset)

	var unset []string

	for _, f := range previouslyTracked {
		if _, ok := currentSet[f]; ok {
			continue
		}

		if _, ok := nounset[f]; ok {
			continue
		}

		unset = append(unset, f)
	}

	return unset
}

func computeTrackedValue(v reflect.Value, out *[]string) {
	// Dereference pointers.
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}

		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		fv := v.Field(i)

		tag := field.Tag.Get(TagKey)
		if tag == "" || tag == "-" {
			// If it's an embedded/anonymous struct, recurse anyway
			// (handles CommonSpec embedding).
			if field.Anonymous && field.Type.Kind() == reflect.Struct {
				computeTrackedValue(fv, out)
			}

			// Also recurse into non-nil struct-pointer fields without a tag —
			// they might contain tagged sub-fields (union types like Email/Queue/Webhook).
			if field.Type.Kind() == reflect.Ptr && !fv.IsNil() {
				elem := fv.Elem()
				if elem.Kind() == reflect.Struct {
					computeTrackedValue(elem, out)
				}
			}

			continue
		}

		name, opts := parseTag(tag)

		if opts.always {
			*out = append(*out, name)
			continue
		}

		switch fv.Kind() { //nolint:exhaustive // Only pointer, slice, map need special handling.
		case reflect.Ptr:
			if !fv.IsNil() {
				// If the pointed-to value is a struct, recurse into it AND track the field.
				if fv.Elem().Kind() == reflect.Struct {
					computeTrackedValue(fv.Elem(), out)
				}

				*out = append(*out, name)
			}
		case reflect.Slice:
			if fv.Len() > 0 {
				*out = append(*out, name)
			}
		case reflect.Map:
			if fv.Len() > 0 {
				if opts.prefix {
					// Each map key becomes a separate tracked param: PREFIX_<key>.
					keys := make([]string, 0, fv.Len())
					for _, k := range fv.MapKeys() {
						keys = append(keys, k.String())
					}

					sort.Strings(keys)

					for _, k := range keys {
						*out = append(*out, name+k)
					}
				} else {
					*out = append(*out, name)
				}
			}
		default:
			// Non-pointer, non-slice: always set (used for required fields
			// that should always be tracked if they have a tag).
			*out = append(*out, name)
		}
	}
}

type tagOpts struct {
	always  bool
	nounset bool
	prefix  bool
}

func parseTag(tag string) (string, tagOpts) {
	parts := strings.SplitN(tag, ",", 2)
	name := parts[0]

	var opts tagOpts
	if len(parts) > 1 {
		for _, opt := range strings.Split(parts[1], ",") {
			switch opt {
			case "always":
				opts.always = true
			case "nounset":
				opts.nounset = true
			case "prefix":
				opts.prefix = true
			}
		}
	}

	return name, opts
}

// collectNounsetType walks a struct TYPE hierarchy (not values) and collects
// all param names tagged with "nounset" or "always,nounset". Walking the type
// ensures nounset names are found even when nested union structs are nil.
func collectNounsetType(t reflect.Type, out map[string]struct{}) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return
	}

	for i := range t.NumField() {
		field := t.Field(i)

		tag := field.Tag.Get(TagKey)
		if tag == "" || tag == "-" {
			// Recurse into embedded structs and pointer-to-struct fields.
			ft := field.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}

			if ft.Kind() == reflect.Struct {
				collectNounsetType(ft, out)
			}

			continue
		}

		name, opts := parseTag(tag)
		if opts.nounset {
			out[name] = struct{}{}
		}

		// Recurse into struct-pointer fields that may contain tagged sub-fields.
		ft := field.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		if ft.Kind() == reflect.Struct {
			collectNounsetType(ft, out)
		}
	}
}
