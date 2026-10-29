package main

// Purpose: the guides/ci block of the wiki sidebar, built from the pages in
// the folder.
// Inputs:  the .md files of .github/wiki/guides/ci.
// Outputs: one link per page, titled by the page's H1.
// Constraints: pages follow the reading order of the fabric set; a page not
// in that order follows it alphabetically, so a new page still appears.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// readingOrder is where a known page sits in the sidebar.
var readingOrder = []string{
	"Getting-Started", "Architecture", "Configuration-and-Policy", "Evidence-and-Machine-Output",
	"Nodes-and-Runners", "Trust-Security-and-Privacy", "Scheduling-and-Cost", "Hosted-Providers",
	"Cache-and-Artifacts", "Walkthrough-Home-Lab", "Walkthrough-Multiple-Projects", "Walkthrough-Team",
	"Walkthrough-Agents", "Migration", "Troubleshooting", "Reference", "Agent-Reference",
}

// sidebarBlock renders the links to every page of dir.
func sidebarBlock(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	rank := map[string]int{}
	for i, name := range readingOrder {
		rank[name] = i
	}
	var pages []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			pages = append(pages, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	sort.Slice(pages, func(i, j int) bool {
		ri, iok := rank[pages[i]]
		rj, jok := rank[pages[j]]
		switch {
		case iok && jok:
			return ri < rj
		case iok != jok:
			return iok
		}
		return pages[i] < pages[j]
	})
	var b strings.Builder
	b.WriteString("### CI\n")
	for _, p := range pages {
		title, err := pageTitle(filepath.Join(dir, p+".md"))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "- [[%s|guides/ci/%s]]\n", title, p)
	}
	return b.String(), nil
}

// pageTitle returns the text of the first H1 of a page.
func pageTitle(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# ")), nil
		}
	}
	return "", fmt.Errorf("%s: no H1", path)
}
