package fake

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/webcartel/sweep/internal/platform"
)

// A fake registry keeps keys, values and the order of the calls, so the
// cleaner's promise — export before delete — can be checked.
func TestRegistryKeepsTheTreeAndTheCalls(t *testing.T) {
	reg := NewRegistry()
	run := `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	reg.AddValue(run, "MyApp", platform.Value{Data: `C:\Gone\app.exe`, Kind: platform.ValueString})
	reg.AddValue(run, "", platform.Value{Data: "default", Kind: platform.ValueString})
	reg.AddKey(`HKCU\Software\Microsoft\Windows\CurrentVersion\Run\Nested`)

	values, err := reg.Values(run)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(values) != 2 || values[0].Name != "" {
		t.Fatalf("values = %+v, want the default value first", values)
	}
	subs, err := reg.SubKeys(run)
	if err != nil || len(subs) != 1 || subs[0] != run+`\Nested` {
		t.Fatalf("subkeys = %v, %v", subs, err)
	}
	if err := reg.DeleteKey(run); err == nil {
		t.Fatal("DeleteKey removed a key with subkeys")
	}

	path := filepath.Join(t.TempDir(), "backup.reg")
	if err := reg.Export(run, path); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the export wrote no file: %v", err)
	}
	if err := reg.DeleteValue(run, "MyApp"); err != nil {
		t.Fatalf("DeleteValue: %v", err)
	}
	want := []string{"export " + run, "delete-value " + run + `\MyApp`}
	if len(reg.Operations) != len(want) || reg.Operations[0] != want[0] || reg.Operations[1] != want[1] {
		t.Fatalf("operations = %v, want %v", reg.Operations, want)
	}
}
