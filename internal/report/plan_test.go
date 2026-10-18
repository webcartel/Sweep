package report_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/report"
)

// samplePlan is the plan every JSON test reads: one path rule with two items
// and one skip, one command rule, one rule whose targets were not there.
func samplePlan() *model.Plan {
	old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return &model.Plan{
		CreatedAt: old,
		Rules: []model.RulePlan{
			{Rule: &model.Rule{ID: "apps.probe.cache"}, Bytes: 110, Files: 3,
				Items: []model.Item{
					{Path: `C:\Cache\a`, Size: 105, Files: 2, IsDir: true, ModTime: old},
					{Path: `C:\Cache\d.tmp`, Size: 5, Files: 1, ModTime: old},
				},
				Skipped: []model.Skip{{Path: `C:\Cache\gone`, Reason: model.ReasonNotFound, Detail: "the path does not exist"}}},
			{Rule: &model.Rule{ID: "dev.npm.cache", Targets: []model.Target{
				{Kind: model.KindCommand, Cmd: "npm", Args: []string{"cache", "clean"}}}},
				Command: &model.CommandPlan{Command: "npm", Args: []string{"cache", "clean"}}},
			{Rule: &model.Rule{ID: "apps.absent"}, Skipped: []model.Skip{
				{Path: `%LOCALAPPDATA%\absent`, Reason: model.ReasonNotFound}}},
		},
	}
}

func TestPlanJSON(t *testing.T) {
	var out bytes.Buffer
	if err := report.PlanJSON(&out, samplePlan()); err != nil {
		t.Fatalf("PlanJSON: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(out.Bytes(), &document); err != nil {
		t.Fatalf("the document is not JSON: %v\n%s", err, out.String())
	}

	if document["schema"] != float64(1) {
		t.Errorf("schema = %v, want 1", document["schema"])
	}
	if document["dry_run"] != true {
		t.Errorf("dry_run = %v, want true", document["dry_run"])
	}
	if len(document["rules"].([]any)) != 3 {
		t.Errorf("rules = %v, want three entries", document["rules"])
	}

	first := document["rules"].([]any)[0].(map[string]any)
	if first["id"] != "apps.probe.cache" {
		t.Errorf("id = %v, want apps.probe.cache", first["id"])
	}
	if got := first["bytes"].(float64); got != 110 {
		t.Errorf("bytes = %v, want 110", got)
	}
	if got := first["files"].(float64); got != 3 {
		t.Errorf("files = %v, want 3", got)
	}
	items := first["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %v, want two entries", items)
	}
	dir := items[0].(map[string]any)
	if dir["path"] != `C:\Cache\a` || dir["size"] != float64(105) ||
		dir["files"] != float64(2) || dir["is_dir"] != true || dir["reparse"] != false {
		t.Errorf("item = %v, want the directory entry", dir)
	}
	if dir["mod_time"] != "2026-01-02T03:04:05Z" {
		t.Errorf("mod_time = %v, want the time in RFC 3339", dir["mod_time"])
	}
	file := items[1].(map[string]any)
	if file["is_dir"] != false || file["files"] != float64(1) {
		t.Errorf("item = %v, want the file entry", file)
	}

	skipped := first["skipped"].([]any)
	if len(skipped) != 1 {
		t.Fatalf("skipped = %v, want one entry", skipped)
	}
	one := skipped[0].(map[string]any)
	if one["path"] != `C:\Cache\gone` || one["reason"] != "not_found" ||
		one["detail"] != "the path does not exist" {
		t.Errorf("skip = %v, want the skip the plan names", one)
	}

	second := document["rules"].([]any)[1].(map[string]any)
	command, ok := second["command"].(map[string]any)
	if !ok || command["command"] != "npm" {
		t.Errorf("command = %v, want the command plan", second["command"])
	}
	if _, ok := second["items"]; ok {
		t.Errorf("the command rule carries items %v, want none", second["items"])
	}

	summary := document["summary"].(map[string]any)
	if summary["rules"] != float64(3) || summary["items"] != float64(2) ||
		summary["files"] != float64(3) || summary["bytes_found"] != float64(110) {
		t.Errorf("summary = %v, want the counts of the whole plan", summary)
	}
}

func TestPlanJSONIsThePlan(t *testing.T) {
	plan := samplePlan()
	var out bytes.Buffer
	if err := report.PlanJSON(&out, plan); err != nil {
		t.Fatalf("PlanJSON: %v", err)
	}

	var document struct {
		Summary struct {
			Rules      int   `json:"rules"`
			Items      int   `json:"items"`
			Files      int64 `json:"files"`
			BytesFound int64 `json:"bytes_found"`
		} `json:"summary"`
		Rules []struct {
			ID       string  `json:"id"`
			Status   string  `json:"status"`
			Bytes    int64   `json:"bytes"`
			Files    int64   `json:"files"`
			Commands *string `json:"-"`
			Items    []struct {
				Path string  `json:"path"`
				Size int64   `json:"size"`
				Dir  bool    `json:"is_dir"`
				Name string  `json:"-"`
				Skip *string `json:"-"`
			} `json:"items"`
			Command struct {
				Command  string   `json:"command"`
				Args     []string `json:"args"`
				ItExists bool     `json:"-"`
			} `json:"command"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(out.Bytes(), &document); err != nil {
		t.Fatalf("the document is not JSON: %v", err)
	}

	var bytesFound, files int64
	var items int
	for _, rule := range plan.Rules {
		bytesFound += rule.Bytes
		files += int64(rule.Files)
		items += len(rule.Items)
	}
	if document.Summary.BytesFound != bytesFound {
		t.Errorf("summary.bytes_found = %d, want the count of the plan alone", document.Summary.BytesFound)
	}
	if document.Summary.Files != files {
		t.Errorf("summary.files = %d, want %d", document.Summary.Files, files)
	}
	if document.Summary.Items != items {
		t.Errorf("summary.items = %d, want %d", document.Summary.Items, items)
	}
	if document.Rules[0].Status != "dry_run" {
		t.Errorf("status = %q, want dry_run", document.Rules[0].Status)
	}
	if document.Rules[1].Command.Command != "npm" {
		t.Errorf("command = %+v, want the plan of the command", document.Rules[1].Command)
	}
}
