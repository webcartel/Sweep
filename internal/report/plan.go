package report

import (
	"encoding/json"
	"io"
	"time"

	"github.com/webcartel/sweep/internal/model"
)

// planDocument is the shape of the JSON a scan prints. It is built by hand,
// not through encoding the model: the field names are the contract, and the
// plan types keep the names they carry inside the process.
type planDocument struct {
	Schema  int            `json:"schema"`
	DryRun  bool           `json:"dry_run"`
	Created string         `json:"created_at"`
	Summary summary        `json:"summary"`
	Rules   []ruleDocument `json:"rules"`
}

type summary struct {
	Rules      int   `json:"rules"`
	Items      int   `json:"items"`
	Files      int64 `json:"files"`
	BytesFound int64 `json:"bytes_found"`
	Skipped    int   `json:"skipped"`
}

type ruleDocument struct {
	ID      string         `json:"id"`
	Status  string         `json:"status"`
	Bytes   int64          `json:"bytes"`
	Files   int64          `json:"files"`
	Command *commandDefine `json:"command,omitempty"`
	Items   []itemDocument `json:"items,omitempty"`
	Skipped []skipDocument `json:"skipped,omitempty"`
}

type commandDefine struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type itemDocument struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Files   int    `json:"files"`
	IsDir   bool   `json:"is_dir"`
	Reparse bool   `json:"reparse"`
	ModTime string `json:"mod_time"`
}

type skipDocument struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// PlanJSON prints the whole plan as the one JSON document a scan answers with:
// schema 1, dry_run true, and what the plan holds — no more, no less.
func PlanJSON(w io.Writer, p *model.Plan) error {
	document := planDocument{
		Schema:  1,
		DryRun:  true,
		Created: p.CreatedAt.UTC().Format(time.RFC3339),
	}
	for _, rule := range p.Rules {
		entry := ruleDocument{
			ID:     ruleName(rule),
			Status: "dry_run",
			Bytes:  rule.Bytes,
			Files:  int64(rule.Files),
		}
		if rule.Command != nil {
			entry.Command = &commandDefine{Command: rule.Command.Command, Args: rule.Command.Args}
		}
		for _, item := range rule.Items {
			entry.Items = append(entry.Items, itemDocument{
				Path:    item.Path,
				Size:    item.Size,
				Files:   item.Files,
				IsDir:   item.IsDir,
				Reparse: item.Reparse,
				ModTime: item.ModTime.UTC().Format(time.RFC3339),
			})
			document.Summary.Items++
			document.Summary.Files += int64(item.Files)
			document.Summary.BytesFound += item.Size
		}
		for _, skip := range rule.Skipped {
			entry.Skipped = append(entry.Skipped, skipDocument{
				Path:   skip.Path,
				Reason: string(skip.Reason),
				Detail: skip.Detail,
			})
			document.Summary.Skipped++
		}
		document.Rules = append(document.Rules, entry)
	}
	document.Summary.Rules = len(document.Rules)

	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// ruleName names the rule of a plan entry; the plan may hold a rule that the
// loader made with no id at all.
func ruleName(rule model.RulePlan) string {
	if rule.Rule == nil {
		return ""
	}
	return rule.Rule.ID
}
