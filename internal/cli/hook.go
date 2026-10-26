package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"
)

// maxHookInput caps how much of the hook payload is read from stdin.
const maxHookInput = 64 << 10

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

	// Invalid tokens and failing runs pass through, so the skill runs and the
	// model can explain the error to the user.
	args := strings.Fields(in.CommandArgs)
	if !validHookArgs(args) {
		return exitOK
	}

	var out, errOut bytes.Buffer
	d := a.Deps
	d.Stdout, d.Stderr = &out, &errOut
	if Run(d, args) != exitOK {
		return exitOK
	}
	return a.block(out.String())
}

func validHookArgs(args []string) bool {
	if len(args) > 0 && args[0] == "hook" { // no recursion
		return false
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
	b, err := json.Marshal(hookBlock{Decision: "block", Reason: strings.TrimRight(text, "\n")})
	if err != nil {
		return a.fail(err, "")
	}
	a.printf(a.Stdout, "%s\n", b)
	return exitOK
}
