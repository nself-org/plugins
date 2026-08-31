package store_test

import (
	"testing"

	"github.com/nself-org/nself-functions-v8/internal/store"
)

func TestDeployValidation_NameRegex(t *testing.T) {
	t.Run("invalid name with space and bang", func(t *testing.T) {
		err := store.ValidateName("my function!")
		if err == nil {
			t.Fatal("expected ErrInvalidFunctionName, got nil")
		}
		if err != store.ErrInvalidFunctionName {
			t.Fatalf("expected ErrInvalidFunctionName, got: %v", err)
		}
	})

	t.Run("valid name", func(t *testing.T) {
		err := store.ValidateName("my-valid-function")
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})

	t.Run("starts with digit is invalid", func(t *testing.T) {
		err := store.ValidateName("1bad")
		if err == nil {
			t.Fatal("expected ErrInvalidFunctionName for leading digit")
		}
	})

	t.Run("uppercase is invalid", func(t *testing.T) {
		err := store.ValidateName("BadName")
		if err == nil {
			t.Fatal("expected ErrInvalidFunctionName for uppercase")
		}
	})

	t.Run("exactly 63 chars is valid", func(t *testing.T) {
		name := "a"
		for i := 0; i < 62; i++ {
			name += "b"
		}
		if len(name) != 63 {
			t.Fatalf("test setup error: name length %d", len(name))
		}
		if err := store.ValidateName(name); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})
}

func TestDeployValidation_SourceSizeLimit(t *testing.T) {
	t.Run("source exceeds 5MB", func(t *testing.T) {
		bigSource := make([]byte, 5*1024*1024+1)
		for i := range bigSource {
			bigSource[i] = 'a'
		}
		err := store.ValidateSource(string(bigSource))
		if err == nil {
			t.Fatal("expected ErrSourceTooLarge, got nil")
		}
		if err != store.ErrSourceTooLarge {
			t.Fatalf("expected ErrSourceTooLarge, got: %v", err)
		}
	})

	t.Run("source exactly at 5MB is accepted", func(t *testing.T) {
		src := make([]byte, 5*1024*1024)
		for i := range src {
			src[i] = 'b'
		}
		err := store.ValidateSource(string(src))
		if err != nil {
			t.Fatalf("expected nil for exactly 5MB, got: %v", err)
		}
	})

	t.Run("empty source is accepted", func(t *testing.T) {
		if err := store.ValidateSource(""); err != nil {
			t.Fatalf("empty source should be accepted: %v", err)
		}
	})
}
