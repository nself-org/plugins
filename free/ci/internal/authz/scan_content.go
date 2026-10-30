package authz

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"slices"
	"sync"
	"unicode/utf16"
	"unicode/utf8"
)

var ErrSecretContent = errors.New("content holds a registered secret")
var ErrScanBudget = errors.New("compressed content exceeds scan budget")

// CleanCandidates checks raw, NUL-stripped and both alignments of UTF-16 text
// (little and big endian). The derived texts are built in pooled scratch
// buffers, one pass each, so scanning a window does not allocate per call.
func CleanCandidates(s ValueScanner, b []byte) bool {
	if !Clean(s, b) {
		return false
	}
	sc := scratchPool.Get().(*scratch)
	defer scratchPool.Put(sc)
	if bytes.IndexByte(b, 0) >= 0 {
		sc.buf = stripNUL(sc.buf[:0], b)
		if !Clean(s, sc.buf) {
			return false
		}
	}
	for _, big := range []bool{false, true} {
		for offset := 0; offset < 2; offset++ {
			sc.buf = appendUTF16(sc.buf[:0], b, offset, big)
			if !Clean(s, sc.buf) {
				return false
			}
		}
	}
	return true
}

type scratch struct{ buf []byte }

var scratchPool = sync.Pool{New: func() any { return new(scratch) }}

// stripNUL appends b without its NUL bytes to dst.
func stripNUL(dst, b []byte) []byte {
	for len(b) > 0 {
		i := bytes.IndexByte(b, 0)
		if i < 0 {
			return append(dst, b...)
		}
		dst = append(dst, b[:i]...)
		b = b[i+1:]
	}
	return dst
}

// appendUTF16 appends the UTF-8 form of b read as UTF-16 code units from
// offset, byte pairs in the given order. It yields exactly the bytes of
// []byte(string(utf16.Decode(units))): a surrogate pair is one rune and an
// unpaired surrogate is U+FFFD.
func appendUTF16(dst, b []byte, offset int, big bool) []byte {
	dst = slices.Grow(dst, 3*(len(b)/2)+utf8.UTFMax)
	unit := func(i int) uint16 {
		if big {
			return uint16(b[i+1]) | uint16(b[i])<<8
		}
		return uint16(b[i]) | uint16(b[i+1])<<8
	}
	for i := offset; i+1 < len(b); i += 2 {
		r := rune(unit(i))
		switch {
		case r < 0x80:
			dst = append(dst, byte(r))
		case r < 0x800:
			dst = append(dst, 0xC0|byte(r>>6), 0x80|byte(r)&0x3F)
		case r < 0xD800 || r >= 0xE000:
			dst = append(dst, 0xE0|byte(r>>12), 0x80|byte(r>>6)&0x3F, 0x80|byte(r)&0x3F)
		case r < 0xDC00 && i+3 < len(b) && unit(i+2) >= 0xDC00 && unit(i+2) < 0xE000:
			r = utf16.DecodeRune(r, rune(unit(i+2)))
			dst = append(dst, 0xF0|byte(r>>18), 0x80|byte(r>>12)&0x3F, 0x80|byte(r>>6)&0x3F, 0x80|byte(r)&0x3F)
			i += 2
		default:
			dst = append(dst, 0xEF, 0xBF, 0xBD) // U+FFFD
		}
	}
	return dst
}

// Compressed reports gzip or ZIP magic without trusting a file extension.
func Compressed(b []byte) bool {
	return len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b || len(b) >= 4 && b[0] == 'P' && b[1] == 'K' && (b[2] == 3 && b[3] == 4 || b[2] == 5 && b[3] == 6 || b[2] == 7 && b[3] == 8)
}

// ScanArchive scans all gzip members or ZIP entries, sharing an expanded-byte budget.
// The caller scans raw bytes separately; malformed gzip is scanned up to its break.
func ScanArchive(src io.ReaderAt, size int64, s ValueScanner, left *int64) error {
	var magic [2]byte
	if _, err := src.ReadAt(magic[:], 0); err != nil {
		return nil
	}
	if magic == [2]byte{0x1f, 0x8b} {
		gz, err := gzip.NewReader(io.NewSectionReader(src, 0, size))
		if err != nil {
			return nil
		}
		defer func() { _ = gz.Close() }()
		return scanExpanded(gz, s, left, true)
	}
	if magic != [2]byte{'P', 'K'} {
		return nil
	}
	zr, err := zip.NewReader(src, size)
	if err != nil {
		return err
	}
	for _, entry := range zr.File {
		r, err := entry.Open()
		if err != nil {
			return err
		}
		err = scanExpanded(r, s, left, false)
		closeErr := r.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func scanExpanded(r io.Reader, s ValueScanner, left *int64, tolerateCorrupt bool) error {
	const window = 1 << 20
	var tail []byte
	for {
		chunk := make([]byte, min(int64(window), *left+1))
		n, err := io.ReadFull(r, chunk)
		*left -= int64(n)
		if *left < 0 {
			return ErrScanBudget
		}
		data := append(tail, chunk[:n]...)
		if !CleanCandidates(s, data) {
			return ErrSecretContent
		}
		tail = append([]byte(nil), data[max(0, len(data)-window):]...)
		if err != nil {
			if !tolerateCorrupt && err != io.EOF && err != io.ErrUnexpectedEOF {
				return err
			}
			return nil
		}
	}
}
