package auto

import (
	"testing"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/credentials"
)

var now = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

// acct builds an enrolled account with data: default lines 90/95, a 5h window
// that resets in 3h and a weekly window that resets in 3 days.
func acct(name string, index int, five, week float64) Account {
	return Account{
		Name: name, Index: index, Status: credentials.StatusOK,
		FiveHourLine: 90, WeekLine: 95,
		Usage: Usage{
			HasData:  true,
			FiveHour: five, FiveHourResetsAt: now.Add(3 * time.Hour),
			Week: week, WeekResetsAt: now.Add(72 * time.Hour),
		},
	}
}

func weekResetsIn(a Account, d time.Duration) Account {
	a.Usage.WeekResetsAt = now.Add(d)
	return a
}

func unknownReset(a Account) Account { a.Usage.WeekResetsAt = time.Time{}; return a }

func withRole(a Account, r Role) Account { a.Role = r; return a }

func withStatus(a Account, s credentials.Status) Account { a.Status = s; return a }

func noData(name string, index int) Account {
	a := acct(name, index, 0, 0)
	a.Usage = Usage{}
	return a
}

func TestDecide(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want Decision
	}{
		{
			name: "no trigger, no principal: stay",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 10, 10), acct("b", 2, 0, 0)}},
			want: Decision{},
		},
		{
			name: "active not enrolled: never switch away",
			in:   Input{Active: "x", Accounts: []Account{acct("a", 1, 99, 99), acct("b", 2, 0, 0)}},
			want: Decision{},
		},
		{
			name: "empty input",
			in:   Input{Active: "a"},
			want: Decision{},
		},
		{
			name: "5h line triggers",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), acct("b", 2, 10, 10)}},
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "week line triggers",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 10, 95), acct("b", 2, 10, 10)}},
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: WindowWeek, Pct: 95},
		},
		{
			name: "both windows past the line: the higher percentage is reported",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 91, 97), acct("b", 2, 10, 10)}},
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: WindowWeek, Pct: 97},
		},
		{
			name: "just under the line: stay",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 89.9, 94.9), acct("b", 2, 0, 0)}},
			want: Decision{},
		},
		{
			name: "per-account lines are honored",
			in: func() Input {
				a := acct("a", 1, 60, 10)
				a.FiveHourLine = 60
				return Input{Active: "a", Accounts: []Account{a, acct("b", 2, 10, 10)}}
			}(),
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 60},
		},
		{
			name: "passed 5h reset projects the active to 0%: no trigger",
			in: func() Input {
				a := acct("a", 1, 95, 10)
				a.Usage.FiveHourResetsAt = now.Add(-time.Minute)
				return Input{Active: "a", Accounts: []Account{a, acct("b", 2, 0, 0)}}
			}(),
			want: Decision{},
		},
		{
			name: "passed weekly reset projects the active to 0%: no trigger",
			in: func() Input {
				a := acct("a", 1, 10, 99)
				a.Usage.WeekResetsAt = now
				return Input{Active: "a", Accounts: []Account{a, acct("b", 2, 0, 0)}}
			}(),
			want: Decision{},
		},
		{
			name: "candidate over the 5h line is not eligible",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), acct("b", 2, 90, 10), acct("c", 3, 50, 10)}},
			want: Decision{Switch: true, To: "c", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "candidate whose 5h window already reset is eligible",
			in: func() Input {
				b := acct("b", 2, 99, 10)
				b.Usage.FiveHourResetsAt = now.Add(-time.Second)
				return Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), b}}
			}(),
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "candidate whose weekly window already reset is eligible",
			in: func() Input {
				b := acct("b", 2, 10, 99)
				b.Usage.WeekResetsAt = now.Add(-time.Second)
				return Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), b}}
			}(),
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "needs_login and unknown are never candidates",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 90, 10),
				withStatus(acct("b", 2, 0, 0), credentials.StatusNeedsLogin),
				withStatus(acct("c", 3, 0, 0), credentials.StatusUnknown),
				withStatus(acct("d", 4, 20, 20), credentials.StatusAccessExpired),
			}},
			want: Decision{Switch: true, To: "d", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "earliest weekly reset wins over more headroom",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 90, 10),
				weekResetsIn(acct("b", 2, 50, 50), 24*time.Hour),
				weekResetsIn(acct("c", 3, 0, 0), 48*time.Hour),
			}},
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "known weekly reset beats unknown reset",
			in: func() Input {
				c := acct("c", 3, 10, 10)
				c.Usage.WeekResetsAt = time.Time{}
				return Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), c, weekResetsIn(acct("b", 2, 40, 40), 100*time.Hour)}}
			}(),
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "same reset: principal wins",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 90, 10), acct("b", 2, 0, 0), withRole(acct("c", 3, 40, 40), RolePrincipal),
			}},
			want: Decision{Switch: true, To: "c", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "same reset, no principal: most headroom wins",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), acct("b", 2, 40, 40), acct("c", 3, 10, 10)}},
			want: Decision{Switch: true, To: "c", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "headroom is the smaller of the two windows",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), acct("b", 2, 0, 80), acct("c", 3, 50, 50)}},
			want: Decision{Switch: true, To: "c", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "full tie: lowest index wins",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), acct("c", 3, 10, 10), acct("b", 2, 10, 10)}},
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "no-data account ranks after an account with data and an unknown reset",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), noData("b", 2), unknownReset(acct("c", 3, 60, 60))}},
			want: Decision{Switch: true, To: "c", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "no-data account is still a candidate when alone",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), noData("b", 2)}},
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "no-data principal does not outrank an account with data",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), withRole(noData("b", 2), RolePrincipal), unknownReset(acct("c", 3, 30, 30))}},
			want: Decision{Switch: true, To: "c", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "reserve is skipped while a regular candidate exists",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 90, 10), withRole(weekResetsIn(acct("r", 2, 0, 0), time.Hour), RoleReserve), acct("c", 3, 60, 60),
			}},
			want: Decision{Switch: true, To: "c", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "reserve is used when no regular candidate is under its lines",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 90, 10), acct("b", 2, 95, 10), withRole(acct("r", 3, 0, 0), RoleReserve),
			}},
			want: Decision{Switch: true, To: "r", Reason: ReasonReserve, Window: Window5h, Pct: 90},
		},
		{
			name: "reserve over its own line is not eligible: no candidate",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 90, 10), withRole(acct("r", 2, 91, 0), RoleReserve),
			}},
			want: Decision{NoCandidate: true},
		},
		{
			name: "no candidate and not everyone past the weekly line: notify, stay",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), acct("b", 2, 95, 10)}},
			want: Decision{NoCandidate: true},
		},
		{
			name: "single enrolled account at its line: no candidate",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 99, 10)}},
			want: Decision{NoCandidate: true},
		},
		{
			name: "equalize: everyone past the weekly line, most raw headroom wins",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 20, 99), acct("b", 2, 10, 96), acct("c", 3, 50, 97),
			}},
			// a is forced? no: 99 < 100. Hysteresis does not apply to equalize, only strict improvement.
			want: Decision{Switch: true, To: "b", Reason: ReasonEqualize, Window: WindowWeek, Pct: 99},
		},
		{
			name: "equalize includes the reserve",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 20, 99), withRole(acct("r", 2, 10, 96), RoleReserve),
			}},
			want: Decision{Switch: true, To: "r", Reason: ReasonEqualize, Window: WindowWeek, Pct: 99},
		},
		{
			name: "equalize skips accounts at 100% in a window",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 20, 99), acct("b", 2, 100, 96), acct("c", 3, 10, 100),
			}},
			want: Decision{NoCandidate: true},
		},
		{
			name: "equalize needs strictly more headroom than the active",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 20, 96), acct("b", 2, 10, 97)}},
			want: Decision{},
		},
		{
			name: "equalize ignores accounts that need login",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 20, 99), withStatus(acct("b", 2, 0, 96), credentials.StatusNeedsLogin),
			}},
			want: Decision{NoCandidate: true},
		},
		{
			name: "equalize needs every enrolled account past its weekly line",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 20, 99), withStatus(acct("b", 2, 0, 10), credentials.StatusNeedsLogin),
			}},
			want: Decision{NoCandidate: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.in, now); got != tt.want {
				t.Errorf("Decide = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecideReturnToPrincipal(t *testing.T) {
	principal := func(five, week float64, weekReset time.Duration) Account {
		return withRole(weekResetsIn(acct("p", 1, five, week), weekReset), RolePrincipal)
	}
	tests := []struct {
		name string
		in   Input
		want Decision
	}{
		{
			name: "principal resets weekly sooner and is under its lines: return",
			in: Input{Active: "b", Accounts: []Account{
				principal(5, 20, 24*time.Hour), weekResetsIn(acct("b", 2, 30, 30), 72*time.Hour),
			}},
			want: Decision{Switch: true, To: "p", Reason: ReasonReturnPrincipal},
		},
		{
			name: "equal weekly resets: return",
			in: Input{Active: "b", Accounts: []Account{
				principal(5, 20, 72*time.Hour), acct("b", 2, 30, 30),
			}},
			want: Decision{Switch: true, To: "p", Reason: ReasonReturnPrincipal},
		},
		{
			name: "principal resets later than the active: stay",
			in: Input{Active: "b", Accounts: []Account{
				principal(5, 20, 96*time.Hour), weekResetsIn(acct("b", 2, 30, 30), 72*time.Hour),
			}},
			want: Decision{},
		},
		{
			name: "principal over its 5h line: stay",
			in: Input{Active: "b", Accounts: []Account{
				principal(90, 20, 24*time.Hour), acct("b", 2, 30, 30),
			}},
			want: Decision{},
		},
		{
			name: "principal over its weekly line: stay",
			in: Input{Active: "b", Accounts: []Account{
				principal(5, 95, 24*time.Hour), acct("b", 2, 30, 30),
			}},
			want: Decision{},
		},
		{
			name: "principal needs login: stay",
			in: Input{Active: "b", Accounts: []Account{
				withStatus(principal(5, 20, 24*time.Hour), credentials.StatusNeedsLogin), acct("b", 2, 30, 30),
			}},
			want: Decision{},
		},
		{
			name: "principal already active: nothing to do",
			in: Input{Active: "p", Accounts: []Account{
				principal(5, 20, 24*time.Hour), acct("b", 2, 0, 0),
			}},
			want: Decision{},
		},
		{
			name: "principal with a passed 5h reset counts as 0%: return",
			in: func() Input {
				p := principal(95, 20, 24*time.Hour)
				p.Usage.FiveHourResetsAt = now.Add(-time.Minute)
				return Input{Active: "b", Accounts: []Account{p, acct("b", 2, 30, 30)}}
			}(),
			want: Decision{Switch: true, To: "p", Reason: ReasonReturnPrincipal},
		},
		{
			name: "principal with an unknown reset does not beat an active with a known one",
			in: func() Input {
				p := principal(5, 20, 24*time.Hour)
				p.Usage.WeekResetsAt = time.Time{}
				return Input{Active: "b", Accounts: []Account{p, acct("b", 2, 30, 30)}}
			}(),
			want: Decision{},
		},
		{
			// The principal spends its weekly quota first, so it usually has less
			// headroom than a fresher account; the 5-point lead does not apply.
			name: "return ignores the hysteresis lead",
			in: Input{Active: "b", Accounts: []Account{
				principal(0, 60, 24*time.Hour), acct("b", 2, 0, 0),
			}},
			want: Decision{Switch: true, To: "p", Reason: ReasonReturnPrincipal},
		},
		{
			name: "a reserve principal is irrelevant; the active reserve returns to the principal",
			in: Input{Active: "r", Accounts: []Account{
				principal(5, 20, 24*time.Hour), withRole(weekResetsIn(acct("r", 2, 30, 30), 72*time.Hour), RoleReserve),
			}},
			want: Decision{Switch: true, To: "p", Reason: ReasonReturnPrincipal},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.in, now); got != tt.want {
				t.Errorf("Decide = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecideAntiFlap(t *testing.T) {
	recent := now.Add(-4 * time.Minute)
	tests := []struct {
		name string
		in   Input
		want Decision
	}{
		{
			name: "hysteresis: target headroom only 3 above the active's: stay",
			// The active is at its line (headroom 0); the target has 3 points left.
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), acct("b", 2, 87, 10)}},
			want: Decision{},
		},
		{
			name: "hysteresis: exactly 5 above passes",
			// active at its line has headroom 0; target 5h 85 gives headroom 5.
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), acct("b", 2, 85, 10)}},
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "hysteresis: 4.9 above fails",
			in:   Input{Active: "a", Accounts: []Account{acct("a", 1, 90, 10), acct("b", 2, 85.1, 10)}},
			want: Decision{},
		},
		{
			name: "cooldown: a threshold switch 4 minutes after the last one is held back",
			in:   Input{Active: "a", LastSwitchAt: recent, Accounts: []Account{acct("a", 1, 90, 10), acct("b", 2, 0, 0)}},
			want: Decision{},
		},
		{
			name: "cooldown: allowed at exactly 5 minutes",
			in:   Input{Active: "a", LastSwitchAt: now.Add(-5 * time.Minute), Accounts: []Account{acct("a", 1, 90, 10), acct("b", 2, 0, 0)}},
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
		},
		{
			name: "cooldown also holds back the return to the principal",
			in: Input{Active: "b", LastSwitchAt: recent, Accounts: []Account{
				withRole(weekResetsIn(acct("p", 1, 0, 0), 24*time.Hour), RolePrincipal), acct("b", 2, 40, 40),
			}},
			want: Decision{},
		},
		{
			name: "active at 100% bypasses the cooldown",
			in:   Input{Active: "a", LastSwitchAt: recent, Accounts: []Account{acct("a", 1, 100, 10), acct("b", 2, 0, 0)}},
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 100},
		},
		{
			name: "active weekly at 100% bypasses the cooldown",
			in:   Input{Active: "a", LastSwitchAt: recent, Accounts: []Account{acct("a", 1, 10, 100), acct("b", 2, 0, 0)}},
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: WindowWeek, Pct: 100},
		},
		{
			name: "active at 100% bypasses the hysteresis too",
			in: func() Input {
				a := acct("a", 1, 100, 10)
				a.FiveHourLine = 100 // headroom 0, so b (headroom 1) would fail the lead
				return Input{Active: "a", Accounts: []Account{a, acct("b", 2, 89, 10)}}
			}(),
			want: Decision{Switch: true, To: "b", Reason: ReasonThreshold, Window: Window5h, Pct: 100},
		},
		{
			name: "forced equalize also bypasses the cooldown",
			in:   Input{Active: "a", LastSwitchAt: recent, Accounts: []Account{acct("a", 1, 20, 100), acct("b", 2, 10, 96)}},
			want: Decision{Switch: true, To: "b", Reason: ReasonEqualize, Window: WindowWeek, Pct: 100},
		},
		{
			name: "a passed 100% window projects to 0%: the cooldown holds again",
			in: func() Input {
				a := acct("a", 1, 100, 10)
				a.Usage.FiveHourResetsAt = now.Add(-time.Minute)
				a.Usage.Week = 95
				return Input{Active: "a", LastSwitchAt: recent, Accounts: []Account{a, acct("b", 2, 0, 0)}}
			}(),
			want: Decision{},
		},
		{
			name: "reserve switches obey the hysteresis",
			in: Input{Active: "a", Accounts: []Account{
				acct("a", 1, 90, 10), withRole(acct("r", 2, 88, 10), RoleReserve),
			}},
			want: Decision{},
		},
		{
			name: "no candidate during the cooldown is still reported",
			in:   Input{Active: "a", LastSwitchAt: recent, Accounts: []Account{acct("a", 1, 90, 10)}},
			want: Decision{NoCandidate: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.in, now); got != tt.want {
				t.Errorf("Decide = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestDecideUserScenario replays the acceptance scenario over time: account 1
// (principal) has its week resetting tomorrow, account 2 on Sunday (5 days).
func TestDecideUserScenario(t *testing.T) {
	start := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC) // Monday 09:00
	one := func(t time.Time, five, week float64, fiveReset time.Duration) Account {
		a := withRole(acct("1", 1, five, week), RolePrincipal)
		a.Usage.FiveHourResetsAt = t.Add(fiveReset)
		a.Usage.WeekResetsAt = start.Add(24 * time.Hour)
		return a
	}
	two := func(t time.Time, five, week float64) Account {
		a := acct("2", 2, five, week)
		a.Usage.FiveHourResetsAt = t.Add(4 * time.Hour)
		a.Usage.WeekResetsAt = start.Add(5 * 24 * time.Hour)
		return a
	}

	steps := []struct {
		at         time.Duration
		accounts   func(t time.Time) []Account
		want       Decision
		wantActive string
	}{
		{
			// The user sits on account 2; account 1's weekly quota expires first.
			at:         0,
			accounts:   func(t time.Time) []Account { return []Account{one(t, 5, 10, 4*time.Hour), two(t, 5, 20)} },
			want:       Decision{Switch: true, To: "1", Reason: ReasonReturnPrincipal},
			wantActive: "1",
		},
		{
			at:         time.Hour,
			accounts:   func(t time.Time) []Account { return []Account{one(t, 50, 45, 3*time.Hour), two(t, 5, 20)} },
			want:       Decision{},
			wantActive: "1",
		},
		{
			// Account 1 reaches its 5h line: move to 2.
			at:         3 * time.Hour,
			accounts:   func(t time.Time) []Account { return []Account{one(t, 90, 55, 1*time.Hour), two(t, 5, 20)} },
			want:       Decision{Switch: true, To: "2", Reason: ReasonThreshold, Window: Window5h, Pct: 90},
			wantActive: "2",
		},
		{
			// Two minutes later: cooldown, and nothing triggers on 2 anyway.
			at:         3*time.Hour + 2*time.Minute,
			accounts:   func(t time.Time) []Account { return []Account{one(t, 90, 55, 1*time.Hour), two(t, 8, 21)} },
			want:       Decision{},
			wantActive: "2",
		},
		{
			// Account 1's 5h window has reset (projected to 0%): return to it.
			at: 4*time.Hour + 5*time.Minute,
			accounts: func(t time.Time) []Account {
				a := one(t, 90, 55, -5*time.Minute)
				return []Account{a, two(t, 30, 30)}
			},
			want:       Decision{Switch: true, To: "1", Reason: ReasonReturnPrincipal},
			wantActive: "1",
		},
		{
			// Everything is past its weekly line: equalize to the account with more headroom.
			at: 8 * time.Hour,
			accounts: func(t time.Time) []Account {
				a := one(t, 20, 98, 2*time.Hour)
				return []Account{a, two(t, 10, 96)}
			},
			want:       Decision{Switch: true, To: "2", Reason: ReasonEqualize, Window: WindowWeek, Pct: 98},
			wantActive: "2",
		},
	}

	active := "2"
	var last time.Time
	for _, st := range steps {
		at := start.Add(st.at)
		in := Input{Active: active, LastSwitchAt: last, Accounts: st.accounts(at)}
		got := Decide(in, at)
		if got != st.want {
			t.Fatalf("t+%v: Decide = %+v, want %+v", st.at, got, st.want)
		}
		if got.Switch {
			active, last = got.To, at
		}
		if active != st.wantActive {
			t.Fatalf("t+%v: active = %s, want %s", st.at, active, st.wantActive)
		}
	}
}
