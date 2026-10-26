package credentials

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBundleAccessors(t *testing.T) {
	b := Bundle{
		ClaudeAIOAuth: json.RawMessage(`{"accessToken":"a","expiresAt":1234}`),
		OAuthAccount:  json.RawMessage(`{"emailAddress":"x@example.com"}`),
	}
	if got := b.Email(); got != "x@example.com" {
		t.Errorf("Email() = %q", got)
	}
	if got := b.ExpiresAt(); got != 1234 {
		t.Errorf("ExpiresAt() = %d", got)
	}
	var empty Bundle
	if empty.Email() != "" || empty.ExpiresAt() != 0 {
		t.Error("empty bundle should yield zero values")
	}
}

func TestBundleRoundTrip(t *testing.T) {
	in := Bundle{
		ClaudeAIOAuth: json.RawMessage(`{"accessToken":"a","refreshToken":"r","expiresAt":1}`),
		OAuthAccount:  json.RawMessage(`{"emailAddress":"x@example.com"}`),
		CapturedAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	b, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	out, err := Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(out.ClaudeAIOAuth) != string(in.ClaudeAIOAuth) ||
		string(out.OAuthAccount) != string(in.OAuthAccount) ||
		!out.CapturedAt.Equal(in.CapturedAt) {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}

func TestUnmarshalErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"not json", "{oops"},
		{"empty", ""},
		{"missing oauth", `{"oauth_account":{},"captured_at":"2026-01-02T03:04:05Z"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Unmarshal([]byte(tt.in))
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestMarshalRejectsMissingOAuth(t *testing.T) {
	if _, err := (Bundle{}).Marshal(); err == nil {
		t.Fatal("expected error for empty bundle")
	}
}

func TestUnmarshalErrorDoesNotLeakSecret(t *testing.T) {
	_, err := Unmarshal([]byte(`{"claudeAiOauth": SECRET-TOKEN-123`))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "SECRET-TOKEN-123") {
		t.Fatalf("error leaks payload: %v", err)
	}
}

func TestBundleStatus(t *testing.T) {
	now := time.UnixMilli(1_000_000)
	tests := []struct {
		name  string
		oauth string
		want  Status
	}{
		{"valid", `{"refreshToken":"r","expiresAt":2000000}`, StatusOK},
		{"access expired", `{"refreshToken":"r","expiresAt":500000}`, StatusAccessExpired},
		{"empty refresh token, future expiry", `{"refreshToken":"","expiresAt":2000000}`, StatusNeedsLogin},
		{"missing refresh token", `{"expiresAt":2000000}`, StatusNeedsLogin},
		{"missing expiry", `{"refreshToken":"r"}`, StatusUnknown},
		{"invalid expiry", `{"refreshToken":"r","expiresAt":"soon"}`, StatusUnknown},
		{"not json", `nope`, StatusUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := Bundle{ClaudeAIOAuth: json.RawMessage(tt.oauth)}
			if got := b.Status(now); got != tt.want {
				t.Errorf("Status() = %q, want %q", got, tt.want)
			}
		})
	}
}
