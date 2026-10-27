package usage

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Level classifies a usage percentage for coloring.
type Level int

// Usage levels: ok below 70, warn below 90, high from 90.
const (
	LevelOK Level = iota
	LevelWarn
	LevelHigh
)

// LevelOf returns the threshold level of pct.
func LevelOf(pct float64) Level {
	switch {
	case pct >= 90:
		return LevelHigh
	case pct >= 70:
		return LevelWarn
	}
	return LevelOK
}

// Bar renders pct (clamped to 0-100) as a bar of width cells.
func Bar(pct float64, width int) string {
	if width <= 0 {
		return ""
	}
	pct = math.Min(math.Max(pct, 0), 100)
	filled := int(math.Round(pct / 100 * float64(width)))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// Countdown renders the time left until resetsAt ("5d 3h", "2h 47m", "12m",
// "<1m"), or "" once it has passed.
func Countdown(resetsAt, now time.Time) string {
	d := resetsAt.Sub(now)
	if resetsAt.IsZero() || d <= 0 {
		return ""
	}
	days := int(d / (24 * time.Hour))
	hours := int(d/time.Hour) % 24
	mins := int(d/time.Minute) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dm", mins)
	}
	return "<1m"
}

// AgeUnit is the unit of an age returned by AgeOf.
type AgeUnit int

// Age units. AgeNone means the reading is too fresh (or too odd) to note.
const (
	AgeNone AgeUnit = iota
	AgeMinutes
	AgeHours
	AgeDays
)

// AgeOf splits how old a reading is into a whole count and a unit, so callers
// can word it in their own language. It reports AgeNone while the reading is
// under two minutes old or dated in the future.
func AgeOf(fetchedAt, now time.Time) (int, AgeUnit) {
	d := now.Sub(fetchedAt)
	switch {
	case fetchedAt.IsZero() || d < 2*time.Minute:
		return 0, AgeNone
	case d < time.Hour:
		return int(d / time.Minute), AgeMinutes
	case d < 24*time.Hour:
		return int(d / time.Hour), AgeHours
	}
	return int(d / (24 * time.Hour)), AgeDays
}

// Age renders how old a reading is ("6m ago", "3h ago", "2d ago"), or "" while
// it is under two minutes old (or dated in the future).
func Age(fetchedAt, now time.Time) string {
	n, unit := AgeOf(fetchedAt, now)
	switch unit {
	case AgeMinutes:
		return fmt.Sprintf("%dm ago", n)
	case AgeHours:
		return fmt.Sprintf("%dh ago", n)
	case AgeDays:
		return fmt.Sprintf("%dd ago", n)
	}
	return ""
}

// Visible returns the windows whose reset is still ahead of now; a window past
// its reset no longer describes the current quota. The input is not modified.
func Visible(windows map[string]Window, now time.Time) map[string]Window {
	out := make(map[string]Window, len(windows))
	for k, w := range windows {
		if w.ResetsAt.After(now) {
			out[k] = w
		}
	}
	return out
}
