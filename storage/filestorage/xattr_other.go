//go:build !linux && !darwin

package filestorage

import (
	"errors"
	"os"
)

func setContentMD5Fd(_ *os.File, _ string) error {
	return errors.ErrUnsupported
}

func getContentMD5Xattr(_ string) string {
	return ""
}
