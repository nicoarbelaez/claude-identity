// Package auto decides when to switch the live Claude Code account by itself.
// The policy is a pure function over the cached usage; the config and state
// stores keep what the user enrolled and what the status line decided. It makes
// no network call and never handles tokens.
package auto

import (
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/credentials"
)

// Role marks an enrolled account as the principal, a reserve, or neither.
type Role string

// Roles.
const (
	RoleNone      Role = ""
	RolePrincipal Role = "principal"
	RoleReserve   Role = "reserve"
)

// Windows a Decision can name.
const (
	Window5h   = "5h"
	WindowWeek = "week"
)

// Reasons of a switch Decision.
const (
	ReasonThreshold       = "threshold"
	ReasonReturnPrincipal = "return-principal"
	ReasonEqualize        = "equalize"
	ReasonReserve         = "reserve"
)

// Anti-flap tuning.
const (
	// hysteresis is how many points of headroom a target must have over the
	// active account for a non-forced switch.
	hysteresis = 5
	// Cooldown is the minimum time between two non-forced switches.
	Cooldown = 5 * time.Minute
)

// Usage is the cached reading of one account. A zero reset time means unknown.
type Usage struct {
	HasData          bool
	FiveHour         float64
	FiveHourResetsAt time.Time
	Week             float64
	WeekResetsAt     time.Time
}

// Account is one enrolled account with its lines already resolved from the
// defaults.
type Account struct {
	Name         string
	Index        int
	Status       credentials.Status
	Role         Role
	FiveHourLine int
	WeekLine     int
	Usage        Usage
}

// Input is everything Decide looks at. Only enrolled accounts are listed.
type Input struct {
	Accounts     []Account
	Active       string
	LastSwitchAt time.Time // zero when cid never switched automatically
}

// Decision is the outcome of one evaluation.
type Decision struct {
	Switch bool
	To     string
	Reason string
	// Window and Pct describe the active account's line that fired (threshold,
	// reserve and equalize). They are empty for return-principal.
	Window string
	Pct    float64
	// NoCandidate means the active account is over a line and nobody can take
	// over. The caller notifies once.
	NoCandidate bool
}

// Decide applies the auto-switch rule to in at time now. It does not look at
// whether auto-switch is enabled; the caller checks that first.
//
// The active account must be enrolled: auto only manages enrolled accounts, so
// when the live account is not one of them Decide never switches away from it.
func Decide(in Input, now time.Time) Decision {
	views := make([]view, len(in.Accounts))
	for i, a := range in.Accounts {
		views[i] = project(a, now)
	}
	active := find(views, in.Active)
	if active == nil {
		return Decision{}
	}
	others := without(views, in.Active)

	d, forced := evaluate(*active, others)
	if d.Switch && !forced && coolingDown(in.LastSwitchAt, now) {
		return Decision{}
	}
	return d
}

// evaluate returns the decision ignoring the cooldown, and whether the active
// account is at 100% of a window (which bypasses cooldown and hysteresis).
func evaluate(active view, others []view) (d Decision, forced bool) {
	forced = active.p5 >= 100 || active.pw >= 100
	window, pct, fired := trigger(active)
	if !fired {
		return returnToPrincipal(active, others), false
	}

	// Rule 3: the best regular candidate under both lines.
	if best, ok := pickBest(filter(others, func(v view) bool { return v.eligible() && v.Role != RoleReserve })); ok {
		return takeOver(active, best, ReasonThreshold, window, pct, forced), forced
	}
	// Rule 6: a reserve, only when no regular candidate exists.
	if best, ok := pickBest(filter(others, func(v view) bool { return v.eligible() && v.Role == RoleReserve })); ok {
		return takeOver(active, best, ReasonReserve, window, pct, forced), forced
	}
	// Rule 5: everybody is past the weekly line; share the load.
	if allPastWeekLine(active, others) {
		candidates := filter(others, func(v view) bool { return v.signedIn() && v.p5 < 100 && v.pw < 100 })
		if best, ok := bestByRawHeadroom(candidates); ok {
			// The 5-point lead cannot be met here (every weekly line is crossed, so
			// raw headroom is at most 5), so the target only has to be strictly better.
			if forced || best.rawHeadroom() > active.rawHeadroom() {
				return Decision{Switch: true, To: best.Name, Reason: ReasonEqualize, Window: window, Pct: pct}, forced
			}
			return Decision{}, forced
		}
	}
	return Decision{NoCandidate: true}, forced
}

// takeOver applies the hysteresis to a switch away from a fired account.
func takeOver(active, target view, reason, window string, pct float64, forced bool) Decision {
	if !forced && target.headroom() < active.headroom()+hysteresis {
		return Decision{}
	}
	return Decision{Switch: true, To: target.Name, Reason: reason, Window: window, Pct: pct}
}

// returnToPrincipal is rule 4. It has no hysteresis: the principal spends its
// weekly quota first, so it usually has less headroom than a fresher account.
// The cooldown and the principal being under its lines keep it from flapping.
func returnToPrincipal(active view, others []view) Decision {
	if active.Role == RolePrincipal {
		return Decision{}
	}
	for _, p := range others {
		if p.Role == RolePrincipal && p.eligible() && resetNoLater(p, active) {
			return Decision{Switch: true, To: p.Name, Reason: ReasonReturnPrincipal}
		}
	}
	return Decision{}
}

func coolingDown(last, now time.Time) bool {
	return !last.IsZero() && now.Sub(last) < Cooldown
}

// view is an account projected to now: windows whose reset has passed count as
// 0%, and a passed weekly reset is an unknown one.
type view struct {
	Account
	p5, pw    float64
	weekReset time.Time // zero = unknown
}

func project(a Account, now time.Time) view {
	v := view{Account: a, p5: a.Usage.FiveHour, pw: a.Usage.Week, weekReset: a.Usage.WeekResetsAt}
	if passed(a.Usage.FiveHourResetsAt, now) {
		v.p5 = 0
	}
	if passed(a.Usage.WeekResetsAt, now) {
		v.pw, v.weekReset = 0, time.Time{}
	}
	if !a.Usage.HasData {
		v.p5, v.pw = 0, 0
	}
	return v
}

func passed(resetsAt, now time.Time) bool { return !resetsAt.IsZero() && !resetsAt.After(now) }

// signedIn reports whether cid can still switch to the account.
func (v view) signedIn() bool {
	return v.Status != credentials.StatusNeedsLogin && v.Status != credentials.StatusUnknown
}

func (v view) underLines() bool {
	return v.p5 < float64(v.FiveHourLine) && v.pw < float64(v.WeekLine)
}

func (v view) eligible() bool { return v.signedIn() && v.underLines() }

// headroom is the room left before the nearest line.
func (v view) headroom() float64 {
	return min(float64(v.FiveHourLine)-v.p5, float64(v.WeekLine)-v.pw)
}

// rawHeadroom ignores the lines: the room left before 100%.
func (v view) rawHeadroom() float64 { return min(100-v.p5, 100-v.pw) }

// trigger reports which window of the active account crossed its line. When
// both did, the one with the higher percentage is reported.
func trigger(v view) (window string, pct float64, fired bool) {
	hit5 := v.p5 >= float64(v.FiveHourLine)
	hitW := v.pw >= float64(v.WeekLine)
	switch {
	case hit5 && hitW:
		if v.pw >= v.p5 {
			return WindowWeek, v.pw, true
		}
		return Window5h, v.p5, true
	case hit5:
		return Window5h, v.p5, true
	case hitW:
		return WindowWeek, v.pw, true
	}
	return "", 0, false
}

// allPastWeekLine reports whether every enrolled account has crossed its
// weekly line.
func allPastWeekLine(active view, others []view) bool {
	if active.pw < float64(active.WeekLine) {
		return false
	}
	for _, v := range others {
		if v.pw < float64(v.WeekLine) {
			return false
		}
	}
	return true
}

// resetNoLater reports whether p's weekly reset is no later than a's. An
// unknown reset sorts last.
func resetNoLater(p, a view) bool {
	switch {
	case a.weekReset.IsZero():
		return true
	case p.weekReset.IsZero():
		return false
	}
	return !p.weekReset.After(a.weekReset)
}

// better orders candidates for rule 3: earliest weekly reset (unknown last),
// accounts with data, the principal, most headroom, lowest index.
func better(a, b view) bool {
	if a.weekReset.IsZero() != b.weekReset.IsZero() {
		return !a.weekReset.IsZero()
	}
	if !a.weekReset.Equal(b.weekReset) {
		return a.weekReset.Before(b.weekReset)
	}
	if a.Usage.HasData != b.Usage.HasData {
		return a.Usage.HasData
	}
	if (a.Role == RolePrincipal) != (b.Role == RolePrincipal) {
		return a.Role == RolePrincipal
	}
	if a.headroom() != b.headroom() {
		return a.headroom() > b.headroom()
	}
	return a.Index < b.Index
}

func pickBest(vs []view) (view, bool) { return pick(vs, better) }

// bestByRawHeadroom ranks for the equalize step: most raw headroom, then the
// principal, then the lowest index.
func bestByRawHeadroom(vs []view) (view, bool) {
	return pick(vs, func(a, b view) bool {
		if a.rawHeadroom() != b.rawHeadroom() {
			return a.rawHeadroom() > b.rawHeadroom()
		}
		if (a.Role == RolePrincipal) != (b.Role == RolePrincipal) {
			return a.Role == RolePrincipal
		}
		return a.Index < b.Index
	})
}

func pick(vs []view, less func(a, b view) bool) (view, bool) {
	if len(vs) == 0 {
		return view{}, false
	}
	top := vs[0]
	for _, v := range vs[1:] {
		if less(v, top) {
			top = v
		}
	}
	return top, true
}

func find(vs []view, name string) *view {
	for i := range vs {
		if vs[i].Name == name {
			return &vs[i]
		}
	}
	return nil
}

func without(vs []view, name string) []view {
	return filter(vs, func(v view) bool { return v.Name != name })
}

func filter(vs []view, keep func(view) bool) []view {
	var out []view
	for _, v := range vs {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}
