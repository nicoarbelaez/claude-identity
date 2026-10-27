package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
	"github.com/nicoarbelaez/claude-identity/internal/swap"
)

type blockOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// hook feeds stdin to `cid hook` and returns the exit code and raw stdout.
func (r *rig) hook(stdin string) (int, string) {
	r.t.Helper()
	r.deps.Stdin = strings.NewReader(stdin)
	code := r.run("hook")
	return code, r.stdout.String()
}

func expansion(name, args string) string {
	b, _ := json.Marshal(map[string]string{
		"hook_event_name": "UserPromptExpansion",
		"command_name":    name,
		"command_args":    args,
	})
	return string(b)
}

func (r *rig) block(stdin string) blockOutput {
	r.t.Helper()
	code, out := r.hook(stdin)
	if code != 0 {
		r.t.Fatalf("exit %d, stderr %q", code, r.stderr)
	}
	var b blockOutput
	if err := json.Unmarshal([]byte(out), &b); err != nil {
		r.t.Fatalf("stdout %q is not JSON: %v", out, err)
	}
	if b.Decision != "block" {
		r.t.Fatalf("decision = %q, want block", b.Decision)
	}
	return b
}

func TestHookPassesThroughOtherEventsAndCommands(t *testing.T) {
	for _, in := range []string{
		`{"hook_event_name":"UserPromptSubmit","command_name":"account","command_args":"list"}`,
		expansion("review", "list"),
		expansion("account-switch:other", ""),
		`not json`,
		``,
	} {
		r := newRig(t)
		code, out := r.hook(in)
		if code != 0 || out != "" || r.stderr.Len() != 0 {
			t.Errorf("%q: exit %d, stdout %q, stderr %q; want silent exit 0", in, code, out, r.stderr)
		}
	}
}

func TestHookBlocksWithCidOutput(t *testing.T) {
	for _, name := range []string{"account", "account-switch:account"} {
		r := newRig(t)
		r.run("list")
		want := strings.TrimRight(r.stdout.String(), "\n")
		if got := r.block(expansion(name, "list")); got.Reason != want {
			t.Errorf("%s: reason = %q, want %q", name, got.Reason, want)
		}
	}
}

func TestHookEmptyArgsListsProfiles(t *testing.T) {
	r := newRig(t)
	if got := r.block(expansion("account", "")); got.Reason != r.msg(i18n.ListEmpty) {
		t.Errorf("reason = %q", got.Reason)
	}
}

func TestHookRunsSwitchInProcess(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	got := r.block(expansion("account", "a"))
	if want := r.msg(i18n.SwitchedFromTo, "b", "a"); !strings.Contains(got.Reason, want) {
		t.Errorf("reason = %q, want it to contain %q", got.Reason, want)
	}
}

type contextOutput struct {
	Decision string `json:"decision"`
	Specific struct {
		Event   string `json:"hookEventName"`
		Context string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// countingSwap records how many times a mutating operation reached the swapper.
type countingSwap struct {
	Swapper
	calls int
}

func (c *countingSwap) Switch(arg string) (swap.Result, error) {
	c.calls++
	return c.Swapper.Switch(arg)
}

func (c *countingSwap) Remove(arg string) (profile.Profile, error) {
	c.calls++
	return c.Swapper.Remove(arg)
}

func (r *rig) context(stdin string) contextOutput {
	r.t.Helper()
	code, out := r.hook(stdin)
	if code != 0 {
		r.t.Fatalf("exit %d, stderr %q", code, r.stderr)
	}
	var c contextOutput
	if err := json.Unmarshal([]byte(out), &c); err != nil {
		r.t.Fatalf("stdout %q is not JSON: %v", out, err)
	}
	if c.Decision != "" || c.Specific.Event != "UserPromptExpansion" {
		r.t.Fatalf("decision %q, event %q; want additionalContext only", c.Decision, c.Specific.Event)
	}
	return c
}

func TestHookFailureAddsContextAndRunsOnce(t *testing.T) {
	for _, args := range []string{"remove b", "switch nope", "bogus"} {
		r := newRig(t)
		r.twoProfiles()
		cs := &countingSwap{Swapper: r.deps.Swap}
		r.deps.Swap = cs
		c := r.context(expansion("account", args))
		if !strings.Contains(c.Specific.Context, "already ran once") ||
			!strings.Contains(c.Specific.Context, "only if the user explicitly asks") {
			t.Errorf("%q: context = %q", args, c.Specific.Context)
		}
		if cs.calls > 1 {
			t.Errorf("%q: swapper called %d times, want at most 1", args, cs.calls)
		}
		active, err := r.prof.Active()
		if err != nil || active.Name != "b" {
			t.Errorf("%q: active = %v, %v; want b", args, active, err)
		}
		if ps, _ := r.prof.List(); len(ps) != 2 {
			t.Errorf("%q: %d profiles, want 2", args, len(ps))
		}
	}
}

func TestHookFailureContextCarriesCidError(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	c := r.context(expansion("account", "remove b"))
	if !strings.Contains(c.Specific.Context, r.msg(i18n.RemoveActive, "b")) {
		t.Errorf("context = %q", c.Specific.Context)
	}
}

func TestHookInvalidTokensAddContextWithoutRunningCid(t *testing.T) {
	for _, args := range []string{"x;rm", "list $(id)", "a|b", "hook", "hook list", "update", "Update now", "statusline", "Statusline install", strings.Repeat("a", 33)} {
		r := newRig(t)
		r.twoProfiles()
		cs := &countingSwap{Swapper: r.deps.Swap}
		r.deps.Swap = cs
		c := r.context(expansion("account", args))
		if !strings.Contains(c.Specific.Context, "not run") {
			t.Errorf("%q: context = %q", args, c.Specific.Context)
		}
		if cs.calls != 0 {
			t.Errorf("%q: swapper called %d times, want 0", args, cs.calls)
		}
	}
}

func TestHookEmptySuccessOutputUsesFallbackReason(t *testing.T) {
	r := newRig(t)
	a := &app{Deps: r.deps}
	if code := a.block("\n"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var b blockOutput
	if err := json.Unmarshal(r.stdout.Bytes(), &b); err != nil || b.Reason != r.msg(i18n.HookDone) {
		t.Errorf("stdout %q, err %v", r.stdout, err)
	}
}

func TestHookContextIsCapped(t *testing.T) {
	got := hookContext(strings.Repeat("é", 20000))
	if len(got) > maxHookContext || !utf8.ValidString(got) {
		t.Errorf("len %d, valid utf8 %v", len(got), utf8.ValidString(got))
	}
}

func TestHookAcceptsJSONFlag(t *testing.T) {
	r := newRig(t)
	if got := r.block(expansion("account", "list --json")); got.Reason != "[]" {
		t.Errorf("reason = %q", got.Reason)
	}
}
