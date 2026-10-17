package guard

import (
	"testing"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform/fake"
	"github.com/webcartel/sweep/internal/rules"
)

// TestBuiltinRulesPassTheGuard is the promise the shipped rules make to the
// guard: every target they name is a place the engine may touch. A new rule
// that reaches into Windows or into a user's documents fails here, rather than
// quietly finding nothing at the end of a scan.
func TestBuiltinRulesPassTheGuard(t *testing.T) {
	files, err := rules.ReadBuiltin()
	if err != nil {
		t.Fatalf("ReadBuiltin: %v", err)
	}

	// guardEnv answers every variable a built-in rule in this repository
	// writes: USERPROFILE, APPDATA, LOCALAPPDATA, TEMP, SystemRoot, ProgramFiles,
	// ProgramData. A rule that names one more has to add it there too.
	g, err := New(fake.NewFS(), guardEnv(), `C:\Users\probe`, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	targets := 0
	for _, file := range files {
		for _, parsed := range file.Rules {
			rule := parsed.Rule
			if rule == nil {
				continue
			}
			for _, target := range rule.Targets {
				targets++
				verdict, err := g.Check(target.Path, target.Mode, target.Include, target.MinAgeDays)
				if err != nil {
					t.Errorf("%s: Check(%q): %v", rule.ID, target.Path, err)
					continue
				}
				if !verdict.Allowed {
					t.Errorf("%s: the built-in target %q is refused: %s", rule.ID, target.Path, verdict.Message)
				}
			}
		}
	}

	if targets == 0 {
		t.Fatal("no built-in target was checked, so this test proves nothing")
	}
}

// TestTheGuardStillRefuses keeps the case above from passing because the guard
// stopped refusing anything: the neighbours of the built-in targets are the
// places the specification names, and they stay refused.
func TestTheGuardStillRefuses(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		mode       model.Mode
		include    []string
		minAgeDays int
	}{
		{
			name: "the Windows tree outside the directories the rules name",
			path: `C:\Windows\System32\kernel32.dll`, mode: model.ModeContents,
		},
		{
			name: "a program directory",
			path: `C:\Program Files\App\app.exe`, mode: model.ModeContents,
		},
		{
			name: "a user's documents",
			path: `%USERPROFILE%\Documents\report.docx`, mode: model.ModeContents,
		},
		{
			name: "another user's profile",
			path: `C:\Users\other\AppData\Local\Temp`, mode: model.ModeContents,
		},
		{
			name: "a drive's recycle bin",
			path: `C:\$Recycle.Bin\S-1-5-21`, mode: model.ModeContents,
		},
		{
			name: "the Windows root itself",
			path: `%SystemRoot%`, mode: model.ModeContents,
		},
		{
			name: "the device namespace reaches nothing",
			path: `\\?\C:\Windows\System32`, mode: model.ModeContents,
		},
		{
			name: "a file rule with no age filter does not reach into a root",
			path: `%SystemRoot%`, mode: model.ModeFiles, include: []string{`*.log`},
		},
		{
			name: "a file rule with no include filter does not reach into a root",
			path: `%SystemRoot%`, mode: model.ModeFiles, minAgeDays: 7,
		},
	}

	g, _ := newGuard(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verdict := check(t, g, test.path, test.mode, test.include, test.minAgeDays)
			if verdict.Allowed {
				t.Errorf("Check(%q) = %+v, want a refusal", test.path, verdict)
			}
			if verdict.Reason != model.ReasonProtected {
				t.Errorf("Reason = %q, want %q", verdict.Reason, model.ReasonProtected)
			}
		})
	}
}
