package phasedapp

import (
	"fmt"

	"github.com/BrianJOC/ansible-host-prep/phases"
)

// ContextKey represents a namespaced key used for sharing values between phases.
type ContextKey string

// Namespace builds a ContextKey combining a namespace and name (e.g., ssh:client).
func Namespace(namespace, name string) ContextKey {
	return ContextKey(fmt.Sprintf("%s:%s", namespace, name))
}

func (k ContextKey) String() string {
	return string(k)
}

// SetContext stores a typed value under the provided key.
func SetContext[T any](ctx *phases.Context, key ContextKey, value T) {
	if ctx == nil {
		return
	}
	ctx.Set(string(key), value)
}

// GetContext retrieves a typed value from the shared context.
func GetContext[T any](ctx *phases.Context, key ContextKey) (T, bool) {
	var zero T
	if ctx == nil {
		return zero, false
	}
	val, ok := ctx.Get(string(key))
	if !ok {
		return zero, false
	}
	casted, ok := val.(T)
	if !ok {
		return zero, false
	}
	return casted, true
}

// MustGetContext retrieves a typed value or panics if missing.
func MustGetContext[T any](ctx *phases.Context, key ContextKey) T {
	val, ok := GetContext[T](ctx, key)
	if !ok {
		panic(fmt.Sprintf("phasedapp: missing context key %s", key))
	}
	return val
}
