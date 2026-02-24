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
