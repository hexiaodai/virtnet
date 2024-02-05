package ptr

import (
	"fmt"
)

// Of returns a pointer to the input. In most cases, callers should just do &t. However, in some cases
// Go cannot take a pointer. For example, `ptr.Of(f())`.
func Of[T any](t T) *T {
	return &t
}

// OrEmpty returns *t if its non-nil, or else an empty T
func OrEmpty[T any](t *T) T {
	if t != nil {
		return *t
	}
	var empty T
	return empty
}

// OrDefault returns *t if its non-nil, or else def.
func OrDefault[T any](t *T, def T) T {
	if t != nil {
		return *t
	}
	return def
}

// NonEmptyOrDefault returns t if its non-empty, or else def.
func NonEmptyOrDefault[T comparable](t T, def T) T {
	var empty T
	if t != empty {
		return t
	}
	return def
}

// Empty returns an empty T type
func Empty[T any]() T {
	var empty T
	return empty
}

// TypeName returns the name of the type
func TypeName[T any]() string {
	var empty T
	return fmt.Sprintf("%T", empty)
}
