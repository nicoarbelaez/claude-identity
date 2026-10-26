package profile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cid", "profiles.json")
	return NewStore(path), path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		wantErr error
	}{
		{"valid simple", []string{"isa"}, nil},
		{"valid with punctuation", []string{"work.acct_1-b"}, nil},
		{"digit first", []string{"1abc"}, nil},
		{"uppercase rejected", []string{"Isa"}, ErrInvalidName},
		{"empty rejected", []string{""}, ErrInvalidName},
		{"leading dash rejected", []string{"-x"}, ErrInvalidName},
		{"space rejected", []string{"a b"}, ErrInvalidName},
		{"too long rejected", []string{"abcdefghijklmnopqrstuvwxyz0123456"}, ErrInvalidName},
		{"max length ok", []string{"abcdefghijklmnopqrstuvwxyz012345"}, nil},
		{"duplicate rejected", []string{"isa", "isa"}, ErrDuplicate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newTestStore(t)
			var err error
			for _, n := range tt.names {
				_, err = s.Add(n)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{"isa": true, "1abc": true, "work.acct_1-b": true, "": false, "Isa": false, "a b": false, "-x": false} {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestAddCaseInsensitiveDuplicate(t *testing.T) {
	// Validation forbids uppercase, so seed a mixed-case name by hand.
	s, path := newTestStore(t)
	writeFile(t, path, `{"profiles":[{"name":"Isa","index":1}]}`)
	if _, err := s.Add("isa"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("err = %v, want ErrDuplicate", err)
	}
}

func TestAddAssignsNextIndex(t *testing.T) {
	s, _ := newTestStore(t)
	for i, n := range []string{"a", "b", "c"} {
		p, err := s.Add(n)
		if err != nil {
			t.Fatal(err)
		}
		if p.Index != i+1 {
			t.Fatalf("%s index = %d, want %d", n, p.Index, i+1)
		}
		if p.AddedAt.IsZero() {
			t.Fatal("AddedAt not set")
		}
	}
	if err := s.Remove("b"); err != nil {
		t.Fatal(err)
	}
	p, err := s.Add("d")
	if err != nil {
		t.Fatal(err)
	}
	if p.Index != 4 {
		t.Fatalf("index after removal = %d, want 4 (indexes are never reused)", p.Index)
	}
}

func TestListSortedAndPersisted(t *testing.T) {
	s, path := newTestStore(t)
	for _, n := range []string{"c", "a", "b"} {
		if _, err := s.Add(n); err != nil {
			t.Fatal(err)
		}
	}
	got, err := NewStore(path).List() // fresh store proves persistence
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, p := range got {
		if p.Name != want[i] || p.Index != i+1 {
			t.Fatalf("got[%d] = %+v", i, p)
		}
	}
}

func TestListEmptyWhenNoFile(t *testing.T) {
	s, _ := newTestStore(t)
	got, err := s.List()
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestResolve(t *testing.T) {
	s, _ := newTestStore(t)
	for _, n := range []string{"isa", "work", "7up"} {
		if _, err := s.Add(n); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		arg     string
		want    string
		wantErr error
	}{
		{"1", "isa", nil},
		{"2", "work", nil},
		{"work", "work", nil},
		{"WORK", "work", nil},
		{"7up", "7up", nil}, // name that starts with a digit
		{"9", "", ErrNotFound},
		{"nobody", "", ErrNotFound},
		{"", "", ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			p, err := s.Resolve(tt.arg)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if p.Name != tt.want {
				t.Fatalf("name = %q, want %q", p.Name, tt.want)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Add("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := s.SetActive("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Active(); !errors.Is(err, ErrNoActive) {
		t.Fatalf("active after removing it: err = %v, want ErrNoActive", err)
	}
}

func TestActive(t *testing.T) {
	s, path := newTestStore(t)
	if _, err := s.Active(); !errors.Is(err, ErrNoActive) {
		t.Fatalf("err = %v, want ErrNoActive", err)
	}
	for _, n := range []string{"a", "b"} {
		if _, err := s.Add(n); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetActive("zzz"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := s.SetActive("b"); err != nil {
		t.Fatal(err)
	}
	p, err := NewStore(path).Active()
	if err != nil || p.Name != "b" {
		t.Fatalf("active = %+v, %v", p, err)
	}
}

func TestSetEmail(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Add("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEmail("a", "x@example.com"); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Resolve("a")
	if p.Email != "x@example.com" {
		t.Fatalf("email = %q", p.Email)
	}
	if err := s.SetEmail("nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestFilePermissionsAndNoTempLeftovers(t *testing.T) {
	s, path := newTestStore(t)
	if _, err := s.Add("a"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries, want 1 (temp file leaked?)", len(entries))
	}
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits not enforced on Windows")
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, want 0600", fi.Mode().Perm())
	}
	di, _ := os.Stat(filepath.Dir(path))
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, want 0700", di.Mode().Perm())
	}
}

func TestCorruptFile(t *testing.T) {
	s, path := newTestStore(t)
	writeFile(t, path, "{not json")
	if _, err := s.List(); err == nil {
		t.Fatal("expected error on corrupt file")
	}
}

func TestDefaultPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CID_CONFIG_DIR", dir)
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "profiles.json"); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
