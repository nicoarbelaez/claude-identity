package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
)

type fakeEnv map[string]string

func (e fakeEnv) get(k string) string { return e[k] }

func newStore(t *testing.T, env fakeEnv, content *string) (*Store, string, *[]fsutil.Notice) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cid", "config.json")
	if content != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(*content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var got []fsutil.Notice
	s := NewStore(path, env.get).WithNotifier(func(n fsutil.Notice) { got = append(got, n) })
	return s, path, &got
}

func str(s string) *string { return &s }

func lang(t *testing.T, s *Store) Value {
	t.Helper()
	v, err := s.Get("language")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDefaultWhenNoFile(t *testing.T) {
	s, _, got := newStore(t, nil, nil)
	if v := lang(t, s); v.Value != "auto" || v.Source != SourceDefault {
		t.Errorf("got %+v", v)
	}
	if len(*got) != 0 {
		t.Errorf("notices: %v", *got)
	}
}

func TestPrecedenceEnvOverFileOverDefault(t *testing.T) {
	s, _, _ := newStore(t, fakeEnv{"CID_LANG": "es_CO.UTF-8"}, str(`{"language":"en"}`))
	if v := lang(t, s); v.Value != "es" || v.Source != SourceEnv {
		t.Errorf("env: %+v", v)
	}
	s, _, _ = newStore(t, fakeEnv{}, str(`{"language":"en"}`))
	if v := lang(t, s); v.Value != "en" || v.Source != SourceFile {
		t.Errorf("file: %+v", v)
	}
	// An unusable env value is skipped, not fatal.
	s, _, _ = newStore(t, fakeEnv{"CID_LANG": "klingon"}, str(`{"language":"en"}`))
	if v := lang(t, s); v.Value != "en" || v.Source != SourceFile {
		t.Errorf("bad env: %+v", v)
	}
}

func TestSetUnsetRoundTrip(t *testing.T) {
	s, path, _ := newStore(t, nil, nil)
	got, err := s.Set("language", "ES")
	if err != nil || got != "es" {
		t.Fatalf("Set = %q, %v", got, err)
	}
	if v := lang(t, NewStore(path, fakeEnv{}.get)); v.Value != "es" || v.Source != SourceFile {
		t.Errorf("not persisted: %+v", v)
	}
	if _, err := os.Stat(fsutil.BackupPath(path)); err != nil {
		t.Errorf("no .bak after set: %v", err)
	}
	if err := s.Unset("language"); err != nil {
		t.Fatal(err)
	}
	if v := lang(t, s); v.Value != "auto" || v.Source != SourceDefault {
		t.Errorf("after unset: %+v", v)
	}
	if err := s.Unset("language"); err != nil { // unsetting an unset key is fine
		t.Errorf("second unset: %v", err)
	}
}

func TestSetRejectsUnknownKeyAndBadValue(t *testing.T) {
	s, path, _ := newStore(t, nil, nil)
	if _, err := s.Set("nope", "x"); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("unknown key err = %v", err)
	}
	if err := s.Unset("nope"); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("unset unknown err = %v", err)
	}
	if _, err := s.Set("language", "fr"); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("bad value err = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Error("a rejected set must not create the file")
	}
}

func TestTruncatedFileIsMovedAside(t *testing.T) {
	s, path, got := newStore(t, nil, str(`{"language": "e`))
	if v := lang(t, s); v.Value != "auto" || v.Source != SourceDefault {
		t.Errorf("got %+v", v)
	}
	if len(*got) != 1 || (*got)[0].Kind != fsutil.KindReset {
		t.Fatalf("notices = %+v", *got)
	}
	if m, _ := filepath.Glob(path + ".corrupt-*"); len(m) != 1 {
		t.Errorf("corrupt copies: %v", m)
	}
	if _, err := s.Set("language", "es"); err != nil {
		t.Errorf("Set after recovery: %v", err)
	}
}

func TestWrongRootTypeIsMovedAside(t *testing.T) {
	for _, c := range []string{`[]`, `"x"`, `null`, `5`} {
		s, _, got := newStore(t, nil, str(c))
		if v := lang(t, s); v.Source != SourceDefault {
			t.Errorf("%s: %+v", c, v)
		}
		if len(*got) != 1 || (*got)[0].Kind != fsutil.KindReset {
			t.Errorf("%s: notices = %+v", c, *got)
		}
	}
}

func TestRestoresValidBackup(t *testing.T) {
	s, path, got := newStore(t, nil, str(`garbage`))
	if err := os.WriteFile(fsutil.BackupPath(path), []byte(`{"language":"es"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if v := lang(t, s); v.Value != "es" || v.Source != SourceFile {
		t.Errorf("got %+v", v)
	}
	if len(*got) != 1 || (*got)[0].Kind != fsutil.KindRestored {
		t.Errorf("notices = %+v", *got)
	}
}

func TestUnknownKeyWarnsOnceAndIsPreserved(t *testing.T) {
	s, path, got := newStore(t, nil, str(`{"language":"en","theme":"dark"}`))
	lang(t, s)
	lang(t, s)
	if len(*got) != 1 || (*got)[0].Kind != fsutil.KindUnknownKey || (*got)[0].Detail != "theme" {
		t.Fatalf("notices = %+v", *got)
	}
	if _, err := s.Set("language", "es"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"theme"`) {
		t.Errorf("unknown key dropped on write: %s", b)
	}
}

func TestInvalidValuesFallBackToDefault(t *testing.T) {
	for _, c := range []string{`{"language":"fr"}`, `{"language":5}`, `{"language":null}`, `{"language":["es"]}`, `{"language":"ES "}`} {
		s, _, got := newStore(t, nil, str(c))
		v := lang(t, s)
		if v.Value != "auto" || v.Source != SourceDefault {
			t.Errorf("%s: got %+v", c, v)
		}
		if len(*got) != 1 || (*got)[0].Kind != fsutil.KindInvalidValue || (*got)[0].Detail != "language" {
			t.Errorf("%s: notices = %+v", c, *got)
		}
	}
}

func TestResolveListsEveryKey(t *testing.T) {
	s, _, _ := newStore(t, nil, nil)
	vs, err := s.Resolve()
	if err != nil || len(vs) != len(Defs()) || vs[0].Key != "language" {
		t.Fatalf("vs=%v err=%v", vs, err)
	}
	if _, err := s.Get("nope"); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("Get unknown err = %v", err)
	}
}
