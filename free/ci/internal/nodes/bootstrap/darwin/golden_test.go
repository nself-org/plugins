package darwin

// Purpose: file helpers and the plist golden of the darwin tests.
// Inputs:  testdata/org.nself.ci-agent.plist; set UPDATE_GOLDEN=1 to rewrite it.
// Constraints: the golden is the exact text installed on a node; a change to Plist() must change it in the
//          same commit.

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"testing"
)

func writeFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o600) }

// wellFormed reports an XML syntax error in s.
func wellFormed(s string) error {
	dec := xml.NewDecoder(bytes.NewReader([]byte(s)))
	dec.Strict = false
	for {
		if _, err := dec.Token(); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
	}
}

func TestPlistGolden(t *testing.T) {
	const path = "testdata/org.nself.ci-agent.plist"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := writeFile(path, Plist()); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != Plist() {
		t.Fatalf("Plist() differs from %s (UPDATE_GOLDEN=1 rewrites it)", path)
	}
}
