package explore

import (
	"context"
	"sort"
	"strings"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
)

// Tick is one path the user chose to remove.
type Tick struct {
	Path string
	Size int64
}

// Removal is what happened to one ticked path.
type Removal struct {
	Path string
	Name string
	Size int64
	// Action is "trashed", "refused" or "failed".
	Action string
	Note   string
}

// RemoveOptions tunes one removal.
type RemoveOptions struct {
	// Guard refuses a path the engine may not touch. A refusal is a result,
	// not a failure: the user sees why the path stayed.
	Guard *guard.Guard
	// Log, when set, gets one line per path.
	Log func(action, path, note string)
}

// Remove sends the ticked paths to the Recycle Bin. A path a ticked parent
// already covers is skipped, so choosing a folder and something inside it
// cannot delete the same thing twice. The Bin is the only way a removal
// takes here: the explorer does not delete for good.
func Remove(ctx context.Context, fsys platform.FS, ticks []Tick, opt RemoveOptions) []Removal {
	chosen := topmost(ticks)
	out := make([]Removal, 0, len(chosen))
	for _, tick := range chosen {
		if ctx.Err() != nil {
			break
		}
		removal := Removal{Path: tick.Path, Name: lastElement(tick.Path), Size: tick.Size}

		if opt.Guard != nil {
			verdict, err := opt.Guard.Check(tick.Path, model.ModeSelf, nil, 0)
			switch {
			case err != nil:
				removal.Action, removal.Note = "failed", err.Error()
			case !verdict.Allowed:
				removal.Action, removal.Note = "refused", verdict.Message
			}
		}
		if removal.Action == "" {
			if err := fsys.Trash(tick.Path); err != nil {
				removal.Action, removal.Note = "failed", err.Error()
			} else {
				removal.Action = "trashed"
			}
		}
		if opt.Log != nil {
			opt.Log(removal.Action, removal.Path, removal.Note)
		}
		out = append(out, removal)
	}
	return out
}

// topmost drops the ticks a shorter tick already covers. The order is the
// order the user sees, shortest path first, so a parent wins over its child.
func topmost(ticks []Tick) []Tick {
	sorted := append([]Tick(nil), ticks...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Path) < len(sorted[j].Path)
	})
	var out []Tick
	for _, tick := range sorted {
		covered := false
		for _, kept := range out {
			if isUnder(tick.Path, kept.Path) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, tick)
		}
	}
	return out
}

// isUnder says whether path sits inside parent, comparing the way Windows
// compares paths.
func isUnder(path, parent string) bool {
	inner := strings.ToLower(strings.TrimSuffix(path, `\`))
	outer := strings.ToLower(strings.TrimSuffix(parent, `\`))
	return inner != outer && strings.HasPrefix(inner, outer+`\`)
}
