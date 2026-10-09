package protocol

import (
	"bufio"
	"bytes"
	"io"
)

const Preamble = "NSELF-CI-AGENT/1\n"
const MaxShellNoise = 64 << 10

// SkipPreamble locates the SSH agent marker with a bounded shell-output scan.
// The returned reader preserves buffered bytes for the hello decoder.
func SkipPreamble(r io.Reader) (io.Reader, error) {
	br := bufio.NewReader(r)
	noise := 0
	for noise <= MaxShellNoise {
		line, err := br.ReadSlice('\n')
		if noise+len(line) > MaxShellNoise+len(Preamble) {
			return nil, invalid("shell_not_clean")
		}
		if bytes.Equal(line, []byte(Preamble)) {
			return br, nil
		}
		noise += len(line)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return nil, invalid("shell_not_clean")
		}
	}
	return nil, invalid("shell_not_clean")
}
