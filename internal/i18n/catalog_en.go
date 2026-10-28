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

	"registry.note.read":        "cannot read %s: %v",
	"registry.note.var":         "unknown variable %%%s%% in %q",
	"registry.note.relative":    "relative command %q cannot be read as a place on the disk",
	"registry.note.path":        "cannot read %q: %v",
	"registry.note.drive":       "cannot read the drive of %q: %v",
	"registry.note.removable":   "%q is not on a local disk; left alone",
	"registry.note.guard":       "outside the guarded branches",
	"registry.note.subkeys":     "has subkeys",
	"registry.note.backup":      "backup failed: %s",
	"registry.err.backup":       "no backup directory is known",
	"registry.reason.uninstall": "uninstall command not found: %s",
	"registry.reason.program":   "program not found: %s",

	"scan.skip.var":         "the variable %s is not set",
	"scan.skip.mask":        "no path the mask names is on the disk",
	"scan.skip.notfound":    "the path does not exist",
	"scan.skip.contents":    "the target is a file, and the contents mode cleans directories",
	"scan.skip.disappeared": "the path disappeared",
	"scan.skip.build":       "the rule names build %d, and this Windows is build %d",
	"scan.warn.walk":        "the rule could not be walked: %v",

	"guard.err.empty":          "the path is empty",
	"guard.err.var":            "the path %s names %%%s%%, which this machine does not set",
	"guard.err.nopath":         "the path %s names no place at all",
	"guard.err.loop":           "the links of %s lead back to themselves",
	"guard.err.depth":          "the links of %s are more than %d deep",
	"guard.err.disk":           "the path %s is not on a fixed local disk",
	"guard.err.exclude.var":    "exclude_paths %s names %%%s%%, which this machine does not set",
	"guard.err.exclude.nopath": "exclude_paths %s names no place at all",
	"guard.err.exclude.mask":   "exclude_paths %s is a mask, and the list takes paths",
}
