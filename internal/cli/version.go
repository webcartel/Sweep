package cli

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Version is the release version of the build. A release sets it with
//
//	-ldflags "-X github.com/webcartel/sweep/internal/cli.Version=v1.2.3"
//
// and a plain `go build` leaves it at "dev".
var Version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), buildLine())
			return err
		},
	}
}

// buildLine is the single line `sweep version` prints: the version, the
// revision the binary was built from and the Go toolchain that built it.
func buildLine() string {
	return fmt.Sprintf("sweep %s (commit %s, %s)", Version, revision(), runtime.Version())
}

// revision returns the VCS revision stamped into the binary by the Go
// toolchain, or "unknown" when the build had no repository information.
func revision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return "unknown"
}
