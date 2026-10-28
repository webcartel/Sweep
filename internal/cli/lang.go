package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/settings"
)

// langEarly answers the language this run speaks, and the path of the
// settings file it reads. It runs before any command is built, because
// cobra prints a command's help from the text that command was built
// with: a language set after that arrives one step too late.
//
// The chain, strongest first: the --lang flag, the saved setting next to
// the config, the language of the system, English.
func langEarly(d deps, args []string) (i18n.Lang, string, error) {
	cfgPath := argValue(args, "--config")
	path := settingsPathOf(d, cfgPath)

	// A run that answers a parser speaks English all the way through, so
	// the document never changes shape with the interface language.
	if argGiven(args, "--json") {
		return i18n.LangEN, path, nil
	}

	if name, given := langFlag(args); given {
		lang, ok := i18n.Supported(name)
		if !ok {
			return "", path, usageError(fmt.Errorf(
				"sweep speaks %q and %q; %q is not one of them",
				string(i18n.LangEN), string(i18n.LangRU), name))
		}
		return lang, path, nil
	}

	// A setting that names a language sweep has lost is read and skipped,
	// rather than stopping a run over a file the user can repair.
	if name, found, err := settings.Read(path); err == nil && found {
		if lang, ok := i18n.Supported(name); ok {
			return lang, path, nil
		}
	}

	if d.System != nil {
		if lang, ok := i18n.Supported(d.System.UILanguage()); ok {
			return lang, path, nil
		}
	}
	return i18n.LangEN, path, nil
}

// settingsPathOf answers where the settings of this run live: next to the
// config it reads, so that a portable copy of the config carries the
// language along with it.
func settingsPathOf(d deps, cfgPath string) string {
	if cfgPath == "" {
		// A deps with no environment has no data directory to look in,
		// and asking would panic rather than answer.
		if d.Env == nil {
			return ""
		}
		dir, err := dataDir(d)
		if err != nil || dir == "" {
			return ""
		}
		cfgPath = filepath.Join(dir, "config.yaml")
	}
	return filepath.Join(filepath.Dir(cfgPath), "settings.yaml")
}

// argValue answers the value a flag carries, in either spelling cobra
// accepts: `--flag value` and `--flag=value`. An empty answer means the
// flag was not given.
func argValue(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, flag+"=") {
			return strings.TrimPrefix(arg, flag+"=")
		}
	}
	return ""
}

// argGiven says whether a flag is on the command line at all.
func argGiven(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

// langFlag answers the language the flag names, and whether the flag was
// on the line at all. The two differ for `--lang=`, which names the empty
// language: that is a value sweep does not speak, not a flag left out.
func langFlag(args []string) (string, bool) {
	for i, arg := range args {
		if arg == "--lang" {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", true
		}
		if strings.HasPrefix(arg, "--lang=") {
			return strings.TrimPrefix(arg, "--lang="), true
		}
	}
	return "", false
}
