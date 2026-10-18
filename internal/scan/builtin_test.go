package scan

import (
	"context"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
	"github.com/webcartel/sweep/internal/rules"
)

// TestBuiltinRulesScanWithoutAlarm reads the rules that ship with the binary
// and scans them over a fake of a machine that holds what they aim at. Nothing
// may tell about itself: no Warning, no Skip apart from a path that is not
// there, and every target with a directory on the fake holds its files.
func TestBuiltinRulesScanWithoutAlarm(t *testing.T) {
	set, err := (rules.Loader{Env: judgeEnv()}).Load()
	if err != nil {
		t.Fatalf("load the built-in rules: %v", err)
	}
	if len(set.Rules) == 0 {
		t.Fatal("no rules at all: the built-in ones did not come along")
	}

	fsys := fake.NewFS()
	for _, path := range []string{
		`C:\Users\probe\AppData\Local\Google`,
		`C:\Users\probe\AppData\Local\Google\Chrome`,
		`C:\Users\probe\AppData\Local\Google\Chrome\User Data`,
		`C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1`,
		`C:\Users\probe\AppData\Local\npm-cache`,
	} {
		fsys.Add(platform.Info{Path: path, IsDir: true})
	}
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\Cache`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\Cache\a.dat`, Size: 10})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\Code Cache`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\Code Cache\b.dat`, Size: 20})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\GPUCache`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\Google\Chrome\User Data\Profile 1\GPUCache\c.dat`, Size: 1})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\npm-cache\d.dat`, Size: 4})
	// The temp rule takes what has been in the folder for a day and leaves a
	// fresh one out: what the filters of the rule mean, not a skip.
	old := time.Now().Add(-24 * time.Hour)
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\Temp`, IsDir: true, ModTime: old})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\Temp\old.log`, Size: 30, ModTime: old})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\Temp\fresh.log`, Size: 7, ModTime: time.Now()})

	s := judgeScanner(t, fsys)
	events := drain(s.Scan(context.Background(), set.Rules, model.ScanOptions{Workers: 2, Throttle: time.Hour}))

	var warnings, bad []string
	var items, found int
	var plan *model.Plan
	for _, event := range events {
		switch one := event.(type) {
		case model.Warning:
			warnings = append(warnings, one.RuleID+": "+one.Message)
		case model.ScanDone:
			plan = one.Plan
		}
	}
	if plan == nil {
		t.Fatalf("events = %+v, want ScanDone with a plan; warnings = %q", events, warnings)
	}
	for _, rule := range plan.Rules {
		switch rule.Rule.ID {
		case "browsers.chrome.cache":
			if len(rule.Items) != 3 || rule.Bytes != 31 {
				t.Errorf("chrome = %d items, %d bytes; want three caches and 31 bytes", len(rule.Items), rule.Bytes)
			}
			items += len(rule.Items)
		case "dev.npm-cache":
			if len(rule.Items) != 1 || rule.Bytes != 4 {
				t.Errorf("npm = %d items, %d bytes; want the one file of the cache", len(rule.Items), rule.Bytes)
			}
			items += len(rule.Items)
		case "system.user-temp":
			if len(rule.Items) != 1 || rule.Bytes != 30 {
				t.Errorf("temp = %d items, %d bytes; want the one old file", len(rule.Items), rule.Bytes)
			}
			items += len(rule.Items)
		}
		for _, skip := range rule.Skipped {
			if skip.Reason != model.ReasonNotFound {
				bad = append(bad, skip.Path+": "+string(skip.Reason))
			}
			found++
		}
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none", warnings)
	}
	if len(bad) != 0 {
		t.Errorf("skips = %q, want only not_found", bad)
	}
	if items != 5 {
		t.Errorf("items = %d, want the five entries the fake holds", items)
	}
}
