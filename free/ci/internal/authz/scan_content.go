package authz

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"unicode/utf16"
)

var ErrSecretContent = errors.New("content holds a registered secret")
var ErrScanBudget = errors.New("compressed content exceeds scan budget")

// CleanCandidates checks raw, NUL-stripped and both alignments of UTF-16 text.
func CleanCandidates(s ValueScanner, b []byte) bool {
	if !Clean(s, b) {
		return false
	}
	if bytes.IndexByte(b, 0) >= 0 && !Clean(s, bytes.ReplaceAll(b, []byte{0}, nil)) {
		return false
	}
	for _, big := range []bool{false, true} {
		for offset := 0; offset < 2; offset++ {
			units := make([]uint16, 0, len(b)/2)
			for i := offset; i+1 < len(b); i += 2 {
				a, c := b[i], b[i+1]
				if big {
					a, c = c, a
				}
				units = append(units, uint16(a)|uint16(c)<<8)
			}
			if !Clean(s, []byte(string(utf16.Decode(units)))) {
				return false
			}
		}
	}
	return true
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
