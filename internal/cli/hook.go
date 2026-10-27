package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/login"
)

// maxHookInput caps how much of the hook payload is read from stdin.
const maxHookInput = 64 << 10

// maxHookContext keeps additionalContext under the documented 10,000-character cap.
const maxHookContext = 9000

const (
	hookFailed = "cid already ran once for this command (from the account-switch hook) and failed. " +
		"Explain the cause in one or two sentences, do not list steps, and offer to retry. " +
		"Run cid again only if the user explicitly asks to retry. Output:\n"
	hookRejected = "The account-switch hook rejected the arguments, so cid was not run. " +
		"Explain in one sentence, do not list steps, and do not run cid unless the user explicitly asks."
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

	var out, errOut bytes.Buffer
	d := a.Deps
	d.Stdout, d.Stderr = &out, &errOut
	d.Quiet = true
	d.Color = false // the captured output is shown as plain text inside Claude Code
	d.Login = hookLogin(d.Login)
	if Run(d, args) != exitOK {
		return a.context(hookFailed + out.String() + errOut.String())
	}
	return a.block(out.String())
}

// hookLogin detaches the login child from the hook's streams: stdin is empty
// (`claude auth login` finishes through the browser) and its output is
// discarded, because the hook's stdout carries the hook JSON. A runner whose
// streams cannot be redirected is dropped, so the login fails safely instead of
// writing into the hook output.
func hookLogin(r login.Runner) login.Runner {
	s, ok := r.(login.Streamer)
	if !ok {
		return nil
	}
	return s.WithStreams(strings.NewReader(""), io.Discard, io.Discard)
}

func validHookArgs(args []string) bool {
	if len(args) > 0 { // no recursion; uninstall, update and statusline are terminal-only
		switch strings.ToLower(args[0]) {
		case "hook", "uninstall", "update", "statusline":
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
