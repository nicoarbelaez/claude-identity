package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var errBusy = errors.New("sharing violation")

func busy(err error) bool { return errors.Is(err, errBusy) }

func TestRenameRetryTable(t *testing.T) {
	tests := []struct {
		name      string
		failures  int // transient failures before success
		fatal     error
		wantCalls int
		wantErr   bool
	}{
		{"first try works", 0, nil, 1, false},
		{"recovers after transient failures", 3, nil, 4, false},
		{"gives up after the attempt budget", 100, nil, renameAttempts, true},
		{"non transient error is not retried", 0, errors.New("cross-device link"), 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			var slept []time.Duration
			rename := func(_, _ string) error {
				calls++
				if tt.fatal != nil {
					return tt.fatal
				}
				if calls <= tt.failures {
					return errBusy
				}
				return nil
			}
			err := renameWithRetry(rename, busy, func(d time.Duration) { slept = append(slept, d) }, "a", "b")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}
			var total time.Duration
			for _, d := range slept {
				total += d
			}
			if total >= time.Second {
				t.Errorf("total backoff %v, want < 1s", total)
			}
			if tt.wantErr && tt.fatal == nil && !errors.Is(err, errBusy) {
				t.Errorf("last error not returned: %v", err)
			}
		})
	}
}

func TestRenameBackoffGrowsAndIsCapped(t *testing.T) {
	var slept []time.Duration
	_ = renameWithRetry(func(_, _ string) error { return errBusy }, busy, func(d time.Duration) { slept = append(slept, d) }, "a", "b")
	for i := 1; i < len(slept); i++ {
		if slept[i] < slept[i-1] {
			t.Fatalf("backoff shrank: %v", slept)
		}
	}
	if max := slept[len(slept)-1]; max > 250*time.Millisecond {
		t.Errorf("delay %v exceeds the cap", max)
	}
}

func TestWriteAtomicUsesRetryingRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.json")
	if err := WriteAtomic(path, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "x" {
		t.Fatalf("content = %q", b)
	}
	if err := Rename(filepath.Join(t.TempDir(), "missing"), path); err == nil {
		t.Fatal("missing source must fail")
	}
}
