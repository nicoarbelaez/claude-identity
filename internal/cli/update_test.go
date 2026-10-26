package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/update"
	"github.com/nicoarbelaez/claude-identity/internal/version"
)

type fakeUpdater struct {
	latest      string
	latestErr   error
	updateErr   error
	latestCalls int
	updatedTo   []string
	deadline    time.Duration // time left on the Latest context when called
}

func (f *fakeUpdater) Latest(ctx context.Context) (string, error) {
	f.latestCalls++
	if d, ok := ctx.Deadline(); ok {
		f.deadline = time.Until(d)
	}
	return f.latest, f.latestErr
}

func (f *fakeUpdater) UpdateTo(_ context.Context, v string) error {
	f.updatedTo = append(f.updatedTo, v)
	return f.updateErr
}

type fakePlugin struct {
	err   error
	calls int
}

func (f *fakePlugin) Refresh(context.Context) error {
	f.calls++
	return f.err
}

var (
	_ update.Updater = (*fakeUpdater)(nil)
	_ update.Plugin  = (*fakePlugin)(nil)
)

// withVersion sets the build version for one test.
func withVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

func updateRig(t *testing.T, cur string, u *fakeUpdater, p *fakePlugin) *rig {
	t.Helper()
	withVersion(t, cur)
	r := newRig(t)
	r.deps.Update = UpdateDeps{Updater: u, Plugin: p}
	return r
}

func TestVersionAnnouncesNewerRelease(t *testing.T) {
	u := &fakeUpdater{latest: "0.6.0"}
	r := updateRig(t, "0.5.0", u, &fakePlugin{})
	if code := r.run("version"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := r.msg(i18n.VersionLine, "0.5.0") + "\n" + r.msg(i18n.UpdateAvailable, "0.6.0") + "\n"
	if r.stdout.String() != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if u.deadline <= 0 || u.deadline > 3*time.Second {
		t.Errorf("check deadline = %s, want within 3s", u.deadline)
	}
}

func TestVersionSpanishAnnouncement(t *testing.T) {
	r := updateRig(t, "0.5.0", &fakeUpdater{latest: "0.6.0"}, &fakePlugin{})
	r.deps.Lang = i18n.ES
	r.run("version")
	if !strings.Contains(r.stdout.String(), "Está disponible cid 0.6.0. Ejecuta: cid update") {
		t.Errorf("stdout = %q", r.stdout)
	}
}

func TestVersionPrintsOnlyVersionWhenNothingToAnnounce(t *testing.T) {
	for name, tc := range map[string]struct {
		cur string
		u   *fakeUpdater
	}{
		"up to date":   {"0.5.0", &fakeUpdater{latest: "0.5.0"}},
		"ahead":        {"0.7.0", &fakeUpdater{latest: "0.6.0"}},
		"offline":      {"0.5.0", &fakeUpdater{latestErr: errors.New("dial tcp: no route")}},
		"unparsable":   {"0.5.0", &fakeUpdater{latest: "garbage"}},
		"dev build":    {"dev", &fakeUpdater{latest: "0.6.0"}},
		"unknown form": {"nightly", &fakeUpdater{latest: "0.6.0"}},
	} {
		t.Run(name, func(t *testing.T) {
			r := updateRig(t, tc.cur, tc.u, &fakePlugin{})
			if code := r.run("version"); code != 0 {
				t.Fatalf("exit %d", code)
			}
			if want := r.msg(i18n.VersionLine, tc.cur) + "\n"; r.stdout.String() != want {
				t.Errorf("stdout = %q, want %q", r.stdout, want)
			}
			if r.stderr.Len() != 0 {
				t.Errorf("stderr = %q", r.stderr)
			}
		})
	}
}

func TestVersionSkipsNetworkForDevBuild(t *testing.T) {
	u := &fakeUpdater{latest: "0.6.0"}
	r := updateRig(t, "dev", u, &fakePlugin{})
	r.run("version")
	if u.latestCalls != 0 {
		t.Errorf("Latest called %d times for a dev build", u.latestCalls)
	}
}

func TestUpdateReplacesBinaryThenRefreshesPlugin(t *testing.T) {
	u, p := &fakeUpdater{latest: "0.6.0"}, &fakePlugin{}
	r := updateRig(t, "0.5.0", u, p)
	if code := r.run("update"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, r.stderr)
	}
	want := r.msg(i18n.UpdateDone, "0.5.0", "0.6.0") + "\n" + r.msg(i18n.UpdatePluginDone) + "\n"
	if r.stdout.String() != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if len(u.updatedTo) != 1 || u.updatedTo[0] != "0.6.0" {
		t.Errorf("updatedTo = %v", u.updatedTo)
	}
	if p.calls != 1 {
		t.Errorf("plugin refreshed %d times", p.calls)
	}
}

func TestUpdateWhenAlreadyLatestStillRefreshesPlugin(t *testing.T) {
	u, p := &fakeUpdater{latest: "0.5.0"}, &fakePlugin{}
	r := updateRig(t, "0.5.0", u, p)
	if code := r.run("update"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := r.msg(i18n.UpdateUpToDate, "0.5.0") + "\n" + r.msg(i18n.UpdatePluginDone) + "\n"
	if r.stdout.String() != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if len(u.updatedTo) != 0 {
		t.Errorf("binary replaced although up to date: %v", u.updatedTo)
	}
	if p.calls != 1 {
		t.Errorf("plugin refreshed %d times", p.calls)
	}
}

func TestUpdateBinaryFailureExitsOneAndSkipsPlugin(t *testing.T) {
	for name, u := range map[string]*fakeUpdater{
		"download":  {latest: "0.6.0", updateErr: errors.New("checksum mismatch")},
		"detection": {latestErr: errors.New("rate limited")},
	} {
		t.Run(name, func(t *testing.T) {
			p := &fakePlugin{}
			r := updateRig(t, "0.5.0", u, p)
			if code := r.run("update"); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			reason := "checksum mismatch"
			if u.latestErr != nil {
				reason = "rate limited"
			}
			if want := r.msg(i18n.UpdateFailed, reason) + "\n"; r.stderr.String() != want {
				t.Errorf("stderr = %q, want %q", r.stderr, want)
			}
			if p.calls != 0 {
				t.Errorf("plugin refreshed after a binary failure")
			}
			if r.stdout.Len() != 0 {
				t.Errorf("stdout = %q", r.stdout)
			}
		})
	}
}

func TestUpdatePluginFailureWarnsButKeepsExitZero(t *testing.T) {
	p := &fakePlugin{err: update.ErrClaudeMissing}
	r := updateRig(t, "0.5.0", &fakeUpdater{latest: "0.6.0"}, p)
	if code := r.run("update"); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if want := r.msg(i18n.UpdateDone, "0.5.0", "0.6.0") + "\n"; r.stdout.String() != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if want := r.msg(i18n.UpdatePluginWarn, update.ErrClaudeMissing.Error()) + "\n"; r.stderr.String() != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
}

func TestUpdateRefusesDevBuild(t *testing.T) {
	u, p := &fakeUpdater{latest: "0.6.0"}, &fakePlugin{}
	r := updateRig(t, "dev", u, p)
	if code := r.run("update"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if want := r.msg(i18n.UpdateDevBuild) + "\n"; r.stderr.String() != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
	if u.latestCalls != 0 || p.calls != 0 {
		t.Errorf("dev build touched network (%d) or plugin (%d)", u.latestCalls, p.calls)
	}
}

func TestUpdateRejectsArguments(t *testing.T) {
	for _, args := range [][]string{{"update", "now"}, {"update", "--bogus"}} {
		u := &fakeUpdater{latest: "0.6.0"}
		r := updateRig(t, "0.5.0", u, &fakePlugin{})
		if code := r.run(args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
		if u.latestCalls != 0 {
			t.Errorf("%v: updater was called", args)
		}
	}
}
