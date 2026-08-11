package cache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestStorePersistsIndependentVerifiedPayloads(t *testing.T) {
	t.Parallel()

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	key, err := BuildKey(testKeyInput())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("cached diagnostics")
	if err := store.Put(context.Background(), key, payload); err != nil {
		t.Fatal(err)
	}
	payload[0] = 'X'
	got, found, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, []byte("cached diagnostics")) {
		t.Fatalf("Get() = %q, %t", got, found)
	}
	got[0] = 'Y'
	again, found, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(again, []byte("cached diagnostics")) {
		t.Fatalf("second Get() = %q, %t", again, found)
	}
}

func TestStoreRejectsConcurrentDifferentValuesForOneKey(t *testing.T) {
	t.Parallel()

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	key, err := BuildKey(testKeyInput())
	if err != nil {
		t.Fatal(err)
	}
	payloads := [][]byte{
		bytes.Repeat([]byte("a"), 1<<20),
		bytes.Repeat([]byte("b"), 1<<20),
	}
	start := make(chan struct{})
	results := make(chan error, len(payloads))
	for _, payload := range payloads {
		payload := payload
		go func() {
			<-start
			results <- store.Put(context.Background(), key, payload)
		}()
	}
	close(start)
	accepted, conflicts := 0, 0
	for range payloads {
		err := <-results
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("Put() error = %v", err)
		}
	}
	if accepted != 1 || conflicts != 1 {
		t.Fatalf("Put() outcomes = accepted %d, conflicts %d", accepted, conflicts)
	}
	got, found, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, payloads[0]) && !bytes.Equal(got, payloads[1]) {
		t.Fatalf("Get() returned invalid winner: found %t, bytes %d", found, len(got))
	}
}

func TestStoresRejectConcurrentDifferentValuesAcrossHandles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	stores := make([]*Store, 2)
	for index := range stores {
		store, err := Open(root)
		if err != nil {
			t.Fatal(err)
		}
		stores[index] = store
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	key, err := BuildKey(testKeyInput())
	if err != nil {
		t.Fatal(err)
	}
	payloads := [][]byte{
		bytes.Repeat([]byte("a"), 1<<20),
		bytes.Repeat([]byte("b"), 1<<20),
	}
	start := make(chan struct{})
	results := make(chan error, len(stores))
	for index, store := range stores {
		payload := payloads[index]
		go func() {
			<-start
			results <- store.Put(context.Background(), key, payload)
		}()
	}
	close(start)
	accepted, conflicts := 0, 0
	for range stores {
		err := <-results
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("Put() error = %v", err)
		}
	}
	if accepted != 1 || conflicts != 1 {
		t.Fatalf("Put() outcomes = accepted %d, conflicts %d", accepted, conflicts)
	}
}

func TestStoreTreatsCorruptionAsAMissAndRepairsIt(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	key, err := BuildKey(testKeyInput())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("recomputable diagnostics")
	if err := store.Put(context.Background(), key, payload); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, entryName(key)), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if found || got != nil {
		t.Fatalf("corrupt Get() = %q, %t", got, found)
	}
	if err := store.Put(context.Background(), key, payload); err != nil {
		t.Fatal(err)
	}
	got, found, err = store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, payload) {
		t.Fatalf("repaired Get() = %q, %t", got, found)
	}
}

func TestStorePrunesCorruptAndOldestEntriesWithinLimits(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	keys := make([]Key, 4)
	for index := range keys {
		input := testKeyInput()
		input.Namespace = fmt.Sprintf("prune-%d", index)
		keys[index], err = BuildKey(input)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Put(context.Background(), keys[index], []byte("payload")); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(int64(index+1), 0)
		if err := store.root.Chtimes(entryName(keys[index]), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, entryName(keys[3])), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(root, entryVersion, "keep-me")
	if err := os.WriteFile(unknown, []byte("caller-owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(root, entryShard(keys[0]), ".active.tmp")
	if err := os.WriteFile(temporary, []byte("in-progress"), 0o600); err != nil {
		t.Fatal(err)
	}
	directoryInput := testKeyInput()
	directoryInput.Namespace = "caller-owned-directory"
	directoryKey, err := BuildKey(directoryInput)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, entryName(directoryKey))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}

	encodedSize := int64(headerSize + len("payload"))
	result, err := store.Prune(context.Background(), PruneOptions{
		MaxEntries: 3,
		MaxBytes:   2 * encodedSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != (PruneResult{
		EntriesBefore:  4,
		BytesBefore:    3*encodedSize + int64(len("corrupt")),
		EntriesRemoved: 2,
		BytesRemoved:   encodedSize + int64(len("corrupt")),
		CorruptRemoved: 1,
		EntriesAfter:   2,
		BytesAfter:     2 * encodedSize,
	}) {
		t.Fatalf("Prune() = %#v", result)
	}
	for index, key := range keys {
		_, found, err := store.Get(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		want := index == 1 || index == 2
		if found != want {
			t.Fatalf("entry %d found = %t, want %t", index, found, want)
		}
	}
	if contents, err := os.ReadFile(unknown); err != nil || string(contents) != "caller-owned" {
		t.Fatalf("unknown cache file = %q, %v", contents, err)
	}
	if contents, err := os.ReadFile(temporary); err != nil || string(contents) != "in-progress" {
		t.Fatalf("temporary cache file = %q, %v", contents, err)
	}
	if information, err := os.Stat(directory); err != nil || !information.IsDir() {
		t.Fatalf("unknown cache directory = %#v, %v", information, err)
	}
}

func TestStorePruneRemovesOnlyCanonicalStaleTemporaryEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	key, err := BuildKey(testKeyInput())
	if err != nil {
		t.Fatal(err)
	}
	staleName, err := temporaryName(key)
	if err != nil {
		t.Fatal(err)
	}
	freshName, err := temporaryName(key)
	if err != nil {
		t.Fatal(err)
	}
	unknownName := filepath.Join(entryShard(key), ".unknown.tmp")
	if err := os.MkdirAll(filepath.Join(root, entryShard(key)), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{staleName, freshName, unknownName} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("temporary"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cutoff := time.Unix(1_000, 0)
	if err := store.root.Chtimes(staleName, cutoff.Add(-time.Second), cutoff.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.root.Chtimes(freshName, cutoff, cutoff); err != nil {
		t.Fatal(err)
	}
	if err := store.root.Chtimes(unknownName, cutoff.Add(-time.Second), cutoff.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}

	result, err := store.Prune(context.Background(), PruneOptions{
		MaxEntries:           1,
		StaleTemporaryBefore: cutoff,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TemporaryRemoved != 1 {
		t.Fatalf("Prune() temporary result = %#v", result)
	}
	if _, err := os.Lstat(filepath.Join(root, staleName)); !os.IsNotExist(err) {
		t.Fatalf("stale canonical temporary remains: %v", err)
	}
	for _, name := range []string{freshName, unknownName} {
		if contents, err := os.ReadFile(filepath.Join(root, name)); err != nil ||
			string(contents) != "temporary" {
			t.Fatalf("preserved temporary %q = %q, %v", name, contents, err)
		}
	}
}

func TestStorePruneBreaksAgeTiesByCanonicalKey(t *testing.T) {
	t.Parallel()

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	keys := make([]Key, 2)
	for index := range keys {
		input := testKeyInput()
		input.Namespace = fmt.Sprintf("tie-%d", index)
		keys[index], err = BuildKey(input)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Put(context.Background(), keys[index], []byte("payload")); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(1, 0)
		if err := store.root.Chtimes(entryName(keys[index]), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left].String() < keys[right].String() })
	if _, err := store.Prune(context.Background(), PruneOptions{MaxEntries: 1}); err != nil {
		t.Fatal(err)
	}
	for index, key := range keys {
		_, found, err := store.Get(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if found != (index == 1) {
			t.Fatalf("tie entry %d found = %t", index, found)
		}
	}
}

func TestStorePruneRacesEqualWritersAcrossHandlesWithoutInvalidData(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	stores := make([]*Store, 2)
	for index := range stores {
		store, err := Open(root)
		if err != nil {
			t.Fatal(err)
		}
		stores[index] = store
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	key, err := BuildKey(testKeyInput())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("recomputable payload")
	if err := stores[0].Put(context.Background(), key, payload); err != nil {
		t.Fatal(err)
	}

	errors_ := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		for range 100 {
			if err := stores[0].Put(context.Background(), key, payload); err != nil {
				errors_ <- err
				return
			}
			got, found, err := stores[0].Get(context.Background(), key)
			if err != nil {
				errors_ <- err
				return
			}
			if found && !bytes.Equal(got, payload) {
				errors_ <- fmt.Errorf("concurrent Get() = %q", got)
				return
			}
		}
	}()
	go func() {
		defer wait.Done()
		for range 100 {
			if _, err := stores[1].Prune(
				context.Background(),
				PruneOptions{MaxBytes: 1},
			); err != nil {
				errors_ <- err
				return
			}
		}
	}()
	wait.Wait()
	close(errors_)
	for err := range errors_ {
		t.Fatal(err)
	}
	if err := stores[0].Put(context.Background(), key, payload); err != nil {
		t.Fatal(err)
	}
	got, found, err := stores[1].Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, payload) {
		t.Fatalf("final Get() = %q, %t", got, found)
	}
}

func TestStorePruneRejectsInvalidOrCanceledRequests(t *testing.T) {
	t.Parallel()

	var nilStore *Store
	if _, err := nilStore.Prune(
		context.Background(),
		PruneOptions{MaxEntries: 1},
	); err == nil {
		t.Fatal("Prune() accepted a nil store")
	}
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		name    string
		context context.Context
		options PruneOptions
	}{
		{name: "nil context", options: PruneOptions{MaxEntries: 1}},
		{name: "canceled", context: canceled, options: PruneOptions{MaxEntries: 1}},
		{name: "no limit", context: context.Background()},
		{name: "negative entries", context: context.Background(), options: PruneOptions{MaxEntries: -1}},
		{name: "negative bytes", context: context.Background(), options: PruneOptions{MaxBytes: -1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.Prune(test.context, test.options); err == nil {
				t.Fatalf("Prune() accepted %s", test.name)
			}
		})
	}
	if err := store.root.RemoveAll(entryVersion); err != nil {
		t.Fatal(err)
	}
	if result, err := store.Prune(
		context.Background(),
		PruneOptions{MaxEntries: 1},
	); err != nil || result != (PruneResult{}) {
		t.Fatalf("Prune() after external cache deletion = %#v, %v", result, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Prune(context.Background(), PruneOptions{MaxEntries: 1}); err == nil {
		t.Fatal("Prune() accepted a closed store")
	}
}

func TestStoreRejectsInvalidOrCanceledRequests(t *testing.T) {
	t.Parallel()

	if _, err := Open("relative/cache"); err == nil {
		t.Fatal("Open() accepted a relative cache root")
	}
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, err := BuildKey(testKeyInput())
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := store.Get(nil, key); err == nil {
		t.Fatal("Get() accepted a nil context")
	}
	if _, _, err := store.Get(context.Background(), Key{}); err == nil {
		t.Fatal("Get() accepted a zero key")
	}
	if _, _, err := store.Get(canceled, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("Get() cancellation error = %v", err)
	}
	if err := store.Put(nil, key, nil); err == nil {
		t.Fatal("Put() accepted a nil context")
	}
	if err := store.Put(context.Background(), Key{}, nil); err == nil {
		t.Fatal("Put() accepted a zero key")
	}
	if err := store.Put(canceled, key, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put() cancellation error = %v", err)
	}
	if err := store.Put(context.Background(), key, make([]byte, MaxEntrySize+1)); err == nil {
		t.Fatal("Put() accepted an oversized payload")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(context.Background(), key); err == nil {
		t.Fatal("Get() accepted a closed store")
	}
	if err := store.Put(context.Background(), key, nil); err == nil {
		t.Fatal("Put() accepted a closed store")
	}
}

func TestOpenValidatedPinsResolvedRootBeforeValidationReturns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged Windows symlink creation is not portable")
	}

	external := t.TempDir()
	project := t.TempDir()
	link := filepath.Join(t.TempDir(), "cache")
	if err := os.Symlink(external, link); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(link, "analysis")
	store, err := OpenValidated(cacheRoot, func(resolved string) error {
		resolvedExternal, err := filepath.EvalSymlinks(external)
		if err != nil {
			return err
		}
		want := filepath.Join(resolvedExternal, "analysis")
		if resolved != want {
			t.Fatalf("validated root = %q, want %q", resolved, want)
		}
		if err := os.Remove(link); err != nil {
			return err
		}
		return os.Symlink(project, link)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	key, err := BuildKey(testKeyInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), key, []byte("pinned")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(external, "analysis", entryName(key))); err != nil {
		t.Fatalf("inspect pinned cache entry: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(project, "analysis")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mutable symlink target received cache data: %v", err)
	}
}

func TestOpenValidatedRejectsBeforeCreatingRoot(t *testing.T) {
	parent := t.TempDir()
	cacheRoot := filepath.Join(parent, "cache", "analysis")
	want := errors.New("rejected cache root")
	store, err := OpenValidated(cacheRoot, func(string) error { return want })
	if store != nil || !errors.Is(err, want) {
		t.Fatalf("OpenValidated() = %#v, %v", store, err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "cache")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected cache root was created: %v", err)
	}
}

func TestStoreRefusesAnEscapingShardSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged Windows symlink creation is not portable")
	}

	root := t.TempDir()
	outside := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	key, err := BuildKey(testKeyInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, entryShard(key))); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), key, []byte("must stay rooted")); err == nil {
		t.Fatal("Put() followed an escaping cache shard symlink")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("escaping shard wrote outside the cache root: %#v", entries)
	}
}
