//go:build windows

package platform

import (
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NewProcs returns the process list of this machine.
func NewProcs() (Procs, error) { return windowsProcs{}, nil }

// windowsProcs is the process list as Windows shows it.
type windowsProcs struct{}

// Running reads the process list once and answers which of the names are on
// it. Windows reports the file name of the running executable, so a name looks
// like "chrome.exe".
func (windowsProcs) Running(names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}

	running, err := runningProcesses()
	if err != nil {
		return nil, err
	}

	var found []string
	seen := map[string]bool{}
	for _, name := range names {
		key := strings.ToLower(name)
		if seen[key] || !running[key] {
			continue
		}
		seen[key] = true
		found = append(found, name)
	}
	return found, nil
}

// runningProcesses takes one snapshot of the process list. A snapshot that
// holds nothing is not an error: it is an empty list. A snapshot that breaks
// halfway is an error, not a short list: a rule would read it as "not running"
// and delete the files of a process it did not see.
func runningProcesses() (map[string]bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()

	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	running := map[string]bool{}

	err = windows.Process32First(snapshot, &entry)
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return running, nil
	}
	if err != nil {
		return nil, err
	}
	for {
		running[strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))] = true
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return running, nil
			}
			return nil, err
		}
	}
}
