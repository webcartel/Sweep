package guard

import (
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

func TestCheckModes(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		mode        model.Mode
		include     []string
		minAgeDays  int
		andRefused  bool
		andMentions string
	}{
		{
			name: "a Windows root may not be emptied",
			path: `C:\Windows`, mode: model.ModeContents, andRefused: true, andMentions: `C:\Windows`,
		},
		{
			name: "a Windows root may not be deleted",
			path: `C:\Windows`, mode: model.ModeSelf, andRefused: true,
		},
		{
			name: "the Windows temporary directory may be emptied",
			path: `C:\Windows\Temp`, mode: model.ModeContents,
		},
		{
			name: "what is inside a protected subtree may not be emptied",
			path: `C:\Windows\System32\x`, mode: model.ModeContents, andRefused: true, andMentions: `C:\Windows\System32`,
		},
		{
			name: "a protected subtree is refused even for files",
			path: `C:\Windows\System32`, mode: model.ModeFiles, include: []string{`*.log`}, minAgeDays: 7,
			andRefused: true,
		},
		{
			name: "a root is refused for files without a filter",
			path: `C:\Windows`, mode: model.ModeFiles,
			andRefused: true,
		},
		{
			name: "a root is refused for files without an age filter",
			path: `C:\Windows`, mode: model.ModeFiles, include: []string{`*.log`},
			andRefused: true,
		},
		{
			name: "files with both filters may reach into a root",
			path: `C:\Windows`, mode: model.ModeFiles, include: []string{`*.log`}, minAgeDays: 7,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g, _ := newGuard(t)
			verdict := check(t, g, test.path, test.mode, test.include, test.minAgeDays)

			if verdict.Allowed == test.andRefused {
				t.Fatalf("Check(%q, %s) = %+v, want refused: %v", test.path, test.mode, verdict, test.andRefused)
			}
			if !test.andRefused {
				return
			}
			if verdict.Reason != model.ReasonProtected {
				t.Errorf("Reason = %q, want %q", verdict.Reason, model.ReasonProtected)
			}
			if !strings.Contains(verdict.Message, test.andMentions) {
				t.Errorf("Message = %q, want it to name %q", verdict.Message, test.andMentions)
			}
		})
	}
}

func TestCheckUserData(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		mode       model.Mode
		include    []string
		minAgeDays int
		refused    bool
	}{
		{
			name: "the Desktop may not be emptied",
			path: `C:\Users\probe\Desktop`, mode: model.ModeContents, refused: true,
		},
		{
			name: "the Desktop may not be deleted",
			path: `C:\Users\probe\Desktop`, mode: model.ModeSelf, refused: true,
		},
		{
			name: "files without an include filter are not enough",
			path: `C:\Users\probe\Desktop`, mode: model.ModeFiles, refused: true,
		},
		{
			name: "files without an age filter are not enough",
			path: `C:\Users\probe\Desktop`, mode: model.ModeFiles, include: []string{`*.png`}, refused: true,
		},
		{
			name: "old files in the Desktop may be taken",
			path: `C:\Users\probe\Desktop`, mode: model.ModeFiles, include: []string{`*.png`}, minAgeDays: 1,
		},
		{
			name: "what is inside a user directory is refused whole",
			path: `C:\Users\probe\Documents\App\Cache`, mode: model.ModeContents, refused: true,
		},
		{
			name: "downloads are user data too",
			path: `%USERPROFILE%\Downloads\setup.exe`, mode: model.ModeContents, refused: true,
		},
		{
			name: "OneDrive is user data too",
			path: `%USERPROFILE%\OneDrive\file.bin`, mode: model.ModeContents, refused: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g, _ := newGuard(t)
			verdict := check(t, g, test.path, test.mode, test.include, test.minAgeDays)

			if verdict.Allowed == test.refused {
				t.Errorf("Check(%q, %s) = %+v, want refused: %v", test.path, test.mode, verdict, test.refused)
			}
		})
	}
}

func TestCheckProfileAndRoots(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		drive   platform.DriveKind
		mode    model.Mode
		refused bool
	}{
		{
			name: "a cache inside LocalAppData may be emptied",
			path: `%LOCALAPPDATA%\Google\Chrome\User Data\Default\Cache`, mode: model.ModeContents,
		},
		{
			name: "a rule may not empty LocalAppData itself",
			path: `%LOCALAPPDATA%`, mode: model.ModeContents, refused: true,
		},
		{
			name: "a rule may not empty AppData itself",
			path: `%APPDATA%`, mode: model.ModeContents, refused: true,
		},
		{
			name: "another profile is not this machine's to clean",
			path: `C:\Users\bob\x`, mode: model.ModeContents, refused: true,
		},
		{
			name: "C:\\Users itself may not be emptied",
			path: `C:\Users`, mode: model.ModeContents, refused: true,
		},
		{
			name: "the profile itself may not be emptied",
			path: `%USERPROFILE%`, mode: model.ModeContents, refused: true,
		},
		{
			name: "a template inside the profile is not user data",
			path: `%USERPROFILE%\AppData\Local\Temp\x`, mode: model.ModeContents,
		},
		{
			name: "a program cache inside the profile is not user data",
			path: `%USERPROFILE%\.gradle\caches`, mode: model.ModeContents,
		},
		{
			name: "ProgramData itself may not be emptied",
			path: `C:\ProgramData`, mode: model.ModeContents, refused: true,
		},
		{
			name: "a cache inside ProgramData may be emptied",
			path: `C:\ProgramData\App\cache`, mode: model.ModeContents,
		},
		{
			name: "a directory on another fixed disk is allowed",
			path: `D:\Games\Cache`, drive: platform.DriveFixed, mode: model.ModeContents,
		},
		{
			name: "a drive root may not be emptied",
			path: `D:\`, drive: platform.DriveFixed, mode: model.ModeContents, refused: true,
		},
		{
			name: "a drive root may not be deleted",
			path: `D:\`, drive: platform.DriveFixed, mode: model.ModeSelf, refused: true,
		},
		{
			name: "the recycle bin of any drive is refused",
			path: `D:\$Recycle.Bin\x`, drive: platform.DriveFixed, mode: model.ModeContents, refused: true,
		},
		{
			name: "system volume information of any drive is refused",
			path: `D:\System Volume Information\x`, drive: platform.DriveFixed, mode: model.ModeContents, refused: true,
		},
		{
			name: "the EFI directory of any drive is refused",
			path: `D:\EFI\BOOT`, drive: platform.DriveFixed, mode: model.ModeContents, refused: true,
		},
		{
			name: "a removable disk is refused whole",
			path: `E:\Games\Cache`, drive: platform.DriveRemovable, mode: model.ModeContents, refused: true,
		},
		{
			name: "a network path is refused whole",
			path: `\\server\share\Games`, drive: platform.DriveRemote, mode: model.ModeContents, refused: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g, fsys := newGuard(t)
			fsys.SetDrive(`D:\`, test.drive)
			fsys.SetDrive(`E:\`, test.drive)
			fsys.SetDrive(`\\server\share`, test.drive)
			verdict := check(t, g, test.path, test.mode, nil, 0)

			if verdict.Allowed == test.refused {
				t.Errorf("Check(%q) = %+v, want refused: %v", test.path, verdict, test.refused)
			}
		})
	}
}

// TestCheckProtectedSpellings takes the paths a rule could write to look like
// something else and asks the guard what it sees.
func TestCheckProtectedSpellings(t *testing.T) {
	refused := []struct {
		name string
		path string
		mode model.Mode
	}{
		{"mixed case is the same path", `c:\windOWs\system32\x`, model.ModeContents},
		{"a trailing dot is the same path", `C:\Windows.\System32`, model.ModeContents},
		{"a trailing space is the same path", `C:\Windows \System32`, model.ModeContents},
		{"the device prefix is the same path", `\\?\C:\Windows\System32`, model.ModeContents},
		{"a dot dot is resolved first", `C:\Windows\..\Windows\System32`, model.ModeContents},
		{"a program directory may not be emptied", `\\?\C:\Program Files\`, model.ModeContents},
		{"what is inside a program directory is refused", `C:\Program Files\App\cache`, model.ModeContents},
		{"what is inside a program directory is refused for files too", `C:\Program Files (x86)\App\cache`, model.ModeFiles},
		{"the system root itself is refused", `%SystemRoot%`, model.ModeContents},
		{"WinSxS is refused", `%SystemRoot%\WinSxS\x`, model.ModeContents},
		{"the boot directory is refused", `%SystemRoot%\Boot\x`, model.ModeContents},
		{"the fonts directory is refused", `%SystemRoot%\Fonts`, model.ModeContents},
		{"the recovery directory of Windows is refused", `%SystemRoot%\Recovery\x`, model.ModeContents},
		{"the recovery directory of a drive is refused", `C:\Recovery\WindowsRE`, model.ModeContents},
		{"a Unicode-looking path is still the same path", `C:\WINDOWS\SYSWOW64\x`, model.ModeContents},
	}

	for _, test := range refused {
		t.Run(test.name, func(t *testing.T) {
			g, _ := newGuard(t)
			verdict := check(t, g, test.path, test.mode, nil, 0)
			if verdict.Allowed {
				t.Errorf("Check(%q) = %+v, want a refusal", test.path, verdict)
			}
			if verdict.Reason != model.ReasonProtected {
				t.Errorf("Reason = %q, want %q", verdict.Reason, model.ReasonProtected)
			}
			if verdict.Message == "" {
				t.Error("Message is empty, want the range that refused the path")
			}
		})
	}

	allowed := []struct {
		name string
		path string
		mode model.Mode
	}{
		{"the Windows temporary directory", `%SystemRoot%\Temp\x`, model.ModeContents},
		{"the Windows logs", `%SystemRoot%\Logs\CBS\a.log`, model.ModeContents},
		{"the Windows update cache", `%SystemRoot%\SoftwareDistribution\Download\a`, model.ModeContents},
		{"the minidump directory", `%SystemRoot%\Minidump\a.dmp`, model.ModeContents},
		{"the delivery optimization cache", `%SystemRoot%\ServiceProfiles\NetworkService\AppData\Local\Microsoft\Windows\DeliveryOptimization\Cache`, model.ModeContents},
		{"a file in the system root, which the specification cleans", `%SystemRoot%\MEMORY.DMP`, model.ModeSelf},
		{"a directory in the system root that the specification leaves alone", `%SystemRoot%\Prefetch`, model.ModeContents},
		{"a path that leaves the system root again", `%SystemRoot%\System32\..\..\Temp\x`, model.ModeContents},
		{"a temporary directory that is not Windows", `C:\Temp\probe`, model.ModeContents},
		{"an AppData cache", `%APPDATA%\App\Cache`, model.ModeContents},
	}

	for _, test := range allowed {
		t.Run(test.name, func(t *testing.T) {
			g, _ := newGuard(t)
			verdict := check(t, g, test.path, test.mode, nil, 0)
			if !verdict.Allowed {
				t.Errorf("Check(%q) = %+v, want it allowed", test.path, verdict)
			}
		})
	}
}

func TestCheckLinks(t *testing.T) {
	t.Run("a link into System32 is refused where it stands", func(t *testing.T) {
		g, fsys := newGuard(t)
		fsys.Add(platform.Info{
			Path: `C:\Users\probe\AppData\Local\link`, IsDir: true,
			Reparse: true, Link: `C:\Windows\System32`,
		})

		verdict := check(t, g, `%LOCALAPPDATA%\link`, model.ModeContents, nil, 0)
		if verdict.Allowed {
			t.Errorf("Check = %+v, want a refusal: the link ends in a protected subtree", verdict)
		}
	})

	t.Run("a link out of a protected subtree does not save it", func(t *testing.T) {
		g, fsys := newGuard(t)
		fsys.Add(platform.Info{
			Path: `C:\Windows\System32\link`, IsDir: true,
			Reparse: true, Link: `C:\Temp\probe`,
		})

		verdict := check(t, g, `C:\Windows\System32\link`, model.ModeContents, nil, 0)
		if verdict.Allowed {
			t.Errorf("Check = %+v, want a refusal: both paths are read, and this one is protected", verdict)
		}
	})

	t.Run("two links into System32 are refused", func(t *testing.T) {
		g, fsys := newGuard(t)
		fsys.Add(platform.Info{
			Path: `C:\Temp\one`, IsDir: true, Reparse: true, Link: `C:\Temp\two`,
		})
		fsys.Add(platform.Info{
			Path: `C:\Temp\two`, IsDir: true, Reparse: true, Link: `C:\Windows\System32`,
		})

		verdict := check(t, g, `C:\Temp\one`, model.ModeContents, nil, 0)
		if verdict.Allowed {
			t.Errorf("Check = %+v, want a refusal: the chain ends in a protected subtree", verdict)
		}
	})
}

func TestCheckExcludePaths(t *testing.T) {
	t.Run("an exclusion adds a refusal", func(t *testing.T) {
		g, _ := newGuard(t, `C:\Windows\Temp`)

		verdict := check(t, g, `C:\Windows\Temp\probe`, model.ModeContents, nil, 0)
		if verdict.Allowed {
			t.Errorf("Check = %+v, want a refusal: the config forbids this directory", verdict)
		}
		if verdict.Reason != model.ReasonProtected {
			t.Errorf("Reason = %q, want %q", verdict.Reason, model.ReasonProtected)
		}
	})

	t.Run("an exclusion never allows anything", func(t *testing.T) {
		g, _ := newGuard(t, `C:\Games`)

		verdict := check(t, g, `C:\Windows\System32\x`, model.ModeContents, nil, 0)
		if verdict.Allowed {
			t.Errorf("Check = %+v, want a refusal: an exclusion cannot widen what is allowed", verdict)
		}
	})

	t.Run("a relative exclusion is taken from the working directory", func(t *testing.T) {
		g, _ := newGuard(t, `Games\Cache`)

		verdict := check(t, g, `C:\Users\probe\Games\Cache\x`, model.ModeContents, nil, 0)
		if verdict.Allowed {
			t.Errorf("Check = %+v, want a refusal: the entry stands for a path under the working directory", verdict)
		}
	})

	t.Run("an exclusion with a variable this machine does not set stops the guard", func(t *testing.T) {
		fsys := fake.NewFS()
		_, err := New(fsys, guardEnv(), `C:\Users\probe`, []string{`%SWEEP_NOPE%\x`})
		if err == nil {
			t.Fatal("New accepted an exclusion with a variable that is not set")
		}
		if !strings.Contains(err.Error(), "SWEEP_NOPE") {
			t.Errorf("error = %v, want it to name the variable", err)
		}
	})

	t.Run("an empty exclusion stops the guard", func(t *testing.T) {
		fsys := fake.NewFS()
		if _, err := New(fsys, guardEnv(), `C:\Users\probe`, []string{``}); err == nil {
			t.Fatal("New accepted an empty exclusion")
		}
	})
}

// check asks the guard about a path and fails the test when the call itself
// breaks: the verdict is what the tests read.
func check(t *testing.T, g *Guard, path string, mode model.Mode, include []string, minAgeDays int) Verdict {
	t.Helper()
	verdict, err := g.Check(path, mode, include, minAgeDays)
	if err != nil {
		t.Fatalf("Check(%q): %v", path, err)
	}
	return verdict
}
