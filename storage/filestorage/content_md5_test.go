//go:build linux || darwin

package filestorage

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cshum/imagor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func readMD5Xattr(t *testing.T, path string) string {
	buf := make([]byte, 128)
	n, err := unix.Getxattr(path, contentMD5Xattr, buf)
	require.NoError(t, err)
	return string(buf[:n])
}

func TestFileStorage_ContentMD5(t *testing.T) {
	ctx := context.Background()
	r := (&http.Request{}).WithContext(ctx)
	dir := t.TempDir()
	s := New(dir)
	path := filepath.Join(dir, "foo", "bar.jpg")

	getMD5 := func() string {
		b, err := s.Get(r, "/foo/bar.jpg")
		require.NoError(t, err)
		require.NotNil(t, b.Stat)
		return b.Stat.ContentMD5
	}

	t.Run("put stores md5:size and get returns md5", func(t *testing.T) {
		require.NoError(t, s.Put(ctx, "/foo/bar.jpg", imagor.NewBlobFromBytes([]byte("hello"))))
		assert.Equal(t, md5hex("hello")+":5", readMD5Xattr(t, path))
		assert.Equal(t, md5hex("hello"), getMD5())
		entries, err := os.ReadDir(filepath.Dir(path))
		require.NoError(t, err)
		assert.Len(t, entries, 1, "временный файл не должен оставаться")
	})

	t.Run("overwrite with other content changes md5", func(t *testing.T) {
		require.NoError(t, s.Put(ctx, "/foo/bar.jpg", imagor.NewBlobFromBytes([]byte("world!"))))
		assert.Equal(t, md5hex("world!")+":6", readMD5Xattr(t, path))
		assert.Equal(t, md5hex("world!"), getMD5())
	})

	t.Run("xattr with wrong size is ignored and rewritten", func(t *testing.T) {
		stale := md5hex("other") + ":" + strconv.Itoa(len("world!")+1)
		require.NoError(t, unix.Setxattr(path, contentMD5Xattr, []byte(stale), 0))
		assert.Equal(t, md5hex("world!"), getMD5())
		assert.Equal(t, md5hex("world!")+":6", readMD5Xattr(t, path))
	})

	t.Run("file without xattr gets md5 on first get", func(t *testing.T) {
		plain := filepath.Join(dir, "foo", "plain.jpg")
		require.NoError(t, os.WriteFile(plain, []byte("legacy"), 0644))
		b, err := s.Get(r, "/foo/plain.jpg")
		require.NoError(t, err)
		assert.Equal(t, md5hex("legacy"), b.Stat.ContentMD5)
		assert.Equal(t, md5hex("legacy")+":6", readMD5Xattr(t, plain))
	})

	t.Run("save err if exists keeps old file and xattr", func(t *testing.T) {
		se := New(dir, WithSaveErrIfExists(true))
		assert.Error(t, se.Put(ctx, "/foo/bar.jpg", imagor.NewBlobFromBytes([]byte("nope"))))
		assert.Equal(t, md5hex("world!")+":6", readMD5Xattr(t, path))
		assert.Equal(t, md5hex("world!"), getMD5())
		entries, err := os.ReadDir(filepath.Dir(path))
		require.NoError(t, err)
		assert.Len(t, entries, 2, "временный файл не должен оставаться")
	})

	t.Run("write permission still honours umask", func(t *testing.T) {
		sp := New(dir, WithWritePermission("0666"))
		require.NoError(t, sp.Put(ctx, "/foo/perm.jpg", imagor.NewBlobFromBytes([]byte("p"))))
		info, err := os.Stat(filepath.Join(dir, "foo", "perm.jpg"))
		require.NoError(t, err)
		old := unix.Umask(0)
		unix.Umask(old)
		assert.Equal(t, os.FileMode(0666&^old), info.Mode().Perm())
	})
}
