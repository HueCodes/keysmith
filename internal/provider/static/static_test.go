package static_test

import (
	"context"
	"testing"

	"github.com/hstores/keysmith/internal/provider/static"
)

func TestStaticName(t *testing.T) {
	s := static.New()
	if s.Name() != "static" {
		t.Errorf("expected name=static, got %q", s.Name())
	}
}

func TestStaticGeneratesPassword(t *testing.T) {
	s := static.New()
	secret, err := s.RotateSecret(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pass, ok := secret["password"]
	if !ok {
		t.Fatal("expected 'password' key in secret")
	}
	if len(pass) == 0 {
		t.Error("expected non-empty password")
	}
}

func TestStaticPasswordsAreUnique(t *testing.T) {
	s := static.New()
	ctx := context.Background()
	s1, err := s.RotateSecret(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s2, err := s.RotateSecret(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(s1["password"]) == string(s2["password"]) {
		t.Error("expected unique passwords on each rotation call")
	}
}

func TestStaticPasswordLength(t *testing.T) {
	s := static.New()
	params := map[string]string{"length": "20"}
	secret, err := s.RotateSecret(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pass := string(secret["password"])
	if len(pass) != 20 {
		t.Errorf("expected password length 20, got %d", len(pass))
	}
}

func TestStaticValidate_ValidLength(t *testing.T) {
	s := static.New()
	if err := s.Validate(map[string]string{"length": "16"}); err != nil {
		t.Errorf("expected no error for valid length, got: %v", err)
	}
}

func TestStaticValidate_InvalidLength(t *testing.T) {
	s := static.New()
	if err := s.Validate(map[string]string{"length": "3"}); err == nil {
		t.Error("expected error for length < 8")
	}
	if err := s.Validate(map[string]string{"length": "not-a-number"}); err == nil {
		t.Error("expected error for non-numeric length")
	}
}

func TestStaticValidate_NoParams(t *testing.T) {
	s := static.New()
	if err := s.Validate(nil); err != nil {
		t.Errorf("expected no error for empty params, got: %v", err)
	}
}
