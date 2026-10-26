package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
)

const goodFile = `{"profiles":[{"name":"work","index":1,"added_at":"2026-01-01T00:00:00Z"},{"name":"home","index":2,"added_at":"2026-01-02T00:00:00Z"}],"active":"work"}`

type notices []fsutil.Notice

func recoveryStore(t *testing.T, content string) (*Store, string, *notices) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cid", "profiles.json")
	writeFile(t, path, content)
	var got notices
	s := NewStore(path).WithNotifier(func(n fsutil.Notice) { got = append(got, n) })
	return s, path, &got
}

func corruptCopies(t *testing.T, path string) []string {
	t.Helper()
	m, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLoadRecoversFromBrokenFiles(t *testing.T) {
	cases := map[string]string{
		"truncated json":    `{"profiles":[{"name":"work","index":1`,
		"not json":          `hello`,
		"empty file":        ``,
		"wrong root type":   `[]`,
		"profiles type":     `{"profiles":"x"}`,
		"name type":         `{"profiles":[{"name":5,"index":1}]}`,
		"index type":        `{"profiles":[{"name":"a","index":"1"}]}`,
		"index below one":   `{"profiles":[{"name":"a","index":0}]}`,
		"invalid name":      `{"profiles":[{"name":"Bad Name!","index":1}]}`,
		"missing name":      `{"profiles":[{"index":1}]}`,
		"duplicate names":   `{"profiles":[{"name":"a","index":1},{"name":"A","index":2}]}`,
		"duplicate index":   `{"profiles":[{"name":"a","index":1},{"name":"b","index":1}]}`,
		"active wrong type": `{"profiles":[],"active":3}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			s, path, got := recoveryStore(t, content)
			ps, err := s.List()
			if err != nil {
				t.Fatalf("List failed: %v", err)
			}
			if len(ps) != 0 {
				t.Errorf("expected empty store, got %v", ps)
			}
			if len(*got) != 1 || (*got)[0].Kind != fsutil.KindReset {
				t.Fatalf("notices = %+v", *got)
			}
			if c := corruptCopies(t, path); len(c) != 1 {
				t.Errorf("corrupt copies = %v", c)
			}
			if _, err := s.Add("fresh"); err != nil { // the store keeps working
				t.Errorf("Add after recovery: %v", err)
			}
		})
	}
}

func TestLoadRestoresValidBackup(t *testing.T) {
	s, path, got := recoveryStore(t, `{"profiles":[`)
	writeFile(t, fsutil.BackupPath(path), goodFile)
	ps, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || ps[0].Name != "work" {
		t.Fatalf("profiles = %v", ps)
	}
	if len(*got) != 1 || (*got)[0].Kind != fsutil.KindRestored {
		t.Fatalf("notices = %+v", *got)
	}
	if len(corruptCopies(t, path)) != 1 {
		t.Error("broken copy not kept")
	}
	if _, err := s.List(); err != nil || len(*got) != 1 {
		t.Errorf("second load should be clean: err=%v notices=%d", err, len(*got))
	}
}

func TestLoadInvalidBackupStartsEmpty(t *testing.T) {
	s, path, got := recoveryStore(t, `{"profiles":[`)
	writeFile(t, fsutil.BackupPath(path), `{"profiles":[{"name":"BAD","index":1}]}`)
	ps, err := s.List()
	if err != nil || len(ps) != 0 {
		t.Fatalf("ps=%v err=%v", ps, err)
	}
	if len(*got) != 1 || (*got)[0].Kind != fsutil.KindReset {
		t.Fatalf("notices = %+v", *got)
	}
}

func TestDanglingActiveIsCleared(t *testing.T) {
	s, path, got := recoveryStore(t, `{"profiles":[{"name":"work","index":1,"added_at":"2026-01-01T00:00:00Z"}],"active":"ghost"}`)
	ps, err := s.List()
	if err != nil || len(ps) != 1 {
		t.Fatalf("ps=%v err=%v", ps, err)
	}
	if len(*got) != 1 || (*got)[0].Kind != fsutil.KindActiveCleared || (*got)[0].Detail != "ghost" {
		t.Fatalf("notices = %+v", *got)
	}
	if _, err := s.Active(); err != ErrNoActive {
		t.Errorf("Active err = %v, want ErrNoActive", err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "ghost") {
		t.Error("dangling marker still on disk")
	}
	if len(corruptCopies(t, path)) != 0 {
		t.Error("a dangling marker must not move the file aside")
	}
}

func TestValidFileLoadsWithoutNotices(t *testing.T) {
	s, _, got := recoveryStore(t, goodFile)
	ps, err := s.List()
	if err != nil || len(ps) != 2 || len(*got) != 0 {
		t.Fatalf("ps=%v err=%v notices=%v", ps, err, *got)
	}
	// null profiles, as written by older versions for an empty store.
	s2, _, got2 := recoveryStore(t, `{"profiles":null}`)
	if ps, err := s2.List(); err != nil || len(ps) != 0 || len(*got2) != 0 {
		t.Fatalf("null profiles: ps=%v err=%v notices=%v", ps, err, *got2)
	}
}

func TestSaveKeepsBackupOfLastGoodState(t *testing.T) {
	s, path := newTestStore(t)
	if _, err := s.Add("one"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add("two"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "garbage typed by hand")
	ps, err := s.List()
	if err != nil || len(ps) != 2 {
		t.Fatalf("restore from .bak failed: ps=%v err=%v", ps, err)
	}
}
