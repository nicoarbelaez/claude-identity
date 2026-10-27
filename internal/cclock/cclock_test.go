package cclock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fast(dir string, stale time.Duration) Options {
	return Options{Dir: dir, Stale: stale, Timeout: 200 * time.Millisecond, Poll: 5 * time.Millisecond, Touch: 20 * time.Millisecond}
}

func TestAcquireCreatesAndReleasesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "x.lock")
	rel, err := Acquire(fast(dir, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("lock dir missing: %v", err)
	}
	rel()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("lock dir not removed: %v", err)
	}
	rel() // idempotent
}

func TestAcquireTimesOutWhenFreshlyHeld(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x.lock")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Acquire(fast(dir, time.Minute))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("someone else's lock was removed: %v", err)
	}
}

func TestAcquireTakesOverStaleLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x.lock")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatal(err)
	}
	rel, err := Acquire(fast(dir, time.Second))
	if err != nil {
		t.Fatalf("stale lock not taken over: %v", err)
	}
	rel()
	if _, err := os.Stat(dir + ".takeover"); !os.IsNotExist(err) {
		t.Fatalf("takeover dir left behind: %v", err)
	}
}

func TestStaleTakeoverDirIsClearedAndNeverBlocksForever(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x.lock")
	for _, d := range []string{dir, dir + ".takeover"} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-time.Hour)
		if err := os.Chtimes(d, old, old); err != nil {
			t.Fatal(err)
		}
	}
	o := fast(dir, time.Second)
	o.Timeout = 2 * time.Second
	rel, err := Acquire(o)
	if err != nil {
		t.Fatalf("dead takeover dir blocked acquire: %v", err)
	}
	rel()
}

func TestHeldLockIsTouchedSoItIsNotStale(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x.lock")
	o := fast(dir, 100*time.Millisecond)
	o.Touch = 10 * time.Millisecond
	rel, err := Acquire(o)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	time.Sleep(350 * time.Millisecond) // well past Stale
	o2 := fast(dir, 100*time.Millisecond)
	o2.Timeout = 100 * time.Millisecond
	if _, err := Acquire(o2); !errors.Is(err, ErrTimeout) {
		t.Fatalf("live lock was stolen: %v", err)
	}
}

func TestReleaseLeavesForeignLockAlone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x.lock")
	o := fast(dir, time.Minute)
	o.Touch = time.Hour
	rel, err := Acquire(o)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a takeover: the dir is replaced by another holder's lock.
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(dir, future, future); err != nil {
		t.Fatal(err)
	}
	rel()
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("release removed another holder's lock: %v", err)
	}
}

func TestDirsFollowClaudeConfigDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Run("default", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", "")
		d := ResolveDirs(home)
		want := Dirs{
			OAuthRefresh: filepath.Join(home, ".claude", ".oauth_refresh.lock"),
			Legacy:       filepath.Join(home, ".claude.lock"),
			Config:       filepath.Join(home, ".claude.json.lock"),
		}
		if d != want {
			t.Fatalf("dirs = %+v, want %+v", d, want)
		}
	})
	t.Run("CLAUDE_CONFIG_DIR", func(t *testing.T) {
		cfg := filepath.Join(t.TempDir(), "alt")
		t.Setenv("CLAUDE_CONFIG_DIR", cfg)
		d := ResolveDirs(home)
		want := Dirs{
			OAuthRefresh: filepath.Join(cfg, ".oauth_refresh.lock"),
			Legacy:       cfg + ".lock",
			Config:       filepath.Join(cfg, ".claude.json.lock"),
		}
		if d != want {
			t.Fatalf("dirs = %+v, want %+v", d, want)
		}
	})
	t.Run("legacy .config.json wins when non-empty", func(t *testing.T) {
		cfg := filepath.Join(t.TempDir(), "alt")
		t.Setenv("CLAUDE_CONFIG_DIR", cfg)
		if err := os.MkdirAll(cfg, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg, ".config.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := ResolveDirs(home).Config; got != filepath.Join(cfg, ".config.json.lock") {
			t.Fatalf("config lock = %s", got)
		}
	})
}

func TestHoldTakesAllLocksInOrderAndReleasesThem(t *testing.T) {
	root := t.TempDir()
	set := Set{
		Dirs:    Dirs{OAuthRefresh: filepath.Join(root, "a", ".oauth_refresh.lock"), Legacy: filepath.Join(root, "a.lock"), Config: filepath.Join(root, "c.json.lock")},
		Timeout: 200 * time.Millisecond, Poll: 5 * time.Millisecond,
	}
	ran := false
	err := set.Hold(func() error {
		ran = true
		for _, d := range []string{set.Dirs.OAuthRefresh, set.Dirs.Legacy, set.Dirs.Config} {
			if _, err := os.Stat(d); err != nil {
				t.Errorf("lock %s not held inside Hold: %v", filepath.Base(d), err)
			}
		}
		return nil
	})
	if err != nil || !ran {
		t.Fatalf("err = %v ran = %v", err, ran)
	}
	for _, d := range []string{set.Dirs.OAuthRefresh, set.Dirs.Legacy, set.Dirs.Config} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("lock %s not released", filepath.Base(d))
		}
	}
}

func TestHoldTimeoutRunsNothingAndReleasesEarlierLocks(t *testing.T) {
	root := t.TempDir()
	set := Set{
		Dirs:    Dirs{OAuthRefresh: filepath.Join(root, "o.lock"), Legacy: filepath.Join(root, "l.lock"), Config: filepath.Join(root, "c.lock")},
		Timeout: 100 * time.Millisecond, Poll: 5 * time.Millisecond,
	}
	if err := os.Mkdir(set.Dirs.Config, 0o700); err != nil { // another holder
		t.Fatal(err)
	}
	ran := false
	err := set.Hold(func() error { ran = true; return nil })
	if !errors.Is(err, ErrTimeout) || ran {
		t.Fatalf("err = %v ran = %v", err, ran)
	}
	for _, d := range []string{set.Dirs.OAuthRefresh, set.Dirs.Legacy} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("lock %s leaked after timeout", filepath.Base(d))
		}
	}
}
