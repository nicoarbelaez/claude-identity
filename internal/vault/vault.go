// Package vault stores per-profile secrets in a secure backend.
package vault

import (
	"errors"
	"sync"
)

// ErrNotFound is returned when a profile has no stored secret.
var ErrNotFound = errors.New("secret not found")

// Vault is the port for secret storage. Implementations must never include
// secret bytes in returned errors.
type Vault interface {
	Put(profile string, secret []byte) error
	Get(profile string) ([]byte, error)
	Delete(profile string) error
}

// MemoryVault is an in-memory Vault for tests.
type MemoryVault struct {
	mu sync.Mutex
	m  map[string][]byte
}

// NewMemoryVault returns an empty MemoryVault.
func NewMemoryVault() *MemoryVault { return &MemoryVault{m: map[string][]byte{}} }

func (v *MemoryVault) Put(profile string, secret []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.m[profile] = append([]byte(nil), secret...)
	return nil
}

func (v *MemoryVault) Get(profile string) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	s, ok := v.m[profile]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), s...), nil
}

func (v *MemoryVault) Delete(profile string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.m[profile]; !ok {
		return ErrNotFound
	}
	delete(v.m, profile)
	return nil
}
