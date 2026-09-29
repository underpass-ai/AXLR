package local

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/domain"
)

type FileAdapter struct{ root *os.Root }

func NewFileAdapter(path string) (*FileAdapter, error) {
	r, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &FileAdapter{root: r}, nil
}
func (a *FileAdapter) Close() error { return a.root.Close() }

func (a *FileAdapter) Read(c domain.ReadCommand) (domain.ReadResult, error) {
	f, err := a.root.OpenFile(string(c.Path), os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return domain.ReadResult{}, fileError(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return domain.ReadResult{}, fileError(err)
	}
	if !info.Mode().IsRegular() {
		return domain.ReadResult{}, domain.Reject("not_regular_file", "read requires a regular file")
	}
	offset := int64(c.Offset)
	max := int(c.Limit)
	if offset > info.Size() {
		return domain.ReadResult{}, domain.Reject("invalid_offset", "offset is beyond end of file")
	}
	if offset < info.Size() {
		var one [1]byte
		if _, err = f.ReadAt(one[:], offset); err != nil {
			return domain.ReadResult{}, fileError(err)
		}
		if one[0]&0xc0 == 0x80 {
			return domain.ReadResult{}, domain.Reject("invalid_offset", "offset splits a UTF-8 character")
		}
	}
	buf := make([]byte, max+4)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return domain.ReadResult{}, fileError(err)
	}
	pos := 0
	for pos < n && pos < max {
		r, size := utf8.DecodeRune(buf[pos:n])
		if r == utf8.RuneError && size == 1 {
			return domain.ReadResult{}, domain.Reject("invalid_text", "file contains invalid UTF-8")
		}
		if r == 0 {
			return domain.ReadResult{}, domain.Reject("invalid_text", "file contains binary data")
		}
		if pos+size > max {
			break
		}
		pos += size
	}
	if pos == 0 && n > 0 {
		return domain.ReadResult{}, domain.Reject("limit_too_small", "max_bytes is smaller than the next UTF-8 character")
	}
	next := offset + int64(pos)
	truncated := next < info.Size() || pos < n
	result := domain.ReadResult{Content: string(buf[:pos]), StartOffset: c.Offset, ReturnedBytes: pos, NextOffset: domain.ByteOffset(next), Truncated: truncated}
	if offset == 0 && !truncated {
		result.Digest = domain.DigestOf(buf[:pos])
	}
	return result, nil
}

func (a *FileAdapter) Load(path domain.RelativePath, max int) (domain.FileSnapshot, error) {
	info, err := a.root.Lstat(string(path))
	if err != nil {
		return domain.FileSnapshot{}, fileError(err)
	}
	if !info.Mode().IsRegular() {
		return domain.FileSnapshot{}, domain.Reject("not_regular_file", "editing requires a regular file, not a symlink or directory")
	}
	if info.Size() > int64(max) {
		return domain.FileSnapshot{}, domain.Reject("file_too_large", "file exceeds editable limit")
	}
	f, err := a.root.OpenFile(string(path), os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return domain.FileSnapshot{}, fileError(err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil {
		return domain.FileSnapshot{}, fileError(err)
	}
	if len(b) > max {
		return domain.FileSnapshot{}, domain.Reject("file_too_large", "file exceeds editable limit")
	}
	if _, err := domain.NewText(string(b)); err != nil {
		return domain.FileSnapshot{}, domain.Reject("invalid_text", "file contains binary or invalid UTF-8 data")
	}
	return domain.FileSnapshot{Content: b, Permissions: domain.NewFilePermissions(uint32(info.Mode().Perm()))}, nil
}

func (a *FileAdapter) Create(path domain.RelativePath, content []byte) error {
	return a.publish(path, content, 0600, true)
}
func (a *FileAdapter) Replace(path domain.RelativePath, content []byte, permissions domain.FilePermissions) error {
	return a.publish(path, content, os.FileMode(permissions)&0777, false)
}

func (a *FileAdapter) publish(path domain.RelativePath, content []byte, mode os.FileMode, create bool) error {
	dir, err := a.root.OpenRoot(filepath.Dir(string(path)))
	if err != nil {
		return fileError(err)
	}
	defer dir.Close()
	base := filepath.Base(string(path))
	var tmp string
	var f *os.File
	for i := 0; i < 4; i++ {
		var entropy [8]byte
		if _, err = rand.Read(entropy[:]); err != nil {
			return domain.Fail("filesystem_error", err.Error())
		}
		tmp = ".axlr-" + hex.EncodeToString(entropy[:])
		f, err = dir.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		break
	}
	if err != nil {
		return fileError(err)
	}
	defer dir.Remove(tmp)
	if _, err = f.Write(content); err != nil {
		f.Close()
		return fileError(err)
	}
	if err = f.Chmod(mode.Perm()); err != nil {
		f.Close()
		return fileError(err)
	}
	if err = f.Close(); err != nil {
		return fileError(err)
	}
	if create {
		err = dir.Link(tmp, base)
		if errors.Is(err, os.ErrExist) {
			return domain.Reject("conflict", "destination already exists")
		}
	} else {
		err = dir.Rename(tmp, base)
	}
	if err != nil {
		return fileError(err)
	}
	return nil
}

func fileError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return domain.Fail("not_found", "file or directory not found")
	}
	if errors.Is(err, os.ErrPermission) {
		return domain.Fail("permission_denied", "file operation denied")
	}
	return domain.Fail("filesystem_error", err.Error())
}
