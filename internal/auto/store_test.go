package auto

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
)

func intp(n int) *int { return &n }

func newStores(t *testing.T) (*ConfigStore, *StateStore, string) {
	t.Helper()
	profiles := filepath.Join(t.TempDir(), "cid", "profiles.json")
	return NewConfigStore(PathFor(profiles)), NewStateStore(StatePathFor(profiles)), profiles
}

func TestPaths(t *testing.T) {
	p := filepath.Join("x", "profiles.json")
	if got, want := PathFor(p), filepath.Join("x", "auto.json"); got != want {
		t.Errorf("PathFor = %q, want %q", got, want)
	}
	if got, want := StatePathFor(p), filepath.Join("x", "auto-state.json"); got != want {
		t.Errorf("StatePathFor = %q, want %q", got, want)
	}
	if got, want := LockFor(PathFor(p)), filepath.Join("x", "auto.lock"); got != want {
		t.Errorf("LockFor = %q, want %q", got, want)
	}
}

func TestConfigDefaultsWhenMissing(t *testing.T) {
	cs, _, _ := newStores(t)
	c, err := cs.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Enabled || c.Defaults.FiveHour != 90 || c.Defaults.Week != 95 || len(c.Accounts) != 0 {
		t.Errorf("default config = %+v", c)
	}
	if _, err := os.Stat(cs.f.path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Load must not create the file, stat err = %v", err)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	cs, _, _ := newStores(t)
	err := cs.Update(func(c *Config) error {
		c.Enabled = true
		if err := c.SetDefaults(intp(80), nil); err != nil {
			return err
		}
		if err := c.Enroll("a", intp(70), nil); err != nil {
			return err
		}
		if err := c.Enroll("b", nil, intp(99)); err != nil {
			return err
		}
		return c.SetRole("a", RolePrincipal)
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := cs.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Enabled || c.Defaults.FiveHour != 80 || c.Defaults.Week != 95 {
		t.Errorf("config = %+v", c)
	}
	five, week, ok := c.Lines("a")
	if !ok || five != 70 || week != 95 {
		t.Errorf("Lines(a) = %d,%d,%v", five, week, ok)
	}
	five, week, ok = c.Lines("b")
	if !ok || five != 80 || week != 99 {
		t.Errorf("Lines(b) = %d,%d,%v", five, week, ok)
	}
	if c.Accounts["a"].Role != RolePrincipal {
		t.Errorf("role a = %q", c.Accounts["a"].Role)
	}
	if _, _, ok := c.Lines("zzz"); ok {
		t.Error("Lines of a stranger must report not enrolled")
	}
}

func TestConfigOnDiskShape(t *testing.T) {
	cs, _, _ := newStores(t)
	if err := cs.Update(func(c *Config) error { return c.Enroll("a", nil, nil) }); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(cs.f.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"schemaVersion": 1`, `"enabled": false`, `"fiveHour": 90`, `"week": 95`, `"fiveHour": null`, `"role": ""`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("auto.json misses %s:\n%s", want, raw)
		}
	}
}

func TestConfigRules(t *testing.T) {
	t.Run("percent range", func(t *testing.T) {
		for _, n := range []int{0, -1, 101, 1000} {
			c := NewConfig()
			if err := c.Enroll("a", intp(n), nil); !errors.Is(err, ErrInvalidPercent) {
				t.Errorf("Enroll 5h=%d: err = %v", n, err)
			}
			if err := c.SetDefaults(nil, intp(n)); !errors.Is(err, ErrInvalidPercent) {
				t.Errorf("SetDefaults week=%d: err = %v", n, err)
			}
		}
		c := NewConfig()
		for _, n := range []int{1, 50, 100} {
			if err := c.Enroll("a", intp(n), intp(n)); err != nil {
				t.Errorf("Enroll %d: %v", n, err)
			}
		}
	})
	t.Run("a rejected change leaves the config untouched", func(t *testing.T) {
		c := NewConfig()
		_ = c.Enroll("a", intp(70), intp(80))
		if err := c.Enroll("a", intp(50), intp(500)); err == nil {
			t.Fatal("want error")
		}
		if five, week, _ := c.Lines("a"); five != 70 || week != 80 {
			t.Errorf("lines changed to %d,%d", five, week)
		}
	})
	t.Run("re-add updates only the given lines and keeps the role", func(t *testing.T) {
		c := NewConfig()
		_ = c.Enroll("a", intp(70), intp(80))
		_ = c.SetRole("a", RoleReserve)
		_ = c.Enroll("a", nil, intp(85))
		five, week, _ := c.Lines("a")
		if five != 70 || week != 85 || c.Accounts["a"].Role != RoleReserve {
			t.Errorf("a = %d,%d,%q", five, week, c.Accounts["a"].Role)
		}
	})
	t.Run("at most one principal", func(t *testing.T) {
		c := NewConfig()
		_ = c.Enroll("a", nil, nil)
		_ = c.Enroll("b", nil, nil)
		_ = c.SetRole("a", RolePrincipal)
		_ = c.SetRole("b", RolePrincipal)
		if c.Accounts["a"].Role != RoleNone || c.Accounts["b"].Role != RolePrincipal {
			t.Errorf("roles = %q, %q", c.Accounts["a"].Role, c.Accounts["b"].Role)
		}
	})
	t.Run("a role needs an enrolled account", func(t *testing.T) {
		c := NewConfig()
		if err := c.SetRole("a", RolePrincipal); !errors.Is(err, ErrNotEnrolled) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("remove", func(t *testing.T) {
		c := NewConfig()
		_ = c.Enroll("a", nil, nil)
		if !c.Remove("a") || c.Remove("a") {
			t.Error("Remove must report whether it removed something")
		}
		if _, _, ok := c.Lines("a"); ok {
			t.Error("a still enrolled")
		}
	})
}

func TestConfigForgetOnlyWritesWhenEnrolled(t *testing.T) {
	cs, _, _ := newStores(t)
	if err := cs.Forget("ghost"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cs.f.path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Forget of a stranger must not create the file, stat err = %v", err)
	}
	if err := cs.Update(func(c *Config) error { return c.Enroll("a", nil, nil) }); err != nil {
		t.Fatal(err)
	}
	if err := cs.Forget("a"); err != nil {
		t.Fatal(err)
	}
	c, _ := cs.Load()
	if _, _, ok := c.Lines("a"); ok {
		t.Error("a still enrolled after Forget")
	}
}

func TestConfigCorruptFileIsMovedAside(t *testing.T) {
	for name, content := range map[string]string{
		"not json":          `{`,
		"wrong version":     `{"schemaVersion":2,"enabled":false,"defaults":{"fiveHour":90,"week":95},"accounts":{}}`,
		"percent too big":   `{"schemaVersion":1,"enabled":false,"defaults":{"fiveHour":900,"week":95},"accounts":{}}`,
		"unknown role":      `{"schemaVersion":1,"enabled":false,"defaults":{"fiveHour":90,"week":95},"accounts":{"a":{"fiveHour":null,"week":null,"role":"boss"}}}`,
		"two principals":    `{"schemaVersion":1,"enabled":true,"defaults":{"fiveHour":90,"week":95},"accounts":{"a":{"fiveHour":null,"week":null,"role":"principal"},"b":{"fiveHour":null,"week":null,"role":"principal"}}}`,
		"zero account line": `{"schemaVersion":1,"enabled":false,"defaults":{"fiveHour":90,"week":95},"accounts":{"a":{"fiveHour":0,"week":null,"role":""}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			cs, _, _ := newStores(t)
			if err := os.MkdirAll(filepath.Dir(cs.f.path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cs.f.path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			var notices []fsutil.Notice
			cs.WithNotifier(func(n fsutil.Notice) { notices = append(notices, n) })
			c, err := cs.Load()
			if err != nil {
				t.Fatal(err)
			}
			if c.Enabled || len(c.Accounts) != 0 {
				t.Errorf("config after recovery = %+v", c)
			}
			if len(notices) != 1 || notices[0].Kind != fsutil.KindReset || notices[0].Moved == "" {
				t.Errorf("notices = %+v", notices)
			}
			if _, err := os.Stat(notices[0].Moved); err != nil {
				t.Errorf("corrupt copy not kept: %v", err)
			}
		})
	}
}

func TestStateRoundTrip(t *testing.T) {
	_, ss, _ := newStores(t)
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	err := ss.Update(func(s *State) {
		s.Pending = &Pending{From: "a", To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 91.5, At: at}
		s.LastSwitchAt = at
		s.Notice = &Notice{Text: "x", Until: at.Add(2 * time.Minute)}
		s.LastNoCandidateAt = at
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := ss.Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Pending == nil || s.Pending.To != "b" || s.Pending.Pct != 91.5 || !s.Pending.At.Equal(at) {
		t.Errorf("pending = %+v", s.Pending)
	}
	if !s.LastSwitchAt.Equal(at) || !s.LastNoCandidateAt.Equal(at) || s.Notice == nil || s.Notice.Text != "x" {
		t.Errorf("state = %+v", s)
	}
}

func TestStateEmptyAndShape(t *testing.T) {
	_, ss, _ := newStores(t)
	s, err := ss.Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Pending != nil || s.Notice != nil || !s.LastSwitchAt.IsZero() {
		t.Errorf("empty state = %+v", s)
	}
	if err := ss.Update(func(s *State) { s.LastSwitchAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(ss.f.path)
	for _, want := range []string{`"schemaVersion": 1`, `"pending": null`, `"notice": null`, `"lastSwitchAt": "2026-01-02T03:04:05Z"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("auto-state.json misses %s:\n%s", want, raw)
		}
	}
}

func TestStateForget(t *testing.T) {
	_, ss, _ := newStores(t)
	if err := ss.Forget("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ss.f.path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Forget on a missing file must not create it, stat err = %v", err)
	}
	if err := ss.Update(func(s *State) { s.Pending = &Pending{From: "a", To: "b"} }); err != nil {
		t.Fatal(err)
	}
	if err := ss.Forget("c"); err != nil {
		t.Fatal(err)
	}
	if s, _ := ss.Load(); s.Pending == nil {
		t.Error("pending of other profiles must survive")
	}
	if err := ss.Forget("b"); err != nil {
		t.Fatal(err)
	}
	if s, _ := ss.Load(); s.Pending != nil {
		t.Error("pending that targets a removed profile must be dropped")
	}
}

func TestStateCorruptFileIsMovedAside(t *testing.T) {
	_, ss, _ := newStores(t)
	if err := os.MkdirAll(filepath.Dir(ss.f.path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ss.f.path, []byte(`{"schemaVersion":1,"pending":"nope"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var notices []fsutil.Notice
	ss.WithNotifier(func(n fsutil.Notice) { notices = append(notices, n) })
	s, err := ss.Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Pending != nil || len(notices) != 1 {
		t.Errorf("state = %+v, notices = %+v", s, notices)
	}
}

func TestStoresShareTheLockWithoutBlockingEachOther(t *testing.T) {
	cs, ss, _ := newStores(t)
	if err := cs.Update(func(c *Config) error { c.Enabled = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := ss.Update(func(s *State) { s.LastSwitchAt = time.Now() }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(LockFor(cs.f.path)); err != nil {
		t.Errorf("lock file missing: %v", err)
	}
}

func TestStateHistoryKeepsTheLastEntries(t *testing.T) {
	s := NewStateStore(filepath.Join(t.TempDir(), "auto-state.json"))
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	for i := range MaxHistory + 5 {
		err := s.Update(func(st *State) {
			st.AddHistory(HistoryEntry{At: base.Add(time.Duration(i) * time.Minute), From: "a", To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 91})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.History) != MaxHistory {
		t.Fatalf("history len = %d, want %d", len(st.History), MaxHistory)
	}
	if want := base.Add(5 * time.Minute); !st.History[0].At.Equal(want) {
		t.Errorf("oldest kept = %v, want %v", st.History[0].At, want)
	}
	if want := base.Add(time.Duration(MaxHistory+4) * time.Minute); !st.History[MaxHistory-1].At.Equal(want) {
		t.Errorf("newest = %v, want %v", st.History[MaxHistory-1].At, want)
	}
}
