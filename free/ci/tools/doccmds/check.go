package main

import (
	"fmt"
	"os"
)

// Runner runs one doc block with its working directory set to dir and returns the block's exit code.
// A non-nil error means the block could not be run at all.
type Runner interface {
	Run(dir, script string) (int, error)
}

// Fixture creates a clean project directory for one block. It returns the directory and a function
// that removes it.
type Fixture func() (dir string, cleanup func(), err error)

// Options holds the injected pieces CheckPage uses, so tests run without spawning processes.
type Options struct {
	Runner  Runner
	Fixture Fixture
}

// Failure reports one bash block that did not behave as its marker says it should.
type Failure struct {
	// Page is the doc page path, as given on the command line.
	Page string
	// Block is the 1-based number of the block among the page's bash blocks.
	Block int
	// ExitCode is the block's exit code, or -1 when the block or its fixture could not run.
	ExitCode int
	// Message says what went wrong, for example "exit 3".
	Message string
}

// Error formats the failure as "page: block N: message".
func (f Failure) Error() string {
	return fmt.Sprintf("%s: block %d: %s", f.Page, f.Block, f.Message)
}

// CheckPage runs every bash block of the page at path, each in its own fresh fixture, and returns the
// failures in page order. A page without bash blocks returns no failures and runs nothing.
func CheckPage(path string, opts Options) ([]Failure, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blocks, err := ExtractBlocks(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var failures []Failure
	for _, b := range blocks {
		if f, failed := checkBlock(path, b, opts); failed {
			failures = append(failures, f)
		}
	}
	return failures, nil
}

// checkBlock runs one block in a fresh fixture and reports whether its exit code breaks the marker.
func checkBlock(page string, b Block, opts Options) (Failure, bool) {
	fail := func(code int, msg string) (Failure, bool) {
		return Failure{Page: page, Block: b.Number, ExitCode: code, Message: msg}, true
	}
	dir, cleanup, err := opts.Fixture()
	if err != nil {
		return fail(-1, fmt.Sprintf("fixture: %v", err))
	}
	defer cleanup()
	code, err := opts.Runner.Run(dir, b.Script)
	if err != nil {
		return fail(-1, fmt.Sprintf("could not run: %v", err))
	}
	if b.ExpectFail {
		if code == 0 {
			return fail(0, "exit 0 (expected non-zero: marked expect-fail)")
		}
		return Failure{}, false
	}
	if code != 0 {
		return fail(code, fmt.Sprintf("exit %d", code))
	}
	return Failure{}, false
}
