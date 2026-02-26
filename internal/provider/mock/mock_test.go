package mock_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hstores/keysmith/internal/provider/mock"
)

func TestMockName(t *testing.T) {
	m := mock.New()
	if m.Name() != "mock" {
		t.Errorf("expected name=mock, got %q", m.Name())
	}
}

func TestMockValidate(t *testing.T) {
	m := mock.New()
	if err := m.Validate(nil); err != nil {
		t.Errorf("mock.Validate should always return nil, got: %v", err)
	}
}

func TestMockRotateIncrementsCounter(t *testing.T) {
	m := mock.New()
	ctx := context.Background()
	s1, err := m.RotateSecret(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s2, err := m.RotateSecret(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(s1["password"]) == string(s2["password"]) {
		t.Error("expected different passwords on consecutive rotations")
	}
}

func TestMockFetchDoesNotIncrement(t *testing.T) {
	m := mock.New()
	ctx := context.Background()
	s1, _ := m.FetchSecret(ctx, nil)
	s2, _ := m.FetchSecret(ctx, nil)
	if string(s1["password"]) != string(s2["password"]) {
		t.Error("FetchSecret should return the same value without rotating")
	}
}

func TestMockWithPrefix(t *testing.T) {
	m := mock.New()
	params := map[string]string{"prefix": "myapp"}
	s, err := m.RotateSecret(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pass := string(s["password"])
	if !strings.HasPrefix(pass, "myapp-") {
		t.Errorf("expected password to start with 'myapp-', got %q", pass)
	}
}

func TestMockContainsExpectedKeys(t *testing.T) {
	m := mock.New()
	s, err := m.RotateSecret(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"password", "username", "apiKey"} {
		if _, ok := s[key]; !ok {
			t.Errorf("expected key %q in mock secret", key)
		}
	}
}
