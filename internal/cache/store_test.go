package cache

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
