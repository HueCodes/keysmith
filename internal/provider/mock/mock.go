// Package mock provides a deterministic fake provider for testing and demos.
// It requires no external dependencies or credentials.
package mock

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/hstores/keysmith/internal/provider"
)

const providerName = "mock"

// Mock returns deterministic fake secret values. Each call to RotateSecret
// increments an internal counter so callers can observe the rotation happened.
type Mock struct {
	rotateCount atomic.Int64
}

// New returns a new Mock provider.
func New() *Mock {
	return &Mock{}
}

func (m *Mock) Name() string { return providerName }

func (m *Mock) Validate(_ map[string]string) error { return nil }

// FetchSecret returns the current mock values without incrementing the counter.
func (m *Mock) FetchSecret(_ context.Context, params map[string]string) (provider.Secret, error) {
	prefix := params["prefix"]
	if prefix == "" {
		prefix = providerName
	}
	n := m.rotateCount.Load()
	return provider.Secret{
		"password": fmt.Appendf(nil, "%s-password-v%d", prefix, n),
		"username": fmt.Appendf(nil, "%s-user", prefix),
		"apiKey":   fmt.Appendf(nil, "%s-apikey-v%d", prefix, n),
	}, nil
}

// RotateSecret increments the internal counter and returns fresh mock values.
func (m *Mock) RotateSecret(_ context.Context, params map[string]string) (provider.Secret, error) {
	n := m.rotateCount.Add(1)
	prefix := params["prefix"]
	if prefix == "" {
		prefix = providerName
	}
	return provider.Secret{
		"password": fmt.Appendf(nil, "%s-password-v%d", prefix, n),
		"username": fmt.Appendf(nil, "%s-user", prefix),
		"apiKey":   fmt.Appendf(nil, "%s-apikey-v%d", prefix, n),
	}, nil
}
