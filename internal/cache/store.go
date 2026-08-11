package cache

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

const (
	entryMagic   = "GOXCACHE\x01"
	entryVersion = "v1"
	headerSize   = len(entryMagic) + sha256.Size + sha256.Size + 8

	// MaxEntrySize bounds one decoded cache value.
	MaxEntrySize = 16 << 20
)

// ErrConflict reports two valid values for one deterministic cache key.
var ErrConflict = errors.New("cache entry conflicts with an existing value")

type entryState uint8

const (
	entryMissing entryState = iota
	entryCorrupt
	entryValid
)

// Store is one rooted, content-verifying persistent cache.
type Store struct {
	mu     sync.RWMutex
	root   *os.Root
	closed bool
}

// Open creates or opens one normalized absolute cache root.
func Open(path string) (*Store, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("cache root %q is not normalized absolute", path)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, fmt.Errorf("create cache root: %w", err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("open cache root: %w", err)
	}
	if err := root.MkdirAll(entryVersion, 0o700); err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("create cache version directory: %w", err)
	}
	return &Store{root: root}, nil
}

// Close releases the rooted cache handle.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.root.Close()
}

// Get returns an independently owned verified payload. Missing and corrupt
// entries are cache misses so callers can recompute them.
func (s *Store) Get(ctx context.Context, key Key) ([]byte, bool, error) {
	if ctx == nil {
		return nil, false, fmt.Errorf("cache get requires a context")
	}
	if key == (Key{}) {
		return nil, false, fmt.Errorf("cache get requires a non-zero key")
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if s == nil {
		return nil, false, fmt.Errorf("cache get requires a store")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, false, fmt.Errorf("cache store is closed")
	}
	payload, state, err := s.load(key)
	if err != nil {
		return nil, false, err
	}
	if state != entryValid {
		return nil, false, nil
	}
	return payload, true, nil
}

// Put atomically publishes one verified payload. A valid different value for
// the same key fails instead of hiding nondeterministic computation.
func (s *Store) Put(ctx context.Context, key Key, payload []byte) (resultErr error) {
	if ctx == nil {
		return fmt.Errorf("cache put requires a context")
	}
	if key == (Key{}) {
		return fmt.Errorf("cache put requires a non-zero key")
	}
	if len(payload) > MaxEntrySize {
		return fmt.Errorf("cache payload is %d bytes; maximum is %d", len(payload), MaxEntrySize)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("cache put requires a store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("cache store is closed")
	}
	existing, state, err := s.load(key)
	if err != nil {
		return err
	}
	switch state {
	case entryValid:
		if bytes.Equal(existing, payload) {
			return nil
		}
		return fmt.Errorf("%w for key %s", ErrConflict, key)
	case entryCorrupt:
		if err := s.root.Remove(entryName(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove corrupt cache entry: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	shard := entryShard(key)
	if err := s.root.MkdirAll(shard, 0o700); err != nil {
		return fmt.Errorf("create cache shard: %w", err)
	}
	temporary, err := temporaryName(key)
	if err != nil {
		return err
	}
	file, err := s.root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create cache temporary entry: %w", err)
	}
	defer func() {
		if err := s.root.Remove(temporary); err != nil && !errors.Is(err, fs.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove cache temporary entry: %w", err))
		}
	}()
	encoded := encodeEntry(key, payload)
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		return fmt.Errorf("write cache temporary entry: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync cache temporary entry: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close cache temporary entry: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.root.Link(temporary, entryName(key)); err != nil {
		current, currentState, loadErr := s.load(key)
		if loadErr == nil && currentState == entryValid && bytes.Equal(current, payload) {
			return nil
		}
		if loadErr == nil && currentState == entryValid {
			return fmt.Errorf("%w for key %s", ErrConflict, key)
		}
		return fmt.Errorf("publish cache entry: %w", err)
	}
	return nil
}

func (s *Store) load(key Key) ([]byte, entryState, error) {
	file, err := s.root.Open(entryName(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, entryMissing, nil
	}
	if err != nil {
		return nil, entryMissing, fmt.Errorf("open cache entry: %w", err)
	}
	defer file.Close()
	information, err := file.Stat()
	if err != nil {
		return nil, entryMissing, fmt.Errorf("stat cache entry: %w", err)
	}
	if !information.Mode().IsRegular() || information.Size() < int64(headerSize) ||
		information.Size() > int64(headerSize+MaxEntrySize) {
		return nil, entryCorrupt, nil
	}
	encoded, err := io.ReadAll(io.LimitReader(file, int64(headerSize+MaxEntrySize+1)))
	if err != nil {
		return nil, entryMissing, fmt.Errorf("read cache entry: %w", err)
	}
	payload, valid := decodeEntry(key, encoded)
	if !valid {
		return nil, entryCorrupt, nil
	}
	return payload, entryValid, nil
}

func encodeEntry(key Key, payload []byte) []byte {
	encoded := make([]byte, headerSize+len(payload))
	copy(encoded, entryMagic)
	offset := len(entryMagic)
	copy(encoded[offset:], key[:])
	offset += len(key)
	digest := sha256.Sum256(payload)
	copy(encoded[offset:], digest[:])
	offset += len(digest)
	binary.BigEndian.PutUint64(encoded[offset:], uint64(len(payload)))
	copy(encoded[headerSize:], payload)
	return encoded
}

func decodeEntry(key Key, encoded []byte) ([]byte, bool) {
	if len(encoded) < headerSize || string(encoded[:len(entryMagic)]) != entryMagic {
		return nil, false
	}
	offset := len(entryMagic)
	if !bytes.Equal(encoded[offset:offset+len(key)], key[:]) {
		return nil, false
	}
	offset += len(key)
	wantDigest := encoded[offset : offset+sha256.Size]
	offset += sha256.Size
	length := binary.BigEndian.Uint64(encoded[offset : offset+8])
	if length > MaxEntrySize || length != uint64(len(encoded)-headerSize) {
		return nil, false
	}
	payload := encoded[headerSize:]
	gotDigest := sha256.Sum256(payload)
	if !bytes.Equal(gotDigest[:], wantDigest) {
		return nil, false
	}
	return bytes.Clone(payload), true
}

func entryShard(key Key) string { return filepath.Join(entryVersion, key.String()[:2]) }

func entryName(key Key) string {
	return filepath.Join(entryShard(key), key.String()+".cache")
}

func temporaryName(key Key) (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("create cache temporary name: %w", err)
	}
	return filepath.Join(
		entryShard(key),
		"."+key.String()+"."+hex.EncodeToString(suffix[:])+".tmp",
	), nil
}
