package rules

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/webcartel/sweep/internal/model"
)

// builtinEnv is the environment the built-in rules are written against. A name
// missing here means a built-in rule names a variable that is not there.
var builtinEnv = fakeEnv{
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
	files := builtinFiles(t)
	if len(files) != 3 {
		t.Fatalf("got %d built-in rule files, want 3", len(files))
	}

	var ids []string
	rules := map[string]*File{}
	for _, file := range files {
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
			ids = append(ids, parsed.Rule.ID)
			rules[parsed.Rule.ID] = file
		}
	}

	slices.Sort(ids)
	want := []string{"browsers.chrome.cache", "dev.npm-cache", "system.user-temp"}
	if !slices.Equal(ids, want) {
		t.Errorf("built-in ids = %v, want %v", ids, want)
	}
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
	want := []string{"browsers/chrome.yaml", "dev/npm.yaml", "system/temp.yaml"}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Errorf("embedded files = %v, want %v", names, want)
	}
}
