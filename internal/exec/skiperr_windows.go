//go:build windows

package exec

import (
	"errors"
	"io/fs"
	"syscall"

	"github.com/webcartel/sweep/internal/model"
)

// skipForErr says which reason of the report an error in putting a path away
// stands for: Windows answers with file numbers, and each of them means
// something of its own.
func skipForErr(err error) (model.SkipReason, bool) {
	switch {
	case errors.Is(err, syscall.Errno(32)): // ERROR_SHARING_VIOLATION
		return model.ReasonInUse, true
	case errors.Is(err, syscall.Errno(33)): // ERROR_LOCK_VIOLATION
		return model.ReasonInUse, true
	case errors.Is(err, syscall.Errno(5)): // ERROR_ACCESS_DENIED
		return model.ReasonAccessDenied, true
	case errors.Is(err, syscall.Errno(145)): // ERROR_DIR_NOT_EMPTY
		return model.ReasonAccessDenied, true
	case errors.Is(err, fs.ErrPermission):
		return model.ReasonAccessDenied, true
	case errors.Is(err, fs.ErrNotExist):
		return model.ReasonNotFound, true
	default:
		return "", false
	}
}
