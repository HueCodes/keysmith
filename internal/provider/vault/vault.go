// Package vault provides a stub HashiCorp Vault KV provider.
// To activate: add github.com/hashicorp/vault-client-go as a dependency
// and implement fetchFromVault and rotateInVault below.
package vault

import (
	"context"
	"fmt"

	"github.com/hstores/keysmith/internal/provider"
)

// Vault fetches and rotates secrets stored in HashiCorp Vault KV.
// This is a stub implementation — see the TODOs below to wire up the real SDK.
type Vault struct{}

// New returns a new HashiCorp Vault provider.
func New() *Vault { return &Vault{} }

func (v *Vault) Name() string { return "vault" }

func (v *Vault) Validate(params map[string]string) error {
	if params["path"] == "" {
		return fmt.Errorf("vault provider: path param is required (e.g. secret/data/myapp)")
	}
	if params["address"] == "" {
		return fmt.Errorf("vault provider: address param is required (e.g. https://vault.example.com)")
	}
	return nil
}

// FetchSecret reads the current KV secret from Vault at the specified path.
//
// TODO: replace stub with real implementation:
//
//	client, err := vault.New(vault.WithAddress(params["address"]))
//	resp, err := client.Secrets.KvV2Read(ctx, params["path"], vault.WithToken(token))
//	// convert resp.Data.Data to provider.Secret
func (v *Vault) FetchSecret(_ context.Context, _ map[string]string) (provider.Secret, error) {
	return nil, fmt.Errorf("vault provider: not yet implemented — add vault-client-go and implement FetchSecret")
}

// RotateSecret generates a new secret value and writes it to Vault, returning the new value.
//
// TODO: replace stub with real implementation:
//
//	newPass, _ := generatePassword()
//	_, err := client.Secrets.KvV2Write(ctx, params["path"], schema.KvV2WriteRequest{
//	    Data: map[string]any{"password": newPass},
//	})
//	return provider.Secret{"password": []byte(newPass)}, nil
func (v *Vault) RotateSecret(_ context.Context, _ map[string]string) (provider.Secret, error) {
	return nil, fmt.Errorf("vault provider: not yet implemented — add vault-client-go and implement RotateSecret")
}
