package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/auto"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
	"github.com/nicoarbelaez/claude-identity/internal/usage"
)

// AutoConfigPort is the auto-switch config store as `cid auto` uses it. A nil
// port disables the command.
type AutoConfigPort interface {
	Load() (auto.Config, error)
	Update(fn func(*auto.Config) error) error
	Forget(profile string) error
}

// AutoStatePort is the auto-switch state store as the CLI uses it.
type AutoStatePort interface {
	Load() (auto.State, error)
	Update(fn func(*auto.State)) error
	Forget(profile string) error
}

const (
	// noticeTTL is how long the status line shows the notice of a switch.
	noticeTTL = 2 * time.Minute
	// pendingMaxAge is how old a pending switch may be and still be applied.
	pendingMaxAge = 10 * time.Minute
	// noCandidateEvery is the minimum gap between two "nobody can take over" notices.
	noCandidateEvery = time.Hour
	// historyShown is how many history entries `cid auto` prints.
	historyShown = 5
)

// jsonAutoLines are the two thresholds in percent.
type jsonAutoLines struct {
	FiveHour int `json:"fiveHour"`
	Week     int `json:"week"`
}

// jsonAutoAccount is one enrolled account with its lines already resolved.
type jsonAutoAccount struct {
	Index    int    `json:"index"`
	Name     string `json:"name"`
	FiveHour int    `json:"fiveHour"`
	Week     int    `json:"week"`
	Role     string `json:"role"`
}

// jsonAuto is the stable machine-readable shape of `cid auto --json`.
type jsonAuto struct {
	Enabled      bool                `json:"enabled"`
	Defaults     jsonAutoLines       `json:"defaults"`
	Accounts     []jsonAutoAccount   `json:"accounts"`
	Pending      *auto.Pending       `json:"pending"`
	LastSwitchAt *time.Time          `json:"lastSwitchAt"`
	History      []auto.HistoryEntry `json:"history"`
}

// autoCmd dispatches `cid auto [on|off|add|remove|principal|reserve|defaults]`.
func (a *app) autoCmd(args []string) int {
	if a.Auto == nil {
		return a.usageError("")
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return a.autoStatus(args)
	}
	rest := args[1:]
	switch args[0] {
	case "on":
		return a.autoToggle(rest, true)
	case "off":
		return a.autoToggle(rest, false)
	case "add":
		return a.autoAdd(rest)
	case "remove":
		return a.autoRemove(rest)
	case "principal":
		return a.autoRole(rest, auto.RolePrincipal)
	case "reserve":
		return a.autoRole(rest, auto.RoleReserve)
	case "defaults":
		return a.autoDefaults(rest)
	}
	return a.usageError("")
}

func (a *app) autoStatus(args []string) int {
	asJSON, _, ok := parse("auto", args, true, 0)
	if !ok {
		return a.usageError("")
	}
	cfg, err := a.Auto.Load()
	if err != nil {
		return a.fail(err, "")
	}
	var st auto.State
	if a.AutoState != nil {
		if st, err = a.AutoState.Load(); err != nil {
			return a.fail(err, "")
		}
	}
	ps, err := a.Profiles.List()
	if err != nil {
		return a.fail(err, "")
	}
	out := jsonAuto{
		Enabled:  cfg.Enabled,
		Defaults: jsonAutoLines{FiveHour: cfg.Defaults.FiveHour, Week: cfg.Defaults.Week},
		Accounts: []jsonAutoAccount{},
		Pending:  st.Pending,
		History:  st.History,
	}
	if out.History == nil {
		out.History = []auto.HistoryEntry{}
	}
	if !st.LastSwitchAt.IsZero() {
		out.LastSwitchAt = &st.LastSwitchAt
	}
	for _, p := range ps {
		if five, week, enrolled := cfg.Lines(p.Name); enrolled {
			out.Accounts = append(out.Accounts, jsonAutoAccount{
				Index: p.Index, Name: p.Name, FiveHour: five, Week: week, Role: string(cfg.Accounts[p.Name].Role),
			})
		}
	}
	if asJSON {
		return a.writeJSON(out)
	}
	a.printAutoStatus(out)
	return exitOK
}

func (a *app) printAutoStatus(s jsonAuto) {
	key := i18n.AutoStatusOff
	if s.Enabled {
		key = i18n.AutoStatusOn
	}
	a.printf(a.Stdout, "%s\n", a.t(key, s.Defaults.FiveHour, s.Defaults.Week))
	if len(s.Accounts) == 0 {
		a.printf(a.Stdout, "%s\n", a.t(i18n.AutoNoAccounts))
	}
	for _, acc := range s.Accounts {
		suffix := ""
		if acc.Role != "" {
			suffix = "  " + a.roleLabel(auto.Role(acc.Role))
		}
		a.printf(a.Stdout, "%s\n", a.t(i18n.AutoAccountLine, acc.Index, acc.Name, acc.FiveHour, acc.Week, suffix))
	}
	if p := s.Pending; p != nil {
		a.printf(a.Stdout, "%s\n", a.t(i18n.AutoPendingLine, p.From, p.To, a.pendingDetail(*p)))
	}
	if s.LastSwitchAt != nil {
		a.printf(a.Stdout, "%s\n", a.t(i18n.AutoLastSwitch, s.LastSwitchAt.Local().Format("2006-01-02 15:04")))
	}
	if len(s.History) == 0 {
		return
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.AutoHistoryHeader))
	recent := s.History[max(0, len(s.History)-historyShown):]
	for i := len(recent) - 1; i >= 0; i-- { // newest first
		e := recent[i]
		when := e.At.Local().Format("2006-01-02 15:04")
		if e.Error != "" {
			a.printf(a.Stdout, "%s\n", a.t(i18n.AutoHistoryFailed, when, e.From, e.To, e.Error))
			continue
		}
		a.printf(a.Stdout, "%s\n", a.t(i18n.AutoHistoryLine, when, e.From, e.To, a.pendingDetail(auto.Pending{Window: e.Window, Pct: e.Pct})))
	}
}

// pendingDetail describes why a switch is pending: the window that fired, or
// the return to the principal.
func (a *app) pendingDetail(p auto.Pending) string {
	switch p.Window {
	case auto.Window5h:
		return fmt.Sprintf("%s %.0f%%", a.t(i18n.UsageLabelFiveHour), p.Pct)
	case auto.WindowWeek:
		return fmt.Sprintf("%s %.0f%%", a.t(i18n.UsageLabelWeek), p.Pct)
	}
	return a.t(i18n.AutoReasonReturn)
}

func (a *app) roleLabel(r auto.Role) string {
	switch r {
	case auto.RolePrincipal:
		return a.t(i18n.AutoRolePrincipal)
	case auto.RoleReserve:
		return a.t(i18n.AutoRoleReserve)
	}
	return ""
}

func (a *app) autoToggle(args []string, on bool) int {
	if len(args) != 0 {
		return a.usageError("")
	}
	if err := a.Auto.Update(func(c *auto.Config) error { c.Enabled = on; return nil }); err != nil {
		return a.fail(err, "")
	}
	key := i18n.AutoOff
	if on {
		key = i18n.AutoOn
	}
	a.printf(a.Stdout, "%s\n", a.t(key))
	return exitOK
}

// autoArgs is the parsed form of `[<n|name>] [--5h N] [--week N]` in any order.
type autoArgs struct {
	pos  []string
	five *int
	week *int
	bad  *string // the raw text of the first invalid percentage
	ok   bool
}

// parseAutoArgs reads positional arguments and the --5h/--week lines. Unlike
// the flag package it accepts flags before and after the profile.
func parseAutoArgs(args []string) autoArgs {
	res := autoArgs{ok: true}
	for i := 0; i < len(args); i++ {
		var dst **int
		switch strings.TrimLeft(args[i], "-") {
		case "5h":
			dst = &res.five
		case "week":
			dst = &res.week
		}
		switch {
		case dst == nil && strings.HasPrefix(args[i], "-"):
			return autoArgs{}
		case dst == nil:
			res.pos = append(res.pos, args[i])
			continue
		case i+1 >= len(args):
			return autoArgs{}
		}
		i++
		n, err := strconv.Atoi(args[i])
		if err != nil || !auto.ValidPercent(n) {
			if res.bad == nil {
				raw := args[i]
				res.bad = &raw
			}
			continue
		}
		*dst = &n
	}
	return res
}

// invalidPercent reports a bad --5h/--week value as a one-line usage error.
func (a *app) invalidPercent(raw string) int {
	a.printf(a.Stderr, "%s\n", a.t(i18n.AutoInvalidPercentage, raw))
	return exitUsage
}

// autoProfile resolves a profile reference. An unknown one is a one-line usage
// error (exit 2), not a runtime failure: nothing went wrong, the input is bad.
func (a *app) autoProfile(ref string) (profile.Profile, int, bool) {
	p, err := a.Profiles.Resolve(ref)
	switch {
	case err == nil:
		return p, exitOK, true
	case errors.Is(err, profile.ErrNotFound):
		a.printf(a.Stderr, "%s\n", a.t(i18n.UnknownProfile, ref))
		return profile.Profile{}, exitUsage, false
	}
	return profile.Profile{}, a.fail(err, ref), false
}

func (a *app) autoAdd(args []string) int {
	pa := parseAutoArgs(args)
	if !pa.ok || len(pa.pos) != 1 {
		return a.usageError("")
	}
	p, code, ok := a.autoProfile(pa.pos[0])
	if !ok {
		return code
	}
	if pa.bad != nil {
		return a.invalidPercent(*pa.bad)
	}
	var five, week int
	err := a.Auto.Update(func(c *auto.Config) error {
		if err := c.Enroll(p.Name, pa.five, pa.week); err != nil {
			return err
		}
		five, week, _ = c.Lines(p.Name)
		return nil
	})
	if err != nil {
		return a.fail(err, p.Name)
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.AutoAdded, p.Name, five, week))
	return exitOK
}

func (a *app) autoRemove(args []string) int {
	pa := parseAutoArgs(args)
	if !pa.ok || len(pa.pos) != 1 || pa.five != nil || pa.week != nil || pa.bad != nil {
		return a.usageError("")
	}
	p, code, ok := a.autoProfile(pa.pos[0])
	if !ok {
		return code
	}
	return a.enrolledOnly(p, func(c *auto.Config) error {
		c.Remove(p.Name)
		return nil
	}, i18n.AutoRemoved)
}

func (a *app) autoRole(args []string, role auto.Role) int {
	pa := parseAutoArgs(args)
	if !pa.ok || len(pa.pos) != 1 || pa.five != nil || pa.week != nil || pa.bad != nil {
		return a.usageError("")
	}
	p, code, ok := a.autoProfile(pa.pos[0])
	if !ok {
		return code
	}
	key := i18n.AutoReserveSet
	if role == auto.RolePrincipal {
		key = i18n.AutoPrincipalSet
	}
	return a.enrolledOnly(p, func(c *auto.Config) error { return c.SetRole(p.Name, role) }, key)
}

// enrolledOnly runs change when p is enrolled and prints done; for a profile
// that is not enrolled it prints the one-line hint and returns exit 2.
func (a *app) enrolledOnly(p profile.Profile, change func(*auto.Config) error, done i18n.Key) int {
	err := a.Auto.Update(func(c *auto.Config) error {
		if _, _, ok := c.Lines(p.Name); !ok {
			return auto.ErrNotEnrolled
		}
		return change(c)
	})
	if errors.Is(err, auto.ErrNotEnrolled) {
		a.printf(a.Stderr, "%s\n", a.t(i18n.AutoNotEnrolled, p.Name, p.Name))
		return exitUsage
	}
	if err != nil {
		return a.fail(err, p.Name)
	}
	a.printf(a.Stdout, "%s\n", a.t(done, p.Name))
	return exitOK
}

// autoDefaults shows the default lines, or changes the ones given.
func (a *app) autoDefaults(args []string) int {
	pa := parseAutoArgs(args)
	if !pa.ok || len(pa.pos) != 0 {
		return a.usageError("")
	}
	if pa.bad != nil {
		return a.invalidPercent(*pa.bad)
	}
	var def auto.Lines
	if pa.five == nil && pa.week == nil {
		cfg, err := a.Auto.Load()
		if err != nil {
			return a.fail(err, "")
		}
		def = cfg.Defaults
	} else {
		err := a.Auto.Update(func(c *auto.Config) error {
			if err := c.SetDefaults(pa.five, pa.week); err != nil {
				return err
			}
			def = c.Defaults
			return nil
		})
		if err != nil {
			return a.fail(err, "")
		}
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.AutoDefaultsSet, def.FiveHour, def.Week))
	return exitOK
}

// forgetAuto drops a removed profile from the auto-switch files, best effort:
// a stale entry is ignored by the policy because the profile no longer exists.
func (a *app) forgetAuto(name string) {
	if a.Auto != nil {
		_ = a.Auto.Forget(name)
	}
	if a.AutoState != nil {
		_ = a.AutoState.Forget(name)
	}
}

// ---- automatic switching: evaluate (status line), apply (prompt hook) ----

// autoInput builds the policy input from local data only: the enrolled
// accounts, their token status from the keyring or live store, and the usage
// cache. It makes no network call and never reads a token value.
func (a *app) autoInput(cfg auto.Config, active string, last time.Time) (auto.Input, error) {
	ps, err := a.Profiles.List()
	if err != nil {
		return auto.Input{}, err
	}
	cache, err := a.Usage.All()
	if err != nil {
		return auto.Input{}, err
	}
	in := auto.Input{Active: active, LastSwitchAt: last}
	for _, p := range ps {
		five, week, enrolled := cfg.Lines(p.Name)
		if !enrolled {
			continue
		}
		acc := auto.Account{
			Name: p.Name, Index: p.Index, Role: cfg.Accounts[p.Name].Role,
			FiveHourLine: five, WeekLine: week,
			Status: a.Swap.Status(p.Name, p.Name == active),
		}
		if e, ok := cache[p.Name]; ok {
			f, w := e.Windows[usage.FiveHour], e.Windows[usage.SevenDay]
			acc.Usage = auto.Usage{
				HasData:  true,
				FiveHour: f.PercentUsed, FiveHourResetsAt: f.ResetsAt,
				Week: w.PercentUsed, WeekResetsAt: w.ResetsAt,
			}
		}
		in.Accounts = append(in.Accounts, acc)
	}
	return in, nil
}

// evaluateAuto decides whether the active account should hand over and records
// the verdict in the auto state: a pending switch for the next prompt, or a
// one-time notice when nobody can take over. It never switches. Best effort: an
// error leaves the state as it was.
func (a *app) evaluateAuto(active string, now time.Time) {
	if a.Auto == nil || a.AutoState == nil || a.Usage == nil || a.Swap == nil {
		return
	}
	cfg, err := a.Auto.Load()
	if err != nil || !cfg.Enabled {
		return
	}
	st, err := a.AutoState.Load()
	if err != nil {
		return
	}
	in, err := a.autoInput(cfg, active, st.LastSwitchAt)
	if err != nil {
		return
	}
	d := auto.Decide(in, now)
	switch {
	case d.Switch:
		p := auto.Pending{From: active, To: d.To, Reason: d.Reason, Window: d.Window, Pct: d.Pct, At: now}
		if cur := st.Pending; cur != nil && cur.From == p.From && cur.To == p.To && cur.Reason == p.Reason {
			return // already pending; keep its time so it can expire
		}
		_ = a.AutoState.Update(func(s *auto.State) { s.Pending = &p })
	case d.NoCandidate && now.Sub(st.LastNoCandidateAt) >= noCandidateEvery:
		_ = a.AutoState.Update(func(s *auto.State) {
			s.Pending = nil
			s.LastNoCandidateAt = now
			s.Notice = &auto.Notice{Text: a.t(i18n.AutoNoticeNoCandidate, active), Until: now.Add(noticeTTL)}
		})
	case st.Pending != nil: // the decision no longer holds
		_ = a.AutoState.Update(func(s *auto.State) { s.Pending = nil })
	}
}

// applyAutoPending applies the pending switch, if any, before the next prompt
// is sent. It never blocks the prompt: whatever happens, it returns normally.
// With nothing pending it only reads the small state file.
func (a *app) applyAutoPending() {
	if a.AutoState == nil {
		return
	}
	st, err := a.AutoState.Load()
	if err != nil || st.Pending == nil {
		return
	}
	p, now := *st.Pending, a.now()
	if !a.pendingStillValid(p, now) {
		_ = a.AutoState.Update(func(s *auto.State) { s.Pending = nil })
		return
	}
	res, err := a.Swap.Switch(p.To)
	entry := auto.HistoryEntry{At: now, From: p.From, To: p.To, Reason: p.Reason, Window: p.Window, Pct: p.Pct}
	switch {
	case err != nil:
		entry.Error = err.Error()
		_ = a.AutoState.Update(func(s *auto.State) {
			s.Pending = nil
			s.LastSwitchAt = now // an attempt counts for the cooldown, so a failure cannot loop
			s.Notice = &auto.Notice{Text: a.t(i18n.AutoNoticeFailed, p.To, p.From), Until: now.Add(noticeTTL)}
			s.AddHistory(entry)
		})
	case res.NoOp:
		_ = a.AutoState.Update(func(s *auto.State) { s.Pending = nil })
	default:
		_ = a.AutoState.Update(func(s *auto.State) {
			s.Pending = nil
			s.LastSwitchAt = now
			s.Notice = &auto.Notice{Text: a.t(i18n.AutoNoticeSwitch, p.From, p.To, a.pendingDetail(p)), Until: now.Add(noticeTTL)}
			s.AddHistory(entry)
		})
	}
}

// pendingStillValid re-checks a pending switch against the world as it is now:
// auto-switch on, the target enrolled, the decision recent, and the live
// account still the one the decision was made for.
func (a *app) pendingStillValid(p auto.Pending, now time.Time) bool {
	if a.Auto == nil || a.Swap == nil || now.Sub(p.At) > pendingMaxAge {
		return false
	}
	cfg, err := a.Auto.Load()
	if err != nil || !cfg.Enabled {
		return false
	}
	if _, _, ok := cfg.Lines(p.To); !ok {
		return false
	}
	cur, err := a.Profiles.Active()
	return err == nil && cur.Name == p.From
}

// printNotice writes the status line notice, if one is still current, on its
// own line. An expired notice is not printed and is cleared.
func (a *app) printNotice(w io.Writer, needNewline bool, now time.Time) {
	if a.AutoState == nil {
		return
	}
	st, err := a.AutoState.Load()
	if err != nil || st.Notice == nil {
		return
	}
	if !now.Before(st.Notice.Until) {
		_ = a.AutoState.Update(func(s *auto.State) {
			if s.Notice != nil && !now.Before(s.Notice.Until) {
				s.Notice = nil
			}
		})
		return
	}
	if needNewline {
		a.printf(w, "\n")
	}
	a.printf(w, "%s\n", st.Notice.Text)
}
