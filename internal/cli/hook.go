package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
)

// maxHookInput caps how much of the hook payload is read from stdin.
const maxHookInput = 64 << 10

// maxHookContext keeps additionalContext under the documented 10,000-character cap.
const maxHookContext = 9000

const (
	hookFailed   = "cid was already run for this command by the account-switch hook and failed. Do not run cid again. Output:\n"
	hookRejected = "The account-switch hook rejected the arguments, so cid was not run. Do not run cid."
)

// hookToken is what `/account` arguments may contain: a profile name or index.
var hookToken = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)

type hookInput struct {
	Event       string `json:"hook_event_name"`
	CommandName string `json:"command_name"`
	CommandArgs string `json:"command_args"`
}

type hookBlock struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// hook implements the internal `cid hook` command. The plugin registers it as a
// UserPromptExpansion hook so `/account ...` is answered with the cid output
// and no model turn. Anything that is not an /account expansion, or is not
// valid JSON, passes through silently (exit 0, no output).
func (a *app) hook() int {
	if a.Stdin == nil {
		return exitOK
	}
	raw, err := io.ReadAll(io.LimitReader(a.Stdin, maxHookInput))
	if err != nil {
		return exitOK
	}
	var in hookInput
	if json.Unmarshal(raw, &in) != nil || in.Event != "UserPromptExpansion" {
		return exitOK
	}
	if in.CommandName != "account" && !strings.HasSuffix(in.CommandName, ":account") {
		return exitOK
	}

	// cid has already run (or was refused) here, so failures are handed to the
	// model as context; the skill body never runs cid again.
	args := strings.Fields(in.CommandArgs)
	if !validHookArgs(args) {
		return a.context(hookRejected)
	}

	if msg, ok := a.hookNeedsTerminal(args); ok {
		return a.block(msg)
	}

	var out, errOut bytes.Buffer
	d := a.Deps
	d.Stdout, d.Stderr = &out, &errOut
	if Run(d, args) != exitOK {
		return a.context(hookFailed + out.String() + errOut.String())
	}
	return a.block(out.String())
}

// hookNeedsTerminal reports whether args would start a browser login (`add`
// without --current, or `relogin`). The hook has no terminal and a short
// timeout, so it explains how to run the command instead. Malformed commands
// and unknown profiles are left to Run, which reports them.
func (a *app) hookNeedsTerminal(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	switch args[0] {
	case "add":
		flags, rest := extractFlags(args[1:], "force", "current", "console", "sso")
		if flags["current"] || len(rest) != 1 {
			return "", false
		}
		return a.t(i18n.HookAddNeedsTerminal, rest[0], rest[0]), true
	case "relogin":
		_, rest := extractFlags(args[1:], "console", "sso")
		if len(rest) != 1 {
			return "", false
		}
		p, err := a.Profiles.Resolve(rest[0])
		if err != nil {
			return "", false
		}
		return a.t(i18n.HookReloginTerminal, p.Name, p.Name), true
	}
	return "", false
}

func validHookArgs(args []string) bool {
	if len(args) > 0 { // no recursion, and uninstall is terminal-only
		switch strings.ToLower(args[0]) {
		case "hook", "uninstall":
			return false
		}
	}
	for _, tok := range args {
		if !hookToken.MatchString(tok) && tok != "--json" {
			return false
		}
	}
	return true
}

// block prints the decision that shows text to the user without a model turn.
func (a *app) block(text string) int {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		text = a.t(i18n.HookDone)
	}
	b, err := json.Marshal(hookBlock{Decision: "block", Reason: text})
	if err != nil {
		return a.fail(err, "")
	}
	a.printf(a.Stdout, "%s\n", b)
	return exitOK
}

// context adds text to the model context without blocking the prompt.
func (a *app) context(text string) int {
	b, err := json.Marshal(map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName":     "UserPromptExpansion",
		"additionalContext": hookContext(text),
	}})
	if err != nil {
		return a.fail(err, "")
	}
	a.printf(a.Stdout, "%s\n", b)
	return exitOK
}

// hookContext truncates s to maxHookContext bytes without splitting a rune.
func hookContext(s string) string {
	if len(s) <= maxHookContext {
		return s
	}
	return strings.ToValidUTF8(s[:maxHookContext], "")
}
