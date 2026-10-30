package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestExtractBlocksNumbersAndMarker(t *testing.T) {
	content := strings.Join([]string{
		"# Page",
		"```sh",
		"exit 9",
		"```",
		"```bash",
		"echo one",
		"echo two",
		"```",
		"<!-- doc-command: expect-fail -->",
		"```bash title=second",
		"exit 2",
		"```",
		"<!-- doc-command: expect-fail -->",
		"",
		"```bash",
		"exit 4",
		"```",
	}, "\n")
	got, err := ExtractBlocks(content)
	if err != nil {
		t.Fatalf("ExtractBlocks: %v", err)
	}
	want := []Block{
		{Number: 1, Line: 5, Script: "echo one\necho two"},
		{Number: 2, Line: 10, Script: "exit 2", ExpectFail: true},
		{Number: 3, Line: 15, Script: "exit 4"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("blocks = %+v\nwant     %+v", got, want)
	}
}

func TestExtractBlocksSkipsNestedFence(t *testing.T) {
	// A four-backtick fence holds a bash fence as plain text, so no block is extracted.
	content := "````text\n```bash\nexit 4\n```\n````\n"
	got, err := ExtractBlocks(content)
	if err != nil || len(got) != 0 {
		t.Errorf("ExtractBlocks = %+v, %v; want no blocks and no error", got, err)
	}
}

func TestExtractBlocksUnclosedBashFails(t *testing.T) {
	_, err := ExtractBlocks("```bash\nexit 1\n")
	if err == nil || err.Error() != "line 1: unclosed bash fence" {
		t.Errorf("err = %v, want unclosed bash fence on line 1", err)
	}
}

func TestExtractBlocksUnclosedOtherFenceRunsToEnd(t *testing.T) {
	got, err := ExtractBlocks("```bash\nexit 1\n```\n```text\nfoo\n")
	if err != nil || len(got) != 1 {
		t.Errorf("ExtractBlocks = %+v, %v; want one block and no error", got, err)
	}
}

func TestParseOpenFence(t *testing.T) {
	cases := []struct {
		line  string
		ticks int
		lang  string
		ok    bool
	}{
		{"```bash", 3, "bash", true},
		{"  ```bash title=x", 3, "bash", true},
		{"````sh", 4, "sh", true},
		{"```", 3, "", true},
		{"``` a`b", 0, "", false},
		{"text ```bash", 0, "", false},
	}
	for _, c := range cases {
		ticks, lang, ok := parseOpenFence(c.line)
		if ticks != c.ticks || lang != c.lang || ok != c.ok {
			t.Errorf("parseOpenFence(%q) = %d, %q, %v; want %d, %q, %v",
				c.line, ticks, lang, ok, c.ticks, c.lang, c.ok)
		}
	}
}
