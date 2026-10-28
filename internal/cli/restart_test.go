//go:build windows

package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// restartCommand builds the root command, runs the harmless `version`
// with the arguments given, and answers it: cobra merges the persistent
// flags into the command's own during Execute, which is what the restart
// reads in production too.
func restartCommand(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	root := newRootCmd(deps{}, args)
	root.SetArgs(append(args, "version"))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return root
}

// An elevated run is a fresh process, and it has to keep speaking the
// language the user picked. The flag travels with the other flags the
// restart already carries.
func TestRestartForwardsLang(t *testing.T) {
	root := restartCommand(t, "--lang", "ru")

	params := restartParamsWith(root, "--preselect", []string{"apps.discord.cache"})
	if !strings.Contains(params, "--lang ru") {
		t.Fatalf("params = %q, want the language forwarded", params)
	}
}

// A run that never named a language forwards nothing, so the child keeps
// reading the saved setting and the system.
func TestRestartOmitsLangWhenNotGiven(t *testing.T) {
	root := restartCommand(t)

	params := restartParamsWith(root, "--preselect", []string{"apps.discord.cache"})
	if strings.Contains(params, "--lang") {
		t.Fatalf("params = %q, want no language on the line", params)
	}
}
