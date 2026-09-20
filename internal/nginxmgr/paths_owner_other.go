//go:build !linux

package nginxmgr

import "os"

func validateTrustedPathsFile(_ string, _ os.FileInfo) error {
	return nil
}
