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

// Status is a local, best-effort view of whether a bundle can still sign in.
type Status string

// Status values. They are stable strings used in JSON output.
const (
	StatusOK            Status = "ok"
	StatusAccessExpired Status = "access_expired"
	StatusNeedsLogin    Status = "needs_login"
	StatusUnknown       Status = "unknown"
)

// Status classifies the bundle at time now. It only checks that a refresh
// token is present, never its value or validity, so a stale stored copy can
// still report ok. A payload with no access token, no refresh token and no
// expiry (null, {} or all-empty fields) is unknown, not needs_login.
func (b Bundle) Status(now time.Time) Status {
	var o struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"`
	}
	if json.Unmarshal(b.ClaudeAIOAuth, &o) != nil {
		return StatusUnknown
	}
	switch {
	case o.AccessToken == "" && o.RefreshToken == "" && o.ExpiresAt == 0:
		return StatusUnknown
	case o.RefreshToken == "":
		return StatusNeedsLogin
	case o.ExpiresAt <= 0:
		return StatusUnknown
	case o.ExpiresAt <= now.UnixMilli():
		return StatusAccessExpired
	}
	return StatusOK
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
