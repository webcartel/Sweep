package exec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

var _ = time.Now

// cleanExec builds an executor over a fake tree: the guard sits over the same
// fake, so what the test adds or denies after the scan shows up to it.
func cleanExec(t *testing.T) (*Executor, *fake.FS) {
	t.Helper()
	fsys := fake.NewFS()
	for _, path := range []string{
		`C:\Users`, `C:\Users\probe`, `C:\Users\probe\AppData`,
		`C:\Users\probe\AppData\Local`,
	} {
		fsys.Add(platform.Info{Path: path, IsDir: true})
	}
	probe := fake.NewEnv()
	probe.Set(`USERPROFILE`, `C:\Users\probe`)
	probe.Set(`LOCALAPPDATA`, `C:\Users\probe\AppData\Local`)
	guarded, err := guard.New(fsys, probe, `C:\Users\probe`, nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	return &Executor{fs: fsys, guard: guarded}, fsys
}

func addFile(t *testing.T, fsys *fake.FS, path string, size int64) {
	t.Helper()
	fsys.Add(platform.Info{Path: path, Size: size})
}

func addDir(t *testing.T, fsys *fake.FS, path string) {
	t.Helper()
	fsys.Add(platform.Info{Path: path, IsDir: true})
}

// runItem talks to the runner the way the executor will, and keeps the events.
func runItem(x *Executor, rule *model.Rule, item model.Item, opt model.ExecOptions) (outcome, []model.Event) {
	var saw []model.Event
	got := x.runItem(rule, item, opt, func(e model.Event) { saw = append(saw, e) })
	return got, saw
}

func TestRunItemRemovesAFileAndSaysIt(t *testing.T) {
	x, fsys := cleanExec(t)
	path := `C:\Users\probe\AppData\Local\cache.log`
	addFile(t, fsys, path, 4096)
	rule := &model.Rule{ID: "test", Risk: model.RiskSafe}
	item := model.Item{Path: path, Size: 4096, Mode: model.ModeContents}

	got, saw := runItem(x, rule, item, model.ExecOptions{})

	if got.freed != 4096 || got.skip != nil || got.pending || got.deadly != nil {
		t.Errorf("outcome = %+v, want the file freed and nothing to say", got)
	}
	if removed := fsys.Removed(); len(removed) != 1 || removed[0] != path {
		t.Errorf("Removed() = %v, want only the file", removed)
	}
	if len(fsys.Trashed()) != 0 {
		t.Errorf("Trashed() = %v, want none", fsys.Trashed())
	}
	if len(saw) != 1 {
		t.Fatalf("saw %d events, want one", len(saw))
	}
	taken, ok := saw[0].(model.ItemRemoved)
	if !ok || taken.Path != path || taken.Bytes != 4096 || taken.ToTrash {
		t.Errorf("event = %+v, want ItemRemoved for the file", saw[0])
	}
}

func TestRunItemSendsToTrashWhenTheRuleAndTheOptionsAsk(t *testing.T) {
	x, fsys := cleanExec(t)
	path := `C:\Users\probe\AppData\Local\notes.bak`
	addFile(t, fsys, path, 512)
	rule := &model.Rule{ID: "test", Trash: true, Risk: model.RiskModerate}
	item := model.Item{Path: path, Size: 512, Mode: model.ModeContents}

	got, saw := runItem(x, rule, item, model.ExecOptions{})
	if len(fsys.Trashed()) != 1 || len(fsys.Removed()) != 0 {
		t.Errorf("trashed %v removed %v, want only trashed", fsys.Trashed(), fsys.Removed())
	}
	taken, ok := saw[0].(model.ItemRemoved)
	if !ok || !taken.ToTrash {
		t.Errorf("event = %+v, want an ItemRemoved sent to trash", saw[0])
	}

	// The path is back, as a fresh copy for the next step.
	fsys.Add(platform.Info{Path: path, Size: 512})
	needDelete := false
	got, _ = runItem(x, rule, item, model.ExecOptions{Trash: &needDelete})
	if len(fsys.Removed()) != 1 || len(fsys.Trashed()) != 1 {
		t.Errorf("removed %v trashed %v, want one of each", fsys.Removed(), fsys.Trashed())
	}
	needTrash := true
	fsys.Add(platform.Info{Path: path, Size: 512})
	runItem(x, rule, item, model.ExecOptions{Trash: &needTrash})
	if len(fsys.Trashed()) != 2 {
		t.Errorf("Trashed() = %v, want two", fsys.Trashed())
	}
	_ = got
}

func TestRunItemDryRunCountsButTouchesNothing(t *testing.T) {
	x, fsys := cleanExec(t)
	path := `C:\Users\probe\AppData\Local\leftover`
	addFile(t, fsys, path, 2048)
	rule := &model.Rule{ID: "test", Risk: model.RiskSafe}
	item := model.Item{Path: path, Size: 2048, Mode: model.ModeContents}

	got, saw := runItem(x, rule, item, model.ExecOptions{DryRun: true})

	if len(fsys.Removed())+len(fsys.Trashed()) != 0 {
		t.Errorf("the disk was touched: removed %v trashed %v", fsys.Removed(), fsys.Trashed())
	}
	if got.freed != 2048 || got.skip != nil || got.pending || got.deadly != nil {
		t.Errorf("outcome = %+v, want the size counted as freed", got)
	}
	if len(saw) != 1 {
		t.Fatalf("saw %d events, want one", len(saw))
	}
	taken, ok := saw[0].(model.ItemRemoved)
	if !ok || taken.Path != path {
		t.Errorf("event = %+v, want ItemRemoved as if it had gone", saw[0])
	}
}

func TestRunItemSaysNotFoundWhenThePathDisappeared(t *testing.T) {
	x, _ := cleanExec(t)
	rule := &model.Rule{ID: "test", Risk: model.RiskSafe}
	item := model.Item{Path: `C:\Users\probe\AppData\Local\gone`,
		Size: 10, Mode: model.ModeContents}

	got, saw := runItem(x, rule, item, model.ExecOptions{})
	if got.skip == nil || got.skip.Reason != model.ReasonNotFound {
		t.Errorf("skip = %+v, want not_found", got.skip)
	}
	if len(saw) != 1 {
		t.Fatalf("saw %d events, want one", len(saw))
	}
	if skipped, ok := saw[0].(model.ItemSkipped); !ok || skipped.Skip.Reason != model.ReasonNotFound {
		t.Errorf("event = %+v, want an ItemSkipped not found", saw[0])
	}
}

func TestRunItemSaysAccessDeniedWhenReadingIsRefused(t *testing.T) {
	x, fsys := cleanExec(t)
	path := `C:\Users\probe\AppData\Local\silent.log`
	addFile(t, fsys, path, 10)
	fsys.Deny(path, os.ErrPermission)
	rule := &model.Rule{ID: "test", Risk: model.RiskSafe}
	item := model.Item{Path: path, Size: 10, Mode: model.ModeContents}

	got, _ := runItem(x, rule, item, model.ExecOptions{})
	if got.skip == nil || got.skip.Reason != model.ReasonAccessDenied {
		t.Errorf("skip = %+v, want access_denied", got.skip)
	}
	if got.freed != 0 || len(fsys.Removed()) != 0 {
		t.Errorf("outcome = %+v, nothing removed", got)
	}
}

func TestRunItemAsksTheGuardAgainAndStepsAside(t *testing.T) {
	x, fsys := cleanExec(t)
	path := `C:\Users\probe\AppData\Local\checked`
	addFile(t, fsys, path, 8)
	// Between the scan and the run the protected list grew: a new guard
	// refuses the path now.
	probe := fake.NewEnv()
	probe.Set(`USERPROFILE`, `C:\Users\probe`)
	strict, err := guard.New(fsys, probe, `C:\Users\probe`, []string{path})
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	x.guard = strict
	rule := &model.Rule{ID: "test", Risk: model.RiskSafe}
	item := model.Item{Path: path, Size: 8, Mode: model.ModeContents}

	got, _ := runItem(x, rule, item, model.ExecOptions{})
	if got.skip == nil || got.skip.Reason != model.ReasonProtected {
		t.Errorf("skip = %+v, want protected", got.skip)
	}
	if len(fsys.Removed()) != 0 {
		t.Errorf("Removed() = %v, want none", fsys.Removed())
	}
}

func TestRunItemFeelsAReparsePointThatAppearedAfterTheScan(t *testing.T) {
	x, fsys := cleanExec(t)
	path := `C:\Users\probe\AppData\Local\link`
	addDir(t, fsys, path)
	rule := &model.Rule{ID: "test", Risk: model.RiskSafe}
	item := model.Item{Path: path, IsDir: true, Mode: model.ModeContents}
	// The plain directory became a junction between the scan and the run.
	fsys.Add(platform.Info{Path: path, IsDir: true, Reparse: true})

	got, _ := runItem(x, rule, item, model.ExecOptions{})
	if got.skip == nil || got.skip.Reason != model.ReasonProtected {
		t.Errorf("skip = %+v, want protected", got.skip)
	}
	if len(fsys.Removed()) != 0 {
		t.Errorf("Removed() = %v, want none", fsys.Removed())
	}
}

func TestRunItemUnwindsATreeAndTakesTheJunctionItself(t *testing.T) {
	x, fsys := cleanExec(t)
	root := `C:\Users\probe\AppData\Local\cache`
	deep := root + `\old`
	nestedFile := deep + `\blobs.bin`
	junction := root + `\link`
	target := `C:\Users\probe\AppData\Local\real`
	addDir(t, fsys, root)
	addDir(t, fsys, deep)
	addFile(t, fsys, nestedFile, 100)
	addDir(t, fsys, target)
	fsys.Add(platform.Info{Path: junction, IsDir: true, Reparse: true, Link: target})
	rule := &model.Rule{ID: "test", Risk: model.RiskSafe}
	item := model.Item{Path: root, IsDir: true, Size: 100, Files: 1, Mode: model.ModeContents}

	got, _ := runItem(x, rule, item, model.ExecOptions{})

	if got.skip != nil || got.deadly != nil || got.freed != 100 {
		t.Errorf("outcome = %+v, want the whole subtree freed", got)
	}
	removed := map[string]bool{}
	for _, path := range fsys.Removed() {
		removed[path] = true
	}
	for _, want := range []string{nestedFile, deep, junction, root} {
		if !removed[want] {
			t.Errorf("%s was never removed", want)
		}
	}
	if removed[target] {
		t.Error("the junction's target was removed")
	}
	if _, err := fsys.Lstat(target); err != nil {
		t.Errorf("Lstat of the target = %v, want it alive", err)
	}
	if _, err := fsys.Lstat(nestedFile); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat of the nested file = %v, want gone", err)
	}
}

func TestRunItemPutsABusyFileOffTillTheReboot(t *testing.T) {
	x, fsys := cleanExec(t)
	path := `C:\Users\probe\AppData\Local\busy.log`
	addFile(t, fsys, path, 700)
	// The path reads fine, but a process holds it: the removal alone is
	// refused.
	fsys.DenyDel(path, os.ErrPermission)
	rule := &model.Rule{ID: "test", Risk: model.RiskModerate}
	item := model.Item{Path: path, Size: 700, Mode: model.ModeContents}

	got, saw := runItem(x, rule, item, model.ExecOptions{OnReboot: true})
	if got.pending != true {
		t.Fatalf("pending = %v, want the path put off", got.pending)
	}
	if pend := fsys.Pending(); len(pend) != 1 || pend[0] != path {
		t.Errorf("Pending() = %v, want the path", pend)
	}
	if got.freed != 0 || len(saw) != 0 {
		t.Errorf("outcome = %+v events %v, want quiet", got, saw)
	}
}

func TestRunItemOnRebootNeverTakesADangerousRule(t *testing.T) {
	x, fsys := cleanExec(t)
	path := `C:\Users\probe\AppData\Local\busy.log`
	addFile(t, fsys, path, 700)
	fsys.DenyDel(path, os.ErrPermission)
	rule := &model.Rule{ID: "test", Risk: model.RiskDangerous, Trash: true}
	item := model.Item{Path: path, Size: 700, Mode: model.ModeContents}

	got, _ := runItem(x, rule, item, model.ExecOptions{OnReboot: true})
	if got.pending {
		t.Error("pending = true for a dangerous rule, want a plain skip")
	}
	if got.skip == nil || got.skip.Reason != model.ReasonAccessDenied {
		t.Errorf("skip = %+v, want access_denied", got.skip)
	}
	if len(fsys.Pending()) != 0 {
		t.Errorf("Pending() = %v, want none", fsys.Pending())
	}
}

func TestRunItemOnRebootThatCannotBeScheduledIsASaidFailure(t *testing.T) {
	x, fsys := cleanExec(t)
	path := `C:\Users\probe\AppData\Local\busy.log`
	addFile(t, fsys, path, 700)
	// The file is busy now and Windows will not take the request for
	// later either: the removal and the scheduling are both refused.
	fsys.DenyDel(path, os.ErrPermission)
	fsys.DenyReboot(path, fmt.Errorf("the request is not taken"))
	rule := &model.Rule{ID: "test", Risk: model.RiskModerate}
	item := model.Item{Path: path, Size: 700, Mode: model.ModeContents}

	got, saw := runItem(x, rule, item, model.ExecOptions{OnReboot: true})
	if got.pending || len(fsys.Pending()) != 0 {
		t.Errorf("outcome = %+v Pending() = %v, want nothing scheduled", got, fsys.Pending())
	}
	if len(saw) != 1 || saw[0].(model.ItemSkipped).Skip.Reason != model.ReasonAccessDenied {
		t.Errorf("saw %v, want one ItemSkipped", saw)
	}
}

func TestRunItemTellsAWhimOfTheMachineAsAFailure(t *testing.T) {
	x, fsys := cleanExec(t)
	path := `C:\Users\probe\AppData\Local\strange.log`
	addFile(t, fsys, path, 3)
	fsys.DenyDel(path, fmt.Errorf("the disk controller said no"))
	rule := &model.Rule{ID: "test", Risk: model.RiskSafe}
	item := model.Item{Path: path, Size: 3, Mode: model.ModeContents}

	got, saw := runItem(x, rule, item, model.ExecOptions{})
	if got.skip != nil || got.pending {
		t.Errorf("outcome = %+v, want a failure instead", got)
	}
	if got.freed != 0 {
		t.Errorf("freed = %d, want 0", got.freed)
	}
	if len(saw) != 1 {
		t.Fatalf("saw %d events, want one", len(saw))
	}
	if failed, ok := saw[0].(model.ItemFailed); !ok || failed.Path != path {
		t.Errorf("event = %+v, want ItemFailed for the path", saw[0])
	}
}

func TestRunItemADangerousRuleWithTrashTakenAwayIsNotRun(t *testing.T) {
	x, fsys := cleanExec(t)
	path := `C:\Users\probe\AppData\Local\save.log`
	addFile(t, fsys, path, 55)
	rule := &model.Rule{ID: "test", Risk: model.RiskDangerous, Trash: true}
	item := model.Item{Path: path, Size: 55, Mode: model.ModeContents}
	no := false

	got, saw := runItem(x, rule, item, model.ExecOptions{Trash: &no})
	if got.deadly == nil {
		t.Error("deadly = nil, want a refusal")
	}
	if len(saw) != 0 || len(fsys.Removed())+len(fsys.Trashed()) != 0 {
		t.Errorf("saw %d events, removed %v, want nothing", len(saw), fsys.Removed())
	}
}
