package pathenv

import "testing"

func TestRemovePathEntry(t *testing.T) {
	t.Setenv("CID_TEST_HOME", `C:\Users\me`)
	tests := []struct {
		name    string
		raw     string
		dir     string
		want    string
		removed bool
	}{
		{"middle entry", `C:\a;C:\cid;C:\b`, `C:\cid`, `C:\a;C:\b`, true},
		{"only entry", `C:\cid`, `C:\cid`, ``, true},
		{"first entry", `C:\cid;C:\a`, `C:\cid`, `C:\a`, true},
		{"last entry", `C:\a;C:\cid`, `C:\cid`, `C:\a`, true},
		{"missing entry", `C:\a;C:\b`, `C:\cid`, `C:\a;C:\b`, false},
		{"case insensitive", `C:\a;c:\CID`, `C:\cid`, `C:\a`, true},
		{"trailing backslash in entry", `C:\a;C:\cid\;C:\b`, `C:\cid`, `C:\a;C:\b`, true},
		{"trailing backslash in dir", `C:\a;C:\cid;C:\b`, `C:\cid\`, `C:\a;C:\b`, true},
		{"var entry matches by expansion", `%CID_TEST_HOME%\bin;C:\a`, `C:\Users\me\bin`, `C:\a`, true},
		{"other var entries stay unexpanded", `%CID_TEST_HOME%\x;C:\cid;%SystemRoot%\system32`, `C:\cid`, `%CID_TEST_HOME%\x;%SystemRoot%\system32`, true},
		{"unknown var is left alone", `%NOPE_UNSET%\bin;C:\a`, `C:\cid`, `%NOPE_UNSET%\bin;C:\a`, false},
		{"quoted entry", `"C:\cid";C:\a`, `C:\cid`, `C:\a`, true},
		{"duplicates all removed", `C:\cid;C:\a;C:\cid\`, `C:\cid`, `C:\a`, true},
		{"empty entries kept", `C:\a;;C:\cid`, `C:\cid`, `C:\a;`, true},
		{"empty raw", ``, `C:\cid`, ``, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, removed := removePathEntry(tc.raw, tc.dir)
			if got != tc.want || removed != tc.removed {
				t.Fatalf("removePathEntry(%q, %q) = (%q, %v), want (%q, %v)", tc.raw, tc.dir, got, removed, tc.want, tc.removed)
			}
		})
	}
}
