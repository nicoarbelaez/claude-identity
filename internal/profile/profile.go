// Package profile models account profiles and persists their metadata.
// It never handles secrets; those live in internal/vault.
package profile

import "time"

// Profile is the non-secret metadata of one saved account.
type Profile struct {
	Name    string    `json:"name"`
	Index   int       `json:"index"`
	Email   string    `json:"email,omitempty"`
	AddedAt time.Time `json:"added_at"`
}
