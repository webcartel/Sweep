//go:build windows

package scan

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
)

// realTree builds the case against the disk the test runs on: a directory the
// test makes below the place Windows gives temporary files, deep and wide
// enough to be past the 260 characters a plain path stops at, and written with
// the names of more than one alphabet.
func realTree(t *testing.T) (string, int64) {
	t.Helper()
	base := t.TempDir()

	// The long path goes through the \\?\\ form: the plain Windows API stops
	// at the 260 characters of its own.
	deep := base
	for i := 0; i < 12; i++ {
		deep += `\Кэш-директория-с-длинным-именем-234567890`
	}
	if err := os.MkdirAll(`\\?\`+deep, 0o700); err != nil {
		t.Fatalf("mkdir the deep tree: %v", err)
	}
	var size int64
	for _, one := range []struct {
		name string
		size int
	}{
		{`файл-первый.dat`, 3},
		{`файл-второй-十.dat`, 5},
	} {
		file := `\\?\` + deep + `\` + one.name
		if err := os.WriteFile(file, []byte(strings.Repeat("x", one.size)), 0o600); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
		size += int64(one.size)
	}
	return deep, size
}

func realGuardFor(t *testing.T, base string) (*Scanner, platform.FS) {
	t.Helper()
	fsys, err := platform.NewFS()
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	env := platform.OSEnv{}
	g, err := guard.New(fsys, env, base, nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	return &Scanner{fs: fsys, env: env, guard: g}, fsys
}

func TestWalkOfTheRealDiskReachesTheLongAndTheUnicode(t *testing.T) {
	deep, _ := realTree(t)
	s, _ := realGuardFor(t, deep)

	plan, warnings, err := s.planFor(context.Background(), &model.Rule{ID: "test.deep",
		Targets: []model.Target{{Kind: model.KindPath, Path: deep, Mode: model.ModeContents}}}, nil)
	if err != nil {
		t.Fatalf("planFor: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none", warnings)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("Items = %d, want the two files of the tree (%q)", len(plan.Items), planItems(plan))
	}
	var total int64
	files := int64(0)
	for _, item := range plan.Items {
		total += item.Size
		files += int64(item.Files)
	}
	if total != 8 || files != 2 {
		t.Errorf("Sizes = %d bytes and %d files, want 8 and 2", total, files)
	}
}

// realJunction is `mklink /J` on the disk of the test, the way the guard's own
// junction cases make them.
func realJunction(t *testing.T, link, target string) {
	t.Helper()
	out, err := exec.Command(`cmd`, `/c`, `mklink`, `/J`, link, target).CombinedOutput()
	if err != nil {
		t.Fatalf("mklink /J %s -> %s: %v\n%s", link, target, err, out)
	}
}

func TestWalkOfTheRealDiskKeepsTheJunctionOnTheOutside(t *testing.T) {
	base := t.TempDir()
	hold := base + `\plain source`
	if err := os.MkdirAll(hold, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(hold+`\secret.txt`, []byte("x"), 0o600); err != nil {
		t.Fatalf("write the file the junction points to: %v", err)
	}
	if err := os.MkdirAll(base+`\tempproj\caches`, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	realJunction(t, base+`\tempproj\caches\plink`, hold)

	s, _ := realGuardFor(t, base)
	rule := &model.Rule{ID: "test.junction", Targets: []model.Target{
		{Kind: model.KindPath, Path: base + `\tempproj\caches`, Mode: model.ModeFiles}}}
	plan, warnings, err := s.planFor(context.Background(), rule, nil)
	if err != nil {
		t.Fatalf("planFor: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none", warnings)
	}
	if len(plan.Items) != 0 {
		t.Errorf("Items = %q, want nothing: the walk never enters the junction", planItems(plan))
	}

	// As the target itself the junction is the one item, and the disk below
	// the target it points at stays out of the plan.
	targetRule := &model.Rule{ID: "test.link", Targets: []model.Target{
		{Kind: model.KindPath, Path: base + `\tempproj\caches\plink`, Mode: model.ModeContents}}}
	plan, warnings, err = s.planFor(context.Background(), targetRule, nil)
	if err != nil {
		t.Fatalf("planFor the junction: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none", warnings)
	}
	if len(plan.Items) != 1 || !plan.Items[0].Reparse {
		t.Errorf("Items = %q, want the junction itself with reparse set", planItems(plan))
	}
}

func TestWalkSkipsSystemRoot(t *testing.T) {
	s, _ := realGuardFor(t, `C:\Windows\Temp`)
	plan, _, err := s.planFor(context.Background(), &model.Rule{ID: "test.root",
		Targets: []model.Target{{Kind: model.KindPath, Path: `%SystemRoot%`, Mode: model.ModeSelf}}}, nil)
	if err != nil {
		t.Fatalf("planFor: %v", err)
	}
	if len(plan.Items) != 0 {
		t.Errorf("Items = %q, want nothing of Windows itself", planItems(plan))
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Reason != model.ReasonProtected {
		t.Errorf("Skipped = %+v, want one skip with the reason protected", plan.Skipped)
	}
}
