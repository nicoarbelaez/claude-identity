package cli

import (
	"errors"
	"strings"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/settings"
)

// SettingsPort is the settings store as `cid config` uses it.
type SettingsPort interface {
	Resolve() ([]settings.Value, error)
	Get(key string) (settings.Value, error)
	Set(key, value string) (string, error)
	Unset(key string) error
}

// jsonSetting is the stable machine-readable shape of one setting.
type jsonSetting struct {
	Key         string   `json:"key"`
	Value       string   `json:"value"`
	Source      string   `json:"source"`
	Default     string   `json:"default"`
	Kind        string   `json:"kind"`
	Allowed     []string `json:"allowed"`
	Env         string   `json:"env"`
	Description string   `json:"description"`
}

type jsonConfig struct {
	Paths    map[string]string `json:"paths"`
	Settings []jsonSetting     `json:"settings"`
}

// config dispatches `cid config [get|set|unset] ...`.
func (a *app) config(args []string) int {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "get":
			return a.configGet(args[1:])
		case "set":
			return a.configSet(args[1:])
		case "unset":
			return a.configUnset(args[1:])
		}
		return a.usageError("")
	}
	return a.configShow(args)
}

func (a *app) configShow(args []string) int {
	asJSON, _, ok := parse("config", args, true, 0)
	if !ok {
		return a.usageError("")
	}
	var vals []settings.Value
	if a.Settings != nil {
		var err error
		if vals, err = a.Settings.Resolve(); err != nil {
			return a.fail(err, "")
		}
	}
	if asJSON {
		out := jsonConfig{
			Paths: map[string]string{
				"profiles":   a.Config.ProfilesPath,
				"config":     a.Config.ConfigPath,
				"claude_dir": a.Config.ClaudeDir,
			},
			Settings: make([]jsonSetting, 0, len(vals)),
		}
		for _, v := range vals {
			out.Settings = append(out.Settings, a.toJSONSetting(v))
		}
		return a.writeJSON(out)
	}
	backend := a.t(i18n.BackendFile)
	if a.Config.Keychain {
		backend = a.t(i18n.BackendKeychain)
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.ConfigProfilesPath, a.Config.ProfilesPath))
	if a.Config.ConfigPath != "" {
		a.printf(a.Stdout, "%s\n", a.t(i18n.ConfigFilePath, a.Config.ConfigPath))
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.ConfigClaudeDir, a.Config.ClaudeDir))
	a.printf(a.Stdout, "%s\n", a.t(i18n.ConfigBackend, backend))
	a.printf(a.Stdout, "%s\n", a.t(i18n.ConfigLanguage, string(a.Lang)))
	for _, v := range vals {
		a.printf(a.Stdout, "%s = %s (%s)\n", v.Key, v.Value, a.sourceLabel(v.Source))
	}
	return exitOK
}

func (a *app) configGet(args []string) int {
	flags, rest := extractFlags(args, "json")
	if len(rest) != 1 || a.Settings == nil {
		return a.usageError("")
	}
	v, err := a.Settings.Get(rest[0])
	if err != nil {
		return a.settingsFail(err, rest[0])
	}
	if flags["json"] {
		return a.writeJSON(a.toJSONSetting(v))
	}
	a.printf(a.Stdout, "%s\n", v.Value)
	return exitOK
}

func (a *app) configSet(args []string) int {
	if len(args) != 2 || a.Settings == nil {
		return a.usageError("")
	}
	val, err := a.Settings.Set(args[0], args[1])
	if err != nil {
		return a.settingsFail(err, args[0])
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.ConfigSetDone, args[0], val))
	return exitOK
}

func (a *app) configUnset(args []string) int {
	if len(args) != 1 || a.Settings == nil {
		return a.usageError("")
	}
	if err := a.Settings.Unset(args[0]); err != nil {
		return a.settingsFail(err, args[0])
	}
	v, err := a.Settings.Get(args[0])
	if err != nil {
		return a.fail(err, "")
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.ConfigUnsetDone, v.Key, v.Value))
	return exitOK
}

// settingsFail reports a registry error (unknown key, invalid value) as a
// one-line usage error with exit 2; any other error is a runtime failure.
func (a *app) settingsFail(err error, key string) int {
	var iv *settings.InvalidValueError
	switch {
	case errors.Is(err, settings.ErrUnknownKey):
		keys := make([]string, 0, len(settings.Defs()))
		for _, d := range settings.Defs() {
			keys = append(keys, d.Key)
		}
		a.printf(a.Stderr, "%s\n", a.t(i18n.ConfigUnknownKey, key, strings.Join(keys, ", ")))
		return exitUsage
	case errors.As(err, &iv):
		a.printf(a.Stderr, "%s\n", a.t(i18n.ConfigInvalidValue, iv.Value, iv.Key, strings.Join(iv.Allowed, ", ")))
		return exitUsage
	}
	return a.fail(err, "")
}

func (a *app) sourceLabel(s settings.Source) string {
	switch s {
	case settings.SourceEnv:
		return a.t(i18n.ConfigSourceEnv)
	case settings.SourceFile:
		return a.t(i18n.ConfigSourceFile)
	}
	return a.t(i18n.ConfigSourceDef)
}

func (a *app) toJSONSetting(v settings.Value) jsonSetting {
	s := jsonSetting{Key: v.Key, Value: v.Value, Source: string(v.Source), Allowed: []string{}}
	if d, err := settings.Lookup(v.Key); err == nil {
		s.Default, s.Kind, s.Env = d.Default, string(d.Kind), d.Env
		s.Description = a.t(d.Desc)
		if allowed := d.AllowedValues(); allowed != nil {
			s.Allowed = allowed
		}
	}
	return s
}
