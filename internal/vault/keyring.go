package vault

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/zalando/go-keyring"
)

// ServiceName is the keyring service under which all entries are stored.
const ServiceName = "cid"

// maxChunkLen is the size in characters of one stored chunk. go-keyring
// rejects Windows passwords over 2560 bytes and macOS entries over ~3000
// bytes (service + user + password), so 2000 leaves headroom on both.
const maxChunkLen = 2000

var errCorrupt = errors.New("vault entry is corrupt")

// keyringAPI is the subset of go-keyring used here, so chunking is testable.
type keyringAPI interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type osKeyring struct{}

func (osKeyring) Set(s, u, p string) error        { return keyring.Set(s, u, p) }
func (osKeyring) Get(s, u string) (string, error) { return keyring.Get(s, u) }
func (osKeyring) Delete(s, u string) error        { return keyring.Delete(s, u) }

// KeyringVault stores secrets in the OS keyring. Each secret is base64-encoded
// and split into chunks stored as "<profile>#<gen>.<i>"; a manifest entry
// "<profile>" ("v1:<gen>:<count>") points at the live generation. Writing a
// new generation before flipping the manifest keeps the old secret intact if
// a Put fails midway.
type KeyringVault struct {
	api      keyringAPI
	chunkLen int
}

// NewKeyringVault returns a Vault backed by the OS keyring.
func NewKeyringVault() *KeyringVault { return newKeyringVault(osKeyring{}, maxChunkLen) }

func newKeyringVault(api keyringAPI, chunkLen int) *KeyringVault {
	return &KeyringVault{api: api, chunkLen: chunkLen}
}

func chunkKey(profile string, gen, i int) string {
	return profile + "#" + strconv.Itoa(gen) + "." + strconv.Itoa(i)
}

func (v *KeyringVault) Put(profile string, secret []byte) error {
	gen := 0
	oldGen, oldN, oldErr := v.manifest(profile)
	if oldErr == nil {
		gen = 1 - oldGen
	}
	enc := base64.StdEncoding.EncodeToString(secret)
	n := 0
	for off := 0; off < len(enc); off += v.chunkLen {
		end := min(off+v.chunkLen, len(enc))
		if err := v.api.Set(ServiceName, chunkKey(profile, gen, n), enc[off:end]); err != nil {
			v.deleteChunks(profile, gen, n)
			return fmt.Errorf("store chunk %d: %w", n, err)
		}
		n++
	}
	if err := v.api.Set(ServiceName, profile, fmt.Sprintf("v1:%d:%d", gen, n)); err != nil {
		v.deleteChunks(profile, gen, n)
		return fmt.Errorf("store manifest: %w", err)
	}
	if oldErr == nil {
		v.deleteChunks(profile, oldGen, oldN)
	}
	return nil
}

func (v *KeyringVault) Get(profile string) ([]byte, error) {
	gen, n, err := v.manifest(profile)
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		part, err := v.api.Get(ServiceName, chunkKey(profile, gen, i))
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, fmt.Errorf("%w: missing chunk %d", errCorrupt, i)
		}
		if err != nil {
			return nil, fmt.Errorf("read chunk %d: %w", i, err)
		}
		sb.WriteString(part)
	}
	out, err := base64.StdEncoding.DecodeString(sb.String())
	if err != nil {
		return nil, fmt.Errorf("%w: bad encoding", errCorrupt)
	}
	return out, nil
}

func (v *KeyringVault) Delete(profile string) error {
	gen, n, err := v.manifest(profile)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return err
		}
		// Corrupt manifest: still remove it so the profile can be re-added.
		_ = v.api.Delete(ServiceName, profile)
		return err
	}
	v.deleteChunks(profile, gen, n)
	if err := v.api.Delete(ServiceName, profile); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("delete manifest: %w", err)
	}
	return nil
}

// manifest reads and parses the manifest entry for profile.
func (v *KeyringVault) manifest(profile string) (gen, n int, err error) {
	raw, err := v.api.Get(ServiceName, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read manifest: %w", err)
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 3 || parts[0] != "v1" {
		return 0, 0, fmt.Errorf("%w: bad manifest", errCorrupt)
	}
	gen, e1 := strconv.Atoi(parts[1])
	n, e2 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || (gen != 0 && gen != 1) || n < 0 {
		return 0, 0, fmt.Errorf("%w: bad manifest", errCorrupt)
	}
	return gen, n, nil
}

// deleteChunks removes chunks best-effort; stale chunks are harmless.
func (v *KeyringVault) deleteChunks(profile string, gen, n int) {
	for i := 0; i < n; i++ {
		_ = v.api.Delete(ServiceName, chunkKey(profile, gen, i))
	}
}
