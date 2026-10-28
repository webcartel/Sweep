package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// scanJSONOn runs `sweep scan --json` over the fake machine whose system
// reports the language given, and answers what it printed.
func scanJSONOn(t *testing.T, systemLang string) string {
	t.Helper()
	restore := i18n.Current()
	t.Cleanup(func() { i18n.Use(restore) })

	d := scanDeps(t)
	sys := fake.NewSystem()
	sys.SetUILanguage(systemLang)
	d.System = sys

	args := []string{"scan", "--json"}
	cmd := newRootCmd(d, args)
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sweep scan --json on a %s system: %v", systemLang, err)
	}
	return out.String()
}

// The machine answer keeps its shape whatever the interface says. A parser
// reading `--json` must see one document in either language, so the run
// that answers a parser speaks English end to end.
func TestJSONAnswerIsTheSameInBothLanguages(t *testing.T) {
	english := scanJSONOn(t, "en")
	russian := scanJSONOn(t, "ru")

	if english == "" {
		t.Fatal("scan --json printed nothing")
	}
	if english != russian {
		t.Fatalf("the --json answer changed with the language:\nen: %q\nru: %q", english, russian)
	}
}

// A flag that names the empty language is a mistake to say, not a flag
// left out: `--lang=` must not silently fall back to the system.
func TestEmptyLangIsAUsageError(t *testing.T) {
	d, _ := langMachine(t, "en")

	_, _, err := langEarly(d, []string{"--lang="})
	if err == nil {
		t.Fatal("langEarly accepted the empty language")
	}
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitUsage {
		t.Fatalf("err = %v, want a usage error with code %d", err, ExitUsage)
	}
}

// The table a person reads does follow the language.
func TestScanTableFollowsTheInterfaceLanguage(t *testing.T) {
	restore := i18n.Current()
	t.Cleanup(func() { i18n.Use(restore) })

	d := scanDeps(t)
	d.System = fake.NewSystem()

	cmd := newRootCmd(d, []string{"--lang", "en", "scan"})
	cmd.SetArgs([]string{"--lang", "en", "scan"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sweep scan: %v", err)
	}
	english := out.String()

	cmd = newRootCmd(d, []string{"--lang", "ru", "scan"})
	cmd.SetArgs([]string{"--lang", "ru", "scan"})
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sweep scan: %v", err)
	}
	russian := out.String()

	if english == russian {
		t.Fatalf("the scan table did not change with the language: %q", english)
	}
}
