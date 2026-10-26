//go:build darwin

package i18n

import "os/exec"

func osLocale() string {
	return darwinLocale(func() ([]byte, error) {
		return exec.Command("defaults", "read", "-g", "AppleLocale").Output()
	})
}
