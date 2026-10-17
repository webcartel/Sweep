package rules

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/webcartel/sweep/internal/model"
)

// riskRank orders the three risks, so that a ceiling can be compared with them.
// A risk that is not one of them is not a ceiling at all, which is what an empty
// MaxRisk is.
var riskRank = map[model.Risk]int{
	model.RiskSafe:      0,
	model.RiskModerate:  1,
	model.RiskDangerous: 2,
}

// Select returns the rules a profile takes, in the order the set keeps them.
//
// The switches of the config win over the profile: a rule the config turned off
// stays out even when the profile takes it, and one it turned on comes in even
// when the profile would not.
//
// A mask of the profile that does not compile takes nothing and is reported
// through the warnings, so that a typo is visible instead of quietly selecting
// the wrong set of rules.
func Select(set *Set, profileName string) ([]*model.Rule, []string, error) {
	profile, ok := set.Profiles[profileName]
	if !ok {
		return nil, nil, fmt.Errorf("unknown profile '%s'; the profiles are %s",
			profileName, profileNames(set))
	}

	warnings := unbuildableMasks(profileName, profile)

	var selected []*model.Rule
	for _, rule := range set.Rules {
		// An exclude mask wins over every selector and over the switch.
		if masksTake(profile.Exclude, rule.ID) {
			continue
		}
		if toggle, switched := set.Toggles[rule.ID]; switched {
			if !toggle.Enabled {
				continue
			}
		} else if !picked(rule, profile) {
			continue
		}
		if ceiling, ceilingSet := riskRank[profile.MaxRisk]; ceilingSet && riskRank[rule.Risk] > ceiling {
			continue
		}
		selected = append(selected, rule)
	}
	return selected, warnings, nil
}

// picked says whether the selectors of a profile take the rule. A profile with
// no selector at all takes every rule, which is how aggressive is written.
func picked(rule *model.Rule, profile model.Profile) bool {
	if len(profile.Include) == 0 && len(profile.IncludeTags) == 0 &&
		len(profile.IncludeCategories) == 0 && !profile.DefaultOnly {
		return true
	}
	if profile.DefaultOnly && rule.DefaultEnabled {
		return true
	}
	if slices.ContainsFunc(rule.Tags, func(tag string) bool { return slices.Contains(profile.IncludeTags, tag) }) {
		return true
	}
	if slices.Contains(profile.IncludeCategories, rule.Category) {
		return true
	}
	return masksTake(profile.Include, rule.ID)
}

// masksTake says whether any mask takes the id. A mask that does not compile
// takes nothing; unbuildableMasks reports it.
func masksTake(masks []string, id string) bool {
	for _, mask := range masks {
		if matched, err := path.Match(mask, id); err == nil && matched {
			return true
		}
	}
	return false
}

// unbuildableMasks names every mask of a profile that is not a valid glob.
func unbuildableMasks(profileName string, profile model.Profile) []string {
	var warnings []string

	for _, mask := range append(slices.Clone(profile.Include), profile.Exclude...) {
		if _, err := path.Match(mask, ""); err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"the profile '%s' mask '%s' is not a valid glob: %v", profileName, mask, err))
		}
	}
	return warnings
}

// profileNames lists the profiles a set holds, in a fixed order, for the message
// about an unknown one.
func profileNames(set *Set) string {
	return strings.Join(slices.Sorted(maps.Keys(set.Profiles)), ", ")
}
