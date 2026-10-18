//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDeviceSpellingAndOddNames checks that the names whose form the Win32
// calls take apart — a trailing dot, a trailing space — are read and taken
// away through the device spelling of the path, and that the plain calls of
// the file system never see them come back.
func TestDeviceSpellingAndOddNames(t *testing.T) {
	dir := t.TempDir()
	odd := filepath.Join(dir, `dot.`)
	raw := `\\?\` + odd
	fsys, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(raw, []byte("odd"), 0o600); err != nil {
		t.Skipf("the machine robs the test of its odd name: %v", err)
	}
	// The plain spelling of the path sees nothing, which is the point:
	// only the device form of it reaches the entry.
	if _, err := fsys.Lstat(odd); err != nil {
		t.Fatalf("Lstat %q: %v", odd, err)
	}
	if err := fsys.Remove(odd); err != nil {
		t.Fatalf("Remove %q: %v", odd, err)
	}
	if _, err := fsys.Lstat(odd); err == nil {
		t.Error("the odd name stayed on the disk")
	}
}
