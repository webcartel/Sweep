package explore

import (
	"context"
	"io/fs"
	"testing"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

const root = `C:\scan`

// tree is one small machine:
//
//	C:\scan\big     a.bin 100, b.bin 50
//	C:\scan\small   c.bin 10
//	C:\scan\top.bin        20
func tree() *fake.FS {
	fsys := fake.NewFS()
	fsys.Add(platform.Info{Path: root, IsDir: true})
	fsys.Add(platform.Info{Path: root + `\big`, IsDir: true})
	fsys.Add(platform.Info{Path: root + `\big\a.bin`, Size: 100})
	fsys.Add(platform.Info{Path: root + `\big\b.bin`, Size: 50})
	fsys.Add(platform.Info{Path: root + `\small`, IsDir: true})
	fsys.Add(platform.Info{Path: root + `\small\c.bin`, Size: 10})
	fsys.Add(platform.Info{Path: root + `\top.bin`, Size: 20})
	return fsys
}

func scan(t *testing.T, fsys platform.FS, opt Options) Result {
	t.Helper()
	res, err := Scan(context.Background(), fsys, root, opt)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return res
}

// A folder's size is its whole subtree, and the children come largest first.
func TestScanMeasuresTheTree(t *testing.T) {
	res := scan(t, tree(), Options{Depth: 2, TopFiles: 5})

	if res.Root == nil {
		t.Fatal("Scan found no root")
	}
	if got, want := res.Root.Size, int64(180); got != want {
		t.Errorf("root size = %d, want %d", got, want)
	}
	if got, want := res.Root.Files, 4; got != want {
		t.Errorf("root files = %d, want %d", got, want)
	}

	names := []string{}
	for _, kid := range res.Root.Kids {
		names = append(names, kid.Name)
	}
	want := []string{"big", "top.bin", "small"}
	if len(names) != len(want) {
		t.Fatalf("kids = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("kids = %v, want %v", names, want)
		}
	}
	if got := res.Root.Kids[0].Size; got != 150 {
		t.Errorf("big = %d, want 150", got)
	}
}

// The screen keeps a few levels; the size still counts every level below.
func TestDepthLimitsTheTreeNotTheSize(t *testing.T) {
	res := scan(t, tree(), Options{Depth: 1})

	if len(res.Root.Kids) != 3 {
		t.Fatalf("root holds %d kids, want the three children", len(res.Root.Kids))
	}
	for _, kid := range res.Root.Kids {
		if kid.IsDir && len(kid.Kids) != 0 {
			t.Errorf("%s carries %d kids, want none at depth 1", kid.Name, len(kid.Kids))
		}
	}
	if got := res.Root.Kids[0].Size; got != 150 {
		t.Errorf("big = %d, want the whole subtree", got)
	}
}

// openDenied is a file system whose one folder can be named but not read, the
// way an ACL looks to a process that may not enter.
type openDenied struct {
	*fake.FS
	deny string
}

func (f openDenied) ReadDir(path string) ([]platform.Info, error) {
	if path == f.deny {
		return nil, &fs.PathError{Op: "readdir", Path: path, Err: fs.ErrPermission}
	}
	return f.FS.ReadDir(path)
}

// A folder the process may not read is counted as unread, not as a failure.
func TestUnreadableFolderIsCounted(t *testing.T) {
	fsys := openDenied{tree(), root + `\small`}

	res := scan(t, fsys, Options{Depth: 2})

	if got, want := res.Root.Denied, 1; got != want {
		t.Errorf("root denied = %d, want %d", got, want)
	}
	if got, want := res.Root.Files, 3; got != want {
		t.Errorf("root files = %d, want the three readable ones", got)
	}
	if got, want := res.Root.Size, int64(170); got != want {
		t.Errorf("root size = %d, want %d", got, want)
	}
	small := res.Root.Kids[len(res.Root.Kids)-1]
	if small.Name != "small" || small.Denied != 1 {
		t.Errorf("the unreadable folder is not marked: %+v", small)
	}
}

// A child that cannot even be named still counts as unread.
func TestUnstatableChildIsCounted(t *testing.T) {
	fsys := tree()
	fsys.Add(platform.Info{Path: root + `\ghost.bin`, Size: 1})
	fsys.Deny(root+`\ghost.bin`, fs.ErrPermission)

	res := scan(t, fsys, Options{Depth: 2})
	if got, want := res.Root.Denied, 1; got != want {
		t.Errorf("root denied = %d, want %d", got, want)
	}
}

// A junction is marked and not entered: a link back to the parent would
// otherwise double the count or never end.
func TestLinkIsNotFollowed(t *testing.T) {
	fsys := tree()
	fsys.Add(platform.Info{Path: root + `\loop`, IsDir: true, Reparse: true,
		Link: root})
	// A child behind the junction must not be counted.
	fsys.Add(platform.Info{Path: root + `\loop\big`, IsDir: true})
	fsys.Add(platform.Info{Path: root + `\loop\big\again.bin`, Size: 999})

	res := scan(t, fsys, Options{Depth: 2})

	if got, want := res.Root.Size, int64(180); got != want {
		t.Errorf("root size = %d, want %d: the junction was entered", got, want)
	}
	loop := res.Root.Kids[len(res.Root.Kids)-1]
	if loop.Name != "loop" || !loop.Link {
		t.Fatalf("the junction is not marked: %+v", loop)
	}
}

// The largest files are kept, not every file on the drive.
func TestTopFilesKeepsTheLargest(t *testing.T) {
	res := scan(t, tree(), Options{Depth: 1, TopFiles: 2})

	if len(res.Top) != 2 {
		t.Fatalf("top holds %d files, want 2", len(res.Top))
	}
	if res.Top[0].Name != "a.bin" || res.Top[0].Size != 100 {
		t.Errorf("top[0] = %+v, want a.bin at 100", res.Top[0])
	}
	if res.Top[1].Name != "b.bin" {
		t.Errorf("top[1] = %+v, want b.bin", res.Top[1])
	}
}

// A path the caller skips is left out entirely: the guard's list reaches the
// walk this way.
func TestSkipLeavesAPathOut(t *testing.T) {
	res := scan(t, tree(), Options{Depth: 2,
		Skip: func(path string) bool { return path == root+`\big` }})

	if got, want := res.Root.Size, int64(30); got != want {
		t.Errorf("root size = %d, want %d", got, want)
	}
}

// A cancelled walk stops and answers what it had, with the reason.
func TestCancelStopsTheWalk(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := Scan(ctx, tree(), root, Options{Depth: 2})
	if err == nil {
		t.Fatal("a cancelled walk answered no error")
	}
	if res.Root != nil {
		t.Errorf("a cancelled walk measured %+v", res.Root)
	}
}

// The progress hook is told where the walk got to.
func TestProgressIsReported(t *testing.T) {
	var last Progress
	scan(t, tree(), Options{Depth: 2, Progress: func(p Progress) { last = p }})

	if last.Files != 4 || last.Dirs == 0 || last.Bytes != 180 {
		t.Fatalf("progress = %+v, want 4 files, some dirs and 180 bytes", last)
	}
}

// A root that is not there is an error, not an empty tree.
func TestMissingRootIsAnError(t *testing.T) {
	_, err := Scan(context.Background(), tree(), root+`\nope`, Options{Depth: 1})
	if err == nil {
		t.Fatal("a missing root answered no error")
	}
}
