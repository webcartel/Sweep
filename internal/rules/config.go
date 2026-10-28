package rules

import (
	"errors"
	"io/fs"
	"os"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/webcartel/sweep/internal/model"
)

// configFields are the fields a config file holds at its top level.
var configFields = []string{"rules", "exclude_paths", "allow_user_commands", "profiles"}

// profileFields are the fields a profile holds. default_only is not among them:
// it belongs to the built-in profiles, and the spec lists no such field for a
// profile a user writes.
var profileFields = []string{"include", "include_tags", "include_categories", "max_risk", "exclude"}

// BuiltinProfiles returns the profiles the binary ships, each time as a fresh
// map so that a caller may adjust what it gets: default takes every rule with
// default_enabled, dev takes those plus the rules tagged dev, aggressive takes
// everything but dangerous.
func BuiltinProfiles() map[string]model.Profile {
	return map[string]model.Profile{
		"default":    {DefaultOnly: true},
		"dev":        {DefaultOnly: true, IncludeTags: []string{"dev"}},
		"aggressive": {MaxRisk: model.RiskModerate},
	}
}

// LoadConfig reads the user's config. A file that is not there is not an error:
// the first run has no config yet, and every default applies.
func LoadConfig(path string) (model.Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.Config{}, nil
	}
	if err != nil {
		return model.Config{}, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return model.Config{}, syntaxError(path, err)
	}
	root := docRoot(&doc)
	if root == nil {
		return model.Config{}, nil
	}
	if root.Kind != yaml.MappingNode {
		return model.Config{}, errorAt(path, root, "rules.err.config.mapping")
	}
	if err := checkFields(path, root, configFields); err != nil {
		return model.Config{}, err
	}

	ruleNodes, err := mappingOf(path, field(root, "rules"), "rules")
	if err != nil {
		return model.Config{}, err
	}
	toggles, err := readToggles(path, ruleNodes)
	if err != nil {
		return model.Config{}, err
	}

	excluded, err := stringValues(path, field(root, "exclude_paths"), "exclude_paths")
	if err != nil {
		return model.Config{}, err
	}
	allow, err := boolValue(path, field(root, "allow_user_commands"), "allow_user_commands")
	if err != nil {
		return model.Config{}, err
	}

	profileNodes, err := mappingOf(path, field(root, "profiles"), "profiles")
	if err != nil {
		return model.Config{}, err
	}
	profiles, err := readProfiles(path, profileNodes)
	if err != nil {
		return model.Config{}, err
	}

	return model.Config{
		Rules:             toggles,
		ExcludePaths:      excluded,
		AllowUserCommands: allow,
		Profiles:          profiles,
	}, nil
}

// readToggles reads the rules.<id>.enabled entries of a config.
func readToggles(file string, rules *yaml.Node) (map[string]model.RuleToggle, error) {
	if rules == nil {
		return nil, nil
	}

	toggles := map[string]model.RuleToggle{}
	for i := 0; i+1 < len(rules.Content); i += 2 {
		id := rules.Content[i]

		toggle, err := mappingOf(file, rules.Content[i+1], "a rule toggle")
		if err != nil {
			return nil, err
		}
		if err := checkFields(file, toggle, []string{"enabled"}); err != nil {
			return nil, err
		}
		enabled, err := boolValue(file, field(toggle, "enabled"), "enabled")
		if err != nil {
			return nil, err
		}
		if _, seen := toggles[id.Value]; seen {
			return nil, errorAt(file, id, "rules.err.config.twicerule", id.Value)
		}
		toggles[id.Value] = model.RuleToggle{Enabled: enabled}
	}
	return toggles, nil
}

// readProfiles reads the named profiles of a config.
func readProfiles(file string, profiles *yaml.Node) (map[string]model.Profile, error) {
	if profiles == nil {
		return nil, nil
	}

	read := map[string]model.Profile{}
	for i := 0; i+1 < len(profiles.Content); i += 2 {
		name := profiles.Content[i]

		body, err := mappingOf(file, profiles.Content[i+1], "a profile")
		if err != nil {
			return nil, err
		}
		profile, err := readProfile(file, body)
		if err != nil {
			return nil, err
		}
		if _, seen := read[name.Value]; seen {
			return nil, errorAt(file, name, "rules.err.config.twiceprofile", name.Value)
		}
		read[name.Value] = profile
	}
	return read, nil
}

// readProfile reads the fields of one profile.
func readProfile(file string, body *yaml.Node) (model.Profile, error) {
	profile := model.Profile{}

	if err := checkFields(file, body, profileFields); err != nil {
		return model.Profile{}, err
	}

	var err error
	if profile.Include, err = stringValues(file, field(body, "include"), "include"); err != nil {
		return model.Profile{}, err
	}
	if profile.IncludeTags, err = stringValues(file, field(body, "include_tags"), "include_tags"); err != nil {
		return model.Profile{}, err
	}
	if profile.IncludeCategories, err = stringValues(file, field(body, "include_categories"), "include_categories"); err != nil {
		return model.Profile{}, err
	}
	if profile.Exclude, err = stringValues(file, field(body, "exclude"), "exclude"); err != nil {
		return model.Profile{}, err
	}

	if ceiling := field(body, "max_risk"); ceiling != nil {
		switch risk := model.Risk(scalar(ceiling)); risk {
		case model.RiskSafe, model.RiskModerate, model.RiskDangerous:
			profile.MaxRisk = risk
		default:
			return model.Profile{},
				errorAt(file, ceiling, "rules.err.config.maxrisk", scalar(ceiling))
		}
	}
	return profile, nil
}

// checkFields reports a key the format does not have and a key that is written
// twice, so that a typo or a repeated key is named instead of silently ignored.
// The config has no JSON Schema to do it.
func checkFields(file string, mapping *yaml.Node, known []string) error {
	if mapping == nil {
		return nil
	}

	seen := map[string]bool{}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		if !slices.Contains(known, key.Value) {
			return errorAt(file, key, "rules.err.config.unknownfield", key.Value)
		}
		if seen[key.Value] {
			return errorAt(file, key, "rules.err.config.fieldtwice", key.Value)
		}
		seen[key.Value] = true
	}
	return nil
}

// mappingOf returns the mapping a field holds. A field that is not there gives
// nil; a field that holds anything else is an error.
func mappingOf(file string, value *yaml.Node, what string) (*yaml.Node, error) {
	if value == nil {
		return nil, nil
	}
	if value.Kind != yaml.MappingNode {
		return nil, errorAt(file, value, "rules.err.config.mappingwhat", what)
	}
	return value, nil
}

// boolValue reads a bool and reports anything else, which the schema-less
// config cannot leave to a later check.
func boolValue(file string, value *yaml.Node, what string) (bool, error) {
	if value == nil {
		return false, nil
	}

	var parsed bool
	if err := value.Decode(&parsed); err != nil {
		return false, errorAt(file, value, "rules.err.config.bool", what)
	}
	return parsed, nil
}

// stringValues reads a list of strings and reports anything else.
func stringValues(file string, value *yaml.Node, what string) ([]string, error) {
	if value == nil {
		return nil, nil
	}

	var parsed []string
	if err := value.Decode(&parsed); err != nil {
		return nil, errorAt(file, value, "rules.err.config.stringlist", what)
	}
	return parsed, nil
}
