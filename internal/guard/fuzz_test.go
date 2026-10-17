package guard

import (
	"testing"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// neverAllowed is the part of the protected list this test carries on its own,
// written out here rather than read from the guard. It is the reason the
// invariant means something: a repair that narrows the guard cannot narrow this
// list in the same breath, so the test notices. Only the ranges that refuse
// every mode belong here; a range a filtered `mode: files` rule may enter does
// not, and neither does a root a rule may empty.
var neverAllowed = []string{
	`c:\windows\system32`,
	`c:\windows\syswow64`,
	`c:\windows\winsxs`,
	`c:\windows\boot`,
	`c:\windows\fonts`,
	`c:\windows\recovery`,
	`c:\program files`,
	`c:\program files (x86)`,
	`c:\$recycle.bin`,
	`c:\system volume information`,
	`c:\efi`,
	`c:\recovery`,
	`c:\users\probe\desktop`,
	`c:\users\probe\documents`,
	`c:\users\probe\pictures`,
	`c:\users\probe\videos`,
	`c:\users\probe\music`,
	`c:\users\probe\downloads`,
	`c:\users\probe\onedrive`,
}

// FuzzNormalize feeds the normalization and the check paths nobody wrote a case
// for, and holds them to the two promises the guard makes: it never falls over,
// and a path it allows is never one of the places the specification forbids.
//
// The file system is the in-memory one, so a run needs no disk: LongPath is the
// identity and every drive is fixed.
func FuzzNormalize(f *testing.F) {
	for _, seed := range []string{
		``,
		`.`,
		`..`,
		`\`,
		`C:`,
		`C:\`,
		`C:\Windows`,
		`C:\Windows\`,
		`C:\Windows\System32`,
		`C:\Windows\System32\drivers\etc\hosts`,
		`C:\Windows\SysWOW64\ntdll.dll`,
		`C:\Windows\WinSxS`,
		`C:\Windows\Boot`,
		`C:\Windows\Fonts\arial.ttf`,
		`C:\Windows\Recovery`,
		`C:\Windows\assembly`,
		`C:\Windows\Temp`,
		`C:\Windows\Temp\file.tmp`,
		`C:\Windows\Logs\CBS\CBS.log`,
		`C:\Windows\Minidump\010126-01.dmp`,
		`C:\Windows\SoftwareDistribution\Download`,
		`C:\Windows.old\Windows`,
		`c:\windows\system32`,
		`C:/Windows/System32`,
		`C:\Windows\System32\..\Temp`,
		`C:\Windows\System32\.`,
		`C:\Windows\System32\ `,
		`C:\Windows\System32.`,
		`\\?\C:\Windows\System32`,
		`\\?\UNC\server\share`,
		`\\.\C:\Windows`,
		`\\server\share\file`,
		`//server/share/file`,
		`\Windows\System32`,
		`C:\Users`,
		`C:\Users\probe`,
		`C:\Users\probe\`,
		`C:\Users\probe\Desktop`,
		`C:\Users\probe\Documents\report.docx`,
		`C:\Users\probe\Downloads`,
		`C:\Users\probe\OneDrive`,
		`C:\Users\other\Documents`,
		`C:\Users\probe\AppData\Local\Temp\file.tmp`,
		`C:\Users\probe\AppData\Roaming\app\cache`,
		`C:\Program Files`,
		`C:\Program Files\App\app.exe`,
		`C:\Program Files (x86)\App`,
		`C:\Program Files (x86)`,
		`C:\ProgramData`,
		`C:\ProgramData\Package Cache`,
		`C:\$Recycle.Bin\S-1-5-21-1`,
		`C:\System Volume Information`,
		`C:\EFI`,
		`C:\Recovery`,
		`C:\Temp`,
		`C:\Temp\file.txt`,
		`C:\Temp\file.txt.`,
		`C:\Temp\file .txt`,
		`C:\Temp\..\..\Windows`,
		`C:\pagefile.sys`,
		`D:\Temp`,
		`D:\System Volume Information`,
		`Temp`,
		`.\Temp`,
		`..\..\Windows`,
		`file.txt`,
		`%SystemRoot%`,
		`%SystemRoot%\System32`,
		`%SystemRoot%\Temp`,
		`%TEMP%`,
		`%TEMP%\file.tmp`,
		`%USERPROFILE%`,
		`%USERPROFILE%\Desktop`,
		`%APPDATA%`,
		`%LOCALAPPDATA%\Google\Chrome\User Data\Default\Cache`,
		`%ProgramFiles%\App`,
		`%ProgramData%\Package Cache`,
		`%USERPROFILE%\Desktop\%TEMP%`,
		`%`,
		`%%`,
		`%NOPE%`,
		`%NOPE%\x`,
		`%SystemRoot%\%NOPE%`,
		`%userprofile%\Desktop`,
		` %SystemRoot% `,
		"a\x00b",
		"C:\\Windows\\System32\\\x00",
		"C:\u00a0Windows",
	} {
		f.Add(seed)
	}

	g := fuzzGuard(f)

	f.Fuzz(func(t *testing.T, given string) {
		verdict, err := g.Check(given, model.ModeContents, nil, 0)
		if err != nil {
			t.Fatalf("Check(%q) failed instead of answering: %v", given, err)
		}

		if !verdict.Allowed {
			if verdict.Reason != model.ReasonProtected {
				t.Fatalf("Check(%q) refused with reason %q, want %q", given, verdict.Reason, model.ReasonProtected)
			}
			if verdict.Message == "" {
				t.Fatalf("Check(%q) refused without saying which range refused it", given)
			}
			return
		}

		if verdict.Reason != "" || verdict.Message != "" {
			t.Fatalf("Check(%q) allowed the path and still carried %q", given, verdict.Message)
		}
		if !isAbsPath(verdict.Path) {
			t.Fatalf("Check(%q) allowed a path that is not absolute: %q", given, verdict.Path)
		}

		for _, prefix := range neverAllowed {
			if atOrBelow(verdict.Path, prefix) {
				t.Fatalf("Check(%q) allowed %q, which is inside the forbidden %q", given, verdict.Path, prefix)
			}
		}
		if atOrBelow(verdict.Path, `C:\Users`) && !atOrBelow(verdict.Path, `C:\Users\probe`) {
			t.Fatalf("Check(%q) allowed %q, which is another user's profile", given, verdict.Path)
		}

		// The path the verdict points at is the one the engine will touch, so
		// checking it again has to answer the same thing.
		again, err := g.Check(verdict.Path, model.ModeContents, nil, 0)
		if err != nil {
			t.Fatalf("Check(%q) failed on the path it had just allowed: %v", verdict.Path, err)
		}
		if !again.Allowed {
			t.Fatalf("Check(%q) allowed %q, and then refused the same path: %s", given, verdict.Path, again.Message)
		}
		if again.Path != verdict.Path {
			t.Fatalf("Check(%q) allowed %q, and then normalized it to %q", given, verdict.Path, again.Path)
		}
	})
}

// fuzzGuard is a guard over a machine in memory: a fixed drive, the directories
// a rule names, and the environment of a user called probe.
func fuzzGuard(f *testing.F) *Guard {
	f.Helper()

	fsys := fake.NewFS()
	for _, dir := range []string{`C:\Temp`, `C:\Windows`, `C:\Users\probe`, `C:\ProgramData`} {
		fsys.Add(platform.Info{Path: dir, IsDir: true})
	}

	g, err := New(fsys, guardEnv(), `C:\Users\probe`, nil)
	if err != nil {
		f.Fatalf("New: %v", err)
	}
	return g
}

// atOrBelow says whether path is dir itself or something inside it, the way a
// range of the protected list covers a path.
func atOrBelow(path, dir string) bool {
	return pathKey(path) == pathKey(dir) || below(path, dir)
}
