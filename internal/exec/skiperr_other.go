//go:build !windows

package exec

import (
	"errors"
	"io/fs"

	"github.com/webcartel/sweep/internal/model"
)

// skipForErr says which reason of the report an error in putting a path away
// stands for. Without Windows only the errors of the standard library are
// known; the ones that take a file number live in the file for Windows.
func skipForErr(err error) (model.SkipReason, bool) {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return model.ReasonAccessDenied, true
	case errors.Is(err, fs.ErrNotExist):
		return model.ReasonNotFound, true
	default:
		return "", false
	}
}
