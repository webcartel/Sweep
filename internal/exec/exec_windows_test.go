//go:build windows

package exec

import (
	"fmt"
	"os"
	cmdexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/scan"
	"golang.org/x/sys/windows"
)

// systemOf hands the system of the machine the tests run on.
func systemOf(t *testing.T) platform.System {
	t.Helper()
	system, err := platform.NewSystem()
	if err != nil {
		t.Skip("the system of the machine is not named in this build")
	}
	return system
}

// execPath builds the executor of the machine the tests run on.
func execPath(t *testing.T) (*Executor, platform.FS) {
	t.Helper()
	fsys, err := platform.NewFS()
	if err != nil {
		t.Skip("this build cannot reach the disk of a Windows machine")
	}
	guarded, err := guard.New(fsys, platform.OSEnv{}, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	return New(Deps{FS: fsys, Guard: guarded}), fsys
}

// planOver scans the machine for one rule and hands the plan back: the way
// a frontend would have come to the executor.
func planOver(t *testing.T, fsys platform.FS, rule *model.Rule) *model.Plan {
	t.Helper()
	guarded, err := guard.New(fsys, platform.OSEnv{}, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	scanner := scan.New(scan.Deps{FS: fsys, Env: platform.OSEnv{}, System: systemOf(t), Guard: guarded})
	for event := range scanner.Scan(t.Context(), []*model.Rule{rule}, model.ScanOptions{}) {
		switch one := event.(type) {
		case model.ItemSkipped:
			if one.Skip.Reason == model.ReasonProtected {
				t.Skipf("the target %s cannot be asked about on this machine", one.Skip.Path)
			}
		case model.ScanDone:
			return one.Plan
		}
	}
	t.Fatal("the scan ended without a plan")
	return nil
}

// runOver executes the plan over the machine and keeps the events.
func runOver(t *testing.T, x *Executor, plan *model.Plan, opt model.ExecOptions) []model.Event {
	t.Helper()
	var seen []model.Event
	for event := range x.Execute(t.Context(), plan, model.Selection{}, opt) {
		seen = append(seen, event)
	}
	return seen
}

// ruleOf names one rule over the root of a temp tree.
func ruleOf(id, root string) *model.Rule {
	return &model.Rule{ID: id, Risk: model.RiskSafe,
		Targets: []model.Target{{Kind: model.KindPath, Path: root, Mode: model.ModeContents}}}
}

func writeTree(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("make %s: %v", root, err)
	}
	for name, size := range map[string]int{"a.txt": 10, "b.txt": 20} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func planItems(t *testing.T, plan *model.Plan, path string) []model.Item {
	t.Helper()
	for _, rp := range plan.Rules {
		if rp.Rule == nil || rp.Rule.ID != "probe" {
			continue
		}
		return rp.Items
	}
	t.Fatalf("the plan holds no rule probe over %s", path)
	return nil
}

func TestCleanOnTheRealDiskWorksAroundABusyFile(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, "other.txt")
	busy := filepath.Join(root, "busy.txt")
	writeTree(t, root)
	if err := os.WriteFile(busy, []byte("held"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The file is open for reading and writing but the share does not
	// say it may be deleted while the handle is open, which is what
	// Windows books under the lock of a sharing violation.
	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(busy),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Skipf("the machine does not hold the test busy: %v", err)
	}
	defer func() {
		if err := windows.CloseHandle(handle); err != nil {
			t.Errorf("close the held handle: %v", err)
		}
	}()

	x, fsys := execPath(t)
	plan := planOver(t, fsys, ruleOf("probe", root))
	saw := runOver(t, x, plan, model.ExecOptions{Workers: 1})

	var (
		skipped int
		removed int
	)
	for _, event := range saw {
		switch one := event.(type) {
		case model.ItemSkipped:
			skipped++
			if one.Skip.Path == busy && one.Skip.Reason != model.ReasonInUse {
				t.Errorf("the busy path was refused as %s, want in_use", one.Skip.Reason)
			}
		case model.ItemRemoved:
			removed++
			if one.Path == busy {
				t.Error("the busy path was removed anyway")
			}
		}
	}
	if removed == 0 {
		t.Error("nothing was removed, want the rest of the tree")
	}
	if _, err := os.Lstat(busy); err != nil {
		t.Errorf("the busy file is gone: %v", err)
	}
	if _, err := os.Lstat(other); !os.IsNotExist(err) {
		t.Errorf("the other file stayed: %v", err)
	}
}

func TestCleanOnTheRealDiskTakesAJunctionAloneAndNotItsTarget(t *testing.T) {
	root := t.TempDir()
	// The target lives outside the ruled tree: whatever the run does to
	// the tree, the target must stay what it was.
	target := filepath.Join(t.TempDir(), "real")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep.dat"), []byte("live"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	joined := cmdexec.Command("cmd", "/c", "mklink", "/J", link, target)
	if out, err := joined.CombinedOutput(); err != nil {
		t.Skipf("mklink /J could not make the junction: %v (%s)", err, out)
	}
	// A file of the tree itself, so that the rule holds something more
	// than the one link.
	if err := os.WriteFile(filepath.Join(root, "own.txt"), []byte("own"), 0o600); err != nil {
		t.Fatal(err)
	}

	x, fsys := execPath(t)
	plan := planOver(t, fsys, ruleOf("probe", root))
	items := planItems(t, plan, root)
	for _, item := range items {
		if item.Path == link && !item.Reparse {
			t.Error("the scan did not name the junction a reparse point")
		}
	}
	saw := runOver(t, x, plan, model.ExecOptions{Workers: 1})
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("the junction stayed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "keep.dat")); err != nil {
		t.Errorf("the junction's target followed the link: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "own.txt")); !os.IsNotExist(err) {
		t.Error("the own file of the tree stayed")
	}
	for _, event := range saw {
		if removed, ok := event.(model.ItemRemoved); ok && removed.Path == target {
			t.Error("the target was said as removed")
		}
	}
}

func TestCleanOnTheRealDiskTakesAlongWindowsAndOddNames(t *testing.T) {
	root := t.TempDir()
	// A path past the 260 marks of the old API.
	deep := root
	for strings.Count(deep, `\`) < 40 {
		deep = filepath.Join(deep, fmt.Sprintf("round%3d", strings.Count(deep, `\`)))
	}
	if len(deep) < 260 {
		t.Skip("the temp folder of this machine is too short to go past 260")
	}
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatalf("make the deep path: %v", err)
	}
	// A name that ends in a dot, which the Win32 names cannot even carry.
	odd := filepath.Join(root, `dot.`)
	if err := os.WriteFile(`\\?\`+odd, []byte("odd"), 0o600); err != nil {
		t.Fatalf("write the odd name: %v", err)
	}

	x, fsys := execPath(t)
	saw := runOver(t, x, planOver(t, fsys, ruleOf("probe", root)),
		model.ExecOptions{Workers: 1})
	if _, err := os.Lstat(deep); !os.IsNotExist(err) {
		t.Errorf("the deep tree stayed: %v", err)
	}
	if _, err := os.Lstat(`\\?\` + odd); !os.IsNotExist(err) {
		t.Errorf("the odd name stayed: %v", err)
	}
	for _, event := range saw {
		if failed, ok := event.(model.ItemFailed); ok {
			t.Errorf("a path failed: %v", failed.Err)
		}
	}
}

func TestCleanOnTheRealDiskLeavesTheTreeInADryRun(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root)
	seen := map[string]int64{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			seen[filepath.Base(path)] = info.Size()
		}
		return nil
	})

	x, fsys := execPath(t)
	runOver(t, x, planOver(t, fsys, ruleOf("probe", root)),
		model.ExecOptions{Workers: 1, DryRun: true})
	still := map[string]int64{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			still[filepath.Base(path)] = info.Size()
		}
		return nil
	})
	for name, size := range seen {
		if still[name] != size {
			t.Errorf("the dry run changed %s: %d bytes became %d", name, size, still[name])
		}
	}
}

func TestCleanOnTheRealDiskSendsTheBinWhereTheRuleAsks(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root)
	x, fsys := execPath(t)
	rule := ruleOf("probe", root)
	rule.Trash = true
	plan := planOver(t, fsys, rule)

	saw := runOver(t, x, plan, model.ExecOptions{Workers: 1})
	if _, err := os.Lstat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Error("the trashed file stayed on the disk")
	}
	var left model.Result
	for _, event := range saw {
		if done, ok := event.(model.ExecDone); ok {
			left = *done.Result
		}
	}
	if left.TrashBytes == 0 {
		t.Errorf("TrashBytes = %d, want the tree said into the bin", left.TrashBytes)
	}
}
