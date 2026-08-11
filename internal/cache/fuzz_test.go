package cache

import (
	"bytes"
	"testing"
)

func FuzzDecodeEntry(f *testing.F) {
	key := Key(DigestOf([]byte("key")))
	valid := encodeEntry(key, []byte("payload"))
	f.Add(valid)
	f.Add([]byte("corrupt"))
	f.Add(append(bytes.Clone(valid), 0))

	f.Fuzz(func(t *testing.T, encoded []byte) {
		payload, ok := decodeEntry(key, encoded)
		if !ok {
			return
		}
		roundTrip, valid := decodeEntry(key, encodeEntry(key, payload))
		if !valid || !bytes.Equal(roundTrip, payload) {
			t.Fatalf("cache entry did not round trip: %x", payload)
		}
	})
}
