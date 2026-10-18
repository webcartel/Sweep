package exec

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/webcartel/sweep/internal/model"
)

// outcome is what one item came to after the executor's step.
type outcome struct {
	freed   int64 // the bytes that went off the disk, or would go in a dry run
	trashed bool
	pending bool // put off till the next start of Windows
	skip    *model.Skip
	deadly  error // not the path's fault; the failure of the run itself
}

// runItem takes one item off the disk. The path is asked twice: the guard
// knows it from the scan, and it is asked again right before the removal,
// because between the scan and the run the machine had time to change.
func (x *Executor) runItem(rule *model.Rule, item model.Item, opt model.ExecOptions, say func(model.Event)) outcome {
	tell := func(reason model.Skip) outcome {
		if say != nil {
			say(model.ItemSkipped{RuleID: rule.ID, Skip: reason})
		}
		return outcome{skip: &reason}
	}
	failed := func(err error) outcome {
		if say != nil {
			say(model.ItemFailed{RuleID: rule.ID, Path: item.Path, Err: err})
		}
		return outcome{deadly: err}
	}

	info, err := x.fs.Lstat(item.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return tell(model.Skip{Path: item.Path, Reason: model.ReasonNotFound,
			Detail: "the path disappeared"})
	case errors.Is(err, fs.ErrPermission):
		return tell(model.Skip{Path: item.Path, Reason: model.ReasonAccessDenied,
			Detail: err.Error()})
	case err != nil:
		return failed(err)
	}
	if info.Reparse && !item.Reparse {
		// A path that read as a plain entry in the scan became a link: the
		// rule was written for what it pointed at, not for where it leads.
		return tell(model.Skip{Path: item.Path, Reason: model.ReasonProtected,
			Detail: "the path became a reparse point"})
	}

	verdict, err := x.guard.Check(item.Path, item.Mode, item.Include, item.MinAgeDays)
	switch {
	case err != nil:
		if reason, caught := skipForErr(err); caught {
			return tell(model.Skip{Path: item.Path, Reason: reason, Detail: err.Error()})
		}
		return failed(err)
	case !verdict.Allowed:
		return tell(model.Skip{Path: verdict.Path, Reason: verdict.Reason,
			Detail: verdict.Message})
	}

	useTrash := rule.Trash
	if opt.Trash != nil {
		useTrash = *opt.Trash
	}
	if rule.Risk == model.RiskDangerous && !useTrash {
		return outcome{deadly: fmt.Errorf(
			"the dangerous rule %s only runs with its data in the recycle bin", rule.ID)}
	}

	if opt.DryRun {
		if say != nil {
			say(model.ItemRemoved{RuleID: rule.ID, Path: item.Path,
				Bytes: item.Size, ToTrash: useTrash})
		}
		return outcome{freed: item.Size, trashed: useTrash}
	}

	err = x.dispose(item, useTrash, info.IsDir)
	if err == nil {
		if say != nil {
			say(model.ItemRemoved{RuleID: rule.ID, Path: item.Path,
				Bytes: item.Size, ToTrash: useTrash})
		}
		return outcome{freed: item.Size, trashed: useTrash}
	}

	reason, caught := skipForErr(err)
	if !caught {
		return failed(err)
	}
	if opt.OnReboot && rule.Risk != model.RiskDangerous {
		if rebootErr := x.fs.RemoveOnReboot(item.Path); rebootErr == nil {
			// The path stays put for now and goes away with the next
			// start of Windows; the run does not count it as freed.
			if say != nil {
				say(model.ItemPending{RuleID: rule.ID, Path: item.Path})
			}
			return outcome{pending: true}
		}
	}
	return tell(model.Skip{Path: item.Path, Reason: reason, Detail: err.Error()})
}

// dispose puts one item away: to the recycle bin when asked, otherwise
// unwound from the bottom up. A reparse point is taken as itself either way:
// reading the inside of a link is another tree's business.
func (x *Executor) dispose(item model.Item, useTrash, isDir bool) error {
	if useTrash {
		if err := x.fs.Trash(item.Path); err != nil {
			return err
		}
		return nil
	}
	if item.Reparse || !isDir {
		return x.fs.Remove(item.Path)
	}
	return x.removeTree(item.Path)
}

// removeTree empties a directory from the bottom up and takes the directory
// itself last. A child that is a reparse point leaves as a link: what it
// points at is never entered.
func (x *Executor) removeTree(path string) error {
	kids, err := x.fs.ReadDir(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var placed []error
	for _, kid := range kids {
		var child error
		if kid.Reparse || !kid.IsDir {
			child = x.fs.Remove(kid.Path)
		} else {
			child = x.removeTree(kid.Path)
		}
		if child != nil {
			placed = append(placed, child)
		}
	}
	if err := errors.Join(placed...); err != nil {
		return err
	}
	return x.fs.Remove(path)
}
