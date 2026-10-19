package signing

import "testing"

func TestCaptureKeepsAtMostMaxOutput(t *testing.T) {
	var c capture
	c.write(make([]byte, maxOutput-1))
	c.write([]byte("ab")) // one byte of room: only "a" is kept
	if len(c.buf) != maxOutput || c.buf[maxOutput-1] != 'a' {
		t.Fatalf("%d bytes", len(c.buf))
	}
	c.write([]byte("c")) // exactly full: nothing more
	if len(c.buf) != maxOutput {
		t.Fatalf("%d bytes after a full buffer", len(c.buf))
	}
	var d capture
	d.write([]byte("hello"))
	d.write([]byte(" world"))
	if string(d.buf) != "hello world" {
		t.Fatalf("%q", d.buf)
	}
}
