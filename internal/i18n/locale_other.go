//go:build !windows && !darwin

package i18n

// osLocale has no extra source on Linux and other Unix systems; LANG and the
// LC_* variables already cover them.
func osLocale() string { return "" }
