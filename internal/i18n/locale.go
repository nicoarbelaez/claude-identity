package i18n

import "strings"

// darwinLocale runs the injected `defaults read -g AppleLocale` runner and
// returns its trimmed output, or "" on any error. Kept untagged so it is
// testable on every platform.
func darwinLocale(run func() ([]byte, error)) string {
	out, err := run()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
