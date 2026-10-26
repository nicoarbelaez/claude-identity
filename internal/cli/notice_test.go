package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
)

func TestNoticeWriterPrintsOneLinePerNotice(t *testing.T) {
	cases := []struct {
		n    fsutil.Notice
		want []string
	}{
		{fsutil.Notice{Kind: fsutil.KindReset, Path: "/c/profiles.json", Moved: "/c/profiles.json.corrupt-1"}, []string{"/c/profiles.json", "/c/profiles.json.corrupt-1", "empty"}},
		{fsutil.Notice{Kind: fsutil.KindRestored, Path: "/c/profiles.json", Moved: "/c/profiles.json.corrupt-1"}, []string{"restored", "/c/profiles.json.corrupt-1"}},
		{fsutil.Notice{Kind: fsutil.KindActiveCleared, Detail: "ghost"}, []string{"ghost"}},
		{fsutil.Notice{Kind: fsutil.KindUnknownKey, Path: "/c/config.json", Detail: "theme"}, []string{"theme", "/c/config.json"}},
		{fsutil.Notice{Kind: fsutil.KindInvalidValue, Path: "/c/config.json", Detail: "language"}, []string{"language", "default"}},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		NoticeWriter(&buf, func() i18n.Lang { return i18n.EN })(c.n)
		out := buf.String()
		if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
			t.Errorf("kind %d: want exactly one line, got %q", c.n.Kind, out)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("kind %d: %q lacks %q", c.n.Kind, out, w)
			}
		}
	}
}

func TestNoticeWriterUsesCurrentLanguage(t *testing.T) {
	lang := i18n.EN
	var buf bytes.Buffer
	w := NoticeWriter(&buf, func() i18n.Lang { return lang })
	lang = i18n.ES
	w(fsutil.Notice{Kind: fsutil.KindUnknownKey, Path: "/c/config.json", Detail: "theme"})
	if !strings.Contains(buf.String(), "desconocido") {
		t.Errorf("not Spanish: %q", buf.String())
	}
}
