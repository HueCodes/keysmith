package provider_test

import (
	"testing"

	secretsv1alpha1 "github.com/hstores/keysmith/api/v1alpha1"
	"github.com/hstores/keysmith/internal/provider"
	"github.com/hstores/keysmith/internal/provider/mock"
	"github.com/hstores/keysmith/internal/provider/static"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := provider.NewRegistry()
	r.Register(mock.New())
	r.Register(static.New())

	p, err := r.Get("mock")
	if err != nil {
		t.Fatalf("expected to get mock provider, got error: %v", err)
	}
	if p.Name() != "mock" {
		t.Errorf("expected name=mock, got %q", p.Name())
	}
}

func TestRegistry_UnknownProvider(t *testing.T) {
	r := provider.NewRegistry()
	_, err := r.Get("nonexistent")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate provider registration")
		}
	}()
	r := provider.NewRegistry()
	r.Register(mock.New())
	r.Register(mock.New()) // should panic
}

func TestMapKeys_Success(t *testing.T) {
	secret := provider.Secret{
		"password": []byte("s3cr3t"),
		"username": []byte("admin"),
	}
	mappings := []secretsv1alpha1.KeyMapping{
		{ProviderKey: "password", SecretKey: "DB_PASSWORD"},
		{ProviderKey: "username", SecretKey: "DB_USER"},
	}
	result, err := provider.MapKeys(secret, mappings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result["DB_PASSWORD"]) != "s3cr3t" {
		t.Errorf("expected DB_PASSWORD=s3cr3t, got %q", result["DB_PASSWORD"])
	}
	if string(result["DB_USER"]) != "admin" {
		t.Errorf("expected DB_USER=admin, got %q", result["DB_USER"])
	}
}

func TestMapKeys_MissingProviderKey(t *testing.T) {
	secret := provider.Secret{
		"password": []byte("s3cr3t"),
	}
	mappings := []secretsv1alpha1.KeyMapping{
		{ProviderKey: "nonexistent", SecretKey: "DB_PASSWORD"},
	}
	_, err := provider.MapKeys(secret, mappings)
	if err == nil {
		t.Error("expected error for missing provider key")
	}
}
