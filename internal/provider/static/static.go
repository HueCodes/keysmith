// Package static generates cryptographically random passwords on each rotation.
// It requires no external backend — useful for auto-generated secrets and demos.
package static

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/hstores/keysmith/internal/provider"
)

const defaultLength = 32

// Static generates cryptographically random passwords on each rotation.
type Static struct{}

// New returns a new Static provider.
func New() *Static { return &Static{} }

func (s *Static) Name() string { return "static" }

func (s *Static) Validate(params map[string]string) error {
	if l, ok := params["length"]; ok {
		n, err := strconv.Atoi(l)
		if err != nil || n < 8 || n > 256 {
			return fmt.Errorf("static provider: length must be an integer between 8 and 256, got %q", l)
		}
	}
	return nil
}

// FetchSecret generates a fresh random password. Static provider has no persistent state.
func (s *Static) FetchSecret(_ context.Context, params map[string]string) (provider.Secret, error) {
	return s.generate(params)
}

// RotateSecret generates a new cryptographically random password.
func (s *Static) RotateSecret(_ context.Context, params map[string]string) (provider.Secret, error) {
	return s.generate(params)
}

func (s *Static) generate(params map[string]string) (provider.Secret, error) {
	length := defaultLength
	if l, ok := params["length"]; ok {
		n, err := strconv.Atoi(l)
		if err == nil {
			length = n
		}
	}

	pass, err := randomString(length)
	if err != nil {
		return nil, fmt.Errorf("static provider: %w", err)
	}

	return provider.Secret{
		"password": []byte(pass),
	}, nil
}

func randomString(n int) (string, error) {
	// Generate enough random bytes to produce n URL-safe base64 characters.
	byteLen := (n*6 + 7) / 8
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	encoded := base64.URLEncoding.EncodeToString(b)
	return encoded[:n], nil
}
