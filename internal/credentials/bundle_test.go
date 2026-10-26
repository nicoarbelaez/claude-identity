package credentials

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

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
