package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func commitFileAs(t *testing.T, dir, author, file, body string) {
	t.Helper()
	writeFiles(t, dir, map[string]string{file: body})
	gitIn(t, dir, "add", "--", file)
	gitIn(t, dir, "-c", "user.name="+author, "-c", "user.email="+strings.ToLower(author)+"@example.com", "commit", "-m", "edit "+file)
}

// leadSection is the part of the markdown audit under the given lead-signal
// heading, up to the next heading.
func leadSection(t *testing.T, md, heading string) string {
	t.Helper()
	_, after, ok := strings.Cut(md, heading+"\n")
	if !ok {
		t.Fatalf("the audit has no %q section:\n%s", heading, md)
	}
	if i := strings.Index(after, "\n#"); i >= 0 {
		after = after[:i]
	}
	return after
}

// authorCounts runs `ownership --json`, which keeps every row even when the text
// output folds a one-author window into one line.
func authorCounts(t *testing.T, args ...string) map[string]int {
	t.Helper()
	out, code := captureStdout(t, func() int { return ownershipCmd(append(args, "--json")) })
	if code != 0 {
		t.Fatalf("ownership %v exited %d", args, code)
	}
	var got struct {
		Hotspots []struct {
			File    string `json:"file"`
			Authors int    `json:"authors"`
		} `json:"hotspots"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not json: %v\n%s", err, out)
	}
	counts := map[string]int{}
	for _, h := range got.Hotspots {
		counts[h.File] = h.Authors
	}
	return counts
}

const heldHeading = "### hotspots one person holds"

func TestReportLeadsWithTheHotspotsOnePersonHolds(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init")
	commitFileAs(t, dir, "Ada", "src/busy.ts", strings.Repeat("if (x) { y(); }\n", 30))
	commitFileAs(t, dir, "Ada", "src/busy.ts", strings.Repeat("if (x) { y(); }\n", 31))
	commitFileAs(t, dir, "Bo", "src/busy.ts", strings.Repeat("if (x) { y(); }\n", 32))
	commitFileAs(t, dir, "Ada", "src/busy.ts", strings.Repeat("if (x) { y(); }\n", 33))
	commitFileAs(t, dir, "Ada", "src/shared.ts", "export const s = 1;\n")
	commitFileAs(t, dir, "Bo", "src/shared.ts", "export const s = 2;\n")

	md, code := captureStdout(t, func() int { return reportCmd([]string{dir}) })
	if code != 0 {
		t.Fatalf("report exited %d", code)
	}
	section := leadSection(t, md, heldHeading)
	if want := "- `src/busy.ts` - Ada made 3 of its 4 commits (75%)\n"; !strings.Contains(section, want) {
		t.Fatalf("want %q under %q, got\n%s", want, heldHeading, section)
	}
	if strings.Contains(section, "src/shared.ts") {
		t.Fatalf("a file split evenly between two people is listed as held by one:\n%s", section)
	}
	if lead, findings := strings.Index(md, heldHeading), strings.Index(md, "## hotspots"); lead > findings {
		t.Fatalf("the section sits below the appendix:\n%s", md)
	}

	out, code := captureStdout(t, func() int { return reportCmd([]string{dir, "--json"}) })
	if code != 0 {
		t.Fatalf("report --json exited %d", code)
	}
	var pack struct {
		WindowAuthors int `json:"window_authors"`
		HeldByOne     []struct {
			File    string  `json:"file"`
			Author  string  `json:"author"`
			Commits int     `json:"commits"`
			Total   int     `json:"total"`
			Share   float64 `json:"share"`
		} `json:"held_by_one"`
	}
	if err := json.Unmarshal([]byte(out), &pack); err != nil {
		t.Fatalf("not json: %v\n%s", err, out)
	}
	if pack.WindowAuthors != 2 || len(pack.HeldByOne) != 1 {
		t.Fatalf("want 2 authors and one held file, got %+v", pack)
	}
	if h := pack.HeldByOne[0]; h.File != "src/busy.ts" || h.Author != "Ada" || h.Commits != 3 || h.Total != 4 || h.Share != 0.75 {
		t.Fatalf("the pack's row differs from the markdown's: %+v", h)
	}

	out, code = captureStdout(t, func() int { return ownershipCmd([]string{dir}) })
	if code != 0 {
		t.Fatalf("ownership exited %d", code)
	}
	if !strings.Contains(out, "authors=2  src/busy.ts\n") || !strings.Contains(out, "authors=2  src/shared.ts\n") {
		t.Fatalf("with two authors ownership lists every hotspot, got\n%s", out)
	}
}

// with one author in the window every row would read the same, so the audit and
// `ownership` say it once instead of listing every hotspot.
func TestReportAndOwnershipSayOnceWhenTheWindowHoldsOneAuthor(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init")
	commitFileAs(t, dir, "Ada", "src/busy.ts", strings.Repeat("if (x) { y(); }\n", 30))
	commitFileAs(t, dir, "Ada", "src/a.ts", "export const a = 1;\n")
	const note = "one author across the window: every file has a bus factor of 1"

	md, code := captureStdout(t, func() int { return reportCmd([]string{dir}) })
	if code != 0 {
		t.Fatalf("report exited %d", code)
	}
	section := leadSection(t, md, heldHeading)
	if !strings.Contains(section, "- "+note+"\n") || strings.Contains(section, "`src/") {
		t.Fatalf("want the one-author note and no file rows, got\n%s", section)
	}

	out, code := captureStdout(t, func() int { return ownershipCmd([]string{dir}) })
	if code != 0 {
		t.Fatalf("ownership exited %d", code)
	}
	if out != note+"\n" {
		t.Fatalf("ownership printed\n%s\nwant the one line %q", out, note)
	}
}
