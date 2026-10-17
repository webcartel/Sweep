//go:build windows

package platform_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/platform"
)

func TestWindowsRunningFindsSelf(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	name := strings.ToUpper(filepath.Base(exe))

	procs := newProcs(t)
	found, err := procs.Running([]string{name})
	if err != nil {
		t.Fatalf("Running: %v", err)
	}
	if len(found) != 1 || found[0] != name {
		t.Errorf("Running(%q) = %q, want the test binary itself", name, found)
	}
}

func TestWindowsRunningIgnoresMissing(t *testing.T) {
	procs := newProcs(t)

	found, err := procs.Running([]string{`no-such-process-6f2a.exe`})
	if err != nil {
		t.Fatalf("Running: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("Running = %q, want nothing for a name that is not running", found)
	}

	found, err = procs.Running(nil)
	if err != nil {
		t.Fatalf("Running(nil): %v", err)
	}
	if len(found) != 0 {
		t.Errorf("Running(nil) = %q, want nothing and no error", found)
	}
}

func TestWindowsRunningKeepsTheOrderOfTheNames(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	lower := strings.ToLower(filepath.Base(exe))

	procs := newProcs(t)
	found, err := procs.Running([]string{lower, `no-such-process-6f2a.exe`, strings.ToUpper(lower)})
	if err != nil {
		t.Fatalf("Running: %v", err)
	}
	if want := []string{lower}; len(found) != 1 || found[0] != want[0] {
		t.Errorf("Running = %q, want %q: the order of the names given, and no repeats", found, want)
	}
}

func newProcs(t *testing.T) platform.Procs {
	t.Helper()
	procs, err := platform.NewProcs()
	if err != nil {
		t.Fatalf("NewProcs: %v", err)
	}
	return procs
}
