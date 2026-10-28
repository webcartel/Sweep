package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/platform"
)

// Options tunes one run of the cleaner.
type Options struct {
	// DryRun counts and says what would happen without touching a key.
	DryRun bool
	// Elevated says the process may change HKLM.
	Elevated bool
	// BackupRoot is where the .reg copies go; it is required unless DryRun.
	BackupRoot string
	// Now moves the clock of the backup directory name.
	Now func() time.Time
	// Log gets one line for each finding, already worded as an action.
	Log func(action, path, note string)
}

// Entry says what happened to one finding.
type Entry struct {
	Finding Finding
	Action  string // planned, deleted, skipped, failed
	Note    string
}

// Result is what one run of the cleaner leaves behind.
type Result struct {
	BackupDir string
	Removed   int
	Skipped   int
	Failed    int
	Entries   []Entry
}

// Select answers the findings named by the selectors. A selector is
// `kind:key[:value]`, compared without case. The second answer names the
// selectors that matched nothing.
func Select(findings []Finding, selectors []string) ([]Finding, []string) {
	var out []Finding
	var missing []string
	used := make([]bool, len(selectors))
	for _, finding := range findings {
		for i, selector := range selectors {
			if matches(finding, selector) {
				out = append(out, finding)
				used[i] = true
				break
			}
		}
	}
	for i, selector := range selectors {
		if !used[i] {
			missing = append(missing, selector)
		}
	}
	return out, missing
}

// matches says whether one selector names the finding.
func matches(finding Finding, selector string) bool {
	parts := strings.SplitN(strings.TrimSpace(selector), ":", 3)
	if len(parts) < 2 {
		return false
	}
	if !strings.EqualFold(parts[0], string(finding.Kind)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(parts[1]), finding.Key) {
		return false
	}
	if finding.Kind != KindRun {
		return true
	}
	if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
		return strings.EqualFold(strings.TrimSpace(parts[2]), finding.Value)
	}
	return finding.Value == ""
}

// Clean exports a backup of every key it will touch, writes the manifest,
// and only then deletes. A failed export cancels the deletions of its key.
func Clean(reg platform.Registry, findings []Finding, opts Options) (Result, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Log == nil {
		opts.Log = func(string, string, string) {}
	}
	result := Result{}
	if opts.DryRun {
		for _, finding := range findings {
			result.Entries = append(result.Entries, Entry{Finding: finding, Action: "planned"})
		}
		return result, nil
	}
	if opts.BackupRoot == "" {
		return result, fmt.Errorf("%s", i18n.T("registry.err.backup"))
	}
	dir, err := backupDir(opts.BackupRoot, opts.Now())
	if err != nil {
		return result, err
	}
	result.BackupDir = dir

	// Decide what can be touched at all, before any export.
	type job struct {
		finding Finding
		entry   Entry
	}
	var jobs []job
	blocked := map[string]string{}
	for _, finding := range findings {
		entry := Entry{Finding: finding}
		switch {
		case !Guarded(finding.Key):
			entry.Action, entry.Note = "failed", i18n.T("registry.note.guard")
		case finding.HasSubkeys:
			entry.Action, entry.Note = "skipped", i18n.T("registry.note.subkeys")
		case finding.Admin && !opts.Elevated:
			entry.Action, entry.Note = "skipped", "needs_admin"
		default:
			entry.Action = "pending"
		}
		if entry.Action == "pending" {
			jobs = append(jobs, job{finding: finding, entry: entry})
			continue
		}
		result.Entries = append(result.Entries, entry)
		countEntry(&result, entry)
		opts.Log("registry_"+entry.Action, keyLine(finding), entry.Note)
	}
	if len(jobs) == 0 {
		if len(findings) == 0 {
			return result, nil
		}
		return result, nil
	}

	// Export every key once, before anything goes.
	backupOf := map[string]string{}
	exported := map[string]bool{}
	for i := range jobs {
		finding := jobs[i].finding
		key := finding.Key
		if exported[strings.ToLower(key)] {
			continue
		}
		exported[strings.ToLower(key)] = true
		file := filepath.Join(dir, fmt.Sprintf("backup-%02d.reg", len(backupOf)+1))
		if err := reg.Export(key, file); err != nil {
			blocked[strings.ToLower(key)] = err.Error()
			continue
		}
		backupOf[strings.ToLower(key)] = file
	}
	var planned []Finding
	for _, job := range jobs {
		planned = append(planned, job.finding)
	}
	if err := writeManifest(dir, planned, backupOf); err != nil {
		return result, err
	}
	for _, job := range jobs {
		finding := job.finding
		entry := Entry{Finding: finding}
		if note, ok := blocked[strings.ToLower(finding.Key)]; ok {
			entry.Action, entry.Note = "failed", i18n.T("registry.note.backup", note)
			result.Entries = append(result.Entries, entry)
			countEntry(&result, entry)
			opts.Log("registry_failed", keyLine(finding), entry.Note)
		}
	}

	for i := range jobs {
		finding := jobs[i].finding
		if _, ok := blocked[strings.ToLower(finding.Key)]; ok {
			continue
		}
		var err error
		if finding.DeleteValue {
			err = reg.DeleteValue(finding.Key, finding.Value)
		} else {
			err = reg.DeleteKey(finding.Key)
		}
		entry := Entry{Finding: finding, Action: "deleted"}
		if err != nil {
			entry.Action, entry.Note = "failed", err.Error()
			opts.Log("registry_failed", keyLine(finding), entry.Note)
		} else {
			opts.Log("registry_delete", keyLine(finding), "")
		}
		result.Entries = append(result.Entries, entry)
		countEntry(&result, entry)
	}
	sort.SliceStable(result.Entries, func(i, j int) bool { return result.Entries[i].Finding.ID < result.Entries[j].Finding.ID })
	return result, nil
}

// countEntry moves one entry into the counters.
func countEntry(result *Result, entry Entry) {
	switch entry.Action {
	case "deleted":
		result.Removed++
	case "skipped":
		result.Skipped++
	case "failed":
		result.Failed++
	}
}

// keyLine names the key (and value) a log line is about.
func keyLine(finding Finding) string {
	if finding.Value == "" {
		return finding.Key
	}
	return finding.Key + `\` + finding.Value
}

// backupDir makes the directory of this run, with a suffix when two runs
// share a second.
func backupDir(root string, at time.Time) (string, error) {
	base := filepath.Join(root, at.Format("20060102-150405"))
	dir := base
	for i := 2; ; i++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			break
		}
		dir = fmt.Sprintf("%s-%d", base, i)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// manifestEntry is one line of the manifest next to the backups.
type manifestEntry struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	Value  string `json:"value"`
	Backup string `json:"backup"`
}

// writeManifest puts the map from a finding to its .reg file next to them.
func writeManifest(dir string, findings []Finding, backupOf map[string]string) error {
	entries := make([]manifestEntry, 0, len(findings))
	for _, finding := range findings {
		entries = append(entries, manifestEntry{
			ID: finding.ID, Kind: string(finding.Kind),
			Key: finding.Key, Value: finding.Value,
			Backup: filepath.Base(backupOf[strings.ToLower(finding.Key)]),
		})
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600)
}
