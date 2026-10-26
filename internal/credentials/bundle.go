// Package credentials defines the secret payload cid keeps per profile.
package credentials

import (
	"encoding/json"
	"errors"
	"time"
)

// Bundle is everything needed to restore one Claude Code identity: the OAuth
// credentials block and the account metadata. Treat it as a secret.
type Bundle struct {
	ClaudeAIOAuth json.RawMessage `json:"claudeAiOauth"`
	OAuthAccount  json.RawMessage `json:"oauth_account,omitempty"`
	CapturedAt    time.Time       `json:"captured_at"`
}

var (
	errMissingOAuth = errors.New("credentials bundle has no OAuth credentials")
	errMalformed    = errors.New("credentials bundle is malformed")
)

// Email returns the account email from the oauth account block, or "".
func (b Bundle) Email() string {
	var a struct {
		Email string `json:"emailAddress"`
	}
	if json.Unmarshal(b.OAuthAccount, &a) != nil {
		return ""
	}
	return a.Email
}

// ExpiresAt returns the access token expiry (Unix ms), or 0 if unknown.
func (b Bundle) ExpiresAt() int64 {
	var o struct {
		ExpiresAt int64 `json:"expiresAt"`
	}
	if json.Unmarshal(b.ClaudeAIOAuth, &o) != nil {
		return 0
	}
	return o.ExpiresAt
}

// Marshal serializes the bundle. It fails if OAuth credentials are absent.
func (b Bundle) Marshal() ([]byte, error) {
	if len(b.ClaudeAIOAuth) == 0 {
		return nil, errMissingOAuth
	}
	return json.Marshal(b)
}

// Unmarshal parses a bundle. Errors never include payload content.
func Unmarshal(data []byte) (Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return Bundle{}, errMalformed
	}
	if len(b.ClaudeAIOAuth) == 0 {
		return Bundle{}, errMissingOAuth
	}
	return b, nil
}
