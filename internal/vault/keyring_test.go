package vault

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/zalando/go-keyring"
)

// fakeAPI mimics the Windows credential limit: values over maxLen are rejected.
type fakeAPI struct {
	maxLen       int
	store        map[string]string
	sets         int
	failSetAfter int // -1 = never fail; otherwise fail once sets > failSetAfter
}

func newFakeAPI(maxLen int) *fakeAPI {
	return &fakeAPI{maxLen: maxLen, store: map[string]string{}, failSetAfter: -1}
}

func (f *fakeAPI) Set(service, user, value string) error {
	f.sets++
	if f.failSetAfter >= 0 && f.sets > f.failSetAfter {
		return errors.New("injected failure")
	}
	if len(value) > f.maxLen {
		return keyring.ErrSetDataTooBig
	}
	f.store[service+"/"+user] = value
	return nil
}

func (f *fakeAPI) Get(service, user string) (string, error) {
	v, ok := f.store[service+"/"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (f *fakeAPI) Delete(service, user string) error {
	k := service + "/" + user
	if _, ok := f.store[k]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.store, k)
	return nil
}

func TestKeyringVaultContractFake(t *testing.T) {
	contractTests(t, func(*testing.T) Vault { return newKeyringVault(newFakeAPI(2560), maxChunkLen) })
}

func TestKeyringVaultContractMockKeyring(t *testing.T) {
	keyring.MockInit()
	contractTests(t, func(t *testing.T) Vault {
		_ = keyring.DeleteAll(ServiceName)
		return NewKeyringVault()
	})
}

func TestKeyringVaultChunking(t *testing.T) {
	tests := []struct {
		name       string
		size       int
		wantChunks int
	}{
		{"empty", 0, 0},
		{"one chunk", 10, 1},
		{"large 16KiB", 16 * 1024, -1}, // computed below
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(2560)
			v := newKeyringVault(api, maxChunkLen)
			secret := bytes.Repeat([]byte{0xAB}, tt.size)
			if err := v.Put("isa", secret); err != nil {
				t.Fatalf("Put: %v", err)
			}
			for k, val := range api.store {
				if len(val) > 2560 {
					t.Fatalf("%s holds %d bytes, over the credential limit", k, len(val))
				}
			}
			got, err := v.Get("isa")
			if err != nil || !bytes.Equal(got, secret) {
				t.Fatalf("round trip failed: len=%d err=%v", len(got), err)
			}
			if tt.wantChunks >= 0 {
				if n := len(api.store) - 1; n != tt.wantChunks { // minus manifest
					t.Fatalf("chunks = %d, want %d", n, tt.wantChunks)
				}
			}
		})
	}
}

func TestKeyringVaultNoPlaintext(t *testing.T) {
	api := newFakeAPI(2560)
	v := newKeyringVault(api, maxChunkLen)
	_ = v.Put("a", []byte("plain-secret"))
	for k, val := range api.store {
		if bytes.Contains([]byte(val), []byte("plain-secret")) {
			t.Fatalf("%s stored raw secret", k)
		}
	}
}

func TestKeyringVaultDeleteRemovesAllChunks(t *testing.T) {
	api := newFakeAPI(2560)
	v := newKeyringVault(api, 8)
	_ = v.Put("a", bytes.Repeat([]byte("x"), 100))
	_ = v.Put("b", []byte("keep"))
	if err := v.Delete("a"); err != nil {
		t.Fatal(err)
	}
	for k := range api.store {
		if k != ServiceName+"/b" && k != ServiceName+"/b#0.0" {
			t.Fatalf("leftover entry %q", k)
		}
	}
}

func TestKeyringVaultShrinkingOverwriteRemovesStaleChunks(t *testing.T) {
	api := newFakeAPI(2560)
	v := newKeyringVault(api, 8)
	_ = v.Put("a", bytes.Repeat([]byte("x"), 100))
	_ = v.Put("a", []byte("y"))
	if len(api.store) != 2 { // manifest + 1 chunk
		t.Fatalf("store has %d entries, want 2: %v", len(api.store), keys(api.store))
	}
}

func TestKeyringVaultFailedPutKeepsOldSecret(t *testing.T) {
	api := newFakeAPI(2560)
	v := newKeyringVault(api, 8)
	if err := v.Put("a", []byte("old-value")); err != nil {
		t.Fatal(err)
	}
	api.sets = 0
	api.failSetAfter = 1 // first chunk write succeeds, second fails
	if err := v.Put("a", bytes.Repeat([]byte("n"), 100)); err == nil {
		t.Fatal("expected failure")
	}
	api.failSetAfter = -1
	got, err := v.Get("a")
	if err != nil || string(got) != "old-value" {
		t.Fatalf("old secret lost: %q, %v", got, err)
	}
}

func TestKeyringVaultCorruptManifest(t *testing.T) {
	api := newFakeAPI(2560)
	v := newKeyringVault(api, maxChunkLen)
	api.store[ServiceName+"/a"] = "garbage"
	if _, err := v.Get("a"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want corruption error", err)
	}
}

func TestKeyringVaultMissingChunk(t *testing.T) {
	api := newFakeAPI(2560)
	v := newKeyringVault(api, 8)
	_ = v.Put("a", bytes.Repeat([]byte("x"), 100))
	delete(api.store, ServiceName+"/a#0.1")
	if _, err := v.Get("a"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want corruption error", err)
	}
}

func TestMaxChunkLenFitsWindowsLimit(t *testing.T) {
	// go-keyring rejects Windows passwords over 2560 bytes; macOS ~3000 total.
	if maxChunkLen > 2560-64 {
		t.Fatal(fmt.Sprintf("maxChunkLen %d leaves too little headroom", maxChunkLen))
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
