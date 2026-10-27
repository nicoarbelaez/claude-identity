package i18n

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

var verbRe = regexp.MustCompile(`%[-+# 0]*[0-9]*[a-zA-Z%]`)

func verbs(s string) []string { return verbRe.FindAllString(s, -1) }

func keysOf(m map[Key]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}

func TestCatalogParity(t *testing.T) {
	if len(catalogEN) == 0 {
		t.Fatal("english catalog is empty")
	}
	en, es := keysOf(catalogEN), keysOf(catalogES)
	if len(en) != len(es) {
		t.Fatalf("key count differs: en=%d es=%d", len(en), len(es))
	}
	for i := range en {
		if en[i] != es[i] {
			t.Fatalf("key set differs at %d: en=%q es=%q", i, en[i], es[i])
		}
	}
	for k, enMsg := range catalogEN {
		if enMsg == "" || catalogES[k] == "" {
			t.Errorf("key %q has an empty message", k)
		}
		if a, b := len(verbs(enMsg)), len(verbs(catalogES[k])); a != b {
			t.Errorf("key %q verb count differs: en=%d es=%d", k, a, b)
		}
	}
}

// numberedStep matches a line that starts a numbered step ("1. ", "2) ").
var numberedStep = regexp.MustCompile(`(?m)^\s*\d+[.)]\s`)

func TestNoMessageHasNumberedSteps(t *testing.T) {
	for lang, cat := range map[Lang]map[Key]string{EN: catalogEN, ES: catalogES} {
		for k, msg := range cat {
			if numberedStep.MatchString(msg) {
				t.Errorf("%s message %q is a numbered step list: %q", lang, k, msg)
			}
		}
	}
}

func TestUsageMentionsEveryCommand(t *testing.T) {
	for _, lang := range []Lang{EN, ES} {
		usage := T(lang, Usage)
		for _, cmd := range []string{"cid list", "cid switch", "cid add", "cid remove", "cid relogin", "cid current", "cid config", "cid config get", "cid config set", "cid config unset", "cid statusline", "cid statusline install", "cid statusline uninstall", "cid uninstall", "cid update", "cid version", "cid help", "--json", "--force", "--current", "--console", "--sso"} {
			if !strings.Contains(usage, cmd) {
				t.Errorf("%s usage does not mention %q", lang, cmd)
			}
		}
	}
}

func TestT(t *testing.T) {
	tests := []struct {
		name string
		lang Lang
		key  Key
		args []any
		want string
	}{
		{"english with args", EN, ProfileAdded, []any{"work"}, "Profile \"work\" added."},
		{"spanish with args", ES, ProfileAdded, []any{"work"}, "Perfil \"work\" agregado."},
		{"unknown lang falls back to english", Lang("fr"), ProfileAdded, []any{"work"}, "Profile \"work\" added."},
		{"missing key returns key", EN, Key("no.such.key"), nil, "no.such.key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := T(tt.lang, tt.key, tt.args...); got != tt.want {
				t.Fatalf("T() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTFallsBackToEnglishWhenSpanishMissing(t *testing.T) {
	const k Key = "test.only.en"
	catalogEN[k] = "only english"
	defer delete(catalogEN, k)
	if got := T(ES, k); got != "only english" {
		t.Fatalf("T(ES) = %q, want english fallback", got)
	}
}
