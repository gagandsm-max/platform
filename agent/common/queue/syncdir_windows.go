//go:build windows

package queue

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

func syncDir(dir string) (err error) {
	path, err := syncDirPath(dir)
	if err != nil {
		return &os.PathError{Op: "open", Path: dir, Err: err}
	}

	// Like os.Open, resolve reparse points normally. The queue's existing
	// path checks still apply. Inspect the resolved handle before flushing.
	handle, err := windows.CreateFile(path, windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return &os.PathError{Op: "CreateFile", Path: dir, Err: err}
	}
	defer func() {
		if closeErr := windows.CloseHandle(handle); closeErr != nil {
			closeErr = &os.PathError{Op: "CloseHandle", Path: dir, Err: closeErr}
			if err == nil {
				err = closeErr
			} else {
				err = errors.Join(err, closeErr)
			}
		}
	}()

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return &os.PathError{Op: "GetFileInformationByHandle", Path: dir, Err: err}
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return &os.PathError{Op: "sync", Path: dir, Err: ErrInvalidDirectory}
	}
	if err := windows.FlushFileBuffers(handle); err != nil {
		return &os.PathError{Op: "FlushFileBuffers", Path: dir, Err: err}
	}
	return nil
}

func syncDirPath(dir string) (*uint16, error) {
	// Queue callers supply absolute paths. Preserve the long drive and UNC
	// paths supported by os.Open, and keep explicit namespaces intact.
	if len(dir) >= 248 {
		normalized := strings.ReplaceAll(dir, "/", "\\")
		switch {
		case strings.HasPrefix(normalized, `\\?\`), strings.HasPrefix(normalized, `\\.\`), strings.HasPrefix(dir, `\??\`):
		default:
			full, err := windows.FullPath(dir)
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(full, `\\`) {
				dir = `\\?\UNC\` + full[2:]
			} else {
				dir = `\\?\` + full
			}
		}
	}
	return windows.UTF16PtrFromString(dir)
}
