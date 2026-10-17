//go:build windows

package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
)

// TestRealJunctionIsFollowedAndLeftAlone is the case a fake file system cannot
// write: a junction on the disk this test runs on. The guard follows it to its
// target, the walk does not go through it, and removing it leaves the target
// alone.
func TestRealJunctionIsFollowedAndLeftAlone(t *testing.T) {
	g, fsys := realGuard(t)
	dir := t.TempDir()

	decoy := filepath.Join(dir, "decoy")
	keep := filepath.Join(decoy, "keep.txt")
	if err := os.MkdirAll(decoy, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", decoy, err)
	}
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", keep, err)
	}

	link := filepath.Join(dir, "link")
	junction(t, link, decoy)

	// The guard follows the link itself and says where it leads. Info describes
	// the place the path ends up; Chain is what says the path itself was a link.
	target, err := g.Normalize(link)
	if err != nil {
		t.Fatalf("Normalize(%q): %v", link, err)
	}
	if !strings.EqualFold(target.Final, decoy) {
		t.Errorf("Final = %q, want %q", target.Final, decoy)
	}
	if len(target.Chain) != 1 || !strings.EqualFold(target.Chain[0], decoy) {
		t.Errorf("Chain = %q, want just %q", target.Chain, decoy)
	}
	if !target.Exists || target.Info.Reparse {
		t.Errorf("Info = %+v, want the plain directory the link leads to", target.Info)
	}

	// A junction that leads nowhere is described itself: the engine removes the
	// link, and there is no directory at the other end to read.
	absent := filepath.Join(dir, "absent")
	if err := os.Mkdir(absent, 0o755); err != nil {
		t.Fatalf("Mkdir(%q): %v", absent, err)
	}
	broken := filepath.Join(dir, "broken")
	junction(t, broken, absent)
	if err := os.Remove(absent); err != nil {
		t.Fatalf("Remove(%q): %v", absent, err)
	}
	gone, err := g.Normalize(broken)
	if err != nil {
		t.Fatalf("Normalize(%q): %v", broken, err)
	}
	if !gone.Info.Reparse {
		t.Errorf("Info.Reparse = false for a junction that leads nowhere")
	}
	if !gone.Exists {
		t.Errorf("Exists = false for a junction that is there")
	}

	// The point of the case is what the walk and the removal do, not where the
	// decoy sits, so a machine whose temp the guard refuses is no place for it.
	if verdict := check(t, g, decoy, model.ModeContents, nil, 0); !verdict.Allowed {
		t.Skipf("the guard does not allow this machine's temp directory: %s", verdict.Message)
	}
	if verdict := check(t, g, link, model.ModeContents, nil, 0); !verdict.Allowed {
		t.Errorf("Check(%q) = %+v, want allowed: it leads to a place the guard allows", link, verdict)
	}

	// The walk does not enter the junction: the decoy file is found once, at
	// its own path.
	found, err := platform.GlobPaths(fsys, filepath.Join(dir, `**`, `keep.txt`))
	if err != nil {
		t.Fatalf("GlobPaths: %v", err)
	}
	if len(found) != 1 || !strings.EqualFold(found[0], keep) {
		t.Errorf("GlobPaths = %q, want just %q", found, keep)
	}

	// Removing the link removes the link.
	if err := fsys.Remove(link); err != nil {
		t.Fatalf("Remove(%q): %v", link, err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("the junction is still there: %v", err)
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "keep" {
		t.Errorf("the decoy file is gone or changed: %q, %v", data, err)
	}
}

// TestRealJunctionToWindowsIsRefused reads the guard at the end of a real link:
// a rule that reaches Windows through a junction is refused all the same.
func TestRealJunctionToWindowsIsRefused(t *testing.T) {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		t.Skip("SystemRoot is not set")
	}
	system32 := filepath.Join(systemRoot, "System32")
	if _, err := os.Stat(system32); err != nil {
		t.Skipf("there is no %s: %v", system32, err)
	}

	g, _ := realGuard(t)
	dir := t.TempDir()

	link := filepath.Join(dir, "windows")
	junction(t, link, system32)

	verdict := check(t, g, link, model.ModeContents, nil, 0)
	if verdict.Allowed {
		t.Fatalf("Check(%q) = %+v, want a refusal", link, verdict)
	}
	if verdict.Reason != model.ReasonProtected {
		t.Errorf("Reason = %q, want %q", verdict.Reason, model.ReasonProtected)
	}
	if !strings.Contains(strings.ToLower(verdict.Message), "system32") {
		t.Errorf("Message = %q, want the range the link leads to", verdict.Message)
	}

	// A plain directory beside it is allowed, so the refusal is about where
	// the link leads rather than about the temp directory the test uses.
	plain := filepath.Join(dir, "plain")
	if err := os.Mkdir(plain, 0o755); err != nil {
		t.Fatalf("Mkdir(%q): %v", plain, err)
	}
	if verdict := check(t, g, plain, model.ModeContents, nil, 0); !verdict.Allowed {
		t.Errorf("Check(%q) = %+v, want allowed", plain, verdict)
	}
}

// realGuard is a guard over the file system and the environment of the machine
// the test runs on, with the working directory of the test process.
func realGuard(t *testing.T) (*Guard, platform.FS) {
	t.Helper()

	fsys, err := platform.NewFS()
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	g, err := New(fsys, platform.OSEnv{}, cwd, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g, fsys
}

// junction makes a junction at link that points at target. A machine that
// refuses to make one has nothing to say about links, so the test is skipped
// rather than failed.
func junction(t *testing.T, link, target string) {
	t.Helper()

	// The arguments go to cmd one by one: a single line would be quoted for
	// cmd by Go, which quotes for CreateProcess, and cmd reads the quotes its
	// own way.
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Skipf("mklink /J %q %q: %v: %s", link, target, err, out)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("the junction %q is not there after mklink: %v", link, err)
	}
}
