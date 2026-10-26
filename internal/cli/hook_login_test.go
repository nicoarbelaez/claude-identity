package cli

import (
	"strings"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
)

func TestHookNeverStartsLoginForAddAndRelogin(t *testing.T) {
	tests := []struct {
		name string
		args string
		want func(r *rig) string
	}{
		{"add", "add c", func(r *rig) string { return r.msg(i18n.HookAddNeedsTerminal, "c", "c") }},
		{"add with flags", "add --console c --force", func(r *rig) string { return r.msg(i18n.HookAddNeedsTerminal, "c", "c") }},
		{"relogin by name", "relogin a", func(r *rig) string { return r.msg(i18n.HookReloginTerminal, "a", "a") }},
		{"relogin by index", "relogin 1", func(r *rig) string { return r.msg(i18n.HookReloginTerminal, "a", "a") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			r.twoProfiles()
			f := loginAs(t, "c@example.com", "tok-c")
			r.useLogin(f)
			b := r.block(expansion("account", tt.args))
			if !strings.Contains(b.Reason, tt.want(r)) {
				t.Errorf("reason = %q, want %q", b.Reason, tt.want(r))
			}
			if len(f.Calls) != 0 {
				t.Error("the hook started a login")
			}
			if _, err := r.prof.Resolve("c"); err == nil {
				t.Error("the hook saved a profile")
			}
		})
	}
}

func TestHookReloginUnknownProfileReportsError(t *testing.T) {
	r := newRig(t)
	f := loginAs(t, "c@example.com", "tok-c")
	r.useLogin(f)
	code, out := r.hook(expansion("account", "relogin nope"))
	if code != 0 || strings.Contains(out, r.msg(i18n.HookReloginTerminal, "nope", "nope")) {
		t.Errorf("exit %d, out %q", code, out)
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
