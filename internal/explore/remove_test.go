package explore

import (
	"context"
	"io/fs"
	"testing"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// guarded builds the guard over the same fake machine the walk uses.
func guarded(t *testing.T, fsys platform.FS) *guard.Guard {
	t.Helper()
	env := fake.NewEnv()
	env.Set(`USERPROFILE`, `C:\Users\probe`)
	env.Set(`SystemRoot`, `C:\Windows`)
	env.Set(`SystemDrive`, `C:`)
	env.Set(`ProgramData`, `C:\ProgramData`)
	env.Set(`APPDATA`, `C:\Users\probe\AppData\Roaming`)
	env.Set(`LOCALAPPDATA`, `C:\Users\probe\AppData\Local`)
	g, err := guard.New(fsys, env, `C:\Users\probe`, nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	return g
}

// A path inside a ticked folder is not removed twice: the folder covers it.
func TestRemovalKeepsTheTopmost(t *testing.T) {
	fsys := tree()
	g := guarded(t, fsys)

	removals := Remove(context.Background(), fsys, []Tick{
		{Path: root + `\big\a.bin`, Size: 100},
		{Path: root + `\big`, Size: 150},
	}, RemoveOptions{Guard: g})

	if len(removals) != 1 {
		t.Fatalf("removed %d paths, want the one folder", len(removals))
	}
	if removals[0].Path != root+`\big` || removals[0].Action != "trashed" {
		t.Fatalf("removal = %+v", removals[0])
	}
	if _, err := fsys.Lstat(root + `\big`); err == nil {
		t.Fatal("the folder is still there")
	}
}

// A path the guard protects is refused, and the refusal is explained.
func TestRemovalRefusesAGuardedPath(t *testing.T) {
	fsys := tree()
	fsys.Add(platform.Info{Path: `C:\Windows`, IsDir: true})
	g := guarded(t, fsys)

	removals := Remove(context.Background(), fsys, []Tick{{Path: `C:\Windows`}}, RemoveOptions{Guard: g})

	if len(removals) != 1 || removals[0].Action != "refused" {
		t.Fatalf("removals = %+v, want a refusal", removals)
	}
	if removals[0].Note == "" {
		t.Error("the refusal says no reason")
	}
	if _, err := fsys.Lstat(`C:\Windows`); err != nil {
		t.Fatal("the guarded path was removed")
	}
}

// A path the system will not give up is a failure with its reason, and the
// rest of the list still goes.
func TestRemovalReportsAFailureAndCarriesOn(t *testing.T) {
	fsys := tree()
	fsys.DenyDel(root+`\top.bin`, fs.ErrPermission)
	g := guarded(t, fsys)

	var logged []string
	removals := Remove(context.Background(), fsys, []Tick{
		{Path: root + `\top.bin`, Size: 20},
		{Path: root + `\small`, Size: 10},
	}, RemoveOptions{Guard: g, Log: func(action, path, note string) {
		logged = append(logged, action)
	}})

	if len(removals) != 2 {
		t.Fatalf("removals = %+v, want two", removals)
	}
	byPath := map[string]Removal{}
	for _, one := range removals {
		byPath[one.Path] = one
	}
	if got := byPath[root+`\top.bin`]; got.Action != "failed" || got.Note == "" {
		t.Errorf("the busy file = %+v, want a failure with a note", got)
	}
	if got := byPath[root+`\small`]; got.Action != "trashed" {
		t.Errorf("the rest = %+v, want it still removed", got)
	}
	if len(logged) != 2 {
		t.Errorf("logged %v, want a line per path", logged)
	}
}

func TestTopmostKeepsSiblings(t *testing.T) {
	got := topmost([]Tick{
		{Path: `C:\a\one`},
		{Path: `C:\a\two`},
		{Path: `C:\a\one\deep`},
	})
	if len(got) != 2 {
		t.Fatalf("topmost = %+v, want the two siblings", got)
	}
}
