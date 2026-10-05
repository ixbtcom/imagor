//go:build !linux && !darwin

package filestorage

import (
	"errors"
	"os"
)

func setContentMD5Fd(_ *os.File, _ string) error {
	return errors.ErrUnsupported
}

func setContentMD5Path(_, _ string) error {
	return errors.ErrUnsupported
}

func getContentMD5Xattr(_ string) string {
	return ""
}
