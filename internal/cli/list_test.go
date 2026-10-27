package cli

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/credentials"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/usage"
)

var listNow = time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)

// statusSwap overrides the token status the list shows.
type statusSwap struct {
	Swapper
	status map[string]credentials.Status
}

func (s statusSwap) Status(name string, _ bool) credentials.Status { return s.status[name] }

// usageRig is a rig with profiles a (inactive), b (active) and client-x, all
// with a healthy login except client-x, and a fixed clock.
func usageRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	r.twoProfiles()
	r.login("x@client.com", "tok-x")
	if code := r.run("add", "client-x", "--current"); code != 0 {
		t.Fatal(r.stderr.String())
	}
	if code := r.run("switch", "b"); code != 0 {
		t.Fatal(r.stderr.String())
	}
	r.deps.Swap = statusSwap{Swapper: r.deps.Swap, status: map[string]credentials.Status{
		"a": credentials.StatusOK, "b": credentials.StatusOK, "client-x": credentials.StatusNeedsLogin,
	}}
	r.deps.Now = func() time.Time { return listNow }
	return r
}

func (r *rig) record(name string, fetched time.Time, windows map[string]usage.Window) {
	r.t.Helper()
	if err := r.usage.Record(name, windows, fetched); err != nil {
		r.t.Fatal(err)
	}
}

func win(pct float64, resets time.Duration) usage.Window {
	return usage.Window{PercentUsed: pct, ResetsAt: listNow.Add(resets)}
}

func (r *rig) stdoutAfter(args ...string) string {
	r.t.Helper()
	if code := r.run(args...); code != 0 {
		r.t.Fatalf("exit %d: %s", code, r.stderr)
	}
	return r.stdout.String()
}

func (r *rig) listLines(args ...string) []string {
	r.t.Helper()
	return strings.Split(strings.TrimRight(r.stdoutAfter(args...), "\n"), "\n")
}

func TestListRendersNumberedRowsWithUsage(t *testing.T) {
	r := usageRig(t)
	r.record("a", listNow.Add(-3*time.Hour), map[string]usage.Window{
		usage.FiveHour: win(25, 62*time.Minute), usage.SevenDay: win(18, 117*time.Hour)})
	r.record("b", listNow.Add(-time.Minute), map[string]usage.Window{
		usage.FiveHour: win(62, 167*time.Minute), usage.SevenDay: win(18, 117*time.Hour)})
	want := []string{
		"Profiles:",
		"  1  a         a@example.com  · 3h ago",
		"     5h    ████░░░░░░░░░░░░   25%  resets 1h 2m",
		"     week  ███░░░░░░░░░░░░░   18%  resets 4d 21h",
		"● 2  b         b@example.com",
		"     5h    ██████████░░░░░░   62%  resets 2h 47m",
		"     week  ███░░░░░░░░░░░░░   18%  resets 4d 21h",
		"  3  client-x  x@client.com   login required",
	}
	for _, args := range [][]string{nil, {"list"}} {
		got := r.listLines(args...)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("args %v:\n%s\nwant:\n%s", args, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

func TestListDropsWindowsThatAlreadyReset(t *testing.T) {
	r := usageRig(t)
	r.record("b", listNow.Add(-time.Minute), map[string]usage.Window{
		usage.FiveHour: win(80, -time.Minute), usage.SevenDay: win(40, 30*time.Hour)})
	out := r.stdoutAfter("list")
	if strings.Contains(out, "5h ") || !strings.Contains(out, "week") {
		t.Errorf("output:\n%s", out)
	}
}

func TestListWithoutUsageShowsHintOnlyOnTheActiveRow(t *testing.T) {
	r := usageRig(t)
	got := r.listLines("list")
	hint := "     " + r.msg(i18n.UsageHint)
	if n := strings.Count(strings.Join(got, "\n"), r.msg(i18n.UsageHint)); n != 1 {
		t.Fatalf("hint shown %d times:\n%s", n, strings.Join(got, "\n"))
	}
	if got[3] != hint || !strings.HasPrefix(got[2], "● 2  b") {
		t.Errorf("hint is not right under the active row:\n%s", strings.Join(got, "\n"))
	}
}

func TestListHintIsSkippedWhenTheWrapperIsInstalled(t *testing.T) {
	r := usageRig(t)
	r.deps.Statusline = StatuslineDeps{Executable: "/opt/cid/cid"}
	r.writeSettings(prevSettings)
	if code := r.run("statusline", "install"); code != 0 {
		t.Fatal(r.stderr.String())
	}
	if out := r.stdoutAfter("list"); strings.Contains(out, "statusline install") {
		t.Errorf("hint shown although the wrapper is installed:\n%s", out)
	}
}

func TestListHintIsSkippedOnceSomethingWasRecorded(t *testing.T) {
	r := usageRig(t)
	r.record("b", listNow.Add(-time.Hour), map[string]usage.Window{usage.FiveHour: win(10, -time.Hour)}) // all passed
	out := r.stdoutAfter("list")
	if strings.Contains(out, "statusline install") {
		t.Errorf("hint shown although a reading exists:\n%s", out)
	}
	if !strings.Contains(out, "1h ago") {
		t.Errorf("age note missing for a stale reading:\n%s", out)
	}
}

func TestListInSpanish(t *testing.T) {
	r := usageRig(t)
	r.deps.Lang = i18n.ES
	r.record("a", listNow.Add(-3*time.Hour), map[string]usage.Window{usage.SevenDay: win(18, 117*time.Hour)})
	out := r.stdoutAfter("list")
	for _, want := range []string{"hace 3 h", "semana", "se reinicia en 4d 21h", i18n.T(i18n.ES, i18n.StatusNeedsLogin)} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestListColors(t *testing.T) {
	r := usageRig(t)
	r.record("a", listNow.Add(-3*time.Hour), map[string]usage.Window{usage.FiveHour: win(25, time.Hour), usage.SevenDay: win(75, 24*time.Hour)})
	r.record("b", listNow.Add(-time.Minute), map[string]usage.Window{usage.FiveHour: win(95, time.Hour)})
	plain := r.stdoutAfter("list")
	if ansi.MatchString(plain) {
		t.Fatalf("plain output has ANSI: %q", plain)
	}
	r.deps.Color = true
	colored := r.stdoutAfter("list")
	for _, want := range []string{"\x1b[32m", "\x1b[33m", "\x1b[31m", "\x1b[2m"} { // ok, warn, high, dim
		if !strings.Contains(colored, want) {
			t.Errorf("missing %q in %q", want, colored)
		}
	}
	if got := ansi.ReplaceAllString(colored, ""); got != plain {
		t.Errorf("colors change the text:\n%s\nvs\n%s", got, plain)
	}
}

func TestColorEnabled(t *testing.T) {
	tests := []struct {
		name string
		tty  bool
		env  map[string]string
		goos string
		want bool
	}{
		{"tty", true, nil, "linux", true},
		{"not a tty", false, nil, "linux", false},
		{"NO_COLOR set", true, map[string]string{"NO_COLOR": "1"}, "linux", false},
		{"NO_COLOR empty is ignored", true, map[string]string{"NO_COLOR": ""}, "linux", true},
		{"dumb terminal", true, map[string]string{"TERM": "dumb"}, "linux", false},
		{"windows console without VT hints", true, nil, "windows", false},
		{"windows terminal", true, map[string]string{"WT_SESSION": "x"}, "windows", true},
		{"windows with TERM", true, map[string]string{"TERM": "xterm-256color"}, "windows", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			get := func(k string) string { return tc.env[k] }
			if got := colorEnabled(tc.tty, get, tc.goos); got != tc.want {
				t.Errorf("colorEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUseColorIsFalseForANonFileWriter(t *testing.T) {
	if UseColor(&strings.Builder{}) {
		t.Error("a buffer is not a terminal")
	}
}

func TestListJSONIncludesUsage(t *testing.T) {
	r := usageRig(t)
	r.record("b", listNow.Add(-90*time.Second), map[string]usage.Window{
		usage.FiveHour: win(62, 167*time.Minute), usage.SevenDay: win(18, -time.Hour)})
	var got []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.stdoutAfter("list", "--json")), &got); err != nil {
		t.Fatalf("%v: %s", err, r.stdout)
	}
	if len(got) != 3 {
		t.Fatalf("got %d profiles", len(got))
	}
	for _, i := range []int{0, 2} {
		if _, ok := got[i]["usage"]; ok {
			t.Errorf("profile %d without a reading must omit usage", i)
		}
	}
	for _, k := range []string{"index", "name", "email", "active", "status"} {
		if _, ok := got[1][k]; !ok {
			t.Errorf("existing field %q missing", k)
		}
	}
	var u struct {
		FetchedAt  string `json:"fetchedAt"`
		AgeSeconds int    `json:"ageSeconds"`
		Windows    []struct {
			Kind        string  `json:"kind"`
			PercentUsed float64 `json:"percentUsed"`
			ResetsAt    string  `json:"resetsAt"`
		} `json:"windows"`
	}
	if err := json.Unmarshal(got[1]["usage"], &u); err != nil {
		t.Fatal(err)
	}
	if u.FetchedAt != "2026-03-04T09:58:30Z" || u.AgeSeconds != 90 {
		t.Errorf("fetchedAt %q, ageSeconds %d", u.FetchedAt, u.AgeSeconds)
	}
	if len(u.Windows) != 1 || u.Windows[0].Kind != "five_hour" || u.Windows[0].PercentUsed != 62 || u.Windows[0].ResetsAt != "2026-03-04T12:47:00Z" {
		t.Errorf("windows = %+v (the window past its reset must be left out)", u.Windows)
	}
}

func TestHookListIsPlainTextEvenWhenColorIsOn(t *testing.T) {
	r := usageRig(t)
	r.deps.Color = true // the real terminal is a TTY, the hook's capture is not
	r.record("b", listNow.Add(-time.Minute), map[string]usage.Window{usage.FiveHour: win(95, time.Hour)})
	got := r.block(expansion("account", "list"))
	if ansi.MatchString(got.Reason) || !strings.Contains(got.Reason, "95%") {
		t.Errorf("hook reason = %q", got.Reason)
	}
}

func TestHookSwitchesByNumberWithoutAModelTurn(t *testing.T) {
	r := usageRig(t)
	got := r.block(expansion("account", "1"))
	if want := r.msg(i18n.SwitchedFromTo, "b", "a"); got.Decision != "block" || !strings.Contains(got.Reason, want) {
		t.Errorf("decision %q, reason %q, want a block containing %q", got.Decision, got.Reason, want)
	}
}
