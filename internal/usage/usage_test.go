package usage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cid", "usage.json")
	return NewStore(path), path
}

func win(pct float64, in time.Duration) Window {
	return Window{PercentUsed: pct, ResetsAt: t0.Add(in)}
}

func TestRecordAndGet(t *testing.T) {
	s, _ := newStore(t)
	w := map[string]Window{FiveHour: win(42, 3*time.Hour), SevenDay: win(10, 48*time.Hour)}
	if err := s.Record("a", w, t0); err != nil {
		t.Fatal(err)
	}
	e, ok, err := s.Get("a")
	if err != nil || !ok {
		t.Fatalf("Get = %v, %v", ok, err)
	}
	if !e.FetchedAt.Equal(t0) || e.Windows[FiveHour] != w[FiveHour] || e.Windows[SevenDay] != w[SevenDay] {
		t.Errorf("entry = %+v", e)
	}
	if _, ok, _ := s.Get("missing"); ok {
		t.Error("Get(missing) reported a reading")
	}
}

func TestRecordReplacesOnlyThatProfile(t *testing.T) {
	s, _ := newStore(t)
	_ = s.Record("a", map[string]Window{FiveHour: win(10, time.Hour), SevenDay: win(20, time.Hour)}, t0)
	_ = s.Record("b", map[string]Window{FiveHour: win(30, time.Hour)}, t0)
	if err := s.Record("a", map[string]Window{FiveHour: win(55, time.Hour)}, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	all, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || len(all["a"].Windows) != 1 || all["a"].Windows[FiveHour].PercentUsed != 55 {
		t.Errorf("a not replaced: %+v", all["a"])
	}
	if all["b"].Windows[FiveHour].PercentUsed != 30 {
		t.Errorf("b changed: %+v", all["b"])
	}
}

func TestForget(t *testing.T) {
	s, path := newStore(t)
	if err := s.Forget("a"); err != nil { // nothing stored: no file is created
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Forget on empty cache created the file (err %v)", err)
	}
	_ = s.Record("a", map[string]Window{FiveHour: win(1, time.Hour)}, t0)
	_ = s.Record("b", map[string]Window{FiveHour: win(2, time.Hour)}, t0)
	if err := s.Forget("a"); err != nil {
		t.Fatal(err)
	}
	all, _ := s.All()
	if _, ok := all["a"]; ok || len(all) != 1 {
		t.Errorf("after Forget(a): %+v", all)
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	s, _ := newStore(t)
	all, err := s.All()
	if err != nil || len(all) != 0 {
		t.Errorf("All = %v, %v", all, err)
	}
}

func TestInvalidFileIsMovedAsideAndNoticed(t *testing.T) {
	tests := []struct{ name, content string }{
		{"not json", "{oops"},
		{"wrong version", `{"schemaVersion":2,"profiles":{}}`},
		{"bad percent", `{"schemaVersion":1,"profiles":{"a":{"fetchedAt":"2026-01-02T12:00:00Z","windows":{"five_hour":{"percentUsed":"x","resetsAt":"2026-01-02T15:00:00Z"}}}}}`},
		{"bad time", `{"schemaVersion":1,"profiles":{"a":{"fetchedAt":"yesterday","windows":{}}}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, path := newStore(t)
			var got []fsutil.Notice
			s.WithNotifier(func(n fsutil.Notice) { got = append(got, n) })
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := s.Record("a", map[string]Window{FiveHour: win(5, time.Hour)}, t0); err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].Kind != fsutil.KindReset {
				t.Fatalf("notices = %+v", got)
			}
			if _, err := os.Stat(got[0].Moved); err != nil {
				t.Errorf("broken copy not kept: %v", err)
			}
			if e, ok, _ := s.Get("a"); !ok || e.Windows[FiveHour].PercentUsed != 5 {
				t.Errorf("fresh record missing after recovery: %+v", e)
			}
		})
	}
}

func TestRecordSkipsWhenLockHeld(t *testing.T) {
	s, path := newStore(t)
	s.timeout = 100 * time.Millisecond
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	held := flock.New(LockFor(path))
	if ok, err := held.TryLock(); err != nil || !ok {
		t.Fatalf("setup lock: %v %v", ok, err)
	}
	defer func() { _ = held.Unlock() }()

	err := s.Record("a", map[string]Window{FiveHour: win(5, time.Hour)}, t0)
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("err = %v, want ErrLockTimeout", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("a skipped record must not write the file")
	}
}

func TestPaths(t *testing.T) {
	p := PathFor(filepath.Join("x", "cid", "profiles.json"))
	if p != filepath.Join("x", "cid", "usage.json") || LockFor(p) != filepath.Join("x", "cid", "usage.lock") {
		t.Errorf("paths = %q, %q", p, LockFor(p))
	}
}
