package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/auto"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
)

func (r *rig) autoConfig() auto.Config {
	r.t.Helper()
	c, err := r.autoCfg.Load()
	if err != nil {
		r.t.Fatal(err)
	}
	return c
}

func (r *rig) mustRun(args ...string) string {
	r.t.Helper()
	if code := r.run(args...); code != 0 {
		r.t.Fatalf("cid %v: exit %d, stderr %q", args, code, r.stderr)
	}
	return r.stdout.String()
}

// oneLineError asserts a usage error (exit 2) whose stderr is exactly msg.
func (r *rig) oneLineError(msg string, args ...string) {
	r.t.Helper()
	if code := r.run(args...); code != exitUsage {
		r.t.Fatalf("cid %v: exit %d, want 2 (stdout %q)", args, code, r.stdout)
	}
	if got := strings.TrimRight(r.stderr.String(), "\n"); got != msg {
		r.t.Errorf("cid %v: stderr = %q, want %q", args, got, msg)
	}
}

func TestAutoStatusEmpty(t *testing.T) {
	r := newRig(t)
	out := r.mustRun("auto")
	for _, want := range []string{r.msg(i18n.AutoStatusOff, 90, 95), r.msg(i18n.AutoNoAccounts)} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout %q misses %q", out, want)
		}
	}
	if _, err := os.Stat(auto.PathFor(r.deps.Config.ProfilesPath)); err == nil {
		t.Error("a status read must not create auto.json")
	}
}

func TestAutoOnOff(t *testing.T) {
	r := newRig(t)
	if out := r.mustRun("auto", "on"); !strings.Contains(out, r.msg(i18n.AutoOn)) || !r.autoConfig().Enabled {
		t.Errorf("on: stdout %q, enabled %v", out, r.autoConfig().Enabled)
	}
	if out := r.mustRun("auto"); !strings.Contains(out, r.msg(i18n.AutoStatusOn, 90, 95)) {
		t.Errorf("status after on: %q", out)
	}
	if out := r.mustRun("auto", "off"); !strings.Contains(out, r.msg(i18n.AutoOff)) || r.autoConfig().Enabled {
		t.Errorf("off: stdout %q, enabled %v", out, r.autoConfig().Enabled)
	}
	if code := r.run("auto", "on", "extra"); code != exitUsage {
		t.Errorf("on with an extra argument: exit %d, want 2", code)
	}
}

func TestAutoAddByIndexAndName(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	if out := r.mustRun("auto", "add", "1"); !strings.Contains(out, r.msg(i18n.AutoAdded, "a", 90, 95)) {
		t.Errorf("add 1: %q", out)
	}
	if out := r.mustRun("auto", "add", "b", "--5h", "80", "--week", "99"); !strings.Contains(out, r.msg(i18n.AutoAdded, "b", 80, 99)) {
		t.Errorf("add b: %q", out)
	}
	c := r.autoConfig()
	if five, week, ok := c.Lines("a"); !ok || five != 90 || week != 95 {
		t.Errorf("a = %d,%d,%v", five, week, ok)
	}
	if five, week, ok := c.Lines("b"); !ok || five != 80 || week != 99 {
		t.Errorf("b = %d,%d,%v", five, week, ok)
	}
}

func TestAutoAddFlagsMayComeFirst(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.mustRun("auto", "add", "--week", "70", "a", "--5h", "60")
	if five, week, _ := r.autoConfig().Lines("a"); five != 60 || week != 70 {
		t.Errorf("a = %d,%d", five, week)
	}
}

func TestAutoReAddUpdatesOnlyGivenLines(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.mustRun("auto", "add", "a", "--5h", "70", "--week", "80")
	r.mustRun("auto", "principal", "a")
	r.mustRun("auto", "add", "a", "--week", "85")
	c := r.autoConfig()
	if five, week, _ := c.Lines("a"); five != 70 || week != 85 || c.Accounts["a"].Role != auto.RolePrincipal {
		t.Errorf("a = %d,%d role %q", five, week, c.Accounts["a"].Role)
	}
}

func TestAutoAddErrors(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.oneLineError(r.msg(i18n.UnknownProfile, "zzz"), "auto", "add", "zzz")
	r.oneLineError(r.msg(i18n.UnknownProfile, "9"), "auto", "add", "9")
	for _, bad := range []string{"0", "101", "-5", "abc", "9.5", ""} {
		r.oneLineError(r.msg(i18n.AutoInvalidPercentage, bad), "auto", "add", "a", "--5h", bad)
		r.oneLineError(r.msg(i18n.AutoInvalidPercentage, bad), "auto", "add", "a", "--week", bad)
	}
	if len(r.autoConfig().Accounts) != 0 {
		t.Error("a rejected add must not enroll anything")
	}
	for _, args := range [][]string{{"auto", "add"}, {"auto", "add", "a", "b"}, {"auto", "add", "a", "--5h"}, {"auto", "add", "a", "--nope", "1"}} {
		if code := r.run(args...); code != exitUsage {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

func TestAutoRemove(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.mustRun("auto", "add", "a")
	if out := r.mustRun("auto", "remove", "1"); !strings.Contains(out, r.msg(i18n.AutoRemoved, "a")) {
		t.Errorf("remove: %q", out)
	}
	if len(r.autoConfig().Accounts) != 0 {
		t.Error("a still enrolled")
	}
	r.oneLineError(r.msg(i18n.AutoNotEnrolled, "a", "a"), "auto", "remove", "a")
	r.oneLineError(r.msg(i18n.UnknownProfile, "zzz"), "auto", "remove", "zzz")
}

func TestAutoRoles(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.mustRun("auto", "add", "a")
	r.mustRun("auto", "add", "b")
	if out := r.mustRun("auto", "principal", "a"); !strings.Contains(out, r.msg(i18n.AutoPrincipalSet, "a")) {
		t.Errorf("principal: %q", out)
	}
	r.mustRun("auto", "principal", "b")
	c := r.autoConfig()
	if c.Accounts["a"].Role != auto.RoleNone || c.Accounts["b"].Role != auto.RolePrincipal {
		t.Errorf("roles = %q, %q", c.Accounts["a"].Role, c.Accounts["b"].Role)
	}
	if out := r.mustRun("auto", "reserve", "a"); !strings.Contains(out, r.msg(i18n.AutoReserveSet, "a")) || r.autoConfig().Accounts["a"].Role != auto.RoleReserve {
		t.Errorf("reserve: %q", out)
	}
	r.mustRun("auto", "remove", "b")
	r.oneLineError(r.msg(i18n.AutoNotEnrolled, "b", "b"), "auto", "principal", "b")
	r.oneLineError(r.msg(i18n.AutoNotEnrolled, "b", "b"), "auto", "reserve", "b")
	r.oneLineError(r.msg(i18n.UnknownProfile, "zzz"), "auto", "principal", "zzz")
	if code := r.run("auto", "principal"); code != exitUsage {
		t.Errorf("principal without a profile: exit %d", code)
	}
}

func TestAutoDefaults(t *testing.T) {
	r := newRig(t)
	if out := r.mustRun("auto", "defaults"); !strings.Contains(out, r.msg(i18n.AutoDefaultsSet, 90, 95)) {
		t.Errorf("show defaults: %q", out)
	}
	if out := r.mustRun("auto", "defaults", "--5h", "80"); !strings.Contains(out, r.msg(i18n.AutoDefaultsSet, 80, 95)) {
		t.Errorf("set 5h: %q", out)
	}
	r.mustRun("auto", "defaults", "--week", "99", "--5h", "75")
	if c := r.autoConfig(); c.Defaults.FiveHour != 75 || c.Defaults.Week != 99 {
		t.Errorf("defaults = %+v", c.Defaults)
	}
	r.oneLineError(r.msg(i18n.AutoInvalidPercentage, "150"), "auto", "defaults", "--5h", "150")
	if code := r.run("auto", "defaults", "extra"); code != exitUsage {
		t.Errorf("defaults with a positional: exit %d", code)
	}
}

func TestAutoStatusListsAccounts(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.mustRun("auto", "add", "a", "--5h", "80")
	r.mustRun("auto", "add", "b")
	r.mustRun("auto", "principal", "b")
	out := r.mustRun("auto")
	for _, want := range []string{
		r.msg(i18n.AutoAccountLine, 1, "a", 80, 95, ""),
		r.msg(i18n.AutoAccountLine, 2, "b", 90, 95, "  "+r.msg(i18n.AutoRolePrincipal)),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout %q misses %q", out, want)
		}
	}
	if strings.Contains(out, r.msg(i18n.AutoNoAccounts)) {
		t.Error("the empty hint must not show when accounts exist")
	}
}

func TestAutoStatusShowsPendingAndLastSwitch(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	at := time.Date(2026, 10, 27, 12, 30, 0, 0, time.UTC)
	err := r.autoState.Update(func(s *auto.State) {
		s.Pending = &auto.Pending{From: "a", To: "b", Reason: auto.ReasonThreshold, Window: auto.Window5h, Pct: 91, At: at}
		s.LastSwitchAt = at
	})
	if err != nil {
		t.Fatal(err)
	}
	out := r.mustRun("auto")
	for _, want := range []string{
		r.msg(i18n.AutoPendingLine, "a", "b", "5h 91%"),
		r.msg(i18n.AutoLastSwitch, at.Local().Format("2006-01-02 15:04")),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout %q misses %q", out, want)
		}
	}
	_ = r.autoState.Update(func(s *auto.State) {
		s.Pending = &auto.Pending{From: "a", To: "b", Reason: auto.ReasonReturnPrincipal, At: at}
	})
	if out := r.mustRun("auto"); !strings.Contains(out, r.msg(i18n.AutoPendingLine, "a", "b", r.msg(i18n.AutoReasonReturn))) {
		t.Errorf("return-principal pending: %q", out)
	}
}

func TestAutoStatusJSON(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.mustRun("auto", "on")
	r.mustRun("auto", "add", "a", "--week", "88")
	r.mustRun("auto", "reserve", "a")
	var got struct {
		Enabled  bool `json:"enabled"`
		Defaults struct {
			FiveHour int `json:"fiveHour"`
			Week     int `json:"week"`
		} `json:"defaults"`
		Accounts []struct {
			Index    int    `json:"index"`
			Name     string `json:"name"`
			FiveHour int    `json:"fiveHour"`
			Week     int    `json:"week"`
			Role     string `json:"role"`
		} `json:"accounts"`
		Pending      json.RawMessage `json:"pending"`
		LastSwitchAt json.RawMessage `json:"lastSwitchAt"`
	}
	out := r.mustRun("auto", "--json")
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if !got.Enabled || got.Defaults.FiveHour != 90 || got.Defaults.Week != 95 {
		t.Errorf("json = %+v", got)
	}
	if len(got.Accounts) != 1 || got.Accounts[0].Name != "a" || got.Accounts[0].Index != 1 ||
		got.Accounts[0].FiveHour != 90 || got.Accounts[0].Week != 88 || got.Accounts[0].Role != "reserve" {
		t.Errorf("accounts = %+v", got.Accounts)
	}
	if string(got.Pending) != "null" || string(got.LastSwitchAt) != "null" {
		t.Errorf("pending = %s, lastSwitchAt = %s, want null", got.Pending, got.LastSwitchAt)
	}

	at := time.Date(2026, 10, 27, 12, 30, 0, 0, time.UTC)
	_ = r.autoState.Update(func(s *auto.State) {
		s.Pending = &auto.Pending{From: "a", To: "b", Reason: auto.ReasonThreshold, Window: auto.Window5h, Pct: 91, At: at}
		s.LastSwitchAt = at
	})
	out = r.mustRun("auto", "--json")
	for _, want := range []string{`"from":"a"`, `"to":"b"`, `"reason":"threshold"`, `"window":"5h"`, `"pct":91`, `"lastSwitchAt":"2026-10-27T12:30:00Z"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json misses %s: %s", want, out)
		}
	}
}

func TestAutoStatusSkipsEnrolledProfilesThatNoLongerExist(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	if err := r.autoCfg.Update(func(c *auto.Config) error { return c.Enroll("ghost", nil, nil) }); err != nil {
		t.Fatal(err)
	}
	if out := r.mustRun("auto"); strings.Contains(out, "ghost") {
		t.Errorf("stdout lists a missing profile: %q", out)
	}
}

func TestAutoUnknownSubcommandAndNilPort(t *testing.T) {
	r := newRig(t)
	if code := r.run("auto", "bogus"); code != exitUsage {
		t.Errorf("unknown subcommand: exit %d, want 2", code)
	}
	r.deps.Auto = nil
	if code := r.run("auto"); code != exitUsage {
		t.Errorf("without a store: exit %d, want 2", code)
	}
}

func TestRemoveProfileCleansAutoEntries(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.mustRun("auto", "add", "a")
	err := r.autoState.Update(func(s *auto.State) { s.Pending = &auto.Pending{From: "b", To: "a"} })
	if err != nil {
		t.Fatal(err)
	}
	r.mustRun("remove", "a")
	if _, _, ok := r.autoConfig().Lines("a"); ok {
		t.Error("removed profile is still enrolled")
	}
	if s, _ := r.autoState.Load(); s.Pending != nil {
		t.Error("pending switch to a removed profile survived")
	}
}

func TestRemoveProfileDoesNotCreateAutoFiles(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.mustRun("remove", "a")
	for _, p := range []string{auto.PathFor(r.deps.Config.ProfilesPath), auto.StatePathFor(r.deps.Config.ProfilesPath)} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s was created by cid remove", p)
		}
	}
}

func TestUsageMentionsAuto(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		if !strings.Contains(i18n.T(lang, i18n.Usage), "cid auto") {
			t.Errorf("%s usage misses cid auto", lang)
		}
	}
}

func TestHookAllowsAutoCommands(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.block(expansion("account", "auto add 1 --5h 80 --week 99"))
	if five, week, ok := r.autoConfig().Lines("a"); !ok || five != 80 || week != 99 {
		t.Errorf("a = %d,%d,%v", five, week, ok)
	}
	r.block(expansion("account", "auto on"))
	if !r.autoConfig().Enabled {
		t.Error("auto on through the hook did not enable auto-switch")
	}
	b := r.block(expansion("account", "auto --json"))
	if !strings.Contains(b.Reason, `"enabled":true`) {
		t.Errorf("auto --json through the hook: %q", b.Reason)
	}
	b = r.block(expansion("account", "auto"))
	if !strings.Contains(b.Reason, r.msg(i18n.AutoStatusOn, 90, 95)) {
		t.Errorf("auto status through the hook: %q", b.Reason)
	}
}

func TestAutoStatusShowsRecentHistory(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	at := time.Date(2026, 10, 27, 12, 30, 0, 0, time.UTC)
	err := r.autoState.Update(func(s *auto.State) {
		for i := range 7 {
			s.AddHistory(auto.HistoryEntry{At: at.Add(time.Duration(i) * time.Hour), From: "a", To: "b", Window: auto.Window5h, Pct: 91})
		}
		s.AddHistory(auto.HistoryEntry{At: at.Add(8 * time.Hour), From: "b", To: "a", Error: "boom"})
	})
	if err != nil {
		t.Fatal(err)
	}
	out := r.mustRun("auto")
	if !strings.Contains(out, r.msg(i18n.AutoHistoryHeader)) {
		t.Errorf("stdout %q misses the history header", out)
	}
	failed := r.msg(i18n.AutoHistoryFailed, at.Add(8*time.Hour).Local().Format("2006-01-02 15:04"), "b", "a", "boom")
	ok := r.msg(i18n.AutoHistoryLine, at.Add(6*time.Hour).Local().Format("2006-01-02 15:04"), "a", "b", "5h 91%")
	if !strings.Contains(out, failed) || !strings.Contains(out, ok) {
		t.Errorf("stdout %q misses %q or %q", out, failed, ok)
	}
	if strings.Count(out, "→") != 5 {
		t.Errorf("want the 5 most recent entries, got:\n%s", out)
	}
	if strings.Index(out, failed) > strings.Index(out, ok) {
		t.Error("newest entry must come first")
	}
}
