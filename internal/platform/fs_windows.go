//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NewFS returns the file system of this machine.
func NewFS() (FS, error) { return windowsFS{}, nil }

// windowsFS is the file system as Windows shows it.
type windowsFS struct{}

// Lstat describes an entry without following a reparse point. Whether the
// entry is a junction is read from the file attributes, as the rules require:
// the Go file mode does not always say so, and a junction that slipped through
// would be walked into.
func (windowsFS) Lstat(path string) (Info, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Info{}, err
	}
	return describe(path, info), nil
}

// ReadDir lists what is inside a directory.
func (windowsFS) ReadDir(path string) ([]Info, error) {
	items, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	entries := make([]Info, 0, len(items))
	for _, item := range items {
		info, err := item.Info()
		if err != nil {
			return nil, err
		}
		entries = append(entries, describe(filepath.Join(path, item.Name()), info))
	}
	return entries, nil
}

// Glob returns the paths the mask takes.
func (f windowsFS) Glob(mask string) ([]string, error) { return GlobPaths(f, mask) }

// LongPath expands 8.3 names, so that the path the rules read is the one
// Windows itself spells.
func (windowsFS) LongPath(path string) (string, error) {
	buffer := make([]uint16, 512)
	for {
		written, err := windows.GetLongPathName(windows.StringToUTF16Ptr(path), &buffer[0], uint32(len(buffer)))
		if err != nil {
			return "", &os.PathError{Op: "LongPath", Path: path, Err: err}
		}
		if int(written) < len(buffer) {
			return windows.UTF16ToString(buffer[:written]), nil
		}
		buffer = make([]uint16, written+1)
	}
}

// DriveType answers what kind of drive holds the path.
func (windowsFS) DriveType(path string) (DriveKind, error) {
	root := filepath.VolumeName(path)
	if root == "" {
		return DriveOther, nil
	}
	switch windows.GetDriveType(windows.StringToUTF16Ptr(root + `\`)) {
	case windows.DRIVE_FIXED:
		return DriveFixed, nil
	case windows.DRIVE_REMOVABLE:
		return DriveRemovable, nil
	case windows.DRIVE_REMOTE:
		return DriveRemote, nil
	case windows.DRIVE_CDROM:
		return DriveCD, nil
	case windows.DRIVE_RAMDISK:
		return DriveRAM, nil
	default:
		return DriveOther, nil
	}
}

// Remove deletes a file, a directory or a reparse point itself: the link goes
// and what it points at stays.
func (windowsFS) Remove(path string) error { return os.Remove(path) }

// Trash sends a path to the recycle bin through the shell.
func (windowsFS) Trash(path string) error { return shellDelete(path) }

// RemoveOnReboot schedules a deletion for the next start of Windows, which is
// what a file that is in use needs.
func (windowsFS) RemoveOnReboot(path string) error {
	err := windows.MoveFileEx(windows.StringToUTF16Ptr(path), nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	if err != nil {
		return &os.PathError{Op: "RemoveOnReboot", Path: path, Err: err}
	}
	return nil
}

// describe turns what the standard library knows about an entry into what the
// rules need to know about it. Whether an entry is a directory comes from the
// attribute Windows sets, not from the Go file mode: a junction to a directory
// carries the reparse attribute, and the file mode does not always call it a
// directory, which would make a folder rule read it as a file.
func describe(path string, info os.FileInfo) Info {
	entry := Info{
		Path:    path,
		Name:    info.Name(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
		Reparse: isReparse(info),
	}
	if attributes, ok := attributesOf(info); ok {
		entry.IsDir = attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	}
	if entry.Reparse {
		entry.Link = linkTarget(path)
	}
	return entry
}

// isReparse reads the attribute Windows sets for junctions, symbolic links and
// every other reparse point.
func isReparse(info os.FileInfo) bool {
	attributes, ok := attributesOf(info)
	if !ok {
		return false
	}
	return attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

// attributesOf reads the file attributes Windows keeps beside every entry.
func attributesOf(info os.FileInfo) (uint32, bool) {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return 0, false
	}
	return data.FileAttributes, true
}

// linkTarget reads where a junction or a symbolic link points. Everything else
// that carries the reparse attribute — a cloud placeholder, a deduplicated
// file — has no target, and an empty one is the honest answer for it.
func linkTarget(path string) string {
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return target
}

// fileOp is SHFILEOPSTRUCTW. The package x/sys/windows does not carry it, and
// one struct is less than the COM interface that would replace it.
type fileOp struct {
	Wnd        windows.Handle
	Func       uint32
	From       *uint16
	To         *uint16
	Flags      uint16
	AnyOps     int32
	NameMapped *uint16
	Progress   *uint16
}

const (
	// opDelete is FO_DELETE.
	opDelete = 0x0003
	// flagSilent, flagAllowUndo and flagNoErrorUI ask the shell to delete into
	// the recycle bin without a progress dialogue and without an error box
	// that nobody is there to close.
	flagSilent    = 0x0004
	flagAllowUndo = 0x0040
	flagNoErrorUI = 0x0400
	// flagWantNukeWarning asks the shell to say so before it deletes a file
	// for good because it does not fit in the recycle bin. A rule with
	// `trash: true` promises the bin, and deleting instead without a word
	// breaks that promise. The shell ignores this flag when flagNoConfirm is
	// set, which is why it is not: the warning is the one dialogue left.
	flagWantNukeWarning = 0x4000
)

var procSHFileOperation = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

// shellDelete puts one path in the recycle bin. The list of names ends with
// two nulls, which is what the shell expects.
func shellDelete(path string) error {
	from := windows.StringToUTF16(path)
	from = append(from, 0, 0)

	op := fileOp{
		Func:  opDelete,
		From:  &from[0],
		Flags: flagSilent | flagAllowUndo | flagNoErrorUI | flagWantNukeWarning,
	}
	result, _, _ := procSHFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	if result != 0 {
		return &os.PathError{Op: "Trash", Path: path, Err: syscall.Errno(result)}
	}
	// The shell reports a cancelled operation as success and says so here:
	// the path is still there and the caller has to know.
	if op.AnyOps != 0 {
		return &os.PathError{Op: "Trash", Path: path, Err: syscall.ECANCELED}
	}
	return nil
}
