package rules

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/model"
)

// validDir holds rule files taken from the spec's examples.
const validDir = "../../testdata/rules/valid"

func loadValid(t *testing.T, name string) *File {
	t.Helper()

	path := filepath.Join(validDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	file, err := LoadFile(path, data)
	if err != nil {
		t.Fatalf("LoadFile(%s): %v", path, err)
	}
	return file
}

func TestLoadFileReadsTheSpecExamples(t *testing.T) {
	file := loadValid(t, "chrome.yaml")

	if len(file.Rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(file.Rules))
	}
	got := file.Rules[0]
	if got.Node == nil {
		t.Fatal("the rule's YAML node is not kept, so no error can name its line")
	}
	if got.Node.Line != 3 {
		t.Errorf("node line = %d, want 3 (the `- id:` line)", got.Node.Line)
	}

	rule := got.Rule
	if rule.ID != "browsers.chrome.cache" {
		t.Errorf("ID = %q, want %q", rule.ID, "browsers.chrome.cache")
	}
	if rule.Name != "Chrome cache" {
		t.Errorf("Name = %q, want %q", rule.Name, "Chrome cache")
	}
	if rule.Category != "browsers" {
		t.Errorf("Category = %q, want %q", rule.Category, "browsers")
	}
	if rule.Risk != model.RiskSafe {
		t.Errorf("Risk = %q, want %q", rule.Risk, model.RiskSafe)
	}
	if !rule.DefaultEnabled {
		t.Error("DefaultEnabled = false, want true")
	}
	if want := []string{"chrome.exe"}; !slices.Equal(rule.RequiresClosed, want) {
		t.Errorf("RequiresClosed = %v, want %v", rule.RequiresClosed, want)
	}
	if rule.Source != filepath.Join(validDir, "chrome.yaml") {
		t.Errorf("Source = %q, want the file path", rule.Source)
	}

	if len(rule.Targets) != 2 {
		t.Fatalf("got %d targets, want 2", len(rule.Targets))
	}
	first := rule.Targets[0]
	if first.Kind != model.KindPath {
		t.Errorf("Kind = %q, want %q (the default)", first.Kind, model.KindPath)
	}
	if first.Mode != model.ModeContents {
		t.Errorf("Mode = %q, want %q", first.Mode, model.ModeContents)
	}
	if want := `%LOCALAPPDATA%\Google\Chrome\User Data\*\Cache`; first.Path != want {
		t.Errorf("Path = %q, want %q", first.Path, want)
	}
	if want := `%LOCALAPPDATA%\Google\Chrome\User Data\*\Code Cache`; rule.Targets[1].Path != want {
		t.Errorf("second target Path = %q, want %q", rule.Targets[1].Path, want)
	}
}

func TestLoadFileReadsEveryField(t *testing.T) {
	file := loadValid(t, "all-fields.yaml")
	if len(file.Rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(file.Rules))
	}
	rule := file.Rules[0].Rule

	if rule.Description != "Exercises every field of the rule format" {
		t.Errorf("Description = %q", rule.Description)
	}
	if want := []string{"dev", "extra"}; !slices.Equal(rule.Tags, want) {
		t.Errorf("Tags = %v, want %v", rule.Tags, want)
	}
	if want := []string{"foo.exe", "bar.exe"}; !slices.Equal(rule.RequiresClosed, want) {
		t.Errorf("RequiresClosed = %v, want %v", rule.RequiresClosed, want)
	}
	if !rule.RequiresAdmin {
		t.Error("RequiresAdmin = false, want true")
	}
	if rule.OSMinBuild != 17763 {
		t.Errorf("OSMinBuild = %d, want 17763", rule.OSMinBuild)
	}
	if !rule.Trash {
		t.Error("Trash = false, want true")
	}
	if rule.Risk != model.RiskModerate {
		t.Errorf("Risk = %q, want %q", rule.Risk, model.RiskModerate)
	}

	if len(rule.Targets) != 3 {
		t.Fatalf("got %d targets, want 3", len(rule.Targets))
	}
	command := rule.Targets[0]
	if command.Kind != model.KindCommand {
		t.Errorf("command target Kind = %q, want %q", command.Kind, model.KindCommand)
	}
	if command.Cmd != "powershell.exe" {
		t.Errorf("Cmd = %q, want %q", command.Cmd, "powershell.exe")
	}
	if want := []string{"-NoProfile", "-Command", "Clear-RecycleBin -Force"}; !slices.Equal(command.Args, want) {
		t.Errorf("Args = %v, want %v", command.Args, want)
	}

	filtered := rule.Targets[1]
	if filtered.Path != `%USERPROFILE%\Downloads` {
		t.Errorf("Path = %q", filtered.Path)
	}
	if filtered.Mode != model.ModeSelf {
		t.Errorf("Mode = %q, want %q", filtered.Mode, model.ModeSelf)
	}
	if want := []string{"*.exe", "*.msi"}; !slices.Equal(filtered.Include, want) {
		t.Errorf("Include = %v, want %v", filtered.Include, want)
	}
	if want := []string{"keep-*.exe"}; !slices.Equal(filtered.Exclude, want) {
		t.Errorf("Exclude = %v, want %v", filtered.Exclude, want)
	}
	if filtered.MinAgeDays != 30 {
		t.Errorf("MinAgeDays = %d, want 30", filtered.MinAgeDays)
	}
	if want := int64(5 * 1024 * 1024); filtered.MinSize != want {
		t.Errorf("MinSize = %d, want %d", filtered.MinSize, want)
	}

	plain := rule.Targets[2]
	if plain.Kind != model.KindPath || plain.Mode != model.ModeContents {
		t.Errorf("a target without kind and mode got %q/%q, want %q/%q",
			plain.Kind, plain.Mode, model.KindPath, model.ModeContents)
	}
	if plain.Path != `%TEMP%` {
		t.Errorf("Path = %q, want %q", plain.Path, `%TEMP%`)
	}
	if plain.MinSize != 0 || plain.MinAgeDays != 0 {
		t.Errorf("filters of a plain target = %d/%d bytes/days, want 0/0", plain.MinSize, plain.MinAgeDays)
	}
}

func TestLoadFileResolvesTrashDefault(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{name: "dangerous without trash", yaml: "rules:\n  - id: user.x\n    risk: dangerous\n", want: true},
		{name: "dangerous with trash false", yaml: "rules:\n  - id: user.x\n    risk: dangerous\n    trash: false\n", want: false},
		{name: "moderate without trash", yaml: "rules:\n  - id: user.x\n    risk: moderate\n", want: false},
		{name: "moderate with trash true", yaml: "rules:\n  - id: user.x\n    risk: moderate\n    trash: true\n", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := LoadFile("test.yaml", []byte(test.yaml))
			if err != nil {
				t.Fatalf("LoadFile: %v", err)
			}
			if len(file.Rules) != 1 {
				t.Fatalf("got %d rules, want 1", len(file.Rules))
			}
			if got := file.Rules[0].Rule.Trash; got != test.want {
				t.Errorf("Trash = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{in: "50MB", want: 50 * 1024 * 1024},
		{in: "1 KB", want: 1024},
		{in: "0", want: 0},
		{in: "2GB", want: 2 * 1024 * 1024 * 1024},
		{in: "150B", want: 150},
		{in: "1tb", want: 1024 * 1024 * 1024 * 1024},
		{in: "50XB", wantErr: true},
		{in: "", wantErr: true},
		{in: "-1KB", wantErr: true},
		{in: "99999999TB", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			got, err := parseSize(test.in)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseSize(%q) = %d, want an error", test.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSize(%q): %v", test.in, err)
			}
			if got != test.want {
				t.Errorf("parseSize(%q) = %d, want %d", test.in, got, test.want)
			}
		})
	}
}

func TestLoadFileReportsBadMinSizeWithPosition(t *testing.T) {
	const source = "schema: 1\nrules:\n  - id: user.x\n    targets:\n      - path: '%TEMP%'\n        min_size: 50XB\n"

	_, err := LoadFile("test.yaml", []byte(source))
	if err == nil {
		t.Fatal("LoadFile accepted a min_size it cannot read")
	}
	var finding Error
	if !errors.As(err, &finding) {
		t.Fatalf("error = %v, want an Error naming the place", err)
	}
	if finding.File != "test.yaml" || finding.Line != 6 || finding.Col != 19 {
		t.Errorf("error at %s:%d:%d, want test.yaml:6:19", finding.File, finding.Line, finding.Col)
	}
	if !strings.Contains(finding.Msg, "50XB") {
		t.Errorf("message = %q, want it to quote the value", finding.Msg)
	}
}
