package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// the scope's run facts come from the command, not the report package: the
// release that ran, the churn window it read, and the source files the graph
// cannot parse.
func TestReportScopeNamesTheReleaseTheWindowAndWhatWasNotParsed(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"src/a.ts":    "import { b } from \"./b\";\nexport const a = b;\n",
		"src/b.ts":    "export const b = 1;\n",
		"cmd/main.go": "package main\n\nfunc main() {}\n",
	})
	md, code := captureStdout(t, func() int { return reportCmd([]string{dir}) })
	if code != 0 {
		t.Fatalf("report exited %d", code)
	}
	for _, want := range []string{
		"- warpmap: " + version + "\n",
		"- churn: no git history to read\n",
		"- coverage: partial\n",
		"- not run: coupling, held-by-one\n",
		"- parsed: 2 files (.ts 2)\n",
		"- not parsed: 1 file (.go 1) - no imports, findings or hotspots for these\n",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("want %q in\n%s", want, md)
		}
	}

	out, code := captureStdout(t, func() int { return reportCmd([]string{dir, "--json"}) })
	if code != 0 {
		t.Fatalf("report --json exited %d", code)
	}
	var pack struct {
		Scope struct {
			Warpmap string `json:"warpmap"`
		} `json:"scope"`
		Limitations struct {
			NotParsed map[string]int `json:"not_parsed"`
		} `json:"limitations"`
	}
	if err := json.Unmarshal([]byte(out), &pack); err != nil {
		t.Fatal(err)
	}
	if pack.Scope.Warpmap != version || pack.Limitations.NotParsed[".go"] != 1 {
		t.Fatalf("the pack's scope is missing the command's facts: %+v", pack)
	}
}
