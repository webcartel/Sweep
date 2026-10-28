//go:build windows

package cli

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/spf13/cobra"
)

// The codes the shell answers with for the ways the user turns the
// elevation down: a refusal goes word for word to the model, which takes
// the admin rules out of the choice.
const (
	// refusal answers noooooooooope, as the shell tells the run that took
	// the question to dialog.
	refusal = 1223
	// refusedVeto answers no too (ERROR_ACCESS_DENIED), an older Windows
	// said it that way.
	refusedVeto = 5
)

// restartAsAdmin asks the shell to host one elevated sweep again: the spec
// names ShellExecuteW with the say of runas. The run comes back into the
// screens with the rules it was agreed to in --preselect, and with the say
// of the flags that named the rules and the drawing.
func restartAsAdmin(c *cobra.Command, preselect []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("restart: %w", err)
	}
	return shellExecuteRunAs(exe, restartParamsWith(c, "--preselect", preselect))
}

// restartAsAdminRegistry is the same restart for a registry choice: the run
// comes back with the findings named in --preselect-registry.
func restartAsAdminRegistry(c *cobra.Command, preselect []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("restart: %w", err)
	}
	return shellExecuteRunAs(exe, restartParamsWith(c, "--preselect-registry", preselect))
}

// restartParamsWith builds the same line for the flag that names the choice.
func restartParamsWith(c *cobra.Command, flag string, preselect []string) string {
	params := flag + " " + strings.Join(preselect, ",")
	if path, _ := c.Flags().GetString("config"); path != "" {
		params += " --config " + quote(path)
	}
	if dirs, _ := c.Flags().GetStringArray("rules-dir"); len(dirs) != 0 {
		params += " --rules-dir " + quote(strings.Join(dirs, ","))
	}
	if ascii, _ := c.Flags().GetBool("ascii"); ascii {
		params += " --ascii"
	}
	if noASCII, _ := c.Flags().GetBool("no-ascii"); noASCII {
		params += " --no-ascii"
	}
	// The child process speaks the language this one was given, rather
	// than falling back to the system or the saved setting.
	if lang, _ := c.Flags().GetString("lang"); lang != "" {
		params += " --lang " + lang
	}
	return params
}

// quote wraps a path the shell may read spaces in.
func quote(path string) string {
	if strings.Contains(path, " ") {
		return `"` + path + `"`
	}
	return path
}

// shellExecuteRunAs begets the process of the shell with the elevation.
func shellExecuteRunAs(exe, params string) error {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	run := shell32.NewProc("ShellExecuteW")
	runas, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	arg, err := syscall.UTF16PtrFromString(params)
	if err != nil {
		return err
	}
	const showNormal = 1
	answer, _, callErr := run.Call(0,
		uintptr(unsafe.Pointer(runas)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(arg)),
		0, showNormal)
	switch {
	case answer > 32:
		return nil
	case callErr == syscall.Errno(refusal) || callErr == syscall.Errno(refusedVeto):
		return &ExitError{Code: ExitNoTerminal, Err: fmt.Errorf(
			"the elevation was refused: %v", callErr)}
	default:
		return &ExitError{Code: ExitInternal, Err: fmt.Errorf(
			"restart as administrator failed: the shell said %d (%v)", answer, callErr)}
	}
}
