// Package pathenv edits the user PATH that cid added its install directory to.
// The registry code is Windows-only; other platforms get a no-op Editor.
package pathenv

import (
	"os"
	"regexp"
	"strings"
)

// Editor removes a directory from the persistent user PATH.
type Editor interface {
	// RemoveDir drops dir from the user PATH and reports whether an entry was
	// removed. It must leave every other entry and the value kind untouched.
	RemoveDir(dir string) (removed bool, err error)
}

var varRef = regexp.MustCompile(`%([^%]+)%`)

// expandVars replaces %VAR% references that are set; unknown ones stay as-is.
func expandVars(s string) string {
	return varRef.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := os.LookupEnv(m[1 : len(m)-1]); ok {
			return v
		}
		return m
	})
}

// normalize prepares a PATH entry for comparison only: it drops quotes and
// trailing separators and expands %VAR% references.
func normalize(entry string) string {
	e := strings.Trim(strings.TrimSpace(entry), `"`)
	return strings.TrimRight(expandVars(e), `\/`)
}

// removePathEntry removes every entry of the ';'-separated raw PATH that names
// dir (case-insensitive, trailing backslash ignored, %VAR% expanded for the
// comparison only). The remaining entries are returned verbatim.
func removePathEntry(raw, dir string) (string, bool) {
	if raw == "" {
		return raw, false
	}
	want := normalize(dir)
	parts := strings.Split(raw, ";")
	kept := make([]string, 0, len(parts))
	removed := false
	for _, p := range parts {
		if p != "" && strings.EqualFold(normalize(p), want) {
			removed = true
			continue
		}
		kept = append(kept, p)
	}
	if !removed {
		return raw, false
	}
	return strings.Join(kept, ";"), true
}
