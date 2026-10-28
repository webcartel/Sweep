package i18n

// catalogEN is the English text of every key sweep prints. It is the
// source of truth: the Russian catalog is this one's translation, and a
// key that lives here must live there too.
var catalogEN = map[string]string{
	"app.title": "Sweep - free space safely",

	"tui.button.back": "Back",

	"tui.hint.back": "Esc back",

	"tui.lang.title":      "Language",
	"tui.lang.hint":       "The choice is saved and used from the next run on.",
	"tui.lang.current":    "Now: %s",
	"tui.lang.saved":      "Saved.",
	"tui.lang.savefailed": "The choice could not be saved",
	"tui.lang.nosettings": "There is no settings file to write to.",

	"tui.rules.title":          "Rules & settings",
	"tui.rules.intro":          "Rules decide what sweep looks for. Built-in rules stay read-only.",
	"tui.rules.counts":         "Rules:     %d built-in, %d of yours",
	"tui.rules.profiles":       "Profiles:  %d available",
	"tui.rules.config":         "Config:    %s (%s)",
	"tui.rules.confignotfound": "not found - defaults are used",
	"tui.rules.configfound":    "found",
	"tui.rules.yourrules":      "Your rules: %s",
	"tui.rules.holds":          "What each screen holds:",
	"tui.rules.holds.browse":   "Browse rules   - every rule, its paths and where it came from",
	"tui.rules.holds.config":   "View config    - profiles, switches and excluded paths",
	"tui.rules.holds.lang":     "Language       - the language the screens speak",
	"tui.rules.holds.mine":     "My rules       - your own files: new, edit, delete",
	"tui.rules.undo":           "You can undo an edit: every save leaves a .bak next to the file.",
	"tui.rules.hint.browse":    "B browse",
	"tui.rules.hint.config":    "C config",
	"tui.rules.hint.lang":      "L language",
	"tui.rules.hint.mine":      "U my rules",
	"tui.rules.browse":         "Browse rules",
	"tui.rules.viewconfig":     "View config",
	"tui.rules.language":       "Language",
	"tui.rules.mine":           "My rules",
}
