//go:build !windows

package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// errNoRestart stands where no shell of the machine host the elevation:
// the spec keeps the restart of a run to the Windows.
var errNoRestart = errors.New("the restart wants a shell of Windows")

// restartAsAdmin, off Windows, says the run cannot restart itself.
func restartAsAdmin(*cobra.Command, []string) error { return errNoRestart }
