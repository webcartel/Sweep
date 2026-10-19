package rules

import (
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/webcartel/sweep/internal/model"
)

// builtinEnv is the environment the built-in rules are written against. A name
// missing here means a built-in rule names a variable that is not there.
var builtinEnv = fakeEnv{
	"APPDATA":      `C:\Users\probe\AppData\Roaming`,
	"TEMP":         `C:\Users\probe\AppData\Local\Temp`,
	"LOCALAPPDATA": `C:\Users\probe\AppData\Local`,
}

// builtinFiles reads the built-in rules and fails the test if they do not read.
func builtinFiles(t *testing.T) []*File {
	t.Helper()

	files, err := ReadBuiltin()
	if err != nil {
		t.Fatalf("ReadBuiltin: %v", err)
	}
	return files
}

func TestBuiltinRulesAreValid(t *testing.T) {
	for _, file := range builtinFiles(t) {
		if findings := ValidateSchema(file); len(findings) != 0 {
			t.Errorf("%s does not fit the schema: %v", file.Path, findings)
		}
		findings, warnings := ValidateSemantics(file, builtinEnv)
		if len(findings) != 0 {
			t.Errorf("%s has semantic findings: %v", file.Path, findings)
		}
		if len(warnings) != 0 {
			t.Errorf("%s warns: %v", file.Path, warnings)
		}
		if len(file.Rules) == 0 {
			t.Errorf("%s holds no rules", file.Path)
		}
		for _, parsed := range file.Rules {
			if parsed.Rule.Source != SourceBuiltin {
				t.Errorf("%s: Source = %q, want %q", file.Path, parsed.Rule.Source, SourceBuiltin)
			}
			if parsed.Rule.ID == "" {
				t.Errorf("%s holds a rule without an id", file.Path)
			}
		}
	}
}

// builtinRuleCount answers how many rules the embed carries, whatever files
// the v1 set holds by now.
func builtinRuleCount(t *testing.T) int {
	t.Helper()
	count := 0
	for _, file := range builtinFiles(t) {
		count += len(file.Rules)
	}
	return count
}

// builtinIDs answers the ids of the built-in set, in the embed's order.
func builtinIDs(t *testing.T) []string {
	t.Helper()
	var ids []string
	for _, file := range builtinFiles(t) {
		for _, parsed := range file.Rules {
			ids = append(ids, parsed.Rule.ID)
		}
	}
	return ids
}

// TestBrowsersAndAppsRules pins the browsers and Electron apps of the v1 set:
// the closed process each names, the risk the spec gives, and the cache
// directories each target asks for. The chrome rule the set already holds
// re-enters the same table, so one table stays one lie detector.
func TestBrowsersAndAppsRules(t *testing.T) {
	six := map[string]struct {
		closed  string
		targets []string
	}{
		"browsers.chrome.cache":  {"chrome.exe", []string{`Cache`, `Code Cache`, `GPUCache`}},
		"browsers.edge.cache":    {"msedge.exe", []string{`Cache`, `Code Cache`, `GPUCache`}},
		"browsers.firefox.cache": {"firefox.exe", []string{`cache2`}},
		"apps.vscode.cache":      {"Code.exe", []string{`Cache`, `CachedData`, `CachedExtensionVSIXs`, `Code Cache`, `GPUCache`}},
		"apps.discord.cache":     {"Discord.exe", []string{`Cache`, `Code Cache`, `GPUCache`}},
		"apps.slack.cache":       {"slack.exe", []string{`Cache`, `Code Cache`, `GPUCache`}},
	}
	byID := map[string]*model.Rule{}
	for _, file := range builtinFiles(t) {
		for _, parsed := range file.Rules {
			byID[parsed.Rule.ID] = parsed.Rule
		}
	}
	for id, spec := range six {
		rule, ok := byID[id]
		if !ok {
			t.Errorf("%s is not among the built-in rules", id)
			continue
		}
		if rule.Risk != model.RiskSafe {
			t.Errorf("%s: risk = %q, want safe", id, rule.Risk)
		}
		if !rule.DefaultEnabled {
			t.Errorf("%s: DefaultEnabled = false, want true", id)
		}
		if !slices.Equal(rule.RequiresClosed, []string{spec.closed}) {
			t.Errorf("%s: requires_closed = %v, want [%s]", id, rule.RequiresClosed, spec.closed)
		}
		paths := ruleTargetPaths(t, rule)
		for _, want := range spec.targets {
			if !slices.Contains(paths, want) {
				t.Errorf("%s: no target hinting at %q (paths %v)", id, want, paths)
			}
		}
	}
}

// ruleTargetPaths collects the last path segment of each path target, the
// piece the closed-app overlay table above compares against.
func ruleTargetPaths(t *testing.T, rule *model.Rule) []string {
	t.Helper()
	var out []string
	for i := range rule.Targets {
		if rule.Targets[i].Kind == model.KindPath {
			out = append(out, filepath.Base(rule.Targets[i].Path))
		}
	}
	return out
}

// TestBuiltinRulesFeedTheProfiles pins what the profiles of stage 1 read: which
// built-in rules the default profile takes, and which tag the dev profile
// selects by.
func TestBuiltinRulesFeedTheProfiles(t *testing.T) {
	var rules []*model.Rule
	for _, file := range builtinFiles(t) {
		for _, parsed := range file.Rules {
			rules = append(rules, parsed.Rule)
		}
	}

	byID := map[string]*model.Rule{}
	for _, rule := range rules {
		byID[rule.ID] = rule
	}

	for _, id := range []string{"browsers.chrome.cache", "system.user-temp"} {
		rule, ok := byID[id]
		if !ok {
			t.Fatalf("%s is not among the built-in rules", id)
		}
		if !rule.DefaultEnabled {
			t.Errorf("%s: DefaultEnabled = false, want it in the default profile", id)
		}
	}

	npm, ok := byID["dev.npm-cache"]
	if !ok {
		t.Fatal("dev.npm-cache is not among the built-in rules")
	}
	if npm.DefaultEnabled {
		t.Error("dev.npm-cache: DefaultEnabled = true, want it out of the default profile")
	}
	if !slices.Contains(npm.Tags, "dev") {
		t.Errorf("dev.npm-cache: Tags = %v, want the dev tag", npm.Tags)
	}
}

func TestReadBuiltinReadsTheCarrier(t *testing.T) {
	if Builtin() == nil {
		t.Fatal("Builtin() is nil: the rules are not embedded")
	}
	names, err := fs.Glob(Builtin(), "*/*.yaml")
	if err != nil {
		t.Fatalf("glob the embedded rules: %v", err)
	}
	for _, carried := range []string{"browsers/chrome.yaml", "dev/npm.yaml", "system/temp.yaml"} {
		if !slices.Contains(names, carried) {
			t.Errorf("the embedded set lost %s, has %v", carried, names)
		}
	}
}
