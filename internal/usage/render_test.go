package usage

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

func TestBar(t *testing.T) {
	tests := []struct {
		name  string
		pct   float64
		width int
		want  string
	}{
		{"empty", 0, 10, "░░░░░░░░░░"},
		{"half", 50, 10, "█████░░░░░"},
		{"rounds", 42, 10, "████░░░░░░"},
		{"full", 100, 10, "██████████"},
		{"over clamps", 130, 10, "██████████"},
		{"negative clamps", -5, 4, "░░░░"},
		{"zero width", 50, 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Bar(tc.pct, tc.width); got != tc.want {
				t.Errorf("Bar(%v, %d) = %q, want %q", tc.pct, tc.width, got, tc.want)
			}
		})
	}
}

func TestCountdown(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"hours and minutes", t0.Add(2*time.Hour + 47*time.Minute + 20*time.Second), "2h 47m"},
		{"days and hours", t0.Add(5*24*time.Hour + 3*time.Hour + 10*time.Minute), "5d 3h"},
		{"minutes only", t0.Add(12*time.Minute + 30*time.Second), "12m"},
		{"under a minute", t0.Add(30 * time.Second), "<1m"},
		{"exactly now", t0, ""},
		{"passed", t0.Add(-time.Minute), ""},
		{"zero time", time.Time{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Countdown(tc.at, t0); got != tc.want {
				t.Errorf("Countdown = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAge(t *testing.T) {
	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"fresh", 30 * time.Second, ""},
		{"just under two minutes", 119 * time.Second, ""},
		{"two minutes", 2 * time.Minute, "2m ago"},
		{"minutes", 6*time.Minute + 40*time.Second, "6m ago"},
		{"an hour", time.Hour, "1h ago"},
		{"hours", 3*time.Hour + 59*time.Minute, "3h ago"},
		{"days", 2*24*time.Hour + 5*time.Hour, "2d ago"},
		{"future clock skew", -time.Hour, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Age(t0.Add(-tc.ago), t0); got != tc.want {
				t.Errorf("Age = %q, want %q", got, tc.want)
			}
		})
	}
	if got := Age(time.Time{}, t0); got != "" {
		t.Errorf("Age(zero) = %q, want empty", got)
	}
}

func TestVisibleDropsPassedWindows(t *testing.T) {
	in := map[string]Window{
		FiveHour: {PercentUsed: 10, ResetsAt: t0.Add(-time.Second)},
		SevenDay: {PercentUsed: 20, ResetsAt: t0.Add(time.Hour)},
	}
	got := Visible(in, t0)
	if len(got) != 1 || got[SevenDay].PercentUsed != 20 {
		t.Fatalf("Visible = %+v", got)
	}
	if len(in) != 2 {
		t.Error("Visible must not mutate its input")
	}
	if got := Visible(nil, t0); len(got) != 0 {
		t.Errorf("Visible(nil) = %+v", got)
	}
}

func TestLevelOf(t *testing.T) {
	tests := []struct {
		pct  float64
		want Level
	}{
		{0, LevelOK}, {69.9, LevelOK}, {70, LevelWarn}, {89.9, LevelWarn}, {90, LevelHigh}, {100, LevelHigh}, {140, LevelHigh},
	}
	for _, tc := range tests {
		if got := LevelOf(tc.pct); got != tc.want {
			t.Errorf("LevelOf(%v) = %v, want %v", tc.pct, got, tc.want)
		}
	}
}
