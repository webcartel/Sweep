package fake_test

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

func TestFakeFS(t *testing.T) {
	fsys := fake.NewFS()
	fsys.Add(platform.Info{Path: `C:\Temp`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Temp\a.txt`, Size: 5})
	fsys.Add(platform.Info{Path: `C:\Temp\sub`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Temp\sub\b.txt`})

	t.Run("lstat reads a path without case", func(t *testing.T) {
		info, err := fsys.Lstat(`c:\temp\A.TXT`)
		if err != nil {
			t.Fatalf("Lstat: %v", err)
		}
		if info.Size != 5 || info.Name != `a.txt` || info.IsDir {
			t.Errorf("Lstat = %+v, want the 5 byte file a.txt", info)
		}
	})

	t.Run("lstat reports a missing path", func(t *testing.T) {
		if _, err := fsys.Lstat(`C:\Temp\nope`); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Lstat error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("readdir lists one directory, sorted", func(t *testing.T) {
		entries, err := fsys.ReadDir(`C:\Temp`)
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name)
		}
		if want := []string{`a.txt`, `sub`}; !reflect.DeepEqual(names, want) {
			t.Errorf("ReadDir = %q, want %q", names, want)
		}
	})

	t.Run("readdir refuses a file", func(t *testing.T) {
		if _, err := fsys.ReadDir(`C:\Temp\a.txt`); err == nil {
			t.Error("ReadDir accepted a file")
		}
	})

	t.Run("glob walks the file system in memory", func(t *testing.T) {
		paths, err := fsys.Glob(`C:\Temp\*\*.txt`)
		if err != nil {
			t.Fatalf("Glob: %v", err)
		}
		if want := []string{`C:\Temp\sub\b.txt`}; !reflect.DeepEqual(paths, want) {
			t.Errorf("Glob = %q, want %q", paths, want)
		}
	})

	t.Run("longpath answers what was mapped and the path itself otherwise", func(t *testing.T) {
		fsys.MapLong(`C:\PROGRA~1`, `C:\Program Files`)
		long, err := fsys.LongPath(`C:\PROGRA~1`)
		if err != nil {
			t.Fatalf("LongPath: %v", err)
		}
		if long != `C:\Program Files` {
			t.Errorf("LongPath = %q, want the mapped name", long)
		}
		long, err = fsys.LongPath(`C:\Temp`)
		if err != nil {
			t.Fatalf("LongPath: %v", err)
		}
		if long != `C:\Temp` {
			t.Errorf("LongPath = %q, want the path itself", long)
		}
	})

	t.Run("drivetype is fixed unless it is set", func(t *testing.T) {
		kind, err := fsys.DriveType(`C:\Temp\a.txt`)
		if err != nil {
			t.Fatalf("DriveType: %v", err)
		}
		if kind != platform.DriveFixed {
			t.Errorf("DriveType = %v, want DriveFixed", kind)
		}
		fsys.SetDrive(`D:\`, platform.DriveRemovable)
		kind, err = fsys.DriveType(`D:\stick\file`)
		if err != nil {
			t.Fatalf("DriveType: %v", err)
		}
		if kind != platform.DriveRemovable {
			t.Errorf("DriveType = %v, want DriveRemovable", kind)
		}
	})

	t.Run("remove deletes one entry", func(t *testing.T) {
		if err := fsys.Remove(`C:\Temp\a.txt`); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if _, err := fsys.Lstat(`C:\Temp\a.txt`); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Lstat after Remove = %v, want fs.ErrNotExist", err)
		}
		if want := []string{`C:\Temp\a.txt`}; !reflect.DeepEqual(fsys.Removed(), want) {
			t.Errorf("Removed = %q, want %q", fsys.Removed(), want)
		}
		if err := fsys.Remove(`C:\Temp\sub`); err == nil {
			t.Error("Remove deleted a directory that still holds something")
		}
		if err := fsys.Remove(`C:\Temp\nope`); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Remove error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("trash deletes the entry and remembers it", func(t *testing.T) {
		if err := fsys.Trash(`C:\Temp\sub\b.txt`); err != nil {
			t.Fatalf("Trash: %v", err)
		}
		if want := []string{`C:\Temp\sub\b.txt`}; !reflect.DeepEqual(fsys.Trashed(), want) {
			t.Errorf("Trashed = %q, want %q", fsys.Trashed(), want)
		}
		if _, err := fsys.Lstat(`C:\Temp\sub\b.txt`); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Lstat after Trash = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("remove on reboot schedules the path and keeps it", func(t *testing.T) {
		if err := fsys.RemoveOnReboot(`C:\Temp\sub\b.txt`); err != nil {
			t.Fatalf("RemoveOnReboot: %v", err)
		}
		if want := []string{`C:\Temp\sub\b.txt`}; !reflect.DeepEqual(fsys.Pending(), want) {
			t.Errorf("Pending = %q, want %q", fsys.Pending(), want)
		}
	})
}

func TestFakeProcs(t *testing.T) {
	procs := fake.NewProcs()
	procs.Set(`chrome.exe`, `Explorer.EXE`)

	got, err := procs.Running([]string{`firefox.exe`, `explorer.exe`, `chrome.exe`, `chrome.exe`})
	if err != nil {
		t.Fatalf("Running: %v", err)
	}
	if want := []string{`explorer.exe`, `chrome.exe`}; !reflect.DeepEqual(got, want) {
		t.Errorf("Running = %q, want %q", got, want)
	}
}

func TestFakeEnv(t *testing.T) {
	env := fake.NewEnv()
	env.Set(`TEMP`, `C:\Temp`)

	if value, ok := env.Get(`TEMP`); !ok || value != `C:\Temp` {
		t.Errorf("Get(TEMP) = %q, %v", value, ok)
	}
	if value, ok := env.Get(`temp`); !ok || value != `C:\Temp` {
		t.Errorf("Get(temp) = %q, %v, want the same variable", value, ok)
	}
	if _, ok := env.Get(`NOPE`); ok {
		t.Error("Get(NOPE) reported a variable that was never set")
	}
}
