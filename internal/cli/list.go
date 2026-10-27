package cli

import (
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/nicoarbelaez/claude-identity/internal/credentials"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
	"github.com/nicoarbelaez/claude-identity/internal/usage"
)

// Usage bar width in cells: the default when the terminal width is unknown,
// and the bounds it is fitted into on a terminal.
const (
	barWidth    = 32
	barWidthMin = 12
	barWidthMax = 40
)

// Glyphs of the list.
const (
	markActive   = "●"
	markInactive = "○"
	markProblem  = "✗"
)

// windowOrder is the order the usage windows are listed in.
var windowOrder = []string{usage.FiveHour, usage.SevenDay}

// ANSI styles. Text is padded before it is styled, so styling never changes
// the layout.
const (
	ansiReset  = "\x1b[0m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
)

// UseColor reports whether colored output suits w: w must be a terminal and
// NO_COLOR must be unset (see https://no-color.org).
func UseColor(w io.Writer) bool {
	f, ok := w.(*os.File)
	return colorEnabled(ok && term.IsTerminal(int(f.Fd())), os.Getenv, runtime.GOOS)
}

// TermWidth returns the width of w in columns when w is a terminal, else 0.
func TermWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return 0
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil || cols < 0 {
		return 0
	}
	return cols
}

// colorEnabled is UseColor with its inputs made explicit. On Windows, ANSI
// escapes only work in consoles that process them, which the common ones
// announce through WT_SESSION, TERM, ANSICON or ConEmuANSI.
func colorEnabled(isTTY bool, getenv func(string) string, goos string) bool {
	if !isTTY || getenv("NO_COLOR") != "" || getenv("TERM") == "dumb" {
		return false
	}
	if goos == "windows" {
		return getenv("WT_SESSION") != "" || getenv("TERM") != "" || getenv("ANSICON") != "" || getenv("ConEmuANSI") == "ON"
	}
	return true
}

// now returns the render clock.
func (a *app) now() time.Time {
	switch {
	case a.Now != nil:
		return a.Now()
	case a.Statusline.Now != nil:
		return a.Statusline.Now()
	}
	return time.Now()
}

// readings returns the stored usage by profile name. ok is false when the
// cache is disabled or unreadable, so callers neither show nor hint at usage.
func (a *app) readings() (map[string]usage.Entry, bool) {
	if a.Usage == nil {
		return nil, false
	}
	all, err := a.Usage.All()
	if err != nil {
		return nil, false
	}
	return all, true
}

func (a *app) list(args []string) int {
	asJSON, _, ok := parse("list", args, true, 0)
	if !ok {
		return a.usageError("")
	}
	ps, err := a.Profiles.List()
	if err != nil {
		return a.fail(err, "")
	}
	activeName, err := a.activeName()
	if err != nil {
		return a.fail(err, "")
	}
	readings, haveUsage := a.readings()
	now := a.now()
	if asJSON {
		out := make([]jsonProfile, 0, len(ps))
		for _, p := range ps {
			active := p.Name == activeName
			jp := toJSON(p, active, a.Swap.Status(p.Name, active))
			if e, ok := readings[p.Name]; ok {
				jp.Usage = toJSONUsage(e, now)
			}
			out = append(out, jp)
		}
		return a.writeJSON(out)
	}
	if len(ps) == 0 {
		a.printf(a.Stdout, "%s\n", a.t(i18n.ListEmpty))
		return exitOK
	}
	a.printf(a.Stdout, "%s\n\n", a.t(i18n.ProfileListHeader))
	hint := haveUsage && !a.wrapperInstalled()
	rows := a.buildRows(ps, activeName, readings, now)
	lay := layoutOf(rows)
	bw := a.barCells(rows, lay)
	for i, row := range rows {
		if i > 0 {
			a.printf(a.Stdout, "\n")
		}
		a.printRow(row, lay, bw)
		if row.active && hint {
			if _, recorded := readings[row.name]; !recorded {
				a.printf(a.Stdout, "%s%s\n", row.indent, a.paint(ansiDim, a.t(i18n.UsageHint)))
			}
		}
	}
	return exitOK
}

// jsonUsage is the machine-readable form of a profile's usage reading.
type jsonUsage struct {
	FetchedAt  time.Time    `json:"fetchedAt"`
	AgeSeconds int64        `json:"ageSeconds"`
	Windows    []jsonWindow `json:"windows"`
}

type jsonWindow struct {
	Kind        string    `json:"kind"`
	PercentUsed float64   `json:"percentUsed"`
	ResetsAt    time.Time `json:"resetsAt"`
}

// toJSONUsage keeps only the windows that have not reset yet.
func toJSONUsage(e usage.Entry, now time.Time) *jsonUsage {
	age := int64(now.Sub(e.FetchedAt) / time.Second)
	if age < 0 {
		age = 0
	}
	u := &jsonUsage{FetchedAt: e.FetchedAt.UTC(), AgeSeconds: age, Windows: []jsonWindow{}}
	visible := usage.Visible(e.Windows, now)
	for _, kind := range windowOrder {
		if w, ok := visible[kind]; ok {
			u.Windows = append(u.Windows, jsonWindow{Kind: kind, PercentUsed: w.PercentUsed, ResetsAt: w.ResetsAt.UTC()})
		}
	}
	return u
}

// wrapperInstalled reports whether Claude Code's status line is cid's wrapper.
// When settings.json cannot be located or read it answers true, so the hint
// that asks the user to install it is not shown on a guess.
func (a *app) wrapperInstalled() bool {
	if a.Config.ClaudeDir == "" {
		return true
	}
	f, err := loadSettings(a.settingsPath())
	if err != nil {
		return true
	}
	cur, has := f.obj.get("statusLine")
	return has && a.wrapperIn(cur)
}

// row is one profile of the list, ready to print.
type row struct {
	name, email string
	active      bool
	marker      string
	index       string
	notes       []note
	indent      string // prefix of the usage lines, aligned with the name column
	lines       []usageLine
}

// note is a short remark at the end of a profile row.
type note struct{ text, style string }

type usageLine struct {
	label  string
	pct    float64 // as recorded; the bar is drawn once its width is known
	shown  int
	resets string
}

func (a *app) buildRows(ps []profile.Profile, activeName string, readings map[string]usage.Entry, now time.Time) []row {
	idxW := 1
	for _, p := range ps {
		idxW = max(idxW, len(strconv.Itoa(p.Index)))
	}
	rows := make([]row, 0, len(ps))
	for _, p := range ps {
		active := p.Name == activeName
		r := row{
			name:   p.Name,
			email:  p.Email,
			active: active,
			marker: markInactive,
			index:  fmt.Sprintf("%*d", idxW, p.Index),
			indent: strings.Repeat(" ", 1+1+2+idxW+2),
		}
		if active {
			r.marker = markActive
		}
		if st := a.Swap.Status(p.Name, active); st != credentials.StatusOK {
			style := ansiYellow
			if st == credentials.StatusNeedsLogin {
				style = ansiRed
			}
			r.notes = append(r.notes, note{markProblem + " " + a.statusLabel(st), style})
		}
		if e, ok := readings[p.Name]; ok {
			if age := a.ageNote(e.FetchedAt, now); age != "" {
				r.notes = append(r.notes, note{age, ansiDim})
			}
			r.lines = a.usageLines(e, now)
		}
		if active {
			r.notes = append(r.notes, note{a.t(i18n.UsageActive), ansiDim})
		}
		rows = append(rows, r)
	}
	return rows
}

// ageNote words the reading's age ("3h ago"), or "" while it is fresh.
func (a *app) ageNote(fetchedAt, now time.Time) string {
	n, unit := usage.AgeOf(fetchedAt, now)
	switch unit {
	case usage.AgeMinutes:
		return a.t(i18n.UsageAgoMinutes, n)
	case usage.AgeHours:
		return a.t(i18n.UsageAgoHours, n)
	case usage.AgeDays:
		return a.t(i18n.UsageAgoDays, n)
	}
	return ""
}

func (a *app) usageLines(e usage.Entry, now time.Time) []usageLine {
	visible := usage.Visible(e.Windows, now)
	var lines []usageLine
	for _, kind := range windowOrder {
		w, ok := visible[kind]
		if !ok {
			continue
		}
		label := a.t(i18n.UsageLabelFiveHour)
		if kind == usage.SevenDay {
			label = a.t(i18n.UsageLabelWeek)
		}
		lines = append(lines, usageLine{
			label:  label,
			pct:    w.PercentUsed,
			shown:  int(math.Round(math.Min(math.Max(w.PercentUsed, 0), 999))),
			resets: a.t(i18n.UsageResets, usage.Countdown(w.ResetsAt, now)),
		})
	}
	return lines
}

// layout holds the column widths shared by every row.
type layout struct{ name, email, label, resets int }

func layoutOf(rows []row) layout {
	var l layout
	for _, r := range rows {
		l.name = max(l.name, utf8.RuneCountInString(r.name)+2)
		l.email = max(l.email, utf8.RuneCountInString(r.email)+2)
		for _, u := range r.lines {
			l.label = max(l.label, utf8.RuneCountInString(u.label))
			l.resets = max(l.resets, utf8.RuneCountInString(u.resets))
		}
	}
	return l
}

// usageFixed is the width of a usage line apart from its bar: the indent, the
// label column, the percent, the resets text and the three gaps of 3 spaces.
func usageFixed(indent int, l layout) int {
	const gaps, pct = 3 * 3, 4
	return indent + l.label + pct + l.resets + gaps
}

// barCells returns the bar width: the default when the terminal width is
// unknown, else as much of the terminal as the line leaves, kept within the
// bounds so a usage line never wraps.
func (a *app) barCells(rows []row, l layout) int {
	if a.Width <= 0 || len(rows) == 0 {
		return barWidth
	}
	free := a.Width - usageFixed(len(rows[0].indent), l) - 1
	return min(max(free, barWidthMin), barWidthMax)
}

// printRow prints one profile and its usage lines.
func (a *app) printRow(r row, l layout, bw int) {
	marker := r.marker
	if r.active {
		marker = a.paint(ansiGreen, marker)
	}
	line := fmt.Sprintf(" %s  %s  %s%s", marker, r.index, pad(r.name, l.name), pad(r.email, l.email))
	for i, n := range r.notes {
		if i > 0 {
			line += "  "
		}
		line += a.paint(n.style, n.text)
	}
	a.printf(a.Stdout, "%s\n", strings.TrimRight(line, " "))
	for _, u := range r.lines {
		style := levelStyle(usage.LevelOf(u.pct))
		a.printf(a.Stdout, "%s%s   %s   %s   %s\n", r.indent,
			a.paint(ansiDim, pad(u.label, l.label)), a.paint(style, usage.Bar(u.pct, bw)),
			a.paint(style, fmt.Sprintf("%3d%%", u.shown)), a.paint(ansiDim, u.resets))
	}
}

// pad right-pads s with spaces to width runes.
func pad(s string, width int) string {
	if n := utf8.RuneCountInString(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// paint wraps s in style when colors are on.
func (a *app) paint(style, s string) string {
	if !a.Color || s == "" {
		return s
	}
	return style + s + ansiReset
}

func levelStyle(l usage.Level) string {
	switch l {
	case usage.LevelHigh:
		return ansiRed
	case usage.LevelWarn:
		return ansiYellow
	}
	return ansiGreen
}
