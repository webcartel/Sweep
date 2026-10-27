//go:build windows

package registry

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows/registry"

	"github.com/webcartel/sweep/internal/platform"
)

// TestCleanRealHive is the one test that touches the real registry. It runs
// only when the machine is asked to: it creates a test-named key under App
// Paths, exports it, deletes it, restores it from the .reg and removes it.
func TestCleanRealHive(t *testing.T) {
	if os.Getenv("SWEEP_REGISTRY_INTEGRATION") != "1" {
		t.Skip("set SWEEP_REGISTRY_INTEGRATION=1 to touch the real registry")
	}
	const key = `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\App Paths\sweep-test.exe`
	open, _, err := registry.CreateKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\App Paths\sweep-test.exe`, registry.ALL_ACCESS)
	if err != nil {
		t.Fatalf("create the test key: %v", err)
	}
	if err := open.SetStringValue("", `C:\Gone\sweep-test.exe`); err != nil {
		_ = open.Close()
		t.Fatalf("set the test value: %v", err)
	}
	_ = open.Close()

	back, err := platform.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	finding := Finding{
		Kind: KindAppPath, ID: "apppath|" + key + "|", Name: "sweep-test.exe",
		Key: key, Path: `C:\Gone\sweep-test.exe`, Reason: "program not found",
	}
	result, err := Clean(back, []Finding{finding}, Options{
		BackupRoot: filepath.Join(t.TempDir(), "backups"), Elevated: true,
	})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if result.Removed != 1 {
		t.Fatalf("result = %+v", result)
	}
	if exists, _ := back.KeyExists(key); exists {
		t.Fatal("the key survived the clean")
	}
	backup := filepath.Join(result.BackupDir, "backup-01.reg")
	if err := exec.Command("reg.exe", "import", backup).Run(); err != nil {
		t.Fatalf("restore the backup: %v", err)
	}
	if exists, _ := back.KeyExists(key); !exists {
		t.Fatal("the restore did not bring the key back")
	}
	if err := back.DeleteKey(key); err != nil {
		t.Fatalf("remove the test key: %v", err)
	}
}
