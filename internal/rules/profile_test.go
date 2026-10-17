package rules

import (
	"slices"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/model"
)

// probeRule builds a rule that carries just what Select reads.
func probeRule(id, category string, risk model.Risk, on bool, tags ...string) *model.Rule {
	return &model.Rule{
		ID:             id,
		Name:           id,
		Category:       category,
		Risk:           risk,
		DefaultEnabled: on,
		Tags:           tags,
	}
}

// probeSet builds a set that holds one profile and the given rules.
func probeSet(name string, profile model.Profile, rules ...*model.Rule) *Set {
	return &Set{
		Rules:    rules,
		Profiles: map[string]model.Profile{name: profile},
		Toggles:  map[string]model.RuleToggle{},
	}
}

// selectedIDs lists the ids of the rules Select returned, in order.
func selectedIDs(rules []*model.Rule) []string {
	ids := make([]string, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	return ids
}

func TestSelectDefaultProfileTakesDefaultEnabled(t *testing.T) {
	set := probeSet("default", BuiltinProfiles()["default"],
		probeRule("browsers.chrome.cache", "browsers", model.RiskSafe, true),
		probeRule("dev.npm-cache", "dev", model.RiskSafe, false, "dev"),
	)

	selected, warnings, err := Select(set, "default")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("got warnings %v, want none", warnings)
	}
	if got := selectedIDs(selected); !slices.Equal(got, []string{"browsers.chrome.cache"}) {
		t.Errorf("selected %v, want the rule that is on by default", got)
	}
}

func TestSelectDevProfileAddsTaggedRules(t *testing.T) {
	set := probeSet("dev", BuiltinProfiles()["dev"],
		probeRule("browsers.chrome.cache", "browsers", model.RiskSafe, true),
		probeRule("dev.npm-cache", "dev", model.RiskSafe, false, "dev"),
		probeRule("system.crash-dumps", "system", model.RiskModerate, false),
	)

	selected, _, err := Select(set, "dev")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	want := []string{"browsers.chrome.cache", "dev.npm-cache"}
	if got := selectedIDs(selected); !slices.Equal(got, want) {
		t.Errorf("selected %v, want %v: the default rules plus the tag", got, want)
	}
}

func TestSelectAggressiveProfileExcludesDangerous(t *testing.T) {
	set := probeSet("aggressive", BuiltinProfiles()["aggressive"],
		probeRule("system.user-temp", "system", model.RiskSafe, true),
		probeRule("system.crash-dumps", "system", model.RiskModerate, false),
		probeRule("system.prefetch", "system", model.RiskDangerous, false),
	)

	selected, _, err := Select(set, "aggressive")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	want := []string{"system.user-temp", "system.crash-dumps"}
	if got := selectedIDs(selected); !slices.Equal(got, want) {
		t.Errorf("selected %v, want %v: everything but dangerous", got, want)
	}
}

func TestSelectUnknownProfileIsAnError(t *testing.T) {
	set := probeSet("probe", model.Profile{},
		probeRule("system.user-temp", "system", model.RiskSafe, true),
	)

	_, _, err := Select(set, "nope")
	if err == nil {
		t.Fatal("Select accepted a profile the set does not hold")
	}
	for _, want := range []string{"nope", "probe"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %s", err, want)
		}
	}
}

func TestSelectDisabledRuleIsExcluded(t *testing.T) {
	set := probeSet("default", BuiltinProfiles()["default"],
		probeRule("browsers.chrome.cache", "browsers", model.RiskSafe, true),
		probeRule("system.user-temp", "system", model.RiskSafe, true),
		probeRule("dev.go-build-cache", "dev", model.RiskSafe, false),
	)
	set.Toggles["system.user-temp"] = model.RuleToggle{Enabled: false}
	set.Toggles["dev.go-build-cache"] = model.RuleToggle{Enabled: true}

	selected, _, err := Select(set, "default")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	want := []string{"browsers.chrome.cache", "dev.go-build-cache"}
	if got := selectedIDs(selected); !slices.Equal(got, want) {
		t.Errorf("selected %v, want %v: off is off, and on brings a rule back", got, want)
	}
}

func TestSelectExcludeMaskWins(t *testing.T) {
	set := probeSet("mine", model.Profile{
		Include:           []string{"dev.*"},
		IncludeCategories: []string{"system"},
		Exclude:           []string{"dev.npm-*"},
	},
		probeRule("dev.npm-cache", "dev", model.RiskSafe, false),
		probeRule("dev.yarn-cache", "dev", model.RiskSafe, false),
		probeRule("system.user-temp", "system", model.RiskSafe, true),
	)
	set.Toggles["dev.npm-cache"] = model.RuleToggle{Enabled: true}

	selected, _, err := Select(set, "mine")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	want := []string{"dev.yarn-cache", "system.user-temp"}
	if got := selectedIDs(selected); !slices.Equal(got, want) {
		t.Errorf("selected %v, want %v: the exclude mask wins over both selectors and the switch", got, want)
	}
}

func TestSelectWarnsAboutAMaskThatDoesNotCompile(t *testing.T) {
	set := probeSet("mine", model.Profile{Include: []string{"[bad"}, Exclude: []string{"[worse"}},
		probeRule("dev.npm-cache", "dev", model.RiskSafe, true),
	)

	selected, warnings, err := Select(set, "mine")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(selected) != 0 {
		t.Errorf("selected %v, want nothing: a mask that does not compile takes nothing", selectedIDs(selected))
	}
	if len(warnings) != 2 {
		t.Fatalf("got %d warnings, want 2, one for each mask: %v", len(warnings), warnings)
	}
	for i, mask := range []string{"[bad", "[worse"} {
		if !strings.Contains(warnings[i], mask) {
			t.Errorf("warning = %q, want it to name the mask %q", warnings[i], mask)
		}
	}
}
