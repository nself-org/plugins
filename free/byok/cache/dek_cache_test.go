package cache

import (
	"testing"
	"time"
)

func TestDEKCacheHitMiss(t *testing.T) {
	c := NewDEKCache(5 * time.Second)

	wrappedDEK := []byte("wrapped-dek-bytes-1234")
	plainDEK := []byte{0x01, 0x02, 0x03, 0x04}

	// Miss before Put.
	got, ok := c.Get(wrappedDEK)
	if ok {
		t.Fatal("expected cache miss before Put, got hit")
	}
	if got != nil {
		t.Fatal("expected nil on miss")
	}

	// Put then Get.
	c.Put(wrappedDEK, plainDEK)
	got, ok = c.Get(wrappedDEK)
	if !ok {
		t.Fatal("expected cache hit after Put")
	}
	if string(got) != string(plainDEK) {
		t.Fatalf("cached DEK mismatch: got %v, want %v", got, plainDEK)
	}

	// Returned slice is a copy — mutating it should not affect cached value.
	got[0] = 0xFF
	got2, _ := c.Get(wrappedDEK)
	if got2[0] != 0x01 {
		t.Fatal("cache returned a reference instead of a copy — mutation affected cached DEK")
	}
}

func TestDEKCacheExpiry(t *testing.T) {
	c := NewDEKCache(10 * time.Millisecond)

	wrapped := []byte("short-lived-key")
	plain := []byte{0xAA, 0xBB}
	c.Put(wrapped, plain)

	_, ok := c.Get(wrapped)
	if !ok {
		t.Fatal("expected hit immediately after Put")
	}

	time.Sleep(25 * time.Millisecond)

	_, ok = c.Get(wrapped)
	if ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestDEKCacheInvalidate(t *testing.T) {
	c := NewDEKCache(1 * time.Minute)
	wrapped := []byte("key-to-revoke")
	plain := []byte{0xCC}
	c.Put(wrapped, plain)

	c.Invalidate(wrapped)

	_, ok := c.Get(wrapped)
	if ok {
		t.Fatal("expected cache miss after Invalidate (revoked key path)")
	}
}

func TestDEKCacheFlush(t *testing.T) {
	c := NewDEKCache(1 * time.Minute)
	c.Put([]byte("k1"), []byte{1})
	c.Put([]byte("k2"), []byte{2})
	c.Put([]byte("k3"), []byte{3})

	if c.Size() != 3 {
		t.Fatalf("expected 3 entries, got %d", c.Size())
	}

	c.Flush()

	if c.Size() != 0 {
		t.Fatalf("expected 0 entries after Flush, got %d", c.Size())
	}
}

func TestNilCacheNoPanic(t *testing.T) {
	var c *DEKCache // nil cache — cache-disabled path

	_, ok := c.Get([]byte("k"))
	if ok {
		t.Fatal("nil cache Get should return miss")
	}
	// These should not panic.
	c.Put([]byte("k"), []byte{1})
	c.Invalidate([]byte("k"))
	c.Flush()
	if c.Size() != 0 {
		t.Fatal("nil cache Size should be 0")
	}
}
