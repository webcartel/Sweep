package platform

import (
	"errors"
	"time"
)

// Info describes one file system entry the way cleaning needs to see it.
type Info struct {
	// Path is the path as the file system names it.
	Path string
	// Name is the last element of Path.
	Name string
	// Size is the length of a file in bytes, and what the file system
	// reports for a directory.
	Size int64
	// ModTime is the last write time.
	ModTime time.Time
	// IsDir says whether the entry is a directory.
	IsDir bool
	// Reparse marks a junction, a symbolic link or any other reparse point.
	// It comes from the file attributes rather than from the Go file mode,
	// because a junction does not always show up there.
	Reparse bool
	// Link is the target of a symbolic link or a junction, and empty for
	// every other entry.
	Link string
}

// DriveKind is what Windows answers for a drive with GetDriveType.
type DriveKind int

const (
	// DriveOther is a root that does not exist, a device without a root
	// directory, or a kind Windows does not name here.
	DriveOther DriveKind = iota
	// DriveFixed is a local disk: the only kind cleaning is allowed on.
	DriveFixed
	// DriveRemovable is a USB stick or a memory card.
	DriveRemovable
	// DriveRemote is a network share, including a mapped drive letter.
	DriveRemote
	// DriveCD is an optical drive.
	DriveCD
	// DriveRAM is a RAM disk.
	DriveRAM
)

// FS is the whole of the file system the core may touch. The implementation
// for Windows lives behind a build tag, and the tests use an in-memory one.
//
// Remove, Trash and RemoveOnReboot act on the path itself: a reparse point is
// removed as a link and what it points at is left alone.
type FS interface {
	// Lstat describes the entry at path without following a reparse point.
	Lstat(path string) (Info, error)
	// ReadDir lists what is inside a directory.
	ReadDir(path string) ([]Info, error)
	// Glob returns the paths that already exist and that the mask takes. The
	// mask is absolute and free of %VARS%; `**` stands for one or more levels
	// of directories, and the walk never enters a reparse point.
	Glob(mask string) ([]string, error)
	// LongPath gives the path the way Windows spells it, with 8.3 names
	// expanded.
	LongPath(path string) (string, error)
	// DriveType answers what kind of drive holds the path.
	DriveType(path string) (DriveKind, error)
	// Remove deletes a file, a directory or a reparse point itself.
	Remove(path string) error
	// Trash moves a file or a directory to the recycle bin.
	Trash(path string) error
	// RemoveOnReboot schedules a deletion for the next start of Windows.
	RemoveOnReboot(path string) error
}

// Procs answers what is running on the machine.
type Procs interface {
	// Running returns those of names that are running, in the order of the
	// names given and without repeats. Matching ignores case, the way
	// Windows compares process names.
	Running(names []string) ([]string, error)
}

// ErrNotWindows is what the constructors of the platform answer away from
// Windows: sweep cleans Windows, and the other builds exist for the tests and
// for the earlier CI signal.
var ErrNotWindows = errors.New("sweep cleans Windows only")
