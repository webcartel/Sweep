//go:build windows

package platform_test

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/webcartel/sweep/internal/platform"
)

func TestWindowsLstat(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write the file: %v", err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatalf("make the directory: %v", err)
	}
	link := filepath.Join(dir, "link")
	junction(t, link, sub)

	fsys := newFS(t)

	info, err := fsys.Lstat(file)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", file, err)
	}
	if info.Size != 5 || info.IsDir || info.Reparse {
		t.Errorf("Lstat(%s) = %+v, want the five byte file", file, info)
	}
	if info.Name != "a.txt" {
		t.Errorf("Lstat(%s).Name = %q, want a.txt", file, info.Name)
	}

	info, err = fsys.Lstat(sub)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", sub, err)
	}
	if !info.IsDir || info.Reparse {
		t.Errorf("Lstat(%s) = %+v, want a directory that is not a reparse point", sub, info)
	}

	info, err = fsys.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", link, err)
	}
	if !info.Reparse {
		t.Errorf("Lstat(%s).Reparse = false, want true: a junction is a reparse point even where the Go file mode does not say so", link)
	}
	if !strings.Contains(strings.ToLower(info.Link), "sub") {
		t.Errorf("Lstat(%s).Link = %q, want the directory the junction points at", link, info.Link)
	}
}

func TestWindowsLongPath(t *testing.T) {
	base := t.TempDir()
	deep := base
	for i := 0; i < 12; i++ {
		deep = filepath.Join(deep, "segment-directory")
	}
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatalf("make the deep directory: %v", err)
	}
	file := filepath.Join(deep, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write the deep file: %v", err)
	}
	if len(file) <= 260 {
		t.Fatalf("the path is %d characters long, want more than 260", len(file))
	}

	fsys := newFS(t)
	info, err := fsys.Lstat(file)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", file, err)
	}
	if info.Size != 1 {
		t.Errorf("Lstat(%s).Size = %d, want 1", file, info.Size)
	}
	entries, err := fsys.ReadDir(deep)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", deep, err)
	}
	if len(entries) != 1 || entries[0].Name != "file.txt" {
		t.Errorf("ReadDir(%s) = %+v, want the one file", deep, entries)
	}

	// A name with a trailing dot exists only through the \\?\ spelling, and
	// has to come back as written.
	odd := filepath.Join(base, "odd.")
	if err := os.WriteFile(`\\?\`+odd, []byte("x"), 0o600); err != nil {
		t.Fatalf("write the odd name: %v", err)
	}
	info, err = fsys.Lstat(`\\?\` + odd)
	if err != nil {
		t.Fatalf("Lstat(\\\\?\\%s): %v", odd, err)
	}
	if info.Name != "odd." {
		t.Errorf("Lstat(\\\\?\\%s).Name = %q, want odd.", odd, info.Name)
	}
}

func TestWindowsLongPath83(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a-long-name-for-eight-three.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write the file: %v", err)
	}

	short := shortPath(t, file)
	if strings.EqualFold(short, file) {
		t.Skip("this volume does not generate 8.3 names")
	}

	fsys := newFS(t)
	long, err := fsys.LongPath(short)
	if err != nil {
		t.Fatalf("LongPath(%s): %v", short, err)
	}
	if !strings.EqualFold(long, file) {
		t.Errorf("LongPath(%s) = %q, want %q", short, long, file)
	}
}

func TestWindowsLongPathKeepsALongName(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a-name-that-is-already-long.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write the file: %v", err)
	}

	fsys := newFS(t)
	long, err := fsys.LongPath(file)
	if err != nil {
		t.Fatalf("LongPath(%s): %v", file, err)
	}
	if !strings.EqualFold(long, file) {
		t.Errorf("LongPath(%s) = %q, want the path itself", file, long)
	}
}

func TestWindowsDriveType(t *testing.T) {
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		t.Skip("SystemDrive is not set")
	}

	fsys := newFS(t)
	kind, err := fsys.DriveType(drive + `\Temp`)
	if err != nil {
		t.Fatalf("DriveType: %v", err)
	}
	if kind != platform.DriveFixed {
		t.Errorf("DriveType(%s\\Temp) = %v, want DriveFixed", drive, kind)
	}

	kind, err = fsys.DriveType(`Q:\nothing`)
	if err != nil {
		t.Fatalf("DriveType: %v", err)
	}
	if kind == platform.DriveFixed {
		t.Errorf("DriveType(Q:\\) = %v, want anything but DriveFixed", kind)
	}
}

func TestWindowsRemove(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write the file: %v", err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatalf("make the directory: %v", err)
	}
	decoy := filepath.Join(dir, "decoy")
	if err := os.Mkdir(decoy, 0o700); err != nil {
		t.Fatalf("make the decoy: %v", err)
	}
	keep := filepath.Join(decoy, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o600); err != nil {
		t.Fatalf("write the file behind the junction: %v", err)
	}
	link := filepath.Join(dir, "link")
	junction(t, link, decoy)

	fsys := newFS(t)
	for _, path := range []string{file, sub, link} {
		if err := fsys.Remove(path); err != nil {
			t.Fatalf("Remove(%s): %v", path, err)
		}
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s is still there after Remove", path)
		}
	}
	if _, err := os.Lstat(keep); err != nil {
		t.Errorf("the file behind the junction went with the link: %v", err)
	}
}

func TestWindowsTrash(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "trash-me.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write the file: %v", err)
	}

	fsys := newFS(t)
	if err := fsys.Trash(file); err != nil {
		t.Fatalf("Trash(%s): %v", file, err)
	}
	if _, err := os.Lstat(file); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s is still there after Trash", file)
	}
	if err := fsys.Trash(file); err == nil {
		t.Error("Trash accepted a path that is not there")
	}
}

func newFS(t *testing.T) platform.FS {
	t.Helper()
	fsys, err := platform.NewFS()
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	return fsys
}

// junction makes one directory point at another. A junction needs no
// administrator, which is why the tests use it and not a symbolic link.
func junction(t *testing.T, link, target string) {
	t.Helper()
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Skipf("mklink /J is not available here: %v (%s)", err, out)
	}
}

// shortPath asks Windows for the 8.3 spelling of a path.
func shortPath(t *testing.T, path string) string {
	t.Helper()
	buffer := make([]uint16, 512)
	n, err := windows.GetShortPathName(windows.StringToUTF16Ptr(path), &buffer[0], uint32(len(buffer)))
	if err != nil {
		t.Skipf("GetShortPathName: %v", err)
	}
	return windows.UTF16ToString(buffer[:n])
}
