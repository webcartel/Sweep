//go:build windows

package platform

import (
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
// holds nothing is not an error: it is an empty list.
func runningProcesses() (map[string]bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()

	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	running := map[string]bool{}
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		running[strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))] = true
	}
	return running, nil
}
