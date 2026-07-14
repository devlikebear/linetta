// Package ptrutil provides explicit pointers for JSON PATCH-style domain inputs.
package ptrutil

// To returns a pointer to value.
func To[T any](value T) *T { return &value }
