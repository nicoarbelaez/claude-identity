// Package cclock cooperates with Claude Code's own advisory locks while cid
// rewrites its login files.
//
// Claude Code guards its OAuth token refresh and its ~/.claude.json writes with
// the npm proper-lockfile package. The lock is a directory: mkdir is the mutex,
// the holder touches the directory mtime while it holds it, and a lock whose
// mtime is older than a stale threshold belongs to a dead holder and may be
// taken over. cid takes the same locks, in the same order, around a swap, so a
// token refresh that Claude Code is doing cannot overwrite the swapped login.
//
// The protocol and the paths mirror claude-swap (MIT), claude_locks.py and
// paths.py. Nothing here touches the network or reads credential values.
package cclock

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Defaults of the proper-lockfile options Claude Code uses.
const (
	// CredentialsStale is how old a credentials lock must be to count as dead.
	CredentialsStale = 60 * time.Second
	// ConfigStale is how old the config lock must be to count as dead.
	ConfigStale = 10 * time.Second
	// TouchInterval is how often a held lock's mtime is refreshed (a little
	// faster than Claude Code's 5 s).
	TouchInterval = 3 * time.Second
	// DefaultTimeout is the per-lock budget for waiting.
	DefaultTimeout = 9 * time.Second

	takeoverSuffix = ".takeover"
	takeoverStale  = 5 * time.Second
)

// ErrTimeout means a Claude Code lock stayed held for the whole timeout.
var ErrTimeout = errors.New("Claude Code is holding its login lock; try again in a few seconds")

// Options describes one lock acquisition. Zero fields take the defaults.
type Options struct {
	Dir     string
	Stale   time.Duration // default ConfigStale
	Timeout time.Duration // default DefaultTimeout
	Touch   time.Duration // default TouchInterval
	Poll    time.Duration // base wait between attempts; default 250 ms
}

func (o Options) withDefaults() Options {
	if o.Stale <= 0 {
		o.Stale = ConfigStale
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Touch <= 0 {
		o.Touch = TouchInterval
	}
	if o.Poll <= 0 {
		o.Poll = 250 * time.Millisecond
	}
	return o
}

// Acquire takes the directory lock and returns its release function, which is
// idempotent. It waits up to o.Timeout (ErrTimeout wrapped with the lock name),
// taking over a lock whose mtime is older than o.Stale.
func Acquire(o Options) (release func(), err error) {
	o = o.withDefaults()
	if err := os.MkdirAll(filepath.Dir(o.Dir), 0o700); err != nil {
		return nil, fmt.Errorf("create lock parent: %w", err)
	}
	start := time.Now()
	for {
		err := os.Mkdir(o.Dir, 0o700)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("take %s: %w", filepath.Base(o.Dir), err)
		}
		if time.Since(start) > o.Timeout {
			return nil, fmt.Errorf("%w (%s)", ErrTimeout, filepath.Base(o.Dir))
		}
		fi, serr := os.Stat(o.Dir)
		if errors.Is(serr, os.ErrNotExist) {
			continue // released between mkdir and stat
		}
		if serr == nil && time.Since(fi.ModTime()) > o.Stale {
			if !removeIfStillStale(o.Dir, o.Stale) {
				time.Sleep(50 * time.Millisecond)
			}
			continue
		}
		time.Sleep(o.Poll + time.Duration(rand.Int64N(int64(o.Poll)+1)))
	}
	return hold(o), nil
}

// removeIfStillStale removes dir when it is still stale, serialized through the
// sibling .takeover directory so two waiters that saw the same dead holder
// cannot remove each other's fresh lock. Claude Code never looks at it.
func removeIfStillStale(dir string, stale time.Duration) bool {
	takeover := dir + takeoverSuffix
	if err := os.Mkdir(takeover, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			if fi, serr := os.Stat(takeover); serr == nil && time.Since(fi.ModTime()) > takeoverStale {
				_ = os.Remove(takeover) // its holder died mid-takeover
			}
		}
		return false
	}
	defer func() { _ = os.Remove(takeover) }()
	// Re-check inside: the waiter before us may already have retaken the lock.
	fi, err := os.Stat(dir)
	if err != nil || time.Since(fi.ModTime()) <= stale {
		return false
	}
	return os.Remove(dir) == nil
}

// hold starts the toucher and returns the release function. The lock counts as
// ours only while its mtime is the one we last observed.
func hold(o Options) func() {
	var (
		mu   sync.Mutex
		mine time.Time
	)
	observe := func() {
		if fi, err := os.Stat(o.Dir); err == nil {
			mine = fi.ModTime()
		}
	}
	ours := func() bool {
		fi, err := os.Stat(o.Dir)
		return err == nil && fi.ModTime().Equal(mine)
	}
	mu.Lock()
	observe()
	mu.Unlock()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(o.Touch)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				mu.Lock()
				if !ours() {
					mu.Unlock()
					return // taken over or removed: nothing left to keep alive
				}
				now := time.Now()
				if os.Chtimes(o.Dir, now, now) == nil {
					observe()
				}
				mu.Unlock()
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			<-done
			mu.Lock()
			defer mu.Unlock()
			if ours() {
				_ = os.Remove(o.Dir)
			}
		})
	}
}

// Dirs are the lock directories Claude Code uses.
type Dirs struct {
	// OAuthRefresh is <config-home>/.oauth_refresh.lock, taken first.
	OAuthRefresh string
	// Legacy is <config-home>.lock (~/.claude.lock), taken second.
	Legacy string
	// Config guards the global config file (~/.claude.json.lock).
	Config string
}

// ResolveDirs derives the lock directories the way Claude Code resolves its
// files: CLAUDE_CONFIG_DIR wins, otherwise ~/.claude, with ~/.claude.json
// sitting at home rather than inside the config dir.
func ResolveDirs(home string) Dirs {
	cfgHome := filepath.Join(home, ".claude")
	base := home
	if env := os.Getenv("CLAUDE_CONFIG_DIR"); env != "" {
		cfgHome, base = env, env
	}
	global := pickGlobalConfig(filepath.Join(cfgHome, ".config.json"), filepath.Join(base, ".claude.json"))
	return Dirs{
		OAuthRefresh: filepath.Join(cfgHome, ".oauth_refresh.lock"),
		Legacy:       filepath.Join(filepath.Dir(cfgHome), filepath.Base(cfgHome)+".lock"),
		Config:       global + ".lock",
	}
}

// pickGlobalConfig is Claude Code's choice between the legacy .config.json and
// .claude.json (claude-swap paths.py _pick_global_config).
func pickGlobalConfig(legacy, global string) string {
	fi, err := os.Stat(legacy)
	if err != nil || !fi.Mode().IsRegular() {
		return global
	}
	if fi.Size() > 0 {
		return legacy
	}
	if fi.Mode().Perm()&0o222 != 0 {
		if _, err := os.Stat(global); errors.Is(err, os.ErrNotExist) {
			return legacy
		}
	}
	return global
}

// Set takes all of Claude Code's locks around a critical section.
type Set struct {
	Dirs Dirs
	// Timeout is the per-lock wait; zero means DefaultTimeout.
	Timeout time.Duration
	// Poll is the base wait between attempts; zero means the default.
	Poll time.Duration
}

// Hold runs fn while holding, in Claude Code's order, the OAuth refresh lock,
// the legacy credentials lock and the config lock. If any of them cannot be
// taken within the timeout, fn is not run, whatever was taken is released, and
// the error wraps ErrTimeout. fn must not do network I/O.
func (s Set) Hold(fn func() error) error {
	locks := []Options{
		{Dir: s.Dirs.OAuthRefresh, Stale: CredentialsStale},
		{Dir: s.Dirs.Legacy, Stale: CredentialsStale},
		{Dir: s.Dirs.Config, Stale: ConfigStale},
	}
	var releases []func()
	defer func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}()
	for _, o := range locks {
		o.Timeout, o.Poll = s.Timeout, s.Poll
		rel, err := Acquire(o)
		if err != nil {
			return err
		}
		releases = append(releases, rel)
	}
	return fn()
}
