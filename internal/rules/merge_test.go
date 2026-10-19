package rules

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/webcartel/sweep/internal/model"
)

const (
	userRuleDir  = "../../testdata/rules/user"
	extraRuleDir = "../../testdata/rules/extra"
)

// userLoader reads the user's own rules with commands allowed, so that the
// command rule of the fixture directory does not stop the read.
func userLoader() Loader {
	return Loader{
		Env:     builtinEnv,
		UserDir: userRuleDir,
		Config:  model.Config{AllowUserCommands: true},
	}
}

// idsOf lists the ids of a set in the order the set keeps them.
func idsOf(set *Set) []string {
	ids := make([]string, 0, len(set.Rules))
	for _, rule := range set.Rules {
		ids = append(ids, rule.ID)
	}
	return ids
}

// findRule returns the rule of a set by id.
func findRule(set *Set, id string) (*model.Rule, bool) {
	for _, rule := range set.Rules {
		if rule.ID == id {
			return rule, true
		}
	}
	return nil, false
}

// writeRuleDir puts rule files in a directory of the test's own.
func writeRuleDir(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// ruleFileWithID builds a valid rule file that defines one rule of that id.
func ruleFileWithID(id string) string {
	return "schema: 1\nrules:\n  - id: " + id +
		"\n    name: Probe\n    category: user\n    risk: safe" +
		"\n    targets:\n      - path: '%LOCALAPPDATA%\\probe'\n"
}

func TestLoaderCarriesTheConfig(t *testing.T) {
	loader := userLoader()
	loader.Config.ExcludePaths = []string{`C:\Games`}

	set, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []string{`C:\Games`}; !reflect.DeepEqual(set.Config.ExcludePaths, want) {
		t.Errorf("set.Config.ExcludePaths = %q, want %q", set.Config.ExcludePaths, want)
	}
}

func TestLoaderUserRuleReplacesBuiltinWithWarning(t *testing.T) {
	set, err := userLoader().Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(set.Warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(set.Warnings), set.Warnings)
	}
	warning := set.Warnings[0]
	if !strings.Contains(warning, "browsers.chrome.cache") || !strings.Contains(warning, "override-chrome.yaml") {
		t.Errorf("warning = %q, want it to name the rule and the file that replaces it", warning)
	}

	chrome, ok := findRule(set, "browsers.chrome.cache")
	if !ok {
		t.Fatal("the user's rule is not in the set")
	}
	if chrome.Name != "Chrome cache (mine)" {
		t.Errorf("Name = %q, want the user's rule whole", chrome.Name)
	}
	if want := filepath.Join(userRuleDir, "override-chrome.yaml"); chrome.Source != want {
		t.Errorf("Source = %q, want %q", chrome.Source, want)
	}
	if len(chrome.Targets) != 1 {
		t.Errorf("got %d targets, want the user's single target, not a merged pair", len(chrome.Targets))
	}
	if got := len(set.Rules); got != builtinRuleCount(t)+1 {
		t.Errorf("the set holds %d rules, want %d: the built-in ids and the user's command rule", got, builtinRuleCount(t)+1)
	}
}

func TestLoaderKeepsFirstAppearanceOrder(t *testing.T) {
	loader := userLoader()
	loader.ExtraDirs = []string{extraRuleDir}

	set, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// The embed's order first (the user's override keeps
	// browsers.chrome.cache in its built-in place), then the user's
	// directory, then the extra one.
	want := slices.Concat(builtinIDs(t), []string{"user.probe-command", "extra.probe"})
	if got := idsOf(set); !slices.Equal(got, want) {
		t.Errorf("rule order = %v, want %v", got, want)
	}
}

func TestLoaderRejectsUserCommandRuleWithoutConfig(t *testing.T) {
	loader := Loader{Env: builtinEnv, UserDir: userRuleDir}

	_, err := loader.Load()
	if err == nil {
		t.Fatal("Load accepted a user rule that runs a command")
	}
	for _, want := range []string{"user.probe-command", "allow_user_commands"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %s", err, want)
		}
	}
}

func TestLoaderAcceptsUserCommandRuleWhenAllowed(t *testing.T) {
	set, err := userLoader().Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	rule, ok := findRule(set, "user.probe-command")
	if !ok {
		t.Fatal("the user's command rule is not in the set")
	}
	if len(rule.Targets) != 1 {
		t.Fatalf("got %d targets, want 1", len(rule.Targets))
	}
	target := rule.Targets[0]
	if target.Kind != model.KindCommand || target.Cmd != "my-tool.exe" || !slices.Equal(target.Args, []string{"--clean"}) {
		t.Errorf("target = %+v, want the command my-tool.exe with its arguments", target)
	}
}

func TestLoaderReportsBrokenUserFile(t *testing.T) {
	dir := writeRuleDir(t, map[string]string{"broken.yaml": `schema: 1
rules:
  - id: user.broken
    name: Broken
    category: user
    risk: safe
    nope: 1
    targets:
      - path: '%LOCALAPPDATA%\probe'
`})

	_, err := Loader{Env: builtinEnv, UserDir: dir}.Load()
	if err == nil {
		t.Fatal("Load accepted a rule file with a field the format does not have")
	}

	var finding Error
	if !errors.As(err, &finding) {
		t.Fatalf("error = %v, want an Error naming the place", err)
	}
	if finding.Line != 7 || finding.Col != 5 {
		t.Errorf("error at %d:%d, want 7:5 (the field itself)", finding.Line, finding.Col)
	}
	if !strings.Contains(finding.File, "broken.yaml") {
		t.Errorf("error names %q, want the file it came from", finding.File)
	}
}

func TestLoaderRejectsDuplicateIDInsideOneSource(t *testing.T) {
	dir := writeRuleDir(t, map[string]string{
		"a.yaml": ruleFileWithID("user.same"),
		"b.yaml": ruleFileWithID("user.same"),
	})

	_, err := Loader{Env: builtinEnv, UserDir: dir}.Load()
	if err == nil {
		t.Fatal("Load accepted one id defined twice in the same source")
	}
	var finding Error
	if !errors.As(err, &finding) {
		t.Fatalf("error = %v, want an Error naming the place", err)
	}
	if !strings.Contains(finding.Error(), "user.same") || !strings.Contains(finding.Error(), "a.yaml") {
		t.Errorf("error = %v, want it to name the id and the file that defined it first", finding)
	}
}

func TestLoaderReadsExtraDirs(t *testing.T) {
	set, err := Loader{Env: builtinEnv, ExtraDirs: []string{extraRuleDir}}.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := findRule(set, "extra.probe"); !ok {
		t.Errorf("extra.probe is not in the set: %v", idsOf(set))
	}

	t.Run("a directory named on the command line must be there", func(t *testing.T) {
		loader := Loader{Env: builtinEnv, ExtraDirs: []string{filepath.Join(t.TempDir(), "nope")}}

		if _, err := loader.Load(); err == nil {
			t.Fatal("Load accepted a rules directory that is not there")
		}
	})

	t.Run("a machine without user rules reads the built-in ones", func(t *testing.T) {
		loader := Loader{Env: builtinEnv, UserDir: filepath.Join(t.TempDir(), "rules")}

		set, err := loader.Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got := len(set.Rules); got != builtinRuleCount(t) {
			t.Errorf("the set holds %d rules, want the %d built-in ones", got, builtinRuleCount(t))
		}
	})
}

func TestLoaderExtraDirWinsOverUser(t *testing.T) {
	extra := writeRuleDir(t, map[string]string{"win.yaml": `schema: 1
rules:
  - id: browsers.chrome.cache
    name: Chrome cache (extra)
    category: extra
    risk: safe
    targets:
      - path: '%LOCALAPPDATA%\Google\Chrome\User Data\*\GPUCache'
        mode: contents
`})

	loader := userLoader()
	loader.ExtraDirs = []string{extra}
	set, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	chrome, ok := findRule(set, "browsers.chrome.cache")
	if !ok {
		t.Fatal("no rule with the id both sources define")
	}
	if chrome.Name != "Chrome cache (extra)" {
		t.Errorf("Name = %q, want the rule of the later source", chrome.Name)
	}

	if len(set.Warnings) != 2 {
		t.Fatalf("got %d warnings, want 2: %v", len(set.Warnings), set.Warnings)
	}
	if !strings.Contains(set.Warnings[1], "win.yaml") {
		t.Errorf("the last warning = %q, want it to name the extra file", set.Warnings[1])
	}
}

func TestLoaderReadsAnInjectedBuiltinFS(t *testing.T) {
	loader := Loader{
		Env: builtinEnv,
		Builtin: fstest.MapFS{
			"probe/probe.yaml": &fstest.MapFile{Data: []byte(ruleFileWithID("probe.one"))},
		},
	}

	set, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := idsOf(set); !slices.Equal(got, []string{"probe.one"}) {
		t.Errorf("rule ids = %v, want the one rule of the given file system", got)
	}
}

func TestLoaderTakesConfigProfilesAndToggles(t *testing.T) {
	loader := Loader{
		Env: builtinEnv,
		Config: model.Config{
			Rules:    map[string]model.RuleToggle{"nope.nope": {Enabled: true}},
			Profiles: map[string]model.Profile{"default": {IncludeTags: []string{"mine"}}},
		},
	}

	set, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !set.Toggles["nope.nope"].Enabled {
		t.Errorf("the config toggle is not in the set: %v", set.Toggles)
	}
	if want := []string{"mine"}; !slices.Equal(set.Profiles["default"].IncludeTags, want) {
		t.Errorf("the default profile = %+v, want the one the config wrote", set.Profiles["default"])
	}
	if got := len(set.Profiles); got != 3 {
		t.Errorf("got %d profiles, want 3: the config replaces a built-in one instead of adding to it", got)
	}

	if len(set.Warnings) != 2 {
		t.Fatalf("got %d warnings, want 2: %v", len(set.Warnings), set.Warnings)
	}
	if !strings.Contains(set.Warnings[0], "nope.nope") || !strings.Contains(set.Warnings[1], "default") {
		t.Errorf("warnings = %v, want one about the unknown rule and one about the replaced profile", set.Warnings)
	}
}
