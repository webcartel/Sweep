package rules

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/model"
)

// configDir holds config files: one that is read, and broken ones.
const configDir = "../../testdata/config"

// writeConfig puts a config file in a directory of the test's own.
func writeConfig(t *testing.T, source string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write the config: %v", err)
	}
	return path
}

func TestLoadConfigReadsTogglesAndProfiles(t *testing.T) {
	config, err := LoadConfig(filepath.Join(configDir, "valid.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(config.Rules) != 2 {
		t.Fatalf("got %d toggles, want 2: %v", len(config.Rules), config.Rules)
	}
	if config.Rules["browsers.chrome.cache"].Enabled {
		t.Error("browsers.chrome.cache is on, want it switched off")
	}
	if !config.Rules["system.user-temp"].Enabled {
		t.Error("system.user-temp is off, want it switched on")
	}
	if want := []string{`D:\Games`, `%USERPROFILE%\Videos`}; !slices.Equal(config.ExcludePaths, want) {
		t.Errorf("ExcludePaths = %v, want %v", config.ExcludePaths, want)
	}
	if !config.AllowUserCommands {
		t.Error("AllowUserCommands = false, want true")
	}

	if len(config.Profiles) != 1 {
		t.Fatalf("got %d profiles, want 1: %v", len(config.Profiles), config.Profiles)
	}
	mine, ok := config.Profiles["mine"]
	if !ok {
		t.Fatalf("no profile 'mine' among %v", config.Profiles)
	}
	if want := []string{"dev.*", "browsers.chrome.cache"}; !slices.Equal(mine.Include, want) {
		t.Errorf("Include = %v, want %v", mine.Include, want)
	}
	if want := []string{"dev"}; !slices.Equal(mine.IncludeTags, want) {
		t.Errorf("IncludeTags = %v, want %v", mine.IncludeTags, want)
	}
	if want := []string{"system"}; !slices.Equal(mine.IncludeCategories, want) {
		t.Errorf("IncludeCategories = %v, want %v", mine.IncludeCategories, want)
	}
	if mine.MaxRisk != model.RiskModerate {
		t.Errorf("MaxRisk = %q, want %q", mine.MaxRisk, model.RiskModerate)
	}
	if want := []string{"dev.docker-prune"}; !slices.Equal(mine.Exclude, want) {
		t.Errorf("Exclude = %v, want %v", mine.Exclude, want)
	}
	if mine.DefaultOnly {
		t.Error("DefaultOnly = true; a profile written in the config does not ask for the default set")
	}
}

func TestLoadConfigUnknownFieldHasPosition(t *testing.T) {
	path := filepath.Join(configDir, "invalid", "unknown-field.yaml")

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig accepted a field the format does not have")
	}
	var finding Error
	if !errors.As(err, &finding) {
		t.Fatalf("error = %v, want an Error naming the place", err)
	}
	if finding.Line != 4 || finding.Col != 1 {
		t.Errorf("error at %d:%d, want 4:1 (the field itself)", finding.Line, finding.Col)
	}
	if !strings.Contains(finding.Msg, "excluded_paths") {
		t.Errorf("message = %q, want it to name the field", finding.Msg)
	}
}

func TestLoadConfigRejectsUnknownFieldsInNestedMappings(t *testing.T) {
	tests := []struct {
		name   string
		source string
		field  string
	}{
		{
			name:   "inside a toggle",
			source: "rules:\n  browsers.chrome.cache:\n    enable: false\n",
			field:  "enable",
		},
		{
			name:   "inside a profile",
			source: "profiles:\n  mine:\n    maxrisk: moderate\n",
			field:  "maxrisk",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, test.source))
			if err == nil {
				t.Fatal("LoadConfig accepted a field the format does not have")
			}
			if !strings.Contains(err.Error(), test.field) {
				t.Errorf("error = %v, want it to name %q", err, test.field)
			}
		})
	}
}

// A key that is written twice is a finding: the config is read once, so a
// repeated key would silently leave whichever half the reader happened to
// reach last.
func TestLoadConfigRejectsAKeyThatIsWrittenTwice(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "a field of the config",
			source: "exclude_paths: ['D:\\Games']\nexclude_paths: ['D:\\Films']\n",
		},
		{
			name:   "a field of a profile",
			source: "profiles:\n  mine:\n    max_risk: safe\n    max_risk: moderate\n",
		},
		{
			name: "a rule that is switched twice",
			source: "rules:\n  browsers.chrome.cache:\n    enabled: false\n" +
				"  browsers.chrome.cache:\n    enabled: true\n",
		},
		{
			name:   "a profile that is written twice",
			source: "profiles:\n  mine:\n    max_risk: safe\n  mine:\n    max_risk: moderate\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, test.source))
			if err == nil {
				t.Fatal("LoadConfig accepted a key that is written twice")
			}
			if !strings.Contains(err.Error(), "twice") {
				t.Errorf("error = %v, want it to say the key is written twice", err)
			}
		})
	}
}

func TestLoadConfigRejectsTheWrongShape(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "a list instead of a mapping", source: "- rules\n"},
		{name: "rules that is not a mapping", source: "rules:\n  - browsers.chrome.cache\n"},
		{name: "a toggle that is not a mapping", source: "rules:\n  browsers.chrome.cache: false\n"},
		{name: "a toggle that is not a bool", source: "rules:\n  browsers.chrome.cache:\n    enabled: 'false'\n"},
		{name: "profiles that are not a mapping", source: "profiles:\n  - mine\n"},
		{name: "a profile that is not a mapping", source: "profiles:\n  mine: fast\n"},
		{name: "exclude_paths that is not a list", source: "exclude_paths: 'D:\\Games'\n"},
		{name: "allow_user_commands that is not a bool", source: "allow_user_commands: 'true'\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LoadConfig(writeConfig(t, test.source)); err == nil {
				t.Fatal("LoadConfig accepted a file of the wrong shape")
			}
		})
	}
}

func TestLoadConfigRejectsUnknownMaxRisk(t *testing.T) {
	path := filepath.Join(configDir, "invalid", "bad-profile.yaml")

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig accepted a risk that is not one of the three")
	}
	var finding Error
	if !errors.As(err, &finding) {
		t.Fatalf("error = %v, want an Error naming the place", err)
	}
	if finding.Line != 3 || finding.Col != 15 {
		t.Errorf("error at %d:%d, want 3:15 (the risk)", finding.Line, finding.Col)
	}
	if !strings.Contains(finding.Msg, "risky") {
		t.Errorf("message = %q, want it to quote the value", finding.Msg)
	}
}

func TestLoadConfigReportsBrokenYAML(t *testing.T) {
	path := writeConfig(t, "rules:\n\t- broken\n")

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig accepted a file it cannot read")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %v, want it to name the config file", err)
	}
}

func TestLoadConfigWithoutAFileIsEmpty(t *testing.T) {
	config, err := LoadConfig(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig on a machine that has no config yet: %v", err)
	}
	if len(config.Rules) != 0 || len(config.Profiles) != 0 || len(config.ExcludePaths) != 0 {
		t.Errorf("got %+v, want an empty config", config)
	}
	if config.AllowUserCommands {
		t.Error("AllowUserCommands = true; the default is false")
	}
}

func TestBuiltinProfiles(t *testing.T) {
	profiles := BuiltinProfiles()
	if len(profiles) != 3 {
		t.Fatalf("got %d built-in profiles, want 3: %v", len(profiles), profiles)
	}

	if !profiles["default"].DefaultOnly {
		t.Error("default: the profile does not take the rules with default_enabled")
	}

	dev := profiles["dev"]
	if !dev.DefaultOnly || !slices.Contains(dev.IncludeTags, "dev") {
		t.Errorf("dev = %+v, want the default set plus the dev tag", dev)
	}

	if got := profiles["aggressive"].MaxRisk; got != model.RiskModerate {
		t.Errorf("aggressive.MaxRisk = %q, want %q: everything but dangerous", got, model.RiskModerate)
	}
}
