package rules

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// invalidDir holds rule files that must be rejected, one defect each.
const invalidDir = "../../testdata/rules/invalid"

func loadInvalid(t *testing.T, name string) *File {
	t.Helper()

	path := filepath.Join(invalidDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	file, err := LoadFile(path, data)
	if err != nil {
		t.Fatalf("LoadFile(%s): %v", path, err)
	}
	return file
}

func TestValidateSchemaNamesAnUnknownFieldWithPosition(t *testing.T) {
	const name = "unknown-rule-field.yaml"
	file := loadInvalid(t, name)

	findings := ValidateSchema(file)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
	}

	got := findings[0]
	if want := filepath.Join(invalidDir, name); got.File != want {
		t.Errorf("File = %q, want %q", got.File, want)
	}
	if got.Line != 7 || got.Col != 5 {
		t.Errorf("finding at %d:%d, want 7:5 (the typo's own key)", got.Line, got.Col)
	}
	const phrase = "additional properties 'defalt_enabled' not allowed"
	if got.Msg != phrase {
		t.Errorf("message = %q, want %q", got.Msg, phrase)
	}
	if want := filepath.Join(invalidDir, name) + ":7:5: " + phrase; got.Error() != want {
		t.Errorf("Error() = %q, want %q", got.Error(), want)
	}
}

func TestValidateSchemaRejectsBrokenFiles(t *testing.T) {
	tests := []struct {
		file    string
		line    int
		col     int
		message string
	}{
		{file: "unknown-target-field.yaml", line: 9, col: 9, message: "additional properties 'modes' not allowed"},
		{file: "unknown-top-level-field.yaml", line: 2, col: 1, message: "additional properties 'version' not allowed"},
		{file: "missing-id.yaml", line: 3, col: 5, message: "missing property 'id'"},
		{file: "missing-name.yaml", line: 3, col: 5, message: "missing property 'name'"},
		{file: "missing-category.yaml", line: 3, col: 5, message: "missing property 'category'"},
		{file: "missing-risk.yaml", line: 3, col: 5, message: "missing property 'risk'"},
		{file: "missing-targets.yaml", line: 3, col: 5, message: "missing property 'targets'"},
		{file: "missing-schema.yaml", line: 1, col: 1, message: "missing property 'schema'"},
		{file: "missing-rules.yaml", line: 1, col: 1, message: "missing property 'rules'"},
		{file: "schema-not-1.yaml", line: 1, col: 9, message: "value must be 1"},
		{file: "schema-not-an-integer.yaml", line: 1, col: 9, message: "value must be 1"},
		{file: "bad-risk.yaml", line: 6, col: 11, message: "value must be one of 'safe', 'moderate', 'dangerous'"},
		{file: "risk-not-a-string.yaml", line: 6, col: 11, message: "value must be one of 'safe', 'moderate', 'dangerous'"},
		{file: "bad-kind.yaml", line: 8, col: 15, message: "value must be one of 'path', 'command'"},
		{file: "bad-mode.yaml", line: 8, col: 15, message: "value must be one of 'contents', 'self', 'files'"},
		{file: "rules-not-a-list.yaml", line: 3, col: 3, message: "got object, want array"},
		{file: "rule-not-an-object.yaml", line: 3, col: 5, message: "got string, want object"},
		{file: "targets-not-a-list.yaml", line: 7, col: 14, message: "got string, want array"},
		{file: "targets-empty.yaml", line: 7, col: 14, message: "minItems: got 0, want 1"},
		{file: "target-not-an-object.yaml", line: 8, col: 9, message: "got string, want object"},
		{file: "tags-not-a-list.yaml", line: 7, col: 11, message: "got string, want array"},
		{file: "requires-closed-not-a-list.yaml", line: 7, col: 22, message: "got string, want array"},
		{file: "default-enabled-not-a-boolean.yaml", line: 7, col: 22, message: "got string, want boolean"},
		{file: "os-min-build-not-an-integer.yaml", line: 7, col: 19, message: "got string, want integer"},
		{file: "min-age-days-negative.yaml", line: 9, col: 23, message: "minimum: got -1, want 0"},
		{file: "args-not-a-list.yaml", line: 10, col: 15, message: "got string, want array"},
		{file: "include-not-a-list.yaml", line: 9, col: 18, message: "got string, want array"},
		{file: "id-bad-pattern.yaml", line: 3, col: 9, message: "does not match pattern"},
		{file: "name-empty.yaml", line: 4, col: 11, message: "minLength: got 0, want 1"},
	}

	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			findings := ValidateSchema(loadInvalid(t, test.file))
			if len(findings) == 0 {
				t.Fatal("the schema accepted a broken file")
			}

			got := findings[0]
			if got.Line != test.line || got.Col != test.col {
				t.Errorf("finding at %d:%d, want %d:%d (%s)", got.Line, got.Col, test.line, test.col, got.Msg)
			}
			if !strings.Contains(got.Msg, test.message) {
				t.Errorf("message = %q, want it to contain %q", got.Msg, test.message)
			}
			if want := filepath.Join(invalidDir, test.file); got.File != want {
				t.Errorf("File = %q, want %q", got.File, want)
			}
		})
	}
}

func TestValidateSchemaReportsEveryUnknownFieldOfOneMapping(t *testing.T) {
	findings := ValidateSchema(loadInvalid(t, "two-unknown-fields.yaml"))

	if len(findings) != 2 {
		t.Fatalf("got %d findings, want one per unknown field: %v", len(findings), findings)
	}
	want := []struct {
		line int
		name string
	}{
		{line: 7, name: "defalt_enabled"},
		{line: 8, name: "requires_closed_by"},
	}
	for i, expected := range want {
		if findings[i].Line != expected.line {
			t.Errorf("finding %d at line %d, want %d", i, findings[i].Line, expected.line)
		}
		if !strings.Contains(findings[i].Msg, expected.name) {
			t.Errorf("finding %d message = %q, want it to name %q", i, findings[i].Msg, expected.name)
		}
	}
}

func TestValidateSchemaAcceptsEmptyRules(t *testing.T) {
	file, err := LoadFile("empty-rules.yaml", []byte("schema: 1\nrules: []\n"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if findings := ValidateSchema(file); len(findings) != 0 {
		t.Errorf("got %v, want no findings for a file that holds no rules", findings)
	}
}

func TestValidateSchemaRejectsEmptyFile(t *testing.T) {
	file, err := LoadFile("empty.yaml", nil)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	findings := ValidateSchema(file)
	if len(findings) == 0 {
		t.Fatal("an empty file was accepted")
	}
	if findings[0].File != "empty.yaml" {
		t.Errorf("File = %q, want %q", findings[0].File, "empty.yaml")
	}
	if findings[0].Msg == "" {
		t.Error("the finding says nothing about what is wrong")
	}
}

func TestValidateSchemaChecksTheShapeOfMinSize(t *testing.T) {
	const source = "schema: 1\nrules:\n  - id: user.probe\n    name: Probe\n    category: user\n" +
		"    risk: safe\n    targets:\n      - path: '%TEMP%'\n        min_size: 50XB\n"

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(source), &doc); err != nil {
		t.Fatalf("parse the fixture: %v", err)
	}

	// Through LoadFile this file never reaches the schema: parseSize rejects the
	// size first. The schema still pins the shape for authors and editors.
	findings := ValidateSchema(&File{Path: "test.yaml", Doc: &doc})
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
	}
	if findings[0].Line != 9 || findings[0].Col != 19 {
		t.Errorf("finding at %d:%d, want 9:19 (the size)", findings[0].Line, findings[0].Col)
	}
	if !strings.Contains(findings[0].Msg, "pattern") {
		t.Errorf("message = %q, want it to mention the pattern", findings[0].Msg)
	}
}

// TestValidateReportsOneFindingPerDefect walks every fixture that holds one
// defect. A second finding for one file means a pass reports the consequence of
// the first mistake as a mistake of its own, which sends the reader after
// something that is not there.
func TestValidateReportsOneFindingPerDefect(t *testing.T) {
	// The one fixture that holds two mistakes on purpose.
	want := map[string]int{
		"two-unknown-fields.yaml": 2,
	}

	entries, err := os.ReadDir(invalidDir)
	if err != nil {
		t.Fatalf("read %s: %v", invalidDir, err)
	}

	env := fakeEnv{"TEMP": `C:\Temp`, "LOCALAPPDATA": `C:\Users\probe\AppData\Local`}
	for _, entry := range entries {
		name := entry.Name()
		count := want[name]
		if count == 0 {
			count = 1
		}

		t.Run(name, func(t *testing.T) {
			findings, _ := Validate(loadInvalid(t, name), env)
			if len(findings) != count {
				t.Errorf("got %d findings, want %d: %v", len(findings), count, findings)
			}
		})
	}
}

// TestValidateAcceptsTheValidFixtures keeps the other direction honest: every
// fixture of the valid set passes both passes. unknown-env.yaml sits there to
// show that a variable this machine does not set is a warning, not a finding.
func TestValidateAcceptsTheValidFixtures(t *testing.T) {
	entries, err := os.ReadDir(validDir)
	if err != nil {
		t.Fatalf("read %s: %v", validDir, err)
	}

	env := fakeEnv{"TEMP": `C:\Temp`, "USERPROFILE": `C:\Users\probe`,
		"LOCALAPPDATA": `C:\Users\probe\AppData\Local`}
	for _, entry := range entries {
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			findings, _ := Validate(loadValid(t, name), env)
			if len(findings) != 0 {
				t.Errorf("got findings, want none: %v", findings)
			}
		})
	}
}

// A file the YAML parser refuses is a finding with a place like any other: the
// parser names the line inside its message, and the column is where that line
// begins.
func TestLoadFilePutsAPositionOnABrokenYAMLFile(t *testing.T) {
	path := filepath.Join("../../testdata/rules/broken", "unclosed-quote.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	_, err = LoadFile(path, data)
	if err == nil {
		t.Fatal("LoadFile accepted a file the YAML parser refuses")
	}

	var finding Error
	if !errors.As(err, &finding) {
		t.Fatalf("error = %v, want a finding with a place", err)
	}
	if finding.Line == 0 || finding.Col == 0 {
		t.Errorf("finding at %d:%d, want a line and a column", finding.Line, finding.Col)
	}
	if finding.File != path {
		t.Errorf("File = %q, want the file it was read from", finding.File)
	}
	if strings.Contains(finding.Msg, "yaml") || strings.Contains(finding.Msg, "line ") {
		t.Errorf("message = %q, want it to say what went wrong and nothing more", finding.Msg)
	}
}
