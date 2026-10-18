package scan

import (
	"time"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
)

// filters holds the file filters of one target, the masks compiled once. The
// masks are matched against the path relative to the target, the age and the
// size against the entry itself.
type filters struct {
	include []*platform.Mask
	exclude []*platform.Mask
	older   time.Duration
	minSize int64
}

// compileFilters reads the filters of a target. A mask that does not compile
// takes nothing out rather than stopping the scan: the loader has rejected
// every mask that does not compile, so what got in despite of it is a rule
// that should simply hold still.
func compileFilters(t model.Target) filters {
	compiled := filters{older: time.Duration(t.MinAgeDays) * 24 * time.Hour, minSize: t.MinSize}
	for _, mask := range t.Include {
		if taken, err := platform.CompileMask(mask); err == nil {
			compiled.include = append(compiled.include, taken)
		}
	}
	for _, mask := range t.Exclude {
		if dropped, err := platform.CompileMask(mask); err == nil {
			compiled.exclude = append(compiled.exclude, dropped)
		}
	}
	return compiled
}

// allow says whether one file is taken. Nothing set at all takes every file.
func (f filters) allow(rel string, info platform.Info) bool {
	if len(f.include) > 0 {
		found := false
		for _, mask := range f.include {
			if mask.Match(rel) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, mask := range f.exclude {
		if mask.Match(rel) {
			return false
		}
	}
	if f.older > 0 && time.Since(info.ModTime) < f.older {
		return false
	}
	if f.minSize > 0 && info.Size < f.minSize {
		return false
	}
	return true
}
