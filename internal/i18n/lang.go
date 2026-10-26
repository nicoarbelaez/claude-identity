// Package i18n provides English and Spanish messages and language detection.
package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Lang is a supported interface language.
type Lang string

// Supported languages.
const (
	EN Lang = "en"
	ES Lang = "es"
)

// Env abstracts everything Detect reads from the outside world.
type Env struct {
	// Getenv returns an environment variable (os.Getenv).
	Getenv func(string) string
	// ReadFile reads a file (os.ReadFile).
	ReadFile func(string) ([]byte, error)
	// UserHomeDir returns the home directory (os.UserHomeDir).
	UserHomeDir func() (string, error)
	// OSLocale returns the OS user locale, or "" when unknown.
	OSLocale func() string
}

// SystemEnv returns an Env backed by the real operating system.
func SystemEnv() Env {
	return Env{
		Getenv:      os.Getenv,
		ReadFile:    os.ReadFile,
		UserHomeDir: os.UserHomeDir,
		OSLocale:    osLocale,
	}
}

// Detect resolves the interface language. Precedence: CID_LANG, the Claude
// Code "language" setting, LC_ALL/LC_MESSAGES/LANG, the OS user locale, then
// English.
func Detect(env Env) Lang {
	getenv := func(k string) string {
		if env.Getenv == nil {
			return ""
		}
		return env.Getenv(k)
	}

	if l, ok := parseCode(getenv("CID_LANG")); ok {
		return l
	}
	if l, ok := claudeSettingsLang(env, getenv); ok {
		return l
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if l, ok := parseCode(getenv(k)); ok {
			return l
		}
	}
	if env.OSLocale != nil {
		if l, ok := parseCode(env.OSLocale()); ok {
			return l
		}
	}
	return EN
}

// parseCode maps a locale or language code such as "es", "ES", "es-CO" or
// "es_CO.UTF-8" to a supported language, looking only at the base language.
func parseCode(s string) (Lang, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, "-_.@"); i >= 0 {
		s = s[:i]
	}
	switch s {
	case "es":
		return ES, true
	case "en":
		return EN, true
	}
	return "", false
}

// parseFreeText maps the free-text Claude Code language setting to a language.
func parseFreeText(s string) (Lang, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "spanish", "español", "espanol", "castellano", "castilian":
		return ES, true
	case "english", "inglés", "ingles":
		return EN, true
	}
	return parseCode(s)
}

func claudeSettingsLang(env Env, getenv func(string) string) (Lang, bool) {
	if env.ReadFile == nil {
		return "", false
	}
	dir := getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		if env.UserHomeDir == nil {
			return "", false
		}
		home, err := env.UserHomeDir()
		if err != nil || home == "" {
			return "", false
		}
		dir = filepath.Join(home, ".claude")
	}
	// settings.local.json overrides settings.json.
	for _, name := range []string{"settings.local.json", "settings.json"} {
		data, err := env.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var s struct {
			Language string `json:"language"`
		}
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		if l, ok := parseFreeText(s.Language); ok {
			return l, true
		}
	}
	return "", false
}
