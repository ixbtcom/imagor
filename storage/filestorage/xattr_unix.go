//go:build linux || darwin

package filestorage

import (
	"os"

	"golang.org/x/sys/unix"
)

// setContentMD5Fd stores the content md5 attribute on an open file
func setContentMD5Fd(f *os.File, value string) error {
	return unix.Fsetxattr(int(f.Fd()), contentMD5Xattr, []byte(value), 0)
}

// getContentMD5Xattr returns the content md5 attribute of a file path, empty if absent
func getContentMD5Xattr(path string) string {
	buf := make([]byte, 64)
	n, err := unix.Getxattr(path, contentMD5Xattr, buf)
	if err != nil || n <= 0 {
		return ""
	}
	return string(buf[:n])
}
