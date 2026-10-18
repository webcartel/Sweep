package scan

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// cacheTree builds the tree every walk test reads:
//
//	C:\Cache                            dir, 15 days old
//	  old.log                           10 bytes, 15 days old
//	  d.tmp                             5 bytes, freshly written
//	  a                                 dir, 15 days old
//	    b.log                           100 bytes, 15 days old
//	    c.tmp                           5 bytes, freshly written
//	  lnk                               a junction, untouched target C:\Outside
//	C:\Outside                          dir, holds secret.txt
func cacheTree() *fake.FS {
	fsys := fake.NewFS()
	old := time.Now().Add(-15 * 24 * time.Hour)
	fresh := time.Now()
	add := func(path string, info platform.Info) {
		info.Path = path
		if info.Name == "" {
			info.Name = lastElement(path)
		}
		if info.ModTime.IsZero() {
			info.ModTime = old
		}
		fsys.Add(info)
	}
	add(`C:\Cache`, platform.Info{IsDir: true})
	add(`C:\Cache\old.log`, platform.Info{Size: 10})
	add(`C:\Cache\d.tmp`, platform.Info{Size: 5, ModTime: fresh})
	add(`C:\Cache\a`, platform.Info{IsDir: true})
	add(`C:\Cache\a\b.log`, platform.Info{Size: 100})
	add(`C:\Cache\a\c.tmp`, platform.Info{Size: 5, ModTime: fresh})
	add(`C:\Cache\lnk`, platform.Info{IsDir: true, Reparse: true})
	add(`C:\Outside`, platform.Info{IsDir: true})
	add(`C:\Outside\secret.txt`, platform.Info{Size: 7})
	return fsys
}

// collectOne runs collect on one target and gathers what it found and skipped.
func collectOne(t *testing.T, s *Scanner, target model.Target) ([]model.Item, []model.Skip) {
	t.Helper()
	var items []model.Item
	var skips []model.Skip
	if err := s.collect(context.Background(), target,
		func(item model.Item) { items = append(items, item) },
		func(reason model.Skip) { skips = append(skips, reason) }); err != nil {
		t.Errorf("collect returned %v", err)
	}
	return items, skips
}

func targetFor(path string, mode model.Mode) model.Target {
	return model.Target{Kind: model.KindPath, Path: path, Mode: mode}
}

func TestWalkSelf(t *testing.T) {
	s := &Scanner{fs: cacheTree(), env: fake.NewEnv()}
	items, skips := collectOne(t, s, targetFor(`C:\Cache`, model.ModeSelf))

	if len(items) != 1 {
		t.Fatalf("got %d items, want the one top level entry", len(items))
	}
	item := items[0]
	if item.Path != `C:\Cache` || !item.IsDir || item.Reparse {
		t.Errorf("item = %+v, want the C:\\Cache directory itself", item)
	}
	if item.Size != 120 {
		t.Errorf("Size = %d, want the sum of every file, 120", item.Size)
	}
	if item.Files != 4 {
		t.Errorf("Files = %d, want each file counted once, 4", item.Files)
	}
	if len(skips) != 0 {
		t.Errorf("Skipped = %+v, want nothing", skips)
	}
}

func TestWalkContents(t *testing.T) {
	s := &Scanner{fs: cacheTree(), env: fake.NewEnv()}
	items, _ := collectOne(t, s, targetFor(`C:\Cache`, model.ModeContents))

	if want := []string{`C:\Cache\a`, `C:\Cache\d.tmp`, `C:\Cache\lnk`, `C:\Cache\old.log`}; !equalPaths(items, want) {
		t.Errorf("items = %+v, want one top level entry per child, %q", items, want)
	}
	for _, item := range items {
		switch item.Path {
		case `C:\Cache\a`:
			if item.Size != 105 || item.Files != 2 || !item.IsDir {
				t.Errorf("item a = %+v, want the directory with its two files", item)
			}
		case `C:\Cache\d.tmp`:
			if item.Size != 5 || item.Files != 1 || item.IsDir {
				t.Errorf("item d.tmp = %+v, want the one file", item)
			}
		case `C:\Cache\lnk`:
			if !item.Reparse || item.Files != 0 {
				t.Errorf("item lnk = %+v, want the junction itself, never walked into", item)
			}
		}
	}
}

func TestWalkFiles(t *testing.T) {
	tests := []struct {
		name   string
		target model.Target
		want   []string
	}{
		{name: "no filters takes every file",
			target: model.Target{Kind: model.KindPath, Path: `C:\Cache`, Mode: model.ModeFiles},
			want:   []string{`C:\Cache\a\b.log`, `C:\Cache\a\c.tmp`, `C:\Cache\d.tmp`, `C:\Cache\old.log`}},
		{name: "one segment mask misses the depth",
			target: model.Target{Kind: model.KindPath, Path: `C:\Cache`, Mode: model.ModeFiles,
				Include: []string{"*.log"}},
			want: []string{`C:\Cache\old.log`}},
		{name: "a double star stands where it is written",
			target: model.Target{Kind: model.KindPath, Path: `C:\Cache`, Mode: model.ModeFiles,
				Include: []string{`**\*.log`}},
			want: []string{`C:\Cache\a\b.log`}},
		{name: "both masks together",
			target: model.Target{Kind: model.KindPath, Path: `C:\Cache`, Mode: model.ModeFiles,
				Include: []string{"*.log", `**\*.log`}},
			want: []string{`C:\Cache\a\b.log`, `C:\Cache\old.log`}},
		{name: "exclude takes the match out",
			target: model.Target{Kind: model.KindPath, Path: `C:\Cache`, Mode: model.ModeFiles,
				Include: []string{"*.tmp", `**\*.tmp`}, Exclude: []string{"d.tmp"}},
			want: []string{`C:\Cache\a\c.tmp`}},
		{name: "min age drops a fresh file",
			target: model.Target{Kind: model.KindPath, Path: `C:\Cache`, Mode: model.ModeFiles,
				MinAgeDays: 1},
			want: []string{`C:\Cache\a\b.log`, `C:\Cache\old.log`}},
		{name: "min size drops what does not reach it",
			target: model.Target{Kind: model.KindPath, Path: `C:\Cache`, Mode: model.ModeFiles,
				MinSize: 500},
			want: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &Scanner{fs: cacheTree(), env: fake.NewEnv()}
			items, _ := collectOne(t, s, test.target)
			if !equalPaths(items, test.want) {
				t.Errorf("items = %q, want %q", paths(items), test.want)
			}
		})
	}
}

func TestWalkReparseCandidate(t *testing.T) {
	s := &Scanner{fs: cacheTree(), env: fake.NewEnv()}

	t.Run("contents on a junction is one item", func(t *testing.T) {
		items, _ := collectOne(t, s, targetFor(`C:\Cache\lnk`, model.ModeContents))
		if len(items) != 1 || items[0].Path != `C:\Cache\lnk` || !items[0].Reparse {
			t.Errorf("items = %+v, want the junction itself", items)
		}
		if len(items) == 1 && items[0].Size != 0 {
			t.Errorf("Size = %d, want the size of the link itself", items[0].Size)
		}
	})

	t.Run("files never reach the junction target", func(t *testing.T) {
		items, _ := collectOne(t, s, targetFor(`C:\Cache\lnk`, model.ModeFiles))
		if len(items) != 0 {
			t.Errorf("items = %q, want none: the junction is not walked into", paths(items))
		}
	})
}

func TestWalkGlobTarget(t *testing.T) {
	fsys := fake.NewFS()
	old := time.Now().Add(-24 * time.Hour)
	for _, info := range []platform.Info{
		{Path: `C:\Profiles`, Name: `Profiles`, IsDir: true, ModTime: old},
		{Path: `C:\Profiles\User Data`, Name: `User Data`, IsDir: true, ModTime: old},
		{Path: `C:\Profiles\User Data\P1`, Name: `P1`, IsDir: true, ModTime: old},
		{Path: `C:\Profiles\User Data\P1\Cache`, Name: `Cache`, IsDir: true, ModTime: old},
		{Path: `C:\Profiles\User Data\P2`, Name: `P2`, IsDir: true, ModTime: old},
		{Path: `C:\Profiles\User Data\P2\Cache`, Name: `Cache`, IsDir: true, ModTime: old},
	} {
		fsys.Add(info)
	}
	s := &Scanner{fs: fsys, env: fake.NewEnv()}

	items, skips := collectOne(t, s, targetFor(`C:\Profiles\User Data\*\Cache`, model.ModeSelf))
	if want := []string{`C:\Profiles\User Data\P1\Cache`, `C:\Profiles\User Data\P2\Cache`}; !equalPaths(items, want) {
		t.Errorf("items = %q, want both profile caches", paths(items))
	}
	if len(skips) != 0 {
		t.Errorf("Skipped = %+v, want nothing", skips)
	}
}

// paths lists the paths of items, for the assertions above.
func paths(items []model.Item) []string {
	got := make([]string, len(items))
	for i, item := range items {
		got[i] = item.Path
	}
	return got
}

func equalPaths(items []model.Item, want []string) bool {
	if len(items) == 0 {
		return len(want) == 0
	}
	return reflect.DeepEqual(paths(items), want)
}
