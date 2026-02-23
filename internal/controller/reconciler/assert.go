package reconciler

import "fmt"

// AssertIdentifier safely casts an Identifier to the concrete type I.
// Returns a descriptive error instead of panicking on type mismatch.
func AssertIdentifier[I Identifier](id Identifier) (I, error) {
	v, ok := id.(I)
	if !ok {
		return v, fmt.Errorf("identifier type mismatch: expected %T, got %T", v, id)
	}

	return v, nil
}

// AssertDetail safely casts an Observation's Detail field to the concrete
// observation type D. Returns a descriptive error instead of panicking.
func AssertDetail[D any](obs *Observation) (D, error) {
	v, ok := obs.Detail.(D)
	if !ok {
		var zero D
		return zero, fmt.Errorf("observation detail type mismatch: expected %T, got %T", zero, obs.Detail)
	}

	return v, nil
}

// AssertAlterOptions safely casts AlterOptions to the concrete type A.
// Returns a descriptive error instead of panicking on type mismatch.
func AssertAlterOptions[A AlterOptions](opts AlterOptions) (A, error) {
	v, ok := opts.(A)
	if !ok {
		var zero A
		return zero, fmt.Errorf("alter options type mismatch: expected %T, got %T", zero, opts)
	}

	return v, nil
}
