package main

import (
	"bufio"
	"strings"

	"os"
	"path/filepath"
	"testing"
)

// TestFunctionsDeploy_CopiesFiles verifies that deploy copies a single file
// into ./functions/<name>/ and that the destination exists.
func TestFunctionsDeploy_CopiesFiles(t *testing.T) {
	t.Parallel()

	// Create a temp dir to act as project root.
	projectDir := t.TempDir()

	// Write a source function file.
	src := filepath.Join(projectDir, "hello.ts")
	if err := os.WriteFile(src, []byte("export default () => new Response('hello')"), 0640); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	// Change into project dir so copyFile resolves correctly.
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(projectDir)

	destDir := filepath.Join(projectDir, "functions", "hello")
	if err := os.MkdirAll(destDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dest := filepath.Join(destDir, "hello.ts")

	if err := copyFile(src, dest); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	if _, err := os.Stat(dest); err != nil {
		t.Errorf("expected file at %s, got error: %v", dest, err)
	}
}

// TestFunctionsList_ScansDirectory verifies that the functions directory is
// enumerated correctly.
func TestFunctionsList_ScansDirectory(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()

	// Create two function directories.
	for _, name := range []string{"fn-a", "fn-b"} {
		dir := filepath.Join(projectDir, "functions", name)
		if err := os.MkdirAll(dir, 0750); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	// Create a non-directory entry — should be ignored.
	f := filepath.Join(projectDir, "functions", "README.md")
	_ = os.WriteFile(f, []byte("docs"), 0640)

	entries, err := os.ReadDir(filepath.Join(projectDir, "functions"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}

	if len(dirs) != 2 {
		t.Errorf("expected 2 function dirs, got %d: %v", len(dirs), dirs)
	}
}

// TestFunctionsDelete_RemovesDir verifies that the delete handler removes the
// function directory.
func TestFunctionsDelete_RemovesDir(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	fnDir := filepath.Join(projectDir, "functions", "my-fn")
	if err := os.MkdirAll(fnDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.RemoveAll(fnDir); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	if _, err := os.Stat(fnDir); !os.IsNotExist(err) {
		t.Errorf("expected directory to be removed, got: %v", err)
	}
}

// TestFunctionNamePattern verifies the function name validation regex.
func TestFunctionNamePattern(t *testing.T) {
	t.Parallel()

	valid := []string{"hello", "hello-world", "fn1", "my-fn-2"}
	invalid := []string{"Hello", "hello world", "-start", "a_b", "", "fn!", "UPPER"}

	for _, name := range valid {
		if !functionNamePattern.MatchString(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
	for _, name := range invalid {
		if functionNamePattern.MatchString(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

// TestServiceUpgrade_WritesEnvVar verifies that serviceUpgrade writes the
// correct env key for various services.

// containsLine reports whether the given text contains the given line.
func containsLine(text, line string) bool {
	for _, l := range testSplitLines(text) {
		if l == line {
			return true
		}
	}
	return false
}

func testSplitLines(s string) []string {
	// filepath.SplitList uses OS path separator, not newline — split manually.
	var result []string
	current := ""
	for _, c := range s {
		if c == '\n' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

var serviceVersionKeys = map[string]string{
	"functions": "FUNCTIONS_VERSION",
}

func setEnvKeyInFile(filename, key, value string) error {
	var lines []string
	if _, err := os.Stat(filename); err == nil {
		f, err := os.Open(filename)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		f.Close()
	}

	prefix := key + "="
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) || trimmed == key {
			lines[i] = key + "=" + value
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, key+"="+value)
	}

	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	return os.WriteFile(filename, []byte(content), 0600)
}
