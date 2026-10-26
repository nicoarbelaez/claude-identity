package update

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIsDev(t *testing.T) {
	for v, want := range map[string]bool{
		"dev": true, "": true, "not-a-version": true,
		"0.5.0": false, "v0.5.0": false, "1.0.0-rc.1": false,
	} {
		if got := IsDev(v); got != want {
			t.Errorf("IsDev(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            bool
	}{
		{"0.5.0", "0.6.0", true},
		{"0.5.0", "v0.6.0", true},
		{"0.5.0", "0.5.0", false},
		{"0.6.0", "0.5.0", false},
		{"0.9.0", "0.10.0", true},
		{"dev", "0.6.0", false},
		{"0.5.0", "garbage", false},
	} {
		if got := Newer(tc.current, tc.latest); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

type call struct {
	name string
	args []string
}

type fakeRunner struct {
	calls []call
	// failAt is the 1-based call that fails; 0 means none.
	failAt int
	err    error
	out    string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, call{name, args})
	if f.failAt == len(f.calls) {
		return f.out, f.err
	}
	return "", nil
}

func TestPluginRefreshRunsMarketplaceThenPlugin(t *testing.T) {
	f := &fakeRunner{}
	if err := (PluginRefresher{Runner: f}).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []call{
		{"claude", []string{"plugin", "marketplace", "update", "nicoarbelaez-tools"}},
		{"claude", []string{"plugin", "update", "account-switch@nicoarbelaez-tools"}},
	}
	if len(f.calls) != len(want) {
		t.Fatalf("calls = %v", f.calls)
	}
	for i, c := range want {
		if f.calls[i].name != c.name || strings.Join(f.calls[i].args, " ") != strings.Join(c.args, " ") {
			t.Errorf("call %d = %v, want %v", i, f.calls[i], c)
		}
	}
}

func TestPluginRefreshStopsAtFirstFailure(t *testing.T) {
	f := &fakeRunner{failAt: 1, err: errors.New("exit status 1"), out: "boom\nsecond line"}
	err := (PluginRefresher{Runner: f}).Refresh(context.Background())
	if err == nil {
		t.Fatal("want an error")
	}
	if len(f.calls) != 1 {
		t.Errorf("calls = %d, want 1", len(f.calls))
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("error must be one line: %q", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should carry the trimmed output: %q", err)
	}
}

func TestPluginRefreshReportsMissingClaude(t *testing.T) {
	f := &fakeRunner{failAt: 1, err: ErrClaudeMissing}
	err := (PluginRefresher{Runner: f}).Refresh(context.Background())
	if !errors.Is(err, ErrClaudeMissing) {
		t.Errorf("err = %v, want ErrClaudeMissing", err)
	}
}

func TestOneLineTruncates(t *testing.T) {
	got := oneLine("  " + strings.Repeat("x", 500) + "\nmore")
	if strings.Contains(got, "\n") || len(got) > 210 {
		t.Errorf("oneLine = %q (%d)", got, len(got))
	}
}

func TestExecRunnerMissingCommand(t *testing.T) {
	_, err := ExecRunner{}.Run(context.Background(), "cid-no-such-command-xyz")
	if !errors.Is(err, ErrClaudeMissing) {
		t.Errorf("err = %v, want ErrClaudeMissing", err)
	}
}

func TestExecRunnerCapturesOutput(t *testing.T) {
	out, err := ExecRunner{}.Run(context.Background(), "go", "version")
	if err != nil || !strings.Contains(out, "go version") {
		t.Errorf("out %q, err %v", out, err)
	}
	out, err = ExecRunner{}.Run(context.Background(), "go", "definitely-not-a-subcommand")
	if err == nil || out == "" {
		t.Errorf("want failure with captured output, got out %q err %v", out, err)
	}
}

func TestNewGitHubUpdaterBuilds(t *testing.T) {
	u, err := NewGitHubUpdater()
	if err != nil || u == nil {
		t.Fatalf("NewGitHubUpdater() = %v, %v", u, err)
	}
}

func TestReleaseTagAddsVPrefix(t *testing.T) {
	// Release tags are "v<semver>" and go-selfupdate matches the tag name exactly.
	for in, want := range map[string]string{"0.5.0": "v0.5.0", "v0.5.0": "v0.5.0"} {
		if got := releaseTag(in); got != want {
			t.Errorf("releaseTag(%q) = %q, want %q", in, got, want)
		}
	}
}
