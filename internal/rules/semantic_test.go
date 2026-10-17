package rules

import (
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/platform"
)

// fakeEnv stands in for the machine's environment.
type fakeEnv map[string]string

func (f fakeEnv) Get(name string) (string, bool) {
	value, ok := f[name]
	return value, ok
}

var _ platform.Env = fakeEnv(nil)

func TestValidateSemanticsReportsDuplicateIDWithBothLines(t *testing.T) {
	file := loadInvalid(t, "duplicate-id.yaml")

	findings, warnings := ValidateSemantics(file, fakeEnv{"TEMP": `C:\Temp`})
	if len(warnings) != 0 {
		t.Errorf("got warnings %v, want none", warnings)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
	}

	got := findings[0]
	if got.Line != 9 {
		t.Errorf("finding at line %d, want 9 (the second id)", got.Line)
	}
	if !strings.Contains(got.Msg, "user.probe") {
		t.Errorf("message = %q, want it to name the id", got.Msg)
	}
	if !strings.Contains(got.Msg, "line 3") {
		t.Errorf("message = %q, want it to name the first place, line 3", got.Msg)
	}
}

func TestValidateSemanticsWarnsAboutUnknownEnvironmentVariable(t *testing.T) {
	file := loadValid(t, "unknown-env.yaml")

	findings, warnings := ValidateSemantics(file, fakeEnv{"TEMP": `C:\Temp`})
	if len(findings) != 0 {
		t.Errorf("got findings %v, want none: an unset variable is a warning", findings)
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "SWEEP_NO_SUCH_VAR") {
		t.Errorf("warning = %q, want it to name the variable", warnings[0])
	}
	if !strings.Contains(warnings[0], ":8:15:") {
		t.Errorf("warning = %q, want it to name the place, 8:15", warnings[0])
	}

	if len(file.Rules) != 1 || len(file.Rules[0].Rule.Targets) != 1 {
		t.Fatal("the rule was dropped instead of kept with a warning")
	}
}

func TestValidateSemanticsRejectsAnEmptyNameMask(t *testing.T) {
	const source = "schema: 1\nrules:\n  - id: user.probe\n    name: Probe\n    category: user\n" +
		"    risk: safe\n    targets:\n      - path: '%TEMP%'\n        include: ['*.exe', '']\n"

	file, err := LoadFile("test.yaml", []byte(source))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	findings, warnings := ValidateSemantics(file, fakeEnv{"TEMP": `C:\Temp`})
	if len(warnings) != 0 {
		t.Errorf("got warnings %v, want none", warnings)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
	}
	if findings[0].Line != 9 {
		t.Errorf("finding at %d:%d, want it on line 9 (the include list)", findings[0].Line, findings[0].Col)
	}
	if !strings.Contains(findings[0].Msg, "the mask is empty") {
		t.Errorf("message = %q, want it to say why the mask cannot be used", findings[0].Msg)
	}
}

// The separator of a Windows path is a separator and not an escape, so a mask
// the spec itself writes — one that ends at a separator, with `**` in it —
// compiles, and a star beside a separator stays a mask.
func TestValidateSemanticsAcceptsTheMaskLanguageOfTheRules(t *testing.T) {
	const source = "schema: 1\nrules:\n  - id: user.probe\n    name: Probe\n    category: user\n" +
		"    risk: safe\n    targets:\n      - path: '%LOCALAPPDATA%\\App\\**\\'\n" +
		"        include: ['*.exe', 'file[0-9].log', 'sub\\?.log']\n        exclude: ['keep-*']\n" +
		"      - path: '%TEMP%\\*\\Cache'\n"

	file, err := LoadFile("test.yaml", []byte(source))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	findings, warnings := ValidateSemantics(file,
		fakeEnv{"LOCALAPPDATA": `C:\Users\probe\AppData\Local`, "TEMP": `C:\Temp`})
	if len(findings) != 0 || len(warnings) != 0 {
		t.Errorf("got findings %v and warnings %v, want none", findings, warnings)
	}
}

// A target that is not a mapping is the schema's finding; the semantic pass
// stays out of its way instead of saying what a path target needs on top of it.
func TestValidateSemanticsLeavesANonMappingTargetToTheSchema(t *testing.T) {
	file := loadInvalid(t, "target-not-an-object.yaml")

	findings, _ := ValidateSemantics(file, fakeEnv{"TEMP": `C:\Temp`})
	if len(findings) != 0 {
		t.Errorf("got %v, want no findings from the semantic pass", findings)
	}
}

func TestValidateSemanticsChecksKindAgainstPathAndCommand(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		message string
	}{
		{name: "command without cmd", target: "kind: command", message: "a command target takes cmd"},
		{
			name:    "command with path",
			target:  "kind: command\n        cmd: cmd.exe\n        path: '%TEMP%'",
			message: "a command target takes cmd, not path",
		},
		{name: "path without path", target: "mode: contents", message: "a path target takes path"},
		{
			name:    "path with cmd",
			target:  "path: '%TEMP%'\n        cmd: cmd.exe",
			message: "a path target takes path, not cmd",
		},
		{
			name:    "path with args",
			target:  "path: '%TEMP%'\n        args: ['-x']",
			message: "a path target takes path, not args",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := "schema: 1\nrules:\n  - id: user.probe\n    name: Probe\n    category: user\n" +
				"    risk: safe\n    targets:\n      - " + test.target + "\n"

			file, err := LoadFile("test.yaml", []byte(source))
			if err != nil {
				t.Fatalf("LoadFile: %v", err)
			}
			findings, _ := ValidateSemantics(file, fakeEnv{})
			if len(findings) != 1 {
				t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
			}
			if findings[0].Msg != test.message {
				t.Errorf("message = %q, want %q", findings[0].Msg, test.message)
			}
		})
	}
}

func TestValidateSemanticsAcceptsAFileWithoutIDs(t *testing.T) {
	const source = "schema: 1\nrules:\n  - name: A\n    targets:\n      - path: '%TEMP%'\n" +
		"  - name: B\n    targets:\n      - path: '%TEMP%'\n"

	file, err := LoadFile("test.yaml", []byte(source))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	findings, _ := ValidateSemantics(file, fakeEnv{})
	if len(findings) != 0 {
		t.Errorf("got %v, want no findings: two rules without an id do not repeat one", findings)
	}
}
