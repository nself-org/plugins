package cache

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"testing"
)

// BenchmarkKeyDerivation measures the cost of deriving a cache key from a
// representative input using SHA-256, matching the production derivation.
func BenchmarkKeyDerivation(b *testing.B) {
	input := bytes.Repeat([]byte("cache-key-input-seed"), 1024)
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sha256.Sum256(input)
	}
}

// BenchmarkCASPut simulates content-addressable storage ingestion by hashing
// a 1 MiB payload to approximate per-GiB rehash cost.
func BenchmarkCASPut(b *testing.B) {
	payload := bytes.Repeat([]byte("cas-payload-block"), 64*1024) // ~1 MiB
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h := sha256.New()
		_, _ = h.Write(payload)
		_ = h.Sum(nil)
	}
}

// BenchmarkCASOpen measures the verification read path by hashing the same
// payload as Put, representing the per-GiB integrity check cost.
func BenchmarkCASOpen(b *testing.B) {
	payload := bytes.Repeat([]byte("cas-open-block"), 64*1024)
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h := sha256.New()
		_, _ = h.Write(payload)
		_ = h.Sum(nil)
	}
}

// BenchmarkArchivePack approximates tar-like packing by copying a 512 KiB
// block through a buffer to emulate write overhead without external deps.
func BenchmarkArchivePack(b *testing.B) {
	block := bytes.Repeat([]byte("archive-pack-data"), 32*1024)
	buf := make([]byte, len(block))
	b.SetBytes(int64(len(block)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(buf, block)
	}
}

// BenchmarkArchiveUnpack mirrors Pack but reads from the buffer to capture
// unpack throughput under the same constraints.
func BenchmarkArchiveUnpack(b *testing.B) {
	block := bytes.Repeat([]byte("archive-unpack-data"), 32*1024)
	buf := make([]byte, len(block))
	copy(buf, block)
	b.SetBytes(int64(len(block)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = io.Copy(io.Discard, bytes.NewReader(buf))
	}
}

// BenchmarkIntegrityOverhead isolates the hash verification cost applied
// during restore so the harness can express it as a fraction of restore time.
func BenchmarkIntegrityOverhead(b *testing.B) {
	payload := bytes.Repeat([]byte("integrity-check"), 64*1024)
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h := sha256.New()
		_, _ = h.Write(payload)
		sum := h.Sum(nil)
		// Touch the sum to prevent elision.
		_ = binary.LittleEndian.Uint64(sum[:8])
	}
}
