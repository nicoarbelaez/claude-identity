package cli

import (
	"fmt"
	"io"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
)

// NoticeWriter returns a callback that prints each recovery notice as one
// localized line on w (stderr in production). lang is read on every call so
// the language can change after the settings were loaded.
func NoticeWriter(w io.Writer, lang func() i18n.Lang) func(fsutil.Notice) {
	return func(n fsutil.Notice) {
		l := lang()
		var msg string
		switch n.Kind {
		case fsutil.KindReset:
			msg = i18n.T(l, i18n.NoticeReset, n.Path, n.Moved)
		case fsutil.KindRestored:
			msg = i18n.T(l, i18n.NoticeRestored, n.Path, n.Moved)
		case fsutil.KindActiveCleared:
			msg = i18n.T(l, i18n.NoticeActiveCleared, n.Detail)
		case fsutil.KindUnknownKey:
			msg = i18n.T(l, i18n.NoticeUnknownKey, n.Path, n.Detail)
		case fsutil.KindInvalidValue:
			msg = i18n.T(l, i18n.NoticeInvalidValue, n.Path, n.Detail)
		default:
			return
		}
		_, _ = fmt.Fprintln(w, msg)
	}
}
