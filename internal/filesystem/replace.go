// Package filesystem owns validated source snapshots and atomic replacement.
package filesystem

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/faustbrian/gox/internal/source"
)

// ErrStale reports that a source path no longer identifies the bytes read.
var ErrStale = errors.New("source changed since it was read")

// Snapshot is one immutable regular-file version eligible for replacement.
type Snapshot struct {
	path string
	root string
	name string
	bytes []byte
	digest [sha256.Size]byte
	info os.FileInfo
	rootInfo os.FileInfo
}

// Read captures one regular file without following a final symlink.
func Read(path string) (*Snapshot, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve source path %q: %w", path, err)
	}
	return ReadWithin(filepath.Dir(absolute), absolute)
}

// ReadWithin captures one regular file without escaping its authorized root.
func ReadWithin(root, path string) (*Snapshot, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve source path %q: %w", path, err)
	}
	rootAbsolute := root
	if rootAbsolute == "" {
		rootAbsolute = filepath.Dir(absolute)
	} else {
		rootAbsolute, err = filepath.Abs(rootAbsolute)
		if err != nil {
			return nil, fmt.Errorf("resolve source root %q: %w", root, err)
		}
	}
	name, err := filepath.Rel(rootAbsolute, absolute)
	if err != nil ||
		name == ".." ||
		strings.HasPrefix(name, ".." + string(filepath.Separator)) {
		return nil, fmt.Errorf(
			"source path %q is outside authorized root %q",
			absolute,
			rootAbsolute,
		)
	}
	boundary, err := os.OpenRoot(rootAbsolute)
	if err != nil {
		return nil, fmt.Errorf("open source root %q: %w", rootAbsolute, err)
	}
	defer boundary.Close()
	rootInfo, err := boundary.Stat(".")
	if err != nil {
		return nil, fmt.Errorf("inspect source root %q: %w", rootAbsolute, err)
	}
	listed, err := boundary.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("inspect source path %q: %w", absolute, err)
	}
	if listed.Mode() & os.ModeSymlink != 0 {
		return nil, fmt.Errorf("source path %q is a symlink", absolute)
	}
	if !listed.Mode().IsRegular() {
		return nil, fmt.Errorf("source path %q is not a regular file", absolute)
	}
	file, err := boundary.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open source path %q: %w", absolute, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened source %q: %w", absolute, err)
	}
	if !os.SameFile(listed, opened) {
		return nil, fmt.Errorf("read source path %q: %w", absolute, ErrStale)
	}
	if err := source.ValidateSize(opened.Size()); err != nil {
		return nil, fmt.Errorf("read source path %q: %w", absolute, err)
	}
	input, err := source.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read source path %q: %w", absolute, err)
	}
	return &Snapshot{
		path: absolute,
		root: rootAbsolute,
		name: name,
		bytes: input,
		digest: sha256.Sum256(input),
		info: opened,
		rootInfo: rootInfo,
	}, nil
}

// Path returns the normalized source identity.
func (s *Snapshot) Path() string {
	return s.path
}

// Bytes returns an independent copy of the captured source bytes.
func (s *Snapshot) Bytes() []byte {
	return bytes.Clone(s.bytes)
}

// Replace validates the source version and atomically replaces changed bytes.
func (s *Snapshot) Replace(output []byte) error {
	if err := source.ValidateSize(int64(len(output))); err != nil {
		return fmt.Errorf("replace source path %q: %w", s.path, err)
	}
	boundary, err := os.OpenRoot(s.root)
	if err != nil {
		return fmt.Errorf("open source root %q: %w", s.root, err)
	}
	defer boundary.Close()
	rootInfo, err := boundary.Stat(".")
	if err != nil || !os.SameFile(s.rootInfo, rootInfo) {
		return fmt.Errorf("validate source root %q: %w", s.root, ErrStale)
	}
	if err := s.validateCurrent(boundary); err != nil {
		return err
	}
	if bytes.Equal(s.bytes, output) {
		return nil
	}
	directory := filepath.Dir(s.name)
	temporaryPath, temporary, err := createTemporary(boundary, directory, filepath.Base(s.name))
	if err != nil {
		return fmt.Errorf("create replacement for %q: %w", s.path, err)
	}
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = boundary.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(output); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write replacement for %q: %w", s.path, err)
	}
	if err := temporary.Chmod(s.info.Mode().Perm()); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set replacement permissions for %q: %w", s.path, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync replacement for %q: %w", s.path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close replacement for %q: %w", s.path, err)
	}
	if err := s.validateCurrent(boundary); err != nil {
		return err
	}
	if err := boundary.Rename(temporaryPath, s.name); err != nil {
		return fmt.Errorf("replace source path %q: %w", s.path, err)
	}
	keepTemporary = false
	directoryHandle, err := boundary.Open(directory)
	if err != nil {
		return fmt.Errorf("open replacement directory %q: %w", directory, err)
	}
	defer directoryHandle.Close()
	if err := directoryHandle.Sync(); err != nil {
		return fmt.Errorf("sync replacement directory %q: %w", directory, err)
	}
	return nil
}

func (s *Snapshot) validateCurrent(boundary *os.Root) error {
	listed, err := boundary.Lstat(s.name)
	if err != nil {
		return fmt.Errorf("validate source path %q: %w (%v)", s.path, ErrStale, err)
	}
	if listed.Mode() & os.ModeSymlink != 0 ||
		!listed.Mode().IsRegular() ||
		!os.SameFile(s.info, listed) ||
		listed.Mode().Perm() != s.info.Mode().Perm() {
		return fmt.Errorf("validate source path %q: %w", s.path, ErrStale)
	}
	file, err := boundary.Open(s.name)
	if err != nil {
		return fmt.Errorf("validate source path %q: %w (%v)", s.path, ErrStale, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect current source %q: %w", s.path, err)
	}
	if !os.SameFile(listed, opened) ||
		!os.SameFile(s.info, opened) ||
		opened.Mode().Perm() != s.info.Mode().Perm() {
		return fmt.Errorf("validate source path %q: %w", s.path, ErrStale)
	}
	if opened.Size() != s.info.Size() {
		return fmt.Errorf("validate source size %q: %w", s.path, ErrStale)
	}
	current, err := source.ReadAll(file)
	if err != nil {
		return fmt.Errorf("read current source %q: %w", s.path, err)
	}
	if sha256.Sum256(current) != s.digest {
		return fmt.Errorf("validate source bytes %q: %w", s.path, ErrStale)
	}
	return nil
}

func createTemporary(boundary *os.Root, directory, base string) (string, *os.File, error) {
	for range 100 {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, err
		}
		name := filepath.Join(
			directory,
			"." + base + ".gox-" + hex.EncodeToString(random[:]),
		)
		file, err := boundary.OpenFile(name, os.O_WRONLY | os.O_CREATE | os.O_EXCL, 0o600)
		if err == nil {
			return name, file, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", nil, err
		}
	}
	return "", nil, errors.New("could not allocate a unique replacement name")
}
