package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// scanDeps collects what `sweep scan` works with in these tests: the fake of a
// machine that holds what the built-in rules aim at, and nothing of the test
// machine itself. The test machine's own rules and config stay out: its data
// directory is a directory of the test's own.
func scanDeps(t *testing.T) (d deps) {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())

	fsys := scanTree()
	env := fake.NewEnv()
	env.Set(`USERPROFILE`, `C:\Users\probe`)
	env.Set(`SystemRoot`, `C:\Windows`)
	env.Set(`SystemDrive`, `C:`)
	env.Set(`ProgramData`, `C:\ProgramData`)
	env.Set(`ProgramFiles`, `C:\Program Files`)
	env.Set(`ProgramFiles(x86)`, `C:\Program Files (x86)`)
	env.Set(`APPDATA`, `C:\Users\probe\AppData\Roaming`)
	env.Set(`LOCALAPPDATA`, `C:\Users\probe\AppData\Local`)
	env.Set(`TEMP`, `C:\Users\probe\AppData\Local\Temp`)

	return deps{FS: fsys, Env: env, System: fake.NewSystem(), Cwd: `C:\Users\probe`}
}

// scanTree builds the fake the built-in rules read. The times are so that
// something in the temp folder is old and something is fresh.
func scanTree() *fake.FS {
	fsys := fake.NewFS()
	for _, path := range []string{
		`C:\Windows`, `C:\Users`, `C:\Users\probe`, `C:\Users\probe\AppData`,
		`C:\Users\probe\AppData\Local`, `C:\Users\probe\AppData\Roaming`,
		`C:\Users\probe\AppData\Local\Temp`,
		`C:\Users\probe\AppData\Local\Google`, `C:\Users\probe\AppData\Local\Google\Chrome`,
		`C:\Users\probe\AppData\Local\Google\Chrome\User Data`,
		`C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1`,
	} {
		fsys.Add(platform.Info{Path: path, IsDir: true})
	}
	old := time.Now().Add(-24 * time.Hour)
	fresh := time.Now()
	for _, info := range []platform.Info{
		{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\Cache`, IsDir: true, ModTime: old},
		{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\Cache\a.dat`, Size: 10, ModTime: old},
		{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\Cache\b.dat`, Size: 20, ModTime: old},
		{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\Code Cache`, IsDir: true, ModTime: old},
		{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\Code Cache\c.dat`, Size: 5, ModTime: old},
		{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\GPUCache`, IsDir: true, ModTime: old},
		{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\GPUCache\d.dat`, Size: 1, ModTime: old},
		{Path: `C:\Users\probe\AppData\Local\Temp\old.log`, Size: 30, ModTime: old},
		{Path: `C:\Users\probe\AppData\Local\Temp\fresh.log`, Size: 7, ModTime: fresh},
		{Path: `C:\Users\probe\AppData\Local\npm-cache`, IsDir: true, ModTime: old},
		{Path: `C:\Users\probe\AppData\Local\npm-cache\index`, Size: 50, ModTime: old},
	} {
		fsys.Add(info)
	}
	return fsys
}

// scanRun runs `sweep scan` with its arguments on the deps of the test and
// returns what it printed and how it ended.
func scanRun(t *testing.T, d deps, args ...string) (string, string, error) {
	t.Helper()
	root := newRootCmd(d, pinEnglish(append([]string{"scan"}, args...)))
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"scan"}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestScanTable(t *testing.T) {
	out, errOut, err := scanRun(t, scanDeps(t))
	if err != nil {
		t.Fatalf("sweep scan: %v", err)
	}
	skipWarnings(t, errOut)

	if !strings.Contains(out, "ID") || !strings.Contains(out, "ITEMS") ||
		!strings.Contains(out, "FILES") || !strings.Contains(out, "SIZE") ||
		!strings.Contains(out, "SKIPPED") {
		t.Errorf("the header is not what the spec names in %q", out)
	}
	if !strings.Contains(out, "browsers.chrome.cache") {
		t.Errorf("the rule of Chrome is not in %q", out)
	}
	if !strings.Contains(out, "system.user-temp") {
		t.Errorf("the rule of temp is not in %q", out)
	}
	if strings.Contains(out, "dev.npm-cache") {
		t.Errorf("the rule out of the profile is in %q", out)
	}
	if !strings.Contains(out, "TOTAL") {
		t.Errorf("the totals row is not in %q", out)
	}

	// Chrome holds three caches with 36 bytes and 3 files in all/temp holds
	// the one old file of 30 bytes.
	if !strings.Contains(out, " 36 B") || !strings.Contains(out, " 30 B") {
		t.Errorf("sizes missing in %q", out)
	}
}

type scanDocument struct {
	Schema  int    `json:"schema"`
	DryRun  bool   `json:"dry_run"`
	Created string `json:"created_at"`
	Summary struct {
		Rules      int   `json:"rules"`
		Items      int   `json:"items"`
		Files      int64 `json:"files"`
		BytesFound int64 `json:"bytes_found"`
		Skipped    int   `json:"skipped"`
	} `json:"summary"`
	Rules []struct {
		ID      string  `json:"id"`
		Status  string  `json:"status"`
		Bytes   int64   `json:"bytes"`
		Files   int64   `json:"files"`
		Command *string `json:"-"`
		Items   []struct {
			Path string  `json:"path"`
			Name string  `json:"-"`
			Skip *string `json:"-"`
		} `json:"items"`
		Skipped []struct {
			Path   string `json:"path"`
			Reason string `json:"reason"`
			Detail string `json:"detail"`
		} `json:"skipped"`
	} `json:"rules"`
}

func TestScanJSONMatchesPlan(t *testing.T) {
	out, _, err := scanRun(t, scanDeps(t), "--json")
	if err != nil {
		t.Fatalf("sweep scan --json: %v", err)
	}

	var doc scanDocument
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("%q is not one document: %v", out, err)
	}
	if doc.Schema != 1 || !doc.DryRun {
		t.Errorf("schema/dry_run = %d/%v, want 1/true", doc.Schema, doc.DryRun)
	}
	if doc.Summary.Rules < 2 {
		t.Errorf("summary.rules = %d, want at least the two rules the fake holds", doc.Summary.Rules)
	}
	if doc.Summary.Items != 5 || doc.Summary.Files != 5 || doc.Summary.BytesFound != 66 {
		t.Errorf("summary = %+v, want what the fake holds: the four caches of Chrome and the one old file", doc.Summary)
	}
	if len(doc.Rules) != doc.Summary.Rules {
		t.Fatalf("rules = %d, want as many as the summary names", len(doc.Rules))
	}
	var chrome struct {
		ID      string  `json:"id"`
		Status  string  `json:"status"`
		Bytes   int64   `json:"bytes"`
		Files   int64   `json:"files"`
		Command *string `json:"-"`
		Items   []struct {
			Path string  `json:"path"`
			Name string  `json:"-"`
			Skip *string `json:"-"`
		} `json:"items"`
		Skipped []struct {
			Path   string `json:"path"`
			Reason string `json:"reason"`
			Detail string `json:"detail"`
		} `json:"skipped"`
	}
	foundChrome := false
	for _, rule := range doc.Rules {
		if rule.ID == "browsers.chrome.cache" {
			chrome, foundChrome = rule, true
			break
		}
	}
	if !foundChrome {
		t.Fatal("the Chrome cache is not among the documented rules")
	}

	if chrome.ID != "browsers.chrome.cache" || chrome.Status != "dry_run" {
		t.Errorf("rule = %+v, want the Chrome cache as dry_run", chrome)
	}
}

func TestScanRuleFlag(t *testing.T) {
	t.Run("a mask reaches the rule out of the profile", func(t *testing.T) {
		out, _, err := scanRun(t, scanDeps(t), "--rule", "dev.*")
		if err != nil {
			t.Fatalf("sweep scan --rule dev.*: %v", err)
		}
		if !strings.Contains(out, "dev.npm-cache") {
			t.Errorf("the rule is not in %q", out)
		}
	})

	t.Run("the flag is repeated", func(t *testing.T) {
		out, _, err := scanRun(t, scanDeps(t), "--rule", "dev.npm-cache", "--rule", "browsers.chrome.cache")
		if err != nil {
			t.Fatalf("sweep scan --rule twice: %v", err)
		}
		if !strings.Contains(out, "dev.npm-cache") || !strings.Contains(out, "browsers.chrome.cache") {
			t.Errorf("the rules are not in %q", out)
		}
	})

	t.Run("a mask that matches nothing ends with the code of the flags", func(t *testing.T) {
		_, _, err := scanRun(t, scanDeps(t), "--rule", "nowhere.*")
		var exit *ExitError
		if !errors.As(err, &exit) || exit.Code != ExitUsage {
			t.Fatalf("error = %v, want exit %d", err, ExitUsage)
		}
	})
}

// leaveConfig writes a config where the command finds one, in the data
// directory the deps' environment names.
func leaveConfig(t *testing.T, d deps, text string) {
	t.Helper()
	// The data directory of the command goes over a directory of the test's
	// own, so that the machine's own config stays out of it.
	place := t.TempDir()
	d.Env.(*fake.Env).Set("APPDATA", place)
	if err := os.MkdirAll(filepath.Join(place, "sweep"), 0o700); err != nil {
		t.Fatalf("make the data directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(place, "sweep", "config.yaml"), []byte(text), 0o600); err != nil {
		t.Fatalf("write the config: %v", err)
	}
}

func TestScanExitCodes(t *testing.T) {
	t.Run("a config that does not parse ends with the code of the flags", func(t *testing.T) {
		d := scanDeps(t)
		leaveConfig(t, d, "rules: [")
		_, _, err := scanRun(t, d)
		var exit *ExitError
		if !errors.As(err, &exit) || exit.Code != ExitUsage {
			t.Fatalf("error = %v, want exit %d", err, ExitUsage)
		}
	})
}

func TestScanCancel(t *testing.T) {
	d := scanDeps(t)
	root := newRootCmd(d, pinEnglish([]string{"scan"}))
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"scan"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := root.ExecuteContext(ctx)
	if err != nil {
		t.Fatalf("an interrupted scan is no failure: %v", err)
	}
	if !strings.Contains(errOut.String(), "scan interrupted") {
		t.Errorf("stderr = %q, want scan interrupted", errOut.String())
	}
}
