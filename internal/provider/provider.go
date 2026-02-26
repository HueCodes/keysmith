// Package provider defines the interface and registry for secret backend providers.
package provider

import (
	"context"
	"fmt"
	"sort"

	secretsv1alpha1 "github.com/hstores/keysmith/api/v1alpha1"
)

// Secret holds key-value pairs fetched from a backend provider.
type Secret map[string][]byte

// Provider is the interface all backend providers must implement.
type Provider interface {
	// Name returns the provider's identifier string (matches ProviderSpec.Name).
	Name() string

	// FetchSecret retrieves the current secret values from the backend without rotating.
	FetchSecret(ctx context.Context, params map[string]string) (Secret, error)

	// RotateSecret requests the backend to generate new values and returns them.
	RotateSecret(ctx context.Context, params map[string]string) (Secret, error)

	// Validate checks that the provider params are complete and well-formed
	// before any rotation is attempted.
	Validate(params map[string]string) error
}

// Registry maps provider names to Provider implementations.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider to the registry. Panics on duplicate names.
func (r *Registry) Register(p Provider) {
	if _, exists := r.providers[p.Name()]; exists {
		panic(fmt.Sprintf("provider %q already registered", p.Name()))
	}
	r.providers[p.Name()] = p
}

// Get retrieves a provider by name.
func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (registered: %s)", name, r.nameList())
	}
	return p, nil
}

// Names returns a sorted list of all registered provider names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) nameList() string {
	names := r.Names()
	result := ""
	for i, n := range names {
		if i > 0 {
			result += ", "
		}
		result += n
	}
	return result
}

// MapKeys applies KeyMappings to a provider Secret, returning only the requested keys
// mapped to their Kubernetes secret key names.
func MapKeys(secret Secret, mappings []secretsv1alpha1.KeyMapping) (map[string][]byte, error) {
	result := make(map[string][]byte, len(mappings))
	for _, m := range mappings {
		val, ok := secret[m.ProviderKey]
		if !ok {
			return nil, fmt.Errorf("provider key %q not found in secret (available keys may differ per provider)", m.ProviderKey)
		}
		result[m.SecretKey] = val
	}
	return result, nil
}
