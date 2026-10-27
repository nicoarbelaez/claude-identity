package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/auto"
	"github.com/nicoarbelaez/claude-identity/internal/swap"
	"github.com/nicoarbelaez/claude-identity/internal/usage"
)

const promptSubmit = `{"hook_event_name":"UserPromptSubmit","prompt":"hi"}`

// autoRig is a statusline rig with a and b enrolled and auto-switch on. b is the
// live account at 95% of its 5h window (past the 90% line); a has plenty of room
// and the earlier weekly reset, so the rule moves b to a.
func autoRig(t *testing.T) (*rig, time.Time) {
	t.Helper()
	r, now := newStatuslineRig(t)
	err := r.autoCfg.Update(func(c *auto.Config) error {
		c.Enabled = true
		if err := c.Enroll("a", nil, nil); err != nil {
			return err
		}
		return c.Enroll("b", nil, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	r.recordUsage("a", 10, 20, now)
	return r, now
}

func (r *rig) recordUsage(name string, five, week float64, now time.Time) {
	r.t.Helper()
	w := map[string]usage.Window{
		usage.FiveHour: {PercentUsed: five, ResetsAt: now.Add(3 * time.Hour)},
		usage.SevenDay: {PercentUsed: week, ResetsAt: now.Add(24 * time.Hour)},
	}
	if err := r.usage.Record(name, w, now); err != nil {
		r.t.Fatal(err)
	}
}

// liveInput is the status line JSON of the live account b.
func liveInput(now time.Time, five float64) string {
	return fmt.Sprintf(`{"rate_limits":{"five_hour":{"used_percentage":%g,"resets_at":%d},"seven_day":{"used_percentage":10,"resets_at":%d}}}`,
		five, now.Add(3*time.Hour).Unix(), now.Add(48*time.Hour).Unix())
}

func (r *rig) state() auto.State {
	r.t.Helper()
	st, err := r.autoState.Load()
	if err != nil {
		r.t.Fatal(err)
	}
	return st
}

func (r *rig) setState(fn func(*auto.State)) {
	r.t.Helper()
	if err := r.autoState.Update(fn); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) statePath() string { return auto.StatePathFor(r.deps.Config.ProfilesPath) }

func (r *rig) activeName() string {
	r.t.Helper()
	p, err := r.prof.Active()
	if err != nil {
		r.t.Fatal(err)
	}
	return p.Name
}

func TestStatuslineEvaluationWritesPending(t *testing.T) {
	r, now := autoRig(t)
	if code := r.statusline(liveInput(now, 95)); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	p := r.state().Pending
	if p == nil {
		t.Fatal("no pending switch written")
	}
	if p.From != "b" || p.To != "a" || p.Reason != auto.ReasonThreshold || p.Window != auto.Window5h || p.Pct != 95 || !p.At.Equal(now) {
		t.Errorf("pending = %+v", p)
	}
	if got := r.activeName(); got != "b" {
		t.Errorf("the status line must not switch; active = %q", got)
	}
}

func TestStatuslineEvaluationTable(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(r *rig, now time.Time)
		five        float64
		wantPending bool
		wantFile    bool
	}{
		{name: "under the lines", five: 40},
		{name: "disabled", five: 95, setup: func(r *rig, _ time.Time) {
			_ = r.autoCfg.Update(func(c *auto.Config) error { c.Enabled = false; return nil })
		}},
		{name: "fires", five: 95, wantPending: true, wantFile: true},
		{name: "stale pending is dropped when the decision no longer holds", five: 40, wantFile: true, setup: func(r *rig, now time.Time) {
			r.setState(func(s *auto.State) {
				s.Pending = &auto.Pending{From: "b", To: "a", Reason: auto.ReasonThreshold, Window: auto.Window5h, Pct: 95, At: now.Add(-time.Minute)}
			})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, now := autoRig(t)
			if tt.setup != nil {
				tt.setup(r, now)
			}
			if code := r.statusline(liveInput(now, tt.five)); code != 0 {
				t.Fatalf("exit %d", code)
			}
			if got := r.state().Pending != nil; got != tt.wantPending {
				t.Errorf("pending present = %v, want %v", got, tt.wantPending)
			}
			_, err := os.Stat(r.statePath())
			if got := err == nil; got != tt.wantFile {
				t.Errorf("state file exists = %v, want %v", got, tt.wantFile)
			}
		})
	}
}

func TestStatuslineEvaluationLeavesTheWrappedCommandAlone(t *testing.T) {
	r, now := autoRig(t)
	r.fakeExec("my line\n", 7)
	code := r.statusline(liveInput(now, 95), "--", "x")
	if code != 7 || r.stdout.String() != "my line\n" {
		t.Errorf("exit %d, stdout %q; want 7 and the wrapped output only", code, r.stdout)
	}
	if r.state().Pending == nil {
		t.Error("pending not written")
	}
}

func TestStatuslineNoCandidateNoticeIsDeduped(t *testing.T) {
	r, now := autoRig(t)
	r.recordUsage("a", 95, 20, now) // nobody can take over
	cur := now
	r.deps.Statusline.Now = func() time.Time { return cur }

	if code := r.statusline(liveInput(now, 95)); code != 0 {
		t.Fatalf("exit %d", code)
	}
	st := r.state()
	if st.Pending != nil || st.Notice == nil || !st.LastNoCandidateAt.Equal(now) {
		t.Fatalf("state = %+v, want a notice and no pending", st)
	}
	if !strings.Contains(st.Notice.Text, "b") || !st.Notice.Until.Equal(now.Add(2*time.Minute)) {
		t.Errorf("notice = %+v", st.Notice)
	}

	r.setState(func(s *auto.State) { s.Notice = nil })
	cur = now.Add(30 * time.Minute)
	_ = r.statusline(liveInput(cur, 95))
	if st := r.state(); st.Notice != nil || !st.LastNoCandidateAt.Equal(now) {
		t.Errorf("within the hour the notice must not repeat: %+v", st)
	}

	cur = now.Add(61 * time.Minute)
	_ = r.statusline(liveInput(cur, 95))
	if st := r.state(); st.Notice == nil || !st.LastNoCandidateAt.Equal(cur) {
		t.Errorf("after the hour the notice repeats: %+v", st)
	}
}

// ---- hook: UserPromptSubmit ----

func (r *rig) setPending(at time.Time) {
	r.setState(func(s *auto.State) {
		s.Pending = &auto.Pending{From: "b", To: "a", Reason: auto.ReasonThreshold, Window: auto.Window5h, Pct: 95, At: at}
	})
}

func TestPromptSubmitWithoutPendingIsANoOp(t *testing.T) {
	r, _ := autoRig(t)
	code, out := r.hook(promptSubmit)
	if code != 0 || out != "" || r.stderr.Len() != 0 {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, r.stderr)
	}
	if _, err := os.Stat(r.statePath()); err == nil {
		t.Error("the fast path must not create the state file")
	}
}

func TestPromptSubmitAppliesPending(t *testing.T) {
	r, now := autoRig(t)
	r.setPending(now.Add(-time.Minute))
	code, out := r.hook(promptSubmit)
	if code != 0 || out != "" {
		t.Fatalf("exit %d, stdout %q; the hook must stay silent", code, out)
	}
	if got := r.activeName(); got != "a" {
		t.Errorf("active = %q, want a", got)
	}
	st := r.state()
	if st.Pending != nil || !st.LastSwitchAt.Equal(now) {
		t.Errorf("pending %+v, lastSwitchAt %v", st.Pending, st.LastSwitchAt)
	}
	if st.Notice == nil || st.Notice.Text != "⇄ b → a (5h 95%)" || !st.Notice.Until.Equal(now.Add(2*time.Minute)) {
		t.Errorf("notice = %+v", st.Notice)
	}
	if len(st.History) != 1 || st.History[0].From != "b" || st.History[0].To != "a" || st.History[0].Error != "" {
		t.Errorf("history = %+v", st.History)
	}
}

func TestPromptSubmitIgnoresInvalidPending(t *testing.T) {
	tests := []struct {
		name  string
		setup func(r *rig, now time.Time)
	}{
		{"older than ten minutes", func(r *rig, now time.Time) { r.setPending(now.Add(-11 * time.Minute)) }},
		{"from is not the live account", func(r *rig, now time.Time) {
			r.setState(func(s *auto.State) {
				s.Pending = &auto.Pending{From: "a", To: "b", Reason: auto.ReasonThreshold, Window: auto.Window5h, Pct: 95, At: now}
			})
		}},
		{"auto turned off", func(r *rig, now time.Time) {
			r.setPending(now)
			_ = r.autoCfg.Update(func(c *auto.Config) error { c.Enabled = false; return nil })
		}},
		{"target no longer enrolled", func(r *rig, now time.Time) {
			r.setPending(now)
			_ = r.autoCfg.Update(func(c *auto.Config) error { c.Remove("a"); return nil })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, now := autoRig(t)
			tt.setup(r, now)
			code, out := r.hook(promptSubmit)
			if code != 0 || out != "" {
				t.Fatalf("exit %d, stdout %q", code, out)
			}
			if got := r.activeName(); got != "b" {
				t.Errorf("active = %q; nothing should have switched", got)
			}
			st := r.state()
			if st.Pending != nil || st.Notice != nil || !st.LastSwitchAt.IsZero() || len(st.History) != 0 {
				t.Errorf("state = %+v, want the pending dropped and nothing else", st)
			}
		})
	}
}

func TestPromptSubmitFailureNeverBlocks(t *testing.T) {
	r, now := autoRig(t)
	r.setPending(now)
	r.deps.Swap = fakeSwap{err: errors.New("boom")}
	code, out := r.hook(promptSubmit)
	if code != 0 || out != "" {
		t.Fatalf("exit %d, stdout %q; a failed switch must not block the prompt", code, out)
	}
	st := r.state()
	if st.Pending != nil {
		t.Error("pending must be cleared after a failure")
	}
	if st.Notice == nil || !strings.Contains(st.Notice.Text, "a") {
		t.Errorf("notice = %+v", st.Notice)
	}
	if len(st.History) != 1 || st.History[0].Error == "" {
		t.Errorf("history = %+v", st.History)
	}
}

func TestPromptSubmitNoOpSwitchLeavesNoNotice(t *testing.T) {
	r, now := autoRig(t)
	r.setPending(now)
	r.deps.Swap = fakeSwap{switchRes: swap.Result{NoOp: true}}
	if code, out := r.hook(promptSubmit); code != 0 || out != "" {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
	if st := r.state(); st.Pending != nil || st.Notice != nil {
		t.Errorf("state = %+v", st)
	}
}

// ---- notice line ----

func TestStatuslinePrintsTheNotice(t *testing.T) {
	tests := []struct {
		name    string
		wrapped string // "" means no wrapped command
		until   time.Duration
		want    string
		cleared bool
	}{
		{name: "no wrapped command", until: time.Minute, want: "⇄ b → a (5h 95%)\n"},
		{name: "after the wrapped output", wrapped: "bar\n", until: time.Minute, want: "bar\n⇄ b → a (5h 95%)\n"},
		{name: "wrapped output without a final newline", wrapped: "bar", until: time.Minute, want: "bar\n⇄ b → a (5h 95%)\n"},
		{name: "expired", wrapped: "bar\n", until: -time.Second, want: "bar\n", cleared: true},
		{name: "expired without a wrapped command", until: 0, want: "", cleared: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, now := autoRig(t)
			r.setState(func(s *auto.State) {
				s.Notice = &auto.Notice{Text: "⇄ b → a (5h 95%)", Until: now.Add(tt.until)}
			})
			args := []string(nil)
			if tt.wrapped != "" {
				r.fakeExec(tt.wrapped, 0)
				args = []string{"--", "x"}
			}
			if code := r.statusline("{}", args...); code != 0 {
				t.Fatalf("exit %d", code)
			}
			if got := r.stdout.String(); got != tt.want {
				t.Errorf("stdout = %q, want %q", got, tt.want)
			}
			if tt.cleared && r.state().Notice != nil {
				t.Error("an expired notice should be cleared")
			}
		})
	}
}

func TestStatuslineWithoutNoticeIsUnchanged(t *testing.T) {
	r, _ := autoRig(t)
	r.fakeExec("bar", 0)
	_ = r.statusline("{}", "--", "x")
	if got := r.stdout.String(); got != "bar" {
		t.Errorf("stdout = %q, want the wrapped output byte for byte", got)
	}
}

// TestAutoSwitchEndToEnd walks the whole path on temp dirs and an in-memory
// vault: the status line queues a switch, the prompt hook applies it, and the
// next status line shows the notice.
func TestAutoSwitchEndToEnd(t *testing.T) {
	r, now := autoRig(t)
	r.fakeExec("model: x\n", 0)

	if code := r.statusline(liveInput(now, 95), "--", "x"); code != 0 {
		t.Fatalf("statusline exit %d", code)
	}
	t.Logf("statusline 1 stdout: %q", r.stdout.String())
	t.Logf("pending: %+v", r.state().Pending)

	code, out := r.hook(promptSubmit)
	t.Logf("hook exit %d, stdout %q, active now %q", code, out, r.activeName())
	if code != 0 || out != "" || r.activeName() != "a" {
		t.Fatalf("hook did not apply the switch quietly")
	}

	if code := r.statusline("{}", "--", "x"); code != 0 {
		t.Fatalf("statusline exit %d", code)
	}
	t.Logf("statusline 2 stdout: %q", r.stdout.String())
	if want := "model: x\n⇄ b → a (5h 95%)\n"; r.stdout.String() != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}
