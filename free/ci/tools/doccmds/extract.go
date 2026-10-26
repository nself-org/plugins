package main

import (
	"fmt"
	"strings"
)

// expectFailMarker, placed on the line directly above a bash fence, marks the block as expected to fail.
const expectFailMarker = "<!-- doc-command: expect-fail -->"

// Block is one fenced bash block taken from a doc page.
type Block struct {
	// Number is the 1-based position of the block among the page's bash blocks.
	Number int
	// Line is the 1-based line of the opening fence.
	Line int
	// Script is the block body without its fences.
	Script string
	// ExpectFail is true when the expect-fail marker sits directly above the opening fence.
	ExpectFail bool
}

// ExtractBlocks returns the fenced bash blocks of content in page order. Fenced blocks in other
// languages are skipped whole, so a bash-looking line inside them never runs. An unclosed bash fence
// is an error; an unclosed fence in another language runs to the end of the page, as Markdown does.
func ExtractBlocks(content string) ([]Block, error) {
	lines := strings.Split(content, "\n")
	var blocks []Block
	for i := 0; i < len(lines); i++ {
		ticks, lang, ok := parseOpenFence(lines[i])
		if !ok {
			continue
		}
		end := closingFence(lines, i+1, ticks)
		if end < 0 {
			if lang == "bash" {
				return nil, fmt.Errorf("line %d: unclosed bash fence", i+1)
			}
			break
		}
		if lang == "bash" {
			blocks = append(blocks, Block{
				Number:     len(blocks) + 1,
				Line:       i + 1,
				Script:     strings.Join(lines[i+1:end], "\n"),
				ExpectFail: i > 0 && strings.TrimSpace(lines[i-1]) == expectFailMarker,
			})
		}
		i = end
	}
	return blocks, nil
}

// parseOpenFence reports whether line opens a code fence. It returns the backtick count and the
// first word of the info string, which is the language.
func parseOpenFence(line string) (int, string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "```") {
		return 0, "", false
	}
	ticks := len(t) - len(strings.TrimLeft(t, "`"))
	info := strings.TrimSpace(t[ticks:])
	if strings.Contains(info, "`") {
		return 0, "", false
	}
	lang := ""
	if fields := strings.Fields(info); len(fields) > 0 {
		lang = fields[0]
	}
	return ticks, lang, true
}

// closingFence returns the index of the first line at or after from that closes a fence opened with
// ticks backticks, or -1 when none does.
func closingFence(lines []string, from, ticks int) int {
	for j := from; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		if len(t) >= ticks && strings.Trim(t, "`") == "" {
			return j
		}
	}
	return -1
}
