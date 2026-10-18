package report_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/report"
)

func sampleResult() *model.Result {
	stopped := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	done := stopped.Add(2 * time.Minute)
	return &model.Result{
		StartedAt:     stopped,
		FinishedAt:    done,
		FreedBytes:    5000,
		TrashBytes:    3000,
		Removed:       3,
		Skipped:       2,
		Failed:        1,
		PendingReboot: []string{`C:\Users\probe\AppData\Local\busy.log`},
		PerRule: []model.RuleResult{
			{Rule: &model.Rule{ID: "one", Name: "The first"}, Status: model.StatusDone,
				Bytes: 4000, Files: 2},
			{Rule: &model.Rule{ID: "two", Name: "The second"}, Status: model.StatusSkipped,
				Bytes: 1000, Files: 1, Skipped: []model.Skip{
					{Path: `C:\Users\probe\AppData\Local\gone`, Reason: model.ReasonNotFound},
					{Path: `C:\Users\probe\AppData\Local\nulls`, Reason: model.ReasonInUse},
				}},
			{Rule: &model.Rule{ID: "three", Name: "The third"}, Status: model.StatusFailed},
			{Rule: &model.Rule{ID: "four", Name: "The fourth"}, Status: model.StatusDryRun},
		},
	}
}

func TestExecJSONMatchesTheContract(t *testing.T) {
	var out bytes.Buffer
	if err := report.ExecJSON(&out, sampleResult()); err != nil {
		t.Fatalf("ExecJSON: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(out.Bytes(), &document); err != nil {
		t.Fatalf("the document is not JSON: %v", err)
	}
	if document["schema"].(float64) != 1 {
		t.Errorf("schema = %v, want 1", document["schema"])
	}
	if document["dry_run"].(bool) {
		t.Error("dry_run = true for a real run")
	}
	if document["started_at"].(string) != "2026-10-18T09:00:00Z" {
		t.Errorf("started_at = %v", document["started_at"])
	}
	summary := document["summary"].(map[string]any)
	for name, want := range map[string]any{
		"rules": 4.0, "bytes_freed": 5000.0, "bytes_trashed": 3000.0,
		"removed": 3.0, "skipped": 2.0, "failed": 1.0,
	} {
		if summary[name] != want {
			t.Errorf("summary.%s = %v, want %v", name, summary[name], want)
		}
	}
	rules := document["rules"].([]any)
	if len(rules) != 4 {
		t.Fatalf("rules = %d, want one per rule", len(rules))
	}
	first := rules[0].(map[string]any)
	if first["id"].(string) != "one" || first["status"].(string) != "done" ||
		first["bytes"].(float64) != 4000.0 || first["files"].(float64) != 2.0 {
		t.Errorf("the first rule = %v", first)
	}
	second := rules[1].(map[string]any)
	skipped := second["skipped"].([]any)[0].(map[string]any)
	if skipped["reason"].(string) != "not_found" {
		t.Errorf("reason = %v, want not_found", skipped["reason"])
	}
	if _, there := second["skipped"]; !there {
		t.Error("the skips of the second rule are nowhere")
	}
	last := rules[3].(map[string]any)
	if last["status"].(string) != "dry_run" {
		t.Errorf("status of the fourth = %v, want dry_run", last["status"])
	}
	if _, there := document["pending_reboot"]; !there {
		t.Error("pending_reboot is not in the document")
	}
}

func TestExecJSONSaysADryRunTrue(t *testing.T) {
	done := sampleResult()
	done.DryRun = true
	var out bytes.Buffer
	if err := report.ExecJSON(&out, done); err != nil {
		t.Fatalf("ExecJSON: %v", err)
	}
	if !strings.Contains(out.String(), `"dry_run": true`) {
		t.Error("the document does not say the run was dry")
	}
}

func TestLogToWritesOneLinePerEntry(t *testing.T) {
	var out bytes.Buffer
	log := report.LogTo(&out)
	stop := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	entries := []report.Entry{
		{Time: stop, RuleID: "one", Path: `C:\Time\a.cas`, Action: "removed", Size: 100, Result: "ok"},
		{Time: stop.Add(time.Second), RuleID: "two", Path: `C:\Time\b.cas`, Action: "trashed", Size: 200, Result: "ok"},
	}
	for _, entry := range entries {
		if err := log.Append(entry); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d lines, want one per entry", len(lines))
	}
	var one map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &one); err != nil {
		t.Fatalf("the second line is not JSON: %v", err)
	}
	for name, want := range map[string]any{
		"time": "2026-10-18T09:00:01Z", "rule": "two", "path": `C:\Time\b.cas`,
		"action": "trashed", "size": 200.0, "result": "ok",
	} {
		if one[name] != want {
			t.Errorf("%s = %v, want %v", name, one[name], want)
		}
	}
}

func TestLogFileNamesTheDay(t *testing.T) {
	at := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	name := report.LogFile(`C:\Users\probe\AppData\Local`, at)
	if !strings.HasSuffix(name, `\sweep\logs\2026-10-18.jsonl`) {
		t.Errorf("LogFile = %s, want the day in the sweep folder", name)
	}
	if !strings.HasPrefix(name, `C:\Users\probe\AppData\Local`) {
		t.Errorf("LogFile = %s, want it under the folder given", name)
	}
}
