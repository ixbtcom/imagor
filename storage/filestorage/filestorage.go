package filestorage

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cshum/imagor"
	"github.com/cshum/imagor/imagorpath"
)

// contentMD5Xattr is the extended attribute holding "<md5>:<size>" of a stored file
const contentMD5Xattr = "user.imagor.md5"

var dotFileRegex = regexp.MustCompile("/\\.")

// FileStorage File Storage implements imagor.Storage interface
type FileStorage struct {
	BaseDir         string
	PathPrefix      string
	Blacklists      []*regexp.Regexp
	MkdirPermission os.FileMode
	WritePermission os.FileMode
	SaveErrIfExists bool
	SafeChars       string
	Expiration      time.Duration

	safeChars imagorpath.SafeChars
}

// New creates FileStorage
func New(baseDir string, options ...Option) *FileStorage {
	s := &FileStorage{
		BaseDir:         baseDir,
		PathPrefix:      "/",
		Blacklists:      []*regexp.Regexp{dotFileRegex},
		MkdirPermission: 0755,
		WritePermission: 0666,
	}
	for _, option := range options {
		option(s)
	}
	s.safeChars = imagorpath.NewSafeChars(s.SafeChars)
	return s
}

// Path transforms and validates image key for storage path
func (s *FileStorage) Path(image string) (string, bool) {
	image = "/" + imagorpath.Normalize(image, s.safeChars)
	for _, blacklist := range s.Blacklists {
		if blacklist.MatchString(image) {
			return "", false
		}
	}
	if !strings.HasPrefix(image, s.PathPrefix) {
		return "", false
	}
	return filepath.Join(s.BaseDir, strings.TrimPrefix(image, s.PathPrefix)), true
}

// Get implements imagor.Storage interface
func (s *FileStorage) Get(_ *http.Request, image string) (*imagor.Blob, error) {
	image, ok := s.Path(image)
	if !ok {
		return nil, imagor.ErrInvalid
	}
	blob := imagor.NewBlobFromFile(image, func(stat os.FileInfo) error {
		if s.Expiration > 0 && time.Now().Sub(stat.ModTime()) > s.Expiration {
			return imagor.ErrExpired
		}
		return nil
	})
	if blob.Err() == nil && blob.Stat != nil {
		blob.Stat.ContentMD5 = contentMD5(image, blob.Stat.Size)
	}
	return blob, blob.Err()
}

// contentMD5 returns the hex md5 of the file content from its "<md5>:<size>" attribute
// when the size still matches, otherwise hashes the file and stores the attribute (best effort)
func contentMD5(path string, size int64) string {
	if sum, n, ok := strings.Cut(getContentMD5Xattr(path), ":"); ok &&
		len(sum) == 32 && n == strconv.FormatInt(size, 10) {
		if _, err := hex.DecodeString(sum); err == nil {
			return sum
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() {
		_ = f.Close()
	}()
	h := md5.New()
	n, err := io.Copy(h, f)
	if err != nil || n != size {
		return ""
	}
	sum := hex.EncodeToString(h.Sum(nil))
	_ = setContentMD5Path(path, sum+":"+strconv.FormatInt(n, 10))
	return sum
}

// Put implements imagor.Storage interface
func (s *FileStorage) Put(_ context.Context, image string, blob *imagor.Blob) (err error) {
	image, ok := s.Path(image)
	if !ok {
		return imagor.ErrInvalid
	}
	if err = os.MkdirAll(filepath.Dir(image), s.MkdirPermission); err != nil {
		return
	}
	reader, _, err := blob.NewReader()
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()
	// write a temp file in the same dir with its md5 attribute, then publish it atomically,
	// so a reader never sees new content with the md5 of the old one
	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		return
	}
	tmp := filepath.Join(filepath.Dir(image), ".imagor-"+hex.EncodeToString(suffix[:]))
	w, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_EXCL, s.WritePermission)
	if err != nil {
		return
	}
	defer func() {
		_ = w.Close()
		_ = os.Remove(tmp)
	}()
	h := md5.New()
	n, err := io.Copy(io.MultiWriter(w, h), reader)
	if err != nil {
		return
	}
	if err = w.Sync(); err != nil {
		return
	}
	_ = setContentMD5Fd(w, hex.EncodeToString(h.Sum(nil))+":"+strconv.FormatInt(n, 10))
	if s.SaveErrIfExists {
		return os.Link(tmp, image)
	}
	return os.Rename(tmp, image)
}

// Delete implements imagor.Storage interface
func (s *FileStorage) Delete(_ context.Context, image string) error {
	image, ok := s.Path(image)
	if !ok {
		return imagor.ErrInvalid
	}
	return os.Remove(image)
}

// Stat implements imagor.Storage interface
func (s *FileStorage) Stat(_ context.Context, image string) (stat *imagor.Stat, err error) {
	image, ok := s.Path(image)
	if !ok {
		return nil, imagor.ErrInvalid
	}
	osStat, err := os.Stat(image)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, imagor.ErrNotFound
		}
		return nil, err
	}
	size := osStat.Size()
	modTime := osStat.ModTime()
	return &imagor.Stat{
		Size:         size,
		ModifiedTime: modTime,
	}, nil
}
