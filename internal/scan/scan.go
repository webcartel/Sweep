package scan

import (
	"github.com/webcartel/sweep/internal/platform"
)

// Scanner reads the disk and builds the plan of what the removal can take.
// Nothing it does changes the disk.
type Scanner struct {
	fs  platform.FS
	env platform.Env
}
