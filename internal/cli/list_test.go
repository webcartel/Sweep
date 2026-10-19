package cli

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// listed is one rule of the `sweep list --json` document, as the test reads it.
type listed struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Category      string `json:"category"`
	Risk          string `json:"risk"`
	InProfile     bool   `json:"in_profile"`
	RequiresAdmin bool   `json:"requires_admin"`
	Source        string `json:"source"`
}

// listDocument is the whole `sweep list --json` answer.
type listDocument struct {
	Schema  int      `json:"schema"`
	Profile string   `json:"profile"`
	Rules   []listed `json:"rules"`
}

// readList runs `sweep list --json` and decodes the document it printed.
func readList(t *testing.T, args ...string) listDocument {
	t.Helper()

	out, errOut, err := execute(t, append([]string{"list", "--json"}, args...)...)
	if err != nil {
		t.Fatalf("sweep list: %v", err)
	}
	if errOut != "" {
		t.Errorf("sweep list wrote %q to stderr, want nothing", errOut)
	}

	var doc listDocument
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("sweep list --json printed %q, which is not one JSON document: %v", out, err)
	}
	return doc
}

// listedIDs lists the ids of a list document, in order.
func listedIDs(doc listDocument) []string {
	ids := make([]string, 0, len(doc.Rules))
	for _, rule := range doc.Rules {
		ids = append(ids, rule.ID)
	}
	return ids
}

func TestListCommandPrintsBuiltinRules(t *testing.T) {
	// The machine's own config and rules must not change what the test reads,
	// so the command gets a data directory of the test's own.
	t.Setenv("APPDATA", t.TempDir())

	doc := readList(t)

	if doc.Schema != 1 {
		t.Errorf("schema = %d, want 1", doc.Schema)
	}
	if doc.Profile != "default" {
		t.Errorf("profile = %q, want default", doc.Profile)
	}
	want := []string{"browsers.chrome.cache", "dev.npm-cache", "system.user-temp"}
	for _, id := range want {
		if !slices.Contains(listedIDs(doc), id) {
			t.Errorf("the three pioneers are not all there: %s is not in %v", id, listedIDs(doc))
		}
	}

	var chrome struct {
		Risk          string
		Source        string
		RequiresAdmin bool
		InProfile     bool
	}
	var foundChrome bool
	for _, rule := range doc.Rules {
		if rule.ID == "browsers.chrome.cache" {
			chrome.Risk = rule.Risk
			chrome.Source = rule.Source
			chrome.RequiresAdmin = rule.RequiresAdmin
			chrome.InProfile = rule.InProfile
			foundChrome = true
			break
		}
	}
	if !foundChrome {
		t.Fatal("the Chrome cache is not in the list")
	}
	if chrome.Risk != "safe" {
		t.Errorf("browsers.chrome.cache: risk = %q, want safe", chrome.Risk)
	}
	if chrome.Source != "builtin" {
		t.Errorf("browsers.chrome.cache: source = %q, want builtin", chrome.Source)
	}
	if chrome.RequiresAdmin {
		t.Error("browsers.chrome.cache: requires_admin = true, want false")
	}
	if !chrome.InProfile {
		t.Error("browsers.chrome.cache: in_profile = false, want it in the default profile")
	}
	for _, rule := range doc.Rules {
		if rule.ID == "dev.npm-cache" && rule.InProfile {
			t.Error("dev.npm-cache: in_profile = true, want it out of the default profile")
		}
	}
}

func TestListCommandFiltersByCategory(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	doc := readList(t, "--category", "dev")
	got := listedIDs(doc)
	if len(got) != 8 || !strings.Contains(strings.Join(got, ","), "dev.npm-cache") {
		t.Errorf("listed %v, want the dev rules of the v1 set (npm among them)", got)
	}

	empty := readList(t, "--category", "nope")
	if len(empty.Rules) != 0 {
		t.Errorf("listed %v, want nothing for a category no rule has", listedIDs(empty))
	}

	out, _, err := execute(t, "list", "--json", "--category", "nope")
	if err != nil {
		t.Fatalf("sweep list: %v", err)
	}
	if !strings.Contains(out, `"rules": []`) {
		t.Errorf("sweep list --json printed %q, want an empty list rather than null", out)
	}
}

func TestListCommandTakesTheProfileItIsGiven(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	doc := readList(t, "--profile", "dev")
	if doc.Profile != "dev" {
		t.Errorf("profile = %q, want dev", doc.Profile)
	}
	var npm listed
	for _, rule := range doc.Rules {
		if rule.ID == "dev.npm-cache" {
			npm = rule
		}
	}
	if npm.ID == "" {
		t.Fatalf("dev.npm-cache is not in %v", listedIDs(doc))
	}
	if !npm.InProfile {
		t.Error("dev.npm-cache: in_profile = false, want it in the dev profile")
	}

	_, _, err := execute(t, "list", "--json", "--profile", "nope")
	if err == nil {
		t.Fatal("sweep list accepted a profile that is not there")
	}
	if code := exitCode(t, err); code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
}

func TestListCommandPrintsATable(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	out, errOut, err := execute(t, "list")
	if err != nil {
		t.Fatalf("sweep list: %v", err)
	}
	if errOut != "" {
		t.Errorf("sweep list wrote %q to stderr, want nothing", errOut)
	}

	for _, want := range []string{"ID", "RISK", "PROFILE", "ADMIN", "browsers.chrome.cache", "dev.npm-cache", "system.user-temp"} {
		if !strings.Contains(out, want) {
			t.Errorf("sweep list printed %q, want it to hold %q", out, want)
		}
	}
}
