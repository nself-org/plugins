//go:build unix

package cloudvm

// Purpose: TestTokenEnvFile*: the token file rules (regular, 0600 or stricter, ours, small, one token),
// the final symlink refusal, and that no error ever repeats the content.
// Inputs:  files in t.TempDir. Outputs: pass/fail. Constraints: synthetic content only.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const syntheticSecret = "synthetic-secret-value-0000"

func writeTok(t *testing.T, name string, mode os.FileMode, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTokenEnvFileRules(t *testing.T) {
	good := writeTok(t, "good", 0o600, syntheticSecret+"\n")
	if tok, err := readTokenFile(good); err != nil || tok != syntheticSecret {
		t.Fatalf("a good file: %q %v", tok, err)
	}
	if tok, err := readTokenFile(writeTok(t, "ro", 0o400, syntheticSecret)); err != nil || tok != syntheticSecret {
		t.Fatalf("a 0400 file is stricter than 0600: %q %v", tok, err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(good, link); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"group readable":  writeTok(t, "g", 0o640, syntheticSecret),
		"world readable":  writeTok(t, "w", 0o644, syntheticSecret),
		"executable":      writeTok(t, "x", 0o700, syntheticSecret),
		"a symlink":       link,
		"a directory":     t.TempDir(),
		"missing":         filepath.Join(t.TempDir(), "nope"),
		"empty":           writeTok(t, "e", 0o600, "\n"),
		"inner space":     writeTok(t, "s", 0o600, "two words"),
		"inner newline":   writeTok(t, "n", 0o600, "one\ntwo"),
		"non printable":   writeTok(t, "b", 0o600, "tok\x01en"),
		"over 4096 bytes": writeTok(t, "big", 0o600, strings.Repeat("a", maxTokenBytes+1)),
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			tok, err := readTokenFile(path)
			if ReasonOf(err) != ReasonTokenFile || CodeOf(err) != "E698" || tok != "" {
				t.Fatalf("want E698 token_file and no token, got %q %v", tok, err)
			}
			if strings.Contains(err.Error(), syntheticSecret) || strings.Contains(err.Error(), "two words") {
				t.Fatalf("the error repeats the file content: %v", err)
			}
		})
	}
}

func TestTokenEnvFileMustBeOwnedByTheCoordinatorUser(t *testing.T) {
	old := tokenFileUID
	t.Cleanup(func() { tokenFileUID = old })
	tokenFileUID = func() int { return os.Getuid() + 1 }
	if _, err := readTokenFile(writeTok(t, "other", 0o600, syntheticSecret)); ReasonOf(err) != ReasonTokenFile {
		t.Fatalf("a file of another user must be refused: %v", err)
	}
}
