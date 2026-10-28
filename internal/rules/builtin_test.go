package rules

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/model"
)

// builtinEnv is the environment the built-in rules are written against. A name
// missing here means a built-in rule names a variable that is not there.
var builtinEnv = fakeEnv{
	"APPDATA":      `C:\Users\probe\AppData\Roaming`,
	"TEMP":         `C:\Users\probe\AppData\Local\Temp`,
	"LOCALAPPDATA": `C:\Users\probe\AppData\Local`,
	"SystemRoot":   `C:\Windows`,
	"SystemDrive":  `C:`,
	"ProgramData":  `C:\ProgramData`,
	"USERPROFILE":  `C:\Users\probe`,
}

// builtinFiles reads the built-in rules and fails the test if they do not read.
func builtinFiles(t *testing.T) []*File {
	t.Helper()

	files, err := ReadBuiltin()
	if err != nil {
		t.Fatalf("ReadBuiltin: %v", err)
	}
	return files
}

func TestBuiltinRulesAreValid(t *testing.T) {
	for _, file := range builtinFiles(t) {
		if findings := ValidateSchema(file); len(findings) != 0 {
			t.Errorf("%s does not fit the schema: %v", file.Path, findings)
		}
		findings, warnings := ValidateSemantics(file, builtinEnv)
		if len(findings) != 0 {
			t.Errorf("%s has semantic findings: %v", file.Path, findings)
		}
		if len(warnings) != 0 {
			t.Errorf("%s warns: %v", file.Path, warnings)
		}
		if len(file.Rules) == 0 {
			t.Errorf("%s holds no rules", file.Path)
		}
		for _, parsed := range file.Rules {
			if parsed.Rule.Source != SourceBuiltin {
				t.Errorf("%s: Source = %q, want %q", file.Path, parsed.Rule.Source, SourceBuiltin)
			}
			if parsed.Rule.ID == "" {
				t.Errorf("%s holds a rule without an id", file.Path)
			}
		}
	}
}

// builtinRuleCount answers how many rules the embed carries, whatever files
// the v1 set holds by now.
func builtinRuleCount(t *testing.T) int {
	t.Helper()
	count := 0
	for _, file := range builtinFiles(t) {
		count += len(file.Rules)
	}
	return count
}

// builtinIDs answers the ids of the built-in set, in the embed's order.
func builtinIDs(t *testing.T) []string {
	t.Helper()
	var ids []string
	for _, file := range builtinFiles(t) {
		for _, parsed := range file.Rules {
			ids = append(ids, parsed.Rule.ID)
		}
	}
	return ids
}

// TestSystemPathRules pins the nine path rules of the system group: the risk,
// the profile membership (with the admin flag) and the shape of each target.
func TestSystemPathRules(t *testing.T) {
	byID := map[string]*model.Rule{}
	for _, file := range builtinFiles(t) {
		for _, parsed := range file.Rules {
			byID[parsed.Rule.ID] = parsed.Rule
		}
	}
	type want struct {
		admin bool
		risk  model.Risk
		modes map[string]model.Mode // path -> mode
		age   map[string]int        // path -> min_age_days
	}
	spec := map[string]want{
		"system.user-temp": {risk: model.RiskSafe, modes: map[string]model.Mode{`%TEMP%`: ""}},
		"system.windows-temp": {
			admin: true, risk: model.RiskSafe,
			modes: map[string]model.Mode{`%SystemRoot%\Temp`: ""},
			age:   map[string]int{`%SystemRoot%\Temp`: 1},
		},
		"system.wer-user": {risk: model.RiskSafe, modes: map[string]model.Mode{`%LOCALAPPDATA%\Microsoft\Windows\WER`: ""}},
		"system.wer-machine": {
			admin: true, risk: model.RiskSafe,
			modes: map[string]model.Mode{
				`%ProgramData%\Microsoft\Windows\WER\ReportArchive`: "",
				`%ProgramData%\Microsoft\Windows\WER\ReportQueue`:   "",
			},
		},
		"system.cbs-logs": {
			admin: true, risk: model.RiskSafe,
			modes: map[string]model.Mode{`%SystemRoot%\Logs\CBS`: model.ModeFiles},
			age:   map[string]int{`%SystemRoot%\Logs\CBS`: 14},
		},
		"system.crash-dumps": {risk: model.RiskModerate,
			modes: map[string]model.Mode{`%LOCALAPPDATA%\CrashDumps`: ""}},
		"system.minidumps": {
			admin: true, risk: model.RiskModerate,
			modes: map[string]model.Mode{
				`%SystemRoot%\Minidump`:   "",
				`%SystemRoot%\MEMORY.DMP`: model.ModeSelf,
			},
		},
		"system.windows-update-cache": {
			admin: true, risk: model.RiskModerate,
			modes: map[string]model.Mode{`%SystemRoot%\SoftwareDistribution\Download`: ""},
		},
		"system.delivery-optimization": {
			admin: true, risk: model.RiskSafe,
			modes: map[string]model.Mode{`%SystemRoot%\ServiceProfiles\NetworkService\AppData\Local\Microsoft\Windows\DeliveryOptimization\Cache`: ""},
		},
		"system.thumbnail-cache": {
			risk:  model.RiskSafe,
			modes: map[string]model.Mode{`%LOCALAPPDATA%\Microsoft\Windows\Explorer\thumbcache_*.db`: model.ModeFiles},
		},
	}
	profileMembership := map[string]bool{
		"system.user-temp": true, "system.windows-temp": true,
		"system.wer-user": true, "system.wer-machine": true,
		"system.cbs-logs": true,
	}
	for _, id := range []string{"system.user-temp", "system.windows-temp", "system.wer-user",
		"system.wer-machine", "system.cbs-logs", "system.crash-dumps", "system.minidumps",
		"system.windows-update-cache", "system.delivery-optimization", "system.thumbnail-cache"} {
		rule, ok := byID[id]
		if !ok {
			t.Errorf("%s is not among the built-in rules", id)
			continue
		}
		spec := spec[id]
		if rule.Risk != spec.risk {
			t.Errorf("%s: risk = %q, want %q", id, rule.Risk, spec.risk)
		}
		if rule.DefaultEnabled != profileMembership[id] {
			t.Errorf("%s: DefaultEnabled = %v", id, rule.DefaultEnabled)
		}
		if rule.RequiresAdmin != spec.admin {
			t.Errorf("%s: requires_admin = %v, want %v", id, rule.RequiresAdmin, spec.admin)
		}
		checkTargetPaths(t, id, rule.Targets, spec.modes, spec.age)

	}
}

// checkTargetPaths pins the modes and ages of the paths a rule names.
func checkTargetPaths(t *testing.T, id string, targets []model.Target, modes map[string]model.Mode, ages map[string]int) {
	t.Helper()
	var seen []string
	for _, target := range targets {
		if target.Kind != model.KindPath {
			t.Errorf("%s: %q is not a path target but %q", id, target.Path, target.Kind)
			continue
		}
		seen = append(seen, target.Path)
		if want, named := modes[target.Path]; named && target.Mode != want &&
			(want != "" || target.Mode != model.ModeContents) {
			t.Errorf("%s: %q mode = %q, want %q", id, target.Path, target.Mode, want)
		}
		if wantAge, named := ages[target.Path]; named && target.MinAgeDays != wantAge {
			t.Errorf("%s: %q min_age_days = %d, want %d", id, target.Path, target.MinAgeDays, wantAge)
		}
	}
	if len(seen) != len(modes) {
		t.Errorf("%s: holds %d targets, want the %d named", id, len(seen), len(modes))
	}
	for path := range modes {
		if !slices.Contains(seen, path) {
			t.Errorf("%s: no target for %q", id, path)
		}
	}
}

// TestSystemCommandAndDevRules pins the command rules of the system group and
// the dev caches: the process and arguments the two commands name, the dev tag
// every cache carries, and the risks the spec gives.
func TestSystemCommandAndDevRules(t *testing.T) {
	byID := map[string]*model.Rule{}
	for _, file := range builtinFiles(t) {
		for _, parsed := range file.Rules {
			byID[parsed.Rule.ID] = parsed.Rule
		}
	}
	type want struct {
		risk  model.Risk
		admin bool
		cmd   string
		args  []string
	}
	spec := map[string]want{
		"system.recycle-bin": {
			risk: model.RiskModerate,
			cmd:  "powershell.exe",
			args: []string{"-NoProfile", "-Command", "Clear-RecycleBin -Force"},
		},
		"system.component-cleanup": {
			risk:  model.RiskModerate,
			admin: true,
			cmd:   "dism.exe",
			args:  []string{"/Online", "/Cleanup-Image", "/StartComponentCleanup"},
		},
	}
	for id, spec := range spec {
		rule, ok := byID[id]
		if !ok {
			t.Errorf("%s is not among the built-in rules", id)
			continue
		}
		if rule.Risk != spec.risk {
			t.Errorf("%s: risk = %q, want %q", id, rule.Risk, spec.risk)
		}
		if rule.DefaultEnabled {
			t.Errorf("%s: DefaultEnabled = true, want it out of the default profile", id)
		}
		if rule.RequiresAdmin != spec.admin {
			t.Errorf("%s: requires_admin = %v, want %v", id, rule.RequiresAdmin, spec.admin)
		}
		if len(rule.Targets) != 1 {
			t.Fatalf("%s: got %d targets, want the one command", id, len(rule.Targets))
		}
		target := rule.Targets[0]
		if target.Kind != model.KindCommand {
			t.Fatalf("%s: kind = %q, want command", id, target.Kind)
		}
		if target.Cmd != spec.cmd {
			t.Errorf("%s: cmd = %q, want %q", id, target.Cmd, spec.cmd)
		}
		if !slices.Equal(target.Args, spec.args) {
			t.Errorf("%s: args = %q, want %q", id, target.Args, spec.args)
		}
	}
	dev := map[string]struct {
		risk model.Risk
	}{
		"dev.npm-cache":      {model.RiskSafe},
		"dev.pnpm-store":     {model.RiskModerate},
		"dev.yarn-cache":     {model.RiskSafe},
		"dev.pip-cache":      {model.RiskSafe},
		"dev.go-build-cache": {model.RiskSafe},
		"dev.nuget-cache":    {model.RiskSafe},
		"dev.gradle-caches":  {model.RiskModerate},
		"dev.docker-prune":   {model.RiskDangerous},
	}
	for _, id := range []string{"dev.npm-cache", "dev.pnpm-store", "dev.yarn-cache", "dev.pip-cache",
		"dev.go-build-cache", "dev.nuget-cache", "dev.gradle-caches", "dev.docker-prune"} {
		rule, ok := byID[id]
		if !ok {
			t.Errorf("%s is not among the built-in rules", id)
			continue
		}
		if rule.Risk != dev[id].risk {
			t.Errorf("%s: risk = %q, want %q", id, rule.Risk, dev[id].risk)
		}
		if !slices.Equal(rule.Tags, []string{"dev"}) {
			t.Errorf("%s: tags = %v, want [dev]", id, rule.Tags)
		}
		if rule.DefaultEnabled {
			t.Errorf("%s: DefaultEnabled = true, want it out of the default profile", id)
		}
	}
	docker, ok := byID["dev.docker-prune"]
	if !ok {
		t.Fatal("dev.docker-prune is not among the built-in rules")
	}
	if len(docker.Targets) != 1 || docker.Targets[0].Kind != model.KindCommand || docker.Targets[0].Cmd != "docker" ||
		!slices.Equal(docker.Targets[0].Args, []string{"system", "prune", "-f"}) {
		t.Errorf("dev.docker-prune: targets = %+v, want one command docker system prune -f", docker.Targets)
	}

	// nuget names two targets, gradle keeps the whole caches folder.
	nuget, ok := byID["dev.nuget-cache"]
	if !ok {
		t.Fatal("dev.nuget-cache is not among the built-in rules")
	}
	if len(nuget.Targets) != 2 {
		t.Errorf("dev.nuget-cache holds %d targets, want the store and the scratch", len(nuget.Targets))
	}
}

// builtinRuleIDs is every id the built-in set carries. The set-shape test
// holds this list against whatever files the embed carries, so a typo in one
// yaml id or a lost file shows up by name. The v1 spec fixed 26 of them; the
// rules added after it are listed here too, because the point of the test is
// that the set changes only on purpose.
var builtinRuleIDs = []string{
	"browsers.chrome.cache", "browsers.edge.cache", "browsers.firefox.cache",
	"system.user-temp", "system.windows-temp", "system.wer-user", "system.wer-machine",
	"system.cbs-logs", "system.crash-dumps", "system.minidumps",
	"system.windows-update-cache", "system.delivery-optimization", "system.thumbnail-cache",
	"system.recycle-bin", "system.component-cleanup", "system.windows-old",
	"dev.npm-cache", "dev.pnpm-store", "dev.yarn-cache", "dev.pip-cache",
	"dev.go-build-cache", "dev.nuget-cache", "dev.gradle-caches", "dev.docker-prune",
	"apps.vscode.cache", "apps.discord.cache", "apps.slack.cache",
	"apps.obsidian.cache", "apps.notion.cache", "apps.telegram.cache", "apps.steam.cache",
	"downloads.old-installers",
}

// TestBuiltinSetShape pins the whole built-in set: one rule per id, no more
// and none lost.
func TestBuiltinSetShape(t *testing.T) {
	ids := builtinIDs(t)
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	seen := map[string]bool{}
	for _, id := range sorted {
		if seen[id] {
			t.Errorf("the built-in set holds %s twice", id)
		}
		seen[id] = true
	}
	want := slices.Clone(builtinRuleIDs)
	slices.Sort(want)
	if !slices.Equal(sorted, want) {
		t.Errorf("the built-in set = %v, want %d ids %v", sorted, len(want), want)
	}
}

// TestBrowsersAndAppsRules pins the browsers and Electron apps: the closed
// process each names, the risk, and the cache directories each target asks
// for. The chrome rule the set already holds re-enters the same table, so one
// table stays one lie detector.
func TestBrowsersAndAppsRules(t *testing.T) {
	pinned := map[string]struct {
		closed  string
		targets []string
	}{
		"browsers.chrome.cache":  {"chrome.exe", []string{`Cache`, `Code Cache`, `GPUCache`}},
		"browsers.edge.cache":    {"msedge.exe", []string{`Cache`, `Code Cache`, `GPUCache`}},
		"browsers.firefox.cache": {"firefox.exe", []string{`cache2`}},
		"apps.vscode.cache": {"Code.exe", []string{`Cache`, `CachedData`, `CachedExtensionVSIXs`,
			`Code Cache`, `GPUCache`, `Crashpad`, `DawnWebGPUCache`, `DawnGraphiteCache`}},
		"apps.discord.cache":  {"Discord.exe", []string{`Cache`, `Code Cache`, `GPUCache`}},
		"apps.slack.cache":    {"slack.exe", []string{`Cache`, `Code Cache`, `GPUCache`}},
		"apps.obsidian.cache": {"Obsidian.exe", []string{`Cache`, `Code Cache`, `GPUCache`}},
		"apps.notion.cache": {"Notion.exe", []string{`Cache`, `Code Cache`, `GPUCache`,
			`DawnCache`, `notionAssetCache-v2`}},
		"apps.telegram.cache": {"Telegram.exe", []string{`cache`, `media_cache`, `webview`, `emoji`, `dumps`}},
		"apps.steam.cache":    {"steam.exe", []string{`htmlcache`}},
	}
	byID := map[string]*model.Rule{}
	for _, file := range builtinFiles(t) {
		for _, parsed := range file.Rules {
			byID[parsed.Rule.ID] = parsed.Rule
		}
	}
	for id, spec := range pinned {
		rule, ok := byID[id]
		if !ok {
			t.Errorf("%s is not among the built-in rules", id)
			continue
		}
		if rule.Risk != model.RiskSafe {
			t.Errorf("%s: risk = %q, want safe", id, rule.Risk)
		}
		if !rule.DefaultEnabled {
			t.Errorf("%s: DefaultEnabled = false, want true", id)
		}
		if !slices.Equal(rule.RequiresClosed, []string{spec.closed}) {
			t.Errorf("%s: requires_closed = %v, want [%s]", id, rule.RequiresClosed, spec.closed)
		}
		paths := ruleTargetPaths(t, rule)
		for _, want := range spec.targets {
			if !slices.Contains(paths, want) {
				t.Errorf("%s: no target hinting at %q (paths %v)", id, want, paths)
			}
		}
	}
}

// ruleTargetPaths collects the last segment of each path target, the piece
// the closed-app overlay table above compares against. Windows paths are
// split by hand: filepath.Base reads a backslash as a plain letter away from
// Windows, and the CI runs these tests on Linux too.
func ruleTargetPaths(t *testing.T, rule *model.Rule) []string {
	t.Helper()
	var out []string
	for i := range rule.Targets {
		if rule.Targets[i].Kind == model.KindPath {
			out = append(out, winBase(rule.Targets[i].Path))
		}
	}
	return out
}

// winBase answers the last segment of a path written with either separator.
func winBase(path string) string {
	if i := strings.LastIndexAny(path, `\\/`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// TestBuiltinRulesFeedTheProfiles pins what the profiles of stage 1 read: which
// built-in rules the default profile takes, and which tag the dev profile
// selects by.
func TestBuiltinRulesFeedTheProfiles(t *testing.T) {
	var rules []*model.Rule
	for _, file := range builtinFiles(t) {
		for _, parsed := range file.Rules {
			rules = append(rules, parsed.Rule)
		}
	}

	byID := map[string]*model.Rule{}
	for _, rule := range rules {
		byID[rule.ID] = rule
	}

	for _, id := range []string{"browsers.chrome.cache", "system.user-temp"} {
		rule, ok := byID[id]
		if !ok {
			t.Fatalf("%s is not among the built-in rules", id)
		}
		if !rule.DefaultEnabled {
			t.Errorf("%s: DefaultEnabled = false, want it in the default profile", id)
		}
	}

	npm, ok := byID["dev.npm-cache"]
	if !ok {
		t.Fatal("dev.npm-cache is not among the built-in rules")
	}
	if npm.DefaultEnabled {
		t.Error("dev.npm-cache: DefaultEnabled = true, want it out of the default profile")
	}
	if !slices.Contains(npm.Tags, "dev") {
		t.Errorf("dev.npm-cache: Tags = %v, want the dev tag", npm.Tags)
	}
}

func TestReadBuiltinReadsTheCarrier(t *testing.T) {
	if Builtin() == nil {
		t.Fatal("Builtin() is nil: the rules are not embedded")
	}
	names, err := fs.Glob(Builtin(), "*/*.yaml")
	if err != nil {
		t.Fatalf("glob the embedded rules: %v", err)
	}
	for _, carried := range []string{"browsers/chrome.yaml", "dev/npm.yaml", "system/temp.yaml"} {
		if !slices.Contains(names, carried) {
			t.Errorf("the embedded set lost %s, has %v", carried, names)
		}
	}
}
