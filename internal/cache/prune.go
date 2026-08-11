package cache

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PruneOptions bounds retained canonical entries and may remove publication
// temporaries older than an explicit cutoff. A zero size field leaves that
// dimension unlimited; at least one positive size limit is required.
type PruneOptions struct {
	MaxEntries           int
	MaxBytes             int64
	StaleTemporaryBefore time.Time
}

// PruneResult reports one best-effort snapshot of canonical cache entries.
// Concurrent stores may change the root after it is scanned.
type PruneResult struct {
	EntriesBefore    int
	BytesBefore      int64
	EntriesRemoved   int
	BytesRemoved     int64
	CorruptRemoved   int
	TemporaryRemoved int
	EntriesAfter     int
	BytesAfter       int64
}

type pruneCandidate struct {
	key      Key
	modified time.Time
	size     int64
}

// Prune removes eligible stale publication temporaries and canonical corrupt
// entries, then evicts the oldest valid entries until both configured limits
// are satisfied. Publication time is represented by file modification time;
// equal times are ordered by key.
func (s *Store) Prune(
	ctx context.Context,
	options PruneOptions,
) (PruneResult, error) {
	var result PruneResult
	if ctx == nil {
		return result, fmt.Errorf("cache prune requires a context")
	}
	if options.MaxEntries < 0 || options.MaxBytes < 0 {
		return result, fmt.Errorf("cache prune limits must not be negative")
	}
	if options.MaxEntries == 0 && options.MaxBytes == 0 {
		return result, fmt.Errorf("cache prune requires an entry or byte limit")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if s == nil {
		return result, fmt.Errorf("cache prune requires a store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return result, fmt.Errorf("cache store is closed")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	candidates, err := s.pruneCandidates(ctx, options, &result)
	if err != nil {
		return result, err
	}
	sort.Slice(candidates, func(left, right int) bool {
		if !candidates[left].modified.Equal(candidates[right].modified) {
			return candidates[left].modified.Before(candidates[right].modified)
		}
		return candidates[left].key.String() < candidates[right].key.String()
	})
	remainingEntries := len(candidates)
	remainingBytes := int64(0)
	for _, candidate := range candidates {
		remainingBytes, err = addPruneBytes(remainingBytes, candidate.size)
		if err != nil {
			return result, err
		}
	}
	for _, candidate := range candidates {
		if !pruneLimitExceeded(options, remainingEntries, remainingBytes) {
			break
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		err := s.root.Remove(entryName(candidate.key))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return result, fmt.Errorf("remove cache entry during prune: %w", err)
		}
		remainingEntries--
		remainingBytes -= candidate.size
		if err == nil {
			result.EntriesRemoved++
			result.BytesRemoved += candidate.size
		}
	}
	result.EntriesAfter = remainingEntries
	result.BytesAfter = remainingBytes
	return result, nil
}

func (s *Store) pruneCandidates(
	ctx context.Context,
	options PruneOptions,
	result *PruneResult,
) ([]pruneCandidate, error) {
	shards, err := readCacheDirectory(s.root, entryVersion)
	if errors.Is(err, fs.ErrNotExist) {
		return []pruneCandidate{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cache version directory: %w", err)
	}
	sort.Slice(shards, func(left, right int) bool { return shards[left].Name() < shards[right].Name() })
	candidates := make([]pruneCandidate, 0)
	for _, shard := range shards {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !canonicalCacheShard(shard) {
			continue
		}
		shardName := filepath.Join(entryVersion, shard.Name())
		entries, err := readCacheDirectory(s.root, shardName)
		if err != nil {
			return nil, fmt.Errorf("read cache shard %q: %w", shard.Name(), err)
		}
		sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
		for _, entry := range entries {
			information, err := entry.Info()
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("inspect cache entry during prune: %w", err)
			}
			if !information.Mode().IsRegular() {
				continue
			}
			if !options.StaleTemporaryBefore.IsZero() &&
				information.ModTime().Before(options.StaleTemporaryBefore) &&
				canonicalCacheTemporary(shard.Name(), entry.Name()) {
				err := s.root.Remove(filepath.Join(shardName, entry.Name()))
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					return nil, fmt.Errorf("remove stale cache temporary during prune: %w", err)
				}
				if err == nil {
					result.TemporaryRemoved++
				}
				continue
			}
			key, canonical := canonicalCacheEntryKey(shard.Name(), entry.Name())
			if !canonical {
				continue
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			path := entryName(key)
			result.EntriesBefore++
			result.BytesBefore, err = addPruneBytes(result.BytesBefore, information.Size())
			if err != nil {
				return nil, err
			}
			_, state, err := s.load(key)
			if err != nil {
				return nil, err
			}
			if state != entryValid {
				err := s.root.Remove(path)
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					return nil, fmt.Errorf("remove corrupt cache entry during prune: %w", err)
				}
				if err == nil {
					result.EntriesRemoved++
					result.BytesRemoved += information.Size()
					result.CorruptRemoved++
				}
				continue
			}
			candidates = append(candidates, pruneCandidate{
				key: key, modified: information.ModTime(), size: information.Size(),
			})
		}
	}
	return candidates, nil
}

func canonicalCacheTemporary(shard, name string) bool {
	const suffix = ".tmp"
	keyLength := hex.EncodedLen(len(Key{}))
	randomLength := hex.EncodedLen(temporaryRandomSize)
	if len(name) != 1+keyLength+1+randomLength+len(suffix) ||
		!strings.HasPrefix(name, ".") || !strings.HasSuffix(name, suffix) {
		return false
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(name, "."), suffix)
	if encoded != strings.ToLower(encoded) || encoded[keyLength] != '.' {
		return false
	}
	key := encoded[:keyLength]
	random := encoded[keyLength+1:]
	keyBytes, keyErr := hex.DecodeString(key)
	randomBytes, randomErr := hex.DecodeString(random)
	return keyErr == nil && len(keyBytes) == len(Key{}) &&
		hex.EncodeToString(keyBytes) == key && strings.HasPrefix(key, shard) &&
		randomErr == nil && len(randomBytes) == temporaryRandomSize &&
		hex.EncodeToString(randomBytes) == random
}

func readCacheDirectory(root *os.Root, name string) ([]os.DirEntry, error) {
	directory, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	return entries, errors.Join(readErr, closeErr)
}

func canonicalCacheShard(entry os.DirEntry) bool {
	name := entry.Name()
	if len(name) != 2 || name != strings.ToLower(name) || !entry.IsDir() {
		return false
	}
	decoded, err := hex.DecodeString(name)
	return err == nil && len(decoded) == 1
}

func canonicalCacheEntryKey(shard, name string) (Key, bool) {
	const suffix = ".cache"
	if len(name) != hex.EncodedLen(len(Key{}))+len(suffix) || !strings.HasSuffix(name, suffix) {
		return Key{}, false
	}
	encoded := strings.TrimSuffix(name, suffix)
	if encoded != strings.ToLower(encoded) || !strings.HasPrefix(encoded, shard) {
		return Key{}, false
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil || len(decoded) != len(Key{}) || hex.EncodeToString(decoded) != encoded {
		return Key{}, false
	}
	var key Key
	copy(key[:], decoded)
	return key, true
}

func pruneLimitExceeded(options PruneOptions, entries int, bytes int64) bool {
	return options.MaxEntries > 0 && entries > options.MaxEntries ||
		options.MaxBytes > 0 && bytes > options.MaxBytes
}

func addPruneBytes(total, size int64) (int64, error) {
	if size < 0 || total > math.MaxInt64-size {
		return 0, fmt.Errorf("cache prune byte total exceeds int64")
	}
	return total + size, nil
}
