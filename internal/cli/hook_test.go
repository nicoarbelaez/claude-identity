package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
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

// Invalid tokens and failing cid runs pass through so the skill (and the
// model) can explain the error.
func TestHookPassesThroughInvalidTokensAndFailures(t *testing.T) {
	for _, args := range []string{"x;rm", "list $(id)", "a|b", "hook", "hook list", strings.Repeat("a", 33), "bogus", "remove", "version extra"} {
		r := newRig(t)
		code, out := r.hook(expansion("account", args))
		if code != 0 || out != "" {
			t.Errorf("%q: exit %d, stdout %q; want silent exit 0", args, code, out)
		}
	}
}

func TestHookAcceptsJSONFlag(t *testing.T) {
	r := newRig(t)
	if got := r.block(expansion("account", "list --json")); got.Reason != "[]" {
		t.Errorf("reason = %q", got.Reason)
	}
}
