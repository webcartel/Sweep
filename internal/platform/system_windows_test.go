//go:build windows

package platform_test

import (
	"testing"

	"github.com/webcartel/sweep/internal/platform"
)

func TestWindowsSystemReportsTheBuild(t *testing.T) {
	sys, err := platform.NewSystem()
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	build, err := sys.OSBuild()
	if err != nil {
		t.Fatalf("OSBuild: %v", err)
	}
	if build <= 0 {
		t.Fatalf("OSBuild = %d, want the build of the running Windows", build)
	}
	t.Logf("the machine reports build %d", build)
}
