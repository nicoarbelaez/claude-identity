package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isObject(b []byte) error {
	s := strings.TrimSpace(string(b))
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		return nil
	}
	return errors.New("not an object")
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWriteAtomicCreatesDirAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b.json")
	if err := WriteAtomic(path, []byte(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte(`{"v":2}`)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != `{"v":2}` {
		t.Errorf("content = %q", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("leftover files: %v", entries)
	}
}

func TestWriteAtomicBackupKeepsBak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	if err := WriteAtomicBackup(path, []byte(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, BackupPath(path)); got != `{"v":1}` {
		t.Errorf("bak = %q", got)
	}
}

func TestLoadValidatedMissingAndValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	data, n, err := LoadValidated(path, isObject)
	if err != nil || data != nil || n != nil {
		t.Fatalf("missing: data=%q n=%v err=%v", data, n, err)
	}
	write(t, path, `{"ok":true}`)
	data, n, err = LoadValidated(path, isObject)
	if err != nil || string(data) != `{"ok":true}` || n != nil {
		t.Fatalf("valid: data=%q n=%v err=%v", data, n, err)
	}
}

func TestLoadValidatedCorruptWithoutBackupStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	write(t, path, `{"ok":`)
	data, n, err := LoadValidated(path, isObject)
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Errorf("data = %q, want nil", data)
	}
	if n == nil || n.Kind != KindReset || n.Path != path {
		t.Fatalf("notice = %+v", n)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("broken file still in place: %v", err)
	}
	if !strings.Contains(n.Moved, ".corrupt-") || read(t, n.Moved) != `{"ok":` {
		t.Errorf("moved copy missing or altered: %q", n.Moved)
	}
}

func TestLoadValidatedRestoresValidBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	write(t, path, `garbage`)
	write(t, BackupPath(path), `{"good":1}`)
	data, n, err := LoadValidated(path, isObject)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"good":1}` || n == nil || n.Kind != KindRestored {
		t.Fatalf("data=%q n=%+v", data, n)
	}
	if got := read(t, path); got != `{"good":1}` {
		t.Errorf("restored file = %q", got)
	}
	if read(t, n.Moved) != `garbage` {
		t.Error("broken copy not kept")
	}
}

func TestLoadValidatedInvalidBackupStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	write(t, path, `garbage`)
	write(t, BackupPath(path), `also garbage`)
	data, n, err := LoadValidated(path, isObject)
	if err != nil {
		t.Fatal(err)
	}
	if data != nil || n == nil || n.Kind != KindReset {
		t.Fatalf("data=%q n=%+v", data, n)
	}
}

func TestLoadValidatedCorruptNamesDoNotCollide(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	seen := map[string]bool{}
	for range 3 {
		write(t, path, `bad`)
		_, n, err := LoadValidated(path, isObject)
		if err != nil || n == nil {
			t.Fatalf("n=%v err=%v", n, err)
		}
		if seen[n.Moved] {
			t.Fatalf("collision on %s", n.Moved)
		}
		seen[n.Moved] = true
	}
}
