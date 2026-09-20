//go:build linux

package nginxmgr

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func validateTrustedPathsFile(path string, info os.FileInfo) error {
	if info.Mode().Perm()&0o022 != 0 {
		return errors.New("trusted paths config must not be group/world writable")
	}
	if path != trustedPathsConfigDefault {
		return nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("cannot verify trusted paths config owner")
	}
	if stat.Uid != 0 {
		return errors.New("trusted paths config must be owned by root")
	}

	parentInfo, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return errors.New("cannot verify trusted paths config directory")
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return errors.New("trusted paths config directory is invalid")
	}
	if parentInfo.Mode().Perm()&0o022 != 0 {
		return errors.New("trusted paths config directory must not be group/world writable")
	}
	parentStat, ok := parentInfo.Sys().(*syscall.Stat_t)
	if !ok || parentStat.Uid != 0 {
		return errors.New("trusted paths config directory must be owned by root")
	}
	return nil
}
