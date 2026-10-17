package rules

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzLoadFile keeps the parser honest about what a rule file may contain. Rule
// files arrive from other people — the spec expects a set built by pull
// requests — and the parser is the first thing they touch.
func FuzzLoadFile(f *testing.F) {
	f.Add("schema: 1\nrules:\n  - id: user.probe\n    name: Probe\n    category: user\n" +
		"    risk: safe\n    targets:\n      - path: '%TEMP%'\n")
	f.Add("schema: 1\nrules:\n  - id: user.probe\n    name: Probe\n    category: user\n" +
		"    risk: safe\n    targets:\n      - path: 'C:\\Temp'\n        min_size: 5MB\n" +
		"        min_age_days: 3\n        include: ['*.log']\n")
	f.Add("schema: 1\nrules:\n  - id: a.b\n    targets:\n      - kind: command\n        cmd: cmd.exe\n" +
		"        args: ['/c', 'echo']\n")
	f.Add("schema: 1\nrules: []\n")
	f.Add("rules:\n  - {}\n")
	f.Add("\ufeffschema: 1\nrules: []\n")
	f.Add("")

	f.Fuzz(func(t *testing.T, source string) {
		file, err := LoadFile("probe.yaml", []byte(source))
		if err != nil {
			return
		}
		// What matters is that both passes answer for anything the parser
		// accepted, whatever the answer is.
		findings, warnings := Validate(file, fakeEnv{})
		t.Logf("%d findings, %d warnings", len(findings), len(warnings))
	})
}

// FuzzLoadConfig does the same for the config, the one input of the project
// that no JSON Schema guards: every field is read and checked by hand.
func FuzzLoadConfig(f *testing.F) {
	f.Add("rules:\n  browsers.chrome.cache:\n    enabled: false\n")
	f.Add("profiles:\n  mine:\n    max_risk: safe\n    include_tags: [dev]\n")
	f.Add("exclude_paths: ['D:\\Games']\nallow_user_commands: true\n")
	f.Add("rules: []\nprofiles: []\n")
	f.Add("")

	f.Fuzz(func(t *testing.T, source string) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Skipf("write the config: %v", err)
		}
		_, _ = LoadConfig(path)
	})
}
