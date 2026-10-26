package vault

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// vaultFactory lets the same contract run against every adapter.
type vaultFactory func(t *testing.T) Vault

func contractTests(t *testing.T, newVault vaultFactory) {
	t.Run("get missing", func(t *testing.T) {
		if _, err := newVault(t).Get("nobody"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("delete missing", func(t *testing.T) {
		if err := newVault(t).Delete("nobody"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("put get delete", func(t *testing.T) {
		v := newVault(t)
		want := []byte("s3cret\x00\xff")
		if err := v.Put("isa", want); err != nil {
			t.Fatal(err)
		}
		got, err := v.Get("isa")
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("got %q, %v", got, err)
		}
		if err := v.Delete("isa"); err != nil {
			t.Fatal(err)
		}
		if _, err := v.Get("isa"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("after delete err = %v, want ErrNotFound", err)
		}
	})
	t.Run("overwrite", func(t *testing.T) {
		v := newVault(t)
		_ = v.Put("a", bytes.Repeat([]byte("x"), 20000))
		if err := v.Put("a", []byte("short")); err != nil {
			t.Fatal(err)
		}
		got, err := v.Get("a")
		if err != nil || string(got) != "short" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("profiles are isolated", func(t *testing.T) {
		v := newVault(t)
		_ = v.Put("a", []byte("1"))
		_ = v.Put("b", []byte("2"))
		_ = v.Delete("a")
		got, err := v.Get("b")
		if err != nil || string(got) != "2" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
}

func TestMemoryVaultContract(t *testing.T) {
	contractTests(t, func(*testing.T) Vault { return NewMemoryVault() })
}

func TestMemoryVaultCopiesInput(t *testing.T) {
	v := NewMemoryVault()
	in := []byte("abc")
	_ = v.Put("a", in)
	in[0] = 'z'
	got, _ := v.Get("a")
	if string(got) != "abc" {
		t.Fatalf("vault aliased caller slice: %q", got)
	}
}

func TestErrorsNeverContainSecret(t *testing.T) {
	secret := "TOP-SECRET-VALUE"
	api := newFakeAPI(10) // far below the secret size once encoded => forces failure
	api.failSetAfter = 0
	v := newKeyringVault(api, 4)
	err := v.Put("a", []byte(secret))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaks secret: %v", err)
	}
}
