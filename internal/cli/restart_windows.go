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

// restartAsAdmin asks the shell to host one elevated sweep again: the spec
// names ShellExecuteW with the say of runas. The run comes back into the
// screens with the rules it was agreed to in --preselect, and with the say
// of the flags that named the rules and the drawing.
func restartAsAdmin(c *cobra.Command, preselect []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("restart: %w", err)
	}
	say := strings.Join(preselect, ",")
	return shellExecuteRunAs(exe, "--preselect "+say)
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
	answer, _, _ := run.Call(0,
		uintptr(unsafe.Pointer(runas)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(arg)),
		0, showNormal)
	if answer <= 32 {
		return fmt.Errorf("restart as administrator failed: the shell said %d", answer)
	}
	return nil
}
