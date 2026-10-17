package platform_test

import (
	"reflect"
	"testing"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

func TestGlobPathsLiterals(t *testing.T) {
	fsys := fake.NewFS()
	fsys.Add(platform.Info{Path: `C:\Temp\Cache`, IsDir: true})

	paths, err := platform.GlobPaths(fsys, `C:\Temp\Cache`)
	if err != nil {
		t.Fatalf("GlobPaths: %v", err)
	}
	if want := []string{`C:\Temp\Cache`}; !reflect.DeepEqual(paths, want) {
		t.Errorf("GlobPaths = %q, want %q", paths, want)
	}

	paths, err = platform.GlobPaths(fsys, `C:\Temp\Absent`)
	if err != nil {
		t.Fatalf("GlobPaths: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("GlobPaths = %q, want nothing for a path that does not exist", paths)
	}
}

func TestGlobPathsSegments(t *testing.T) {
	fsys := fake.NewFS()
	fsys.Add(platform.Info{Path: `C:\Temp`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Temp\probe`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Temp\probe\b.txt`})
	fsys.Add(platform.Info{Path: `C:\Temp\probe\a.log`})
	fsys.Add(platform.Info{Path: `C:\Temp\probe\sub`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Temp\probe\sub\c.log`})

	tests := []struct {
		name string
		mask string
		want []string
	}{
		{
			name: "a star takes what one segment holds",
			mask: `C:\Temp\probe\*.log`,
			want: []string{`C:\Temp\probe\a.log`},
		},
		{
			name: "matching ignores case, as Windows does",
			mask: `c:\temp\PROBE\*.LOG`,
			want: []string{`C:\Temp\probe\a.log`},
		},
		{
			name: "a question takes one character",
			mask: `C:\Temp\probe\a?log`,
			want: []string{`C:\Temp\probe\a.log`},
		},
		{
			name: "a star takes directories too, and the result is sorted",
			mask: `C:\Temp\probe\*`,
			want: []string{`C:\Temp\probe\a.log`, `C:\Temp\probe\b.txt`, `C:\Temp\probe\sub`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths, err := platform.GlobPaths(fsys, test.mask)
			if err != nil {
				t.Fatalf("GlobPaths(%q): %v", test.mask, err)
			}
			if !reflect.DeepEqual(paths, test.want) {
				t.Errorf("GlobPaths(%q) = %q, want %q", test.mask, paths, test.want)
			}
		})
	}
}

func TestGlobPathsDoubleStar(t *testing.T) {
	fsys := fake.NewFS()
	for _, path := range []string{
		`C:\Temp\a`, `C:\Temp\a\b`, `C:\Temp\a\b2`,
		`C:\Temp\a\x`, `C:\Temp\a\x\b`, `C:\Temp\a\x\y`, `C:\Temp\a\x\y\b`,
	} {
		fsys.Add(platform.Info{Path: path, IsDir: true})
	}

	tests := []struct {
		name string
		mask string
		want []string
		// unique asks only that no path is reported twice: the mask below
		// reaches one path in more than one way, and the walk has to say so
		// once.
		unique bool
	}{
		{
			name: "two stars take one or more levels",
			mask: `C:\Temp\a\**\b`,
			want: []string{`C:\Temp\a\x\b`, `C:\Temp\a\x\y\b`},
		},
		{
			name: "two stars at the end take everything below",
			mask: `C:\Temp\a\**`,
			want: []string{
				`C:\Temp\a\b`, `C:\Temp\a\b2`, `C:\Temp\a\x`,
				`C:\Temp\a\x\b`, `C:\Temp\a\x\y`, `C:\Temp\a\x\y\b`,
			},
		},
		{
			name:   "a path reached by two ways is still one path",
			mask:   `C:\Temp\a\**\**`,
			unique: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths, err := platform.GlobPaths(fsys, test.mask)
			if err != nil {
				t.Fatalf("GlobPaths(%q): %v", test.mask, err)
			}
			if test.unique {
				if len(paths) == 0 {
					t.Fatalf("GlobPaths(%q) found nothing", test.mask)
				}
				seen := map[string]int{}
				for _, path := range paths {
					seen[path]++
				}
				for path, count := range seen {
					if count > 1 {
						t.Errorf("GlobPaths(%q) holds %q %d times", test.mask, path, count)
					}
				}
				return
			}
			if !reflect.DeepEqual(paths, test.want) {
				t.Errorf("GlobPaths(%q) = %q, want %q", test.mask, paths, test.want)
			}
		})
	}
}

func TestGlobPathsReparse(t *testing.T) {
	fsys := fake.NewFS()
	fsys.Add(platform.Info{Path: `C:\Temp`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Temp\own.txt`})
	fsys.Add(platform.Info{Path: `C:\Temp\sub`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Temp\sub\deep.txt`})
	fsys.Add(platform.Info{Path: `C:\Temp\link`, IsDir: true, Reparse: true, Link: `C:\Elsewhere`})
	fsys.Add(platform.Info{Path: `C:\Temp\link\decoy.txt`})

	paths, err := platform.GlobPaths(fsys, `C:\Temp\**\*.txt`)
	if err != nil {
		t.Fatalf("GlobPaths: %v", err)
	}
	if want := []string{`C:\Temp\sub\deep.txt`}; !reflect.DeepEqual(paths, want) {
		t.Errorf("GlobPaths = %q, want %q: the walk must not enter a reparse point", paths, want)
	}

	paths, err = platform.GlobPaths(fsys, `C:\Temp\*`)
	if err != nil {
		t.Fatalf("GlobPaths: %v", err)
	}
	want := []string{`C:\Temp\link`, `C:\Temp\own.txt`, `C:\Temp\sub`}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("GlobPaths = %q, want %q: a reparse point is a match of its own", paths, want)
	}

	// A mask that starts inside a junction has no way through it either.
	for _, mask := range []string{`C:\Temp\link\**\*.txt`, `C:\Temp\link\*`} {
		paths, err := platform.GlobPaths(fsys, mask)
		if err != nil {
			t.Fatalf("GlobPaths(%q): %v", mask, err)
		}
		if len(paths) != 0 {
			t.Errorf("GlobPaths(%q) = %q, want nothing: the walk starts inside a reparse point", mask, paths)
		}
	}

	// The link itself is still found when the mask names it.
	paths, err = platform.GlobPaths(fsys, `C:\Temp\link`)
	if err != nil {
		t.Fatalf("GlobPaths: %v", err)
	}
	if want := []string{`C:\Temp\link`}; !reflect.DeepEqual(paths, want) {
		t.Errorf("GlobPaths = %q, want %q", paths, want)
	}
}

func TestGlobPathsNoLiteralPrefix(t *testing.T) {
	fsys := fake.NewFS()
	fsys.Add(platform.Info{Path: `C:\Temp`, IsDir: true})

	if _, err := platform.GlobPaths(fsys, `**\x`); err == nil {
		t.Error("GlobPaths accepted a mask with no literal prefix")
	}

	paths, err := platform.GlobPaths(fsys, `C:\Absent\*\x`)
	if err != nil {
		t.Fatalf("GlobPaths: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("GlobPaths = %q, want nothing below a path that does not exist", paths)
	}
}
