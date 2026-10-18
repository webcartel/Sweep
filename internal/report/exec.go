package report

import (
	"encoding/json"
	"io"
	"time"

	"github.com/webcartel/sweep/internal/model"
)

// execDocument is the shape of the JSON a run answers with. Built by hand,
// like the plan document: the field names are the contract.
type execDocument struct {
	Schema    int         `json:"schema"`
	DryRun    bool        `json:"dry_run"`
	Started   string      `json:"started_at"`
	Finished  string      `json:"finished_at"`
	Summary   execSummary `json:"summary"`
	Rules     []execRule  `json:"rules"`
	OutOfTime []string    `json:"pending_reboot"`
}

type execSummary struct {
	Rules        int   `json:"rules"`
	Items        int   `json:"items"`
	Files        int64 `json:"files"`
	BytesFound   int64 `json:"bytes_found"`
	Removed      int   `json:"removed"`
	Skipped      int   `json:"skipped"`
	Failed       int   `json:"failed"`
	BytesFreed   int64 `json:"bytes_freed"`
	BytesTrashed int64 `json:"bytes_trashed"`
}

type execRule struct {
	ID      string         `json:"id"`
	Status  string         `json:"status"`
	Bytes   int64          `json:"bytes"`
	Files   int64          `json:"files"`
	Command *commandDefine `json:"command,omitempty"`
	Skipped []skipDocument `json:"skipped,omitempty"`
}

// ExecJSON prints the answer of a run: what went, what stayed, why, and what
// waits for the next start of Windows.
func ExecJSON(w io.Writer, r *model.Result) error {
	document := execDocument{
		Schema:   1,
		DryRun:   r.DryRun,
		Started:  r.StartedAt.UTC().Format(time.RFC3339),
		Finished: r.FinishedAt.UTC().Format(time.RFC3339),
	}
	for i := range r.PerRule {
		per := &r.PerRule[i]
		entry := execRule{
			ID:     ruleID(*per),
			Status: per.Status,
			Bytes:  per.Bytes,
			Files:  per.Files,
		}
		for _, skip := range per.Skipped {
			entry.Skipped = append(entry.Skipped, skipDocument{
				Path:   skip.Path,
				Reason: string(skip.Reason),
				Detail: skip.Detail,
			})
		}
		if per.Rule != nil && per.Rule.Targets != nil {
			for _, target := range per.Rule.Targets {
				if target.Kind == model.KindCommand {
					entry.Command = &commandDefine{Command: target.Cmd, Args: target.Args}
					break
				}
			}
		}
		document.Rules = append(document.Rules, entry)
	}
	document.Summary.Rules = len(document.Rules)
	document.Summary.Items = r.Items
	document.Summary.Files = r.Files
	document.Summary.BytesFound = r.BytesFound
	document.Summary.Removed = r.Removed
	document.Summary.Skipped = r.Skipped
	document.Summary.Failed = r.Failed
	document.Summary.BytesFreed = r.FreedBytes
	document.Summary.BytesTrashed = r.TrashBytes
	document.OutOfTime = append(document.OutOfTime, r.PendingReboot...)

	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// ruleID names the rule of a result; a rule the loader made with no id at
// all is still spoken about, under an empty name.
func ruleID(per model.RuleResult) string {
	if per.Rule == nil {
		return ""
	}
	return per.Rule.ID
}

// Entry is one operation of a run, in the order the log names it: the hour,
// the rule, the path, what was done with it, how big it was, and how it
// came out.
type Entry struct {
	Time   time.Time
	RuleID string
	Path   string
	Action string
	Size   int64
	Result string
}

// LogSink takes one entry per operation. The frontend owns the file; the
// sink owns the shape of a line.
type LogSink interface {
	Append(entry Entry) error
}

type jsonLog struct {
	to io.Writer
}

// LogTo answers a sink that writes one JSON line per entry to the writer
// given. The first Append that goes wrong answers its error from then on:
// a log that cannot be written to must not pass unnoticed.
func LogTo(to io.Writer) LogSink { return &jsonLog{to: to} }

type entryDocument struct {
	Time   string `json:"time"`
	Rule   string `json:"rule"`
	Path   string `json:"path"`
	Action string `json:"action"`
	Size   int64  `json:"size"`
	Coming string `json:"result"`
}

func (j *jsonLog) Append(entry Entry) error {
	data, err := json.Marshal(entryDocument{
		Time:   entry.Time.UTC().Format(time.RFC3339),
		Rule:   entry.RuleID,
		Path:   entry.Path,
		Action: entry.Action,
		Size:   entry.Size,
		Coming: entry.Result,
	})
	if err != nil {
		return err
	}
	_, err = j.to.Write(append(data, '\n'))
	return err
}

// LogFile names today's log under the data folder of the user: one line per
// operation, in one file per day.
func LogFile(dir string, at time.Time) string {
	return dir + `\sweep\logs\` + at.Format("2006-01-02") + ".jsonl"
}
