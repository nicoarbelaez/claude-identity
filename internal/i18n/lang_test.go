package i18n

import (
	"errors"
	"path/filepath"
	"testing"
)

type fakeEnv struct {
	vars   map[string]string
	files  map[string]string
	home   string
	locale string
}

func (f fakeEnv) env() Env {
	return Env{
		Getenv: func(k string) string { return f.vars[k] },
		ReadFile: func(p string) ([]byte, error) {
			if s, ok := f.files[filepath.ToSlash(p)]; ok {
				return []byte(s), nil
			}
			return nil, errors.New("not found")
		},
		UserHomeDir: func() (string, error) { return f.home, nil },
		OSLocale:    func() string { return f.locale },
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		env  fakeEnv
		want Lang
	}{
		{"default is english", fakeEnv{home: "/h"}, EN},
		{"CID_LANG es", fakeEnv{vars: map[string]string{"CID_LANG": "es"}}, ES},
		{"CID_LANG case-insensitive", fakeEnv{vars: map[string]string{"CID_LANG": "ES"}}, ES},
		{"CID_LANG beats claude settings", fakeEnv{
			vars:  map[string]string{"CID_LANG": "en"},
			files: map[string]string{"/h/.claude/settings.json": `{"language":"spanish"}`}, home: "/h"}, EN},
		{"CID_LANG invalid is ignored", fakeEnv{vars: map[string]string{"CID_LANG": "fr", "LANG": "es_CO.UTF-8"}}, ES},
		{"claude setting spanish", fakeEnv{home: "/h",
			files: map[string]string{"/h/.claude/settings.json": `{"language":"Spanish"}`}}, ES},
		{"claude setting español", fakeEnv{home: "/h",
			files: map[string]string{"/h/.claude/settings.json": `{"language":"español"}`}}, ES},
		{"claude setting es-CO", fakeEnv{home: "/h",
			files: map[string]string{"/h/.claude/settings.json": `{"language":"es-CO"}`}}, ES},
		{"claude setting english beats LANG es", fakeEnv{home: "/h",
			vars:  map[string]string{"LANG": "es_ES.UTF-8"},
			files: map[string]string{"/h/.claude/settings.json": `{"language":"english"}`}}, EN},
		{"claude setting unknown falls through", fakeEnv{home: "/h",
			vars:  map[string]string{"LANG": "es_ES.UTF-8"},
			files: map[string]string{"/h/.claude/settings.json": `{"language":"japanese"}`}}, ES},
		{"claude setting invalid json falls through", fakeEnv{home: "/h", locale: "es-MX",
			files: map[string]string{"/h/.claude/settings.json": `{oops`}}, ES},
		{"CLAUDE_CONFIG_DIR used", fakeEnv{home: "/h",
			vars:  map[string]string{"CLAUDE_CONFIG_DIR": "/cfg"},
			files: map[string]string{"/cfg/settings.json": `{"language":"spanish"}`, "/h/.claude/settings.json": `{"language":"english"}`}}, ES},
		{"settings.local.json overrides settings.json", fakeEnv{home: "/h",
			files: map[string]string{
				"/h/.claude/settings.json":       `{"language":"english"}`,
				"/h/.claude/settings.local.json": `{"language":"spanish"}`}}, ES},
		{"local unknown defers to settings.json", fakeEnv{home: "/h",
			files: map[string]string{
				"/h/.claude/settings.json":       `{"language":"spanish"}`,
				"/h/.claude/settings.local.json": `{"language":"klingon"}`}}, ES},
		{"LC_ALL beats LC_MESSAGES and LANG", fakeEnv{
			vars: map[string]string{"LC_ALL": "es_CO.UTF-8", "LC_MESSAGES": "en_US", "LANG": "en_US"}}, ES},
		{"LC_MESSAGES beats LANG", fakeEnv{
			vars: map[string]string{"LC_MESSAGES": "es_AR", "LANG": "en_US.UTF-8"}}, ES},
		{"LANG en_US beats OS locale", fakeEnv{vars: map[string]string{"LANG": "en_US.UTF-8"}, locale: "es-ES"}, EN},
		{"LANG C ignored, falls to OS", fakeEnv{vars: map[string]string{"LANG": "C"}, locale: "es-ES"}, ES},
		{"OS locale es-419", fakeEnv{locale: "es-419"}, ES},
		{"OS locale underscore", fakeEnv{locale: "es_ES"}, ES},
		{"OS locale other language", fakeEnv{locale: "fr-FR"}, EN},
		{"nil funcs are safe", fakeEnv{}, EN},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Detect(tt.env.env()); got != tt.want {
				t.Fatalf("Detect() = %q, want %q", got, tt.want)
			}
		})
	}
	t.Run("zero Env does not panic", func(t *testing.T) {
		if got := Detect(Env{}); got != EN {
			t.Fatalf("Detect(Env{}) = %q, want en", got)
		}
	})
}

func TestDarwinLocale(t *testing.T) {
	ok := func() ([]byte, error) { return []byte("es_CO\n"), nil }
	bad := func() ([]byte, error) { return nil, errors.New("no defaults") }
	if got := darwinLocale(ok); got != "es_CO" {
		t.Fatalf("got %q, want es_CO", got)
	}
	if got := darwinLocale(bad); got != "" {
		t.Fatalf("got %q, want empty on error", got)
	}
}
