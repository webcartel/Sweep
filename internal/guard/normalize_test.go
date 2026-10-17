package guard

import (
	"errors"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

func TestNormalizeEnv(t *testing.T) {
	g, _ := newGuard(t)

	path, err := g.Normalize(`%TEMP%\probe`)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if want := `C:\Temp\probe`; path.Given != want {
		t.Errorf("Given = %q, want %q", path.Given, want)
	}

	_, err = g.Normalize(`%SWEEP_NOPE%\probe`)
	refusal := refusalOf(t, err)
	if refusal.Reason != model.ReasonProtected {
		t.Errorf("Reason = %q, want %q", refusal.Reason, model.ReasonProtected)
	}
	if !strings.Contains(refusal.Message, "SWEEP_NOPE") {
		t.Errorf("Message = %q, want the name of the variable it did not find", refusal.Message)
	}
}

func TestNormalizeShape(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"the device prefix goes away", `\\?\C:\Temp`, `C:\Temp`},
		{"a trailing separator goes away", `C:\Temp\`, `C:\Temp`},
		{"a trailing dot goes away", `C:\Temp\probe..`, `C:\Temp\probe`},
		{"a trailing space goes away", `C:\Temp\probe `, `C:\Temp\probe`},
		{"a trailing dot with a space goes away", `C:\Temp\probe. `, `C:\Temp\probe`},
		{"a dot segment goes away", `C:\Temp\.\probe`, `C:\Temp\probe`},
		{"a dot dot segment is resolved", `C:\Temp\..\Windows`, `C:\Windows`},
		{"a dot dot above the root is dropped", `C:\..\Temp`, `C:\Temp`},
		{"either separator is a separator", `C:/Temp/probe`, `C:\Temp\probe`},
		{"the drive letter is written in capitals, the rest is left alone", `c:\temp\Probe`, `C:\temp\Probe`},
		{"a relative path is taken from the working directory", `probe\cache`, `C:\Users\probe\probe\cache`},
		{"a drive root keeps its separator", `C:\`, `C:\`},
		{"a long name with a device prefix is left as written", `\\?\C:\Temp\probe.txt`, `C:\Temp\probe.txt`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g, _ := newGuard(t)
			path, err := g.Normalize(test.path)
			if err != nil {
				t.Fatalf("Normalize(%q): %v", test.path, err)
			}
			if path.Given != test.want {
				t.Errorf("Normalize(%q).Given = %q, want %q", test.path, path.Given, test.want)
			}
		})
	}
}

func TestNormalizeRefusesWhatHasNoPathAtAll(t *testing.T) {
	g, _ := newGuard(t)

	for _, path := range []string{``, `\`, `.`} {
		_, err := g.Normalize(path)
		refusal := refusalOf(t, err)
		if refusal.Reason != model.ReasonProtected {
			t.Errorf("Normalize(%q).Reason = %q, want %q", path, refusal.Reason, model.ReasonProtected)
		}
	}
}

func TestNormalizeLongNames(t *testing.T) {
	g, fsys := newGuard(t)
	fsys.MapLong(`C:\PROGRA~1`, `C:\Program Files`)

	path, err := g.Normalize(`C:\PROGRA~1\probe`)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if want := `C:\Program Files\probe`; path.Given != want || path.Final != want {
		t.Errorf("Given, Final = %q, %q, want %q", path.Given, path.Final, want)
	}
}

func TestNormalizeLinks(t *testing.T) {
	t.Run("a junction ends at its target and keeps the chain", func(t *testing.T) {
		g, fsys := newGuard(t)
		fsys.Add(platform.Info{Path: `C:\Temp\link`, IsDir: true, Reparse: true, Link: `\??\C:\Windows\System32`})

		path, err := g.Normalize(`C:\Temp\link`)
		if err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		if want := `C:\Windows\System32`; path.Final != want {
			t.Errorf("Final = %q, want %q", path.Final, want)
		}
		if path.Given != `C:\Temp\link` {
			t.Errorf("Given = %q, want the path as it was written", path.Given)
		}
		if want := []string{`C:\Windows\System32`}; len(path.Chain) != 1 || path.Chain[0] != want[0] {
			t.Errorf("Chain = %q, want %q", path.Chain, want)
		}
		if !path.Exists {
			t.Error("Exists = false for a path whose target is there")
		}
	})

	t.Run("a chain of two junctions is followed whole", func(t *testing.T) {
		g, fsys := newGuard(t)
		fsys.Add(platform.Info{Path: `C:\Temp\one`, IsDir: true, Reparse: true, Link: `C:\Temp\two`})
		fsys.Add(platform.Info{Path: `C:\Temp\two`, IsDir: true, Reparse: true, Link: `C:\Windows\System32`})

		path, err := g.Normalize(`C:\Temp\one`)
		if err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		if want := `C:\Windows\System32`; path.Final != want {
			t.Errorf("Final = %q, want %q", path.Final, want)
		}
		if len(path.Chain) != 2 {
			t.Errorf("Chain = %q, want both steps of the way", path.Chain)
		}
	})

	t.Run("a reparse point with no target ends where it stands", func(t *testing.T) {
		g, fsys := newGuard(t)
		fsys.Add(platform.Info{Path: `C:\Temp\cloud`, Reparse: true})

		path, err := g.Normalize(`C:\Temp\cloud`)
		if err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		if want := `C:\Temp\cloud`; path.Final != want {
			t.Errorf("Final = %q, want %q", path.Final, want)
		}
	})

	t.Run("links that lead back to themselves are refused", func(t *testing.T) {
		g, fsys := newGuard(t)
		fsys.Add(platform.Info{Path: `C:\Temp\a`, IsDir: true, Reparse: true, Link: `C:\Temp\b`})
		fsys.Add(platform.Info{Path: `C:\Temp\b`, IsDir: true, Reparse: true, Link: `C:\Temp\a`})

		_, err := g.Normalize(`C:\Temp\a`)
		refusal := refusalOf(t, err)
		if refusal.Reason != model.ReasonProtected {
			t.Errorf("Reason = %q, want %q", refusal.Reason, model.ReasonProtected)
		}
	})
}

func TestNormalizeDrives(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		drive   platform.DriveKind
		refused bool
	}{
		{"a fixed disk is the one that is allowed", `C:\Temp`, platform.DriveFixed, false},
		{"a network share is not", `\\server\share\probe`, platform.DriveRemote, true},
		{"a stick is not", `D:\probe`, platform.DriveRemovable, true},
		{"a drive Windows does not know is not", `Q:\probe`, platform.DriveOther, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g, fsys := newGuard(t)
			fsys.SetDrive(`D:\`, test.drive)
			fsys.SetDrive(`Q:\`, test.drive)
			fsys.SetDrive(`\\server\share`, test.drive)

			_, err := g.Normalize(test.path)
			if !test.refused {
				if err != nil {
					t.Fatalf("Normalize(%q): %v", test.path, err)
				}
				return
			}
			refusal := refusalOf(t, err)
			if refusal.Reason != model.ReasonProtected {
				t.Errorf("Normalize(%q).Reason = %q, want %q", test.path, refusal.Reason, model.ReasonProtected)
			}
		})
	}
}

func TestNormalizeAcceptsAPathThatIsNotThere(t *testing.T) {
	g, _ := newGuard(t)

	path, err := g.Normalize(`C:\Temp\absent`)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if path.Exists {
		t.Error("Exists = true for a path that is not on the disk")
	}
	if want := `C:\Temp\absent`; path.Final != want {
		t.Errorf("Final = %q, want %q", path.Final, want)
	}
}

// newGuard builds a guard over a small machine: one fixed drive, a few
// directories and the environment a rule can name.
func newGuard(t *testing.T) (*Guard, *fake.FS) {
	t.Helper()
	fsys := fake.NewFS()
	fsys.Add(platform.Info{Path: `C:\Temp`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Windows`, IsDir: true})

	env := fake.NewEnv()
	env.Set(`USERPROFILE`, `C:\Users\probe`)
	env.Set(`TEMP`, `C:\Temp`)
	env.Set(`SystemRoot`, `C:\Windows`)

	g, err := New(fsys, env, `C:\Users\probe`, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g, fsys
}

// refusalOf takes the refusal out of an error, and fails the test when there
// is none.
func refusalOf(t *testing.T, err error) *Refusal {
	t.Helper()
	if err == nil {
		t.Fatal("the call was allowed, want a refusal")
	}
	var refusal *Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want a *Refusal", err)
	}
	return refusal
}
