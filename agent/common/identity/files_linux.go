//go:build linux

package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func openIdentityFile(path string) (*os.File, error) {
	// A replacement symlink or FIFO must not redirect or block metadata loading.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

func singleIdentityLink(info os.FileInfo) bool {
	s, ok := info.Sys().(*syscall.Stat_t)
	return ok && s.Nlink == 1 && s.Uid == uint32(os.Geteuid())
}

func validateIdentityAncestors(path string) error {
	for parent := path; ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("IDENTITY_PARENT_INVALID: %w", err)
		}
		if err == nil && !trustedIdentityDirectory(info) {
			return fmt.Errorf("IDENTITY_PARENT_UNSAFE: state ancestry must be a non-symlink protected directory")
		}
		if parent == filepath.Dir(parent) {
			return nil
		}
	}
}

func trustedIdentityDirectory(info os.FileInfo) bool {
	s, ok := info.Sys().(*syscall.Stat_t)
	// A foreign owner can rename its directory even when its current mode is
	// not group/world writable. Only root and this Agent's user are trusted.
	return ok && (s.Uid == 0 || s.Uid == uint32(os.Geteuid())) &&
		info.Mode()&os.ModeSymlink == 0 && info.IsDir() &&
		(info.Mode().Perm()&0022 == 0 || info.Mode()&os.ModeSticky != 0)
}

func syncIdentityPath(path string, syncDir func(string) error) error {
	// MkdirAll may have created several directory entries. Reconcile the
	// entire validated ancestry, including after an uncertain prior attempt.
	for {
		if err := syncDir(path); err != nil {
			return err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

func publishIdentity(temp, target string) error {
	// Atomic no-replace publication has no intermediate hard-link state to
	// reconcile after a crash. Unsupported filesystems fail closed.
	return unix.Renameat2(unix.AT_FDCWD, temp, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
}

func syncIdentityDir(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("IDENTITY_DIRECTORY_SYNC_FAILED: %w", err)
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("IDENTITY_DIRECTORY_SYNC_FAILED: %w", err)
	}
	return nil
}
