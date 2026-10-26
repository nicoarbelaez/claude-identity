package cli

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/login"
)

var numberedStep = regexp.MustCompile(`(?m)^\s*\d+[.)]\s`)

func TestHookAddRunsLoginAndBlocksWithResult(t *testing.T) {
	for _, args := range []string{"add c", "add --console c --force"} {
		t.Run(args, func(t *testing.T) {
			r := newRig(t)
			r.twoProfiles()
			f := loginAs(t, "c@example.com", "tok-c")
			r.useLogin(f)
			b := r.block(expansion("account", args))
			if !strings.Contains(b.Reason, r.msg(i18n.ProfileAdded, "c")) {
				t.Errorf("reason = %q, want the added message", b.Reason)
			}
			if len(f.Calls) != 1 {
				t.Fatalf("login calls = %d, want 1", len(f.Calls))
			}
			if _, err := r.prof.Resolve("c"); err != nil {
				t.Errorf("profile c not saved: %v", err)
			}
			dirGone(t, f)
		})
	}
}

func TestHookReloginRunsLogin(t *testing.T) {
	for _, ref := range []string{"a", "1"} {
		t.Run(ref, func(t *testing.T) {
			r := newRig(t)
			r.twoProfiles()
			f := loginAs(t, "a@example.com", "tok-a2")
			r.useLogin(f)
			b := r.block(expansion("account", "relogin "+ref))
			if !strings.Contains(b.Reason, r.msg(i18n.ReloginDone, "a")) {
				t.Errorf("reason = %q, want the relogin message", b.Reason)
			}
			if len(f.Calls) != 1 || f.Calls[0].Opts.Email != "a@example.com" {
				t.Errorf("calls = %+v, want one call with the profile email", f.Calls)
			}
		})
	}
}

func TestHookLoginFailurePassesContextWithoutBlocking(t *testing.T) {
	failures := map[string]error{
		"failed":         errors.New("exit status 1"),
		"timeout":        login.ErrTimeout,
		"claude missing": login.ErrNotInstalled,
	}
	for name, loginErr := range failures {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.twoProfiles()
			f := &login.FakeRunner{Fn: func(string, login.Options) error { return loginErr }}
			r.useLogin(f)
			c := r.context(expansion("account", "add c"))
			ctx := c.Specific.Context
			for _, want := range []string{"already ran", "do not list steps", "retry"} {
				if !strings.Contains(strings.ToLower(ctx), want) {
					t.Errorf("context = %q, missing %q", ctx, want)
				}
			}
			if !strings.Contains(ctx, r.msg(i18n.AddFallback, "c")) {
				t.Errorf("context = %q, missing the one-line retry", ctx)
			}
			if numberedStep.MatchString(ctx) {
				t.Errorf("context has a numbered step list: %q", ctx)
			}
			if len(f.Calls) != 1 {
				t.Errorf("login calls = %d, want 1", len(f.Calls))
			}
		})
	}
}

func TestHookReloginEmailMismatchPassesContext(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.useLogin(loginAs(t, "other@example.com", "tok-x"))
	c := r.context(expansion("account", "relogin a"))
	if !strings.Contains(c.Specific.Context, "other@example.com") || numberedStep.MatchString(c.Specific.Context) {
		t.Errorf("context = %q", c.Specific.Context)
	}
}

func TestHookDarwinPassesOneLineContext(t *testing.T) {
	r := newRig(t)
	r.deps.GOOS = "darwin"
	f := loginAs(t, "c@example.com", "tok-c")
	r.useLogin(f)
	c := r.context(expansion("account", "add c"))
	if !strings.Contains(c.Specific.Context, r.msg(i18n.LoginUnsupportedOS, "c")) || len(f.Calls) != 0 {
		t.Errorf("context = %q, calls = %d", c.Specific.Context, len(f.Calls))
	}
}

func TestHookLoginChildOutputNeverReachesHookStdout(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	f := loginAs(t, "c@example.com", "tok-c")
	f.Noise = "Opening browser... LOGIN-NOISE\n"
	r.useLogin(f)
	code, out := r.hook(expansion("account", "add c"))
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, r.stderr)
	}
	var b blockOutput
	if err := json.Unmarshal([]byte(out), &b); err != nil {
		t.Fatalf("stdout %q is not a single JSON document: %v", out, err)
	}
	if strings.Contains(out, "LOGIN-NOISE") || b.Decision != "block" {
		t.Errorf("stdout = %q", out)
	}
	if !f.Streamed || f.InputRead != "" {
		t.Errorf("streams not redirected: streamed=%v input=%q", f.Streamed, f.InputRead)
	}
}

func TestHookReloginUnknownProfileReportsError(t *testing.T) {
	r := newRig(t)
	f := loginAs(t, "c@example.com", "tok-c")
	r.useLogin(f)
	c := r.context(expansion("account", "relogin nope"))
	if !strings.Contains(c.Specific.Context, r.msg(i18n.UnknownProfile, "nope")) {
		t.Errorf("context = %q", c.Specific.Context)
	}
	if len(f.Calls) != 0 {
		t.Error("the hook started a login")
	}
}

func TestHookAddCurrentStillWorks(t *testing.T) {
	r := newRig(t)
	r.login("a@example.com", "tok-a")
	f := loginAs(t, "z@example.com", "tok-z")
	r.useLogin(f)
	b := r.block(expansion("account-switch:account", "add a --current"))
	if !strings.Contains(b.Reason, r.msg(i18n.ProfileAdded, "a")) {
		t.Errorf("reason = %q", b.Reason)
	}
	if act, err := r.prof.Active(); err != nil || act.Name != "a" {
		t.Errorf("active = %+v, %v", act, err)
	}
	if len(f.Calls) != 0 {
		t.Error("add --current started a login")
	}
}
