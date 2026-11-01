package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// couplingProject commits a.ts and b.ts together three times with no import
// between them, c.ts and d.ts three times with c importing d, and a test beside
// a.ts every time.
func couplingProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init")
	for i := 0; i < 3; i++ {
		writeFiles(t, dir, map[string]string{
			"src/a.ts":      fmt.Sprintf("export const a = %d;\n", i),
			"src/b.ts":      fmt.Sprintf("export const b = %d;\n", i),
			"src/a.test.ts": fmt.Sprintf("import { a } from \"./a\";\n// %d\n", i),
		})
		gitIn(t, dir, "add", "--", "src")
		gitIn(t, dir, "commit", "-m", fmt.Sprintf("a and b %d", i))
		writeFiles(t, dir, map[string]string{
			"src/c.ts": fmt.Sprintf("import { d } from \"./d\";\nexport const c = d + %d;\n", i),
			"src/d.ts": fmt.Sprintf("export const d = %d;\n", i),
		})
		gitIn(t, dir, "add", "--", "src")
		gitIn(t, dir, "commit", "-m", fmt.Sprintf("c and d %d", i))
	}
	return dir
}

func TestCouplingNamesFilesThatChangeTogetherWithNoImport(t *testing.T) {
	dir := couplingProject(t)
	var out string
	errOut := captureStderr(t, func() int {
		var code int
		out, code = captureStdout(t, func() int { return couplingCmd([]string{dir}) })
		return code
	})
	want := "files that change together, 3 or more shared commits at a score of 0.30 or more (pairs an import joins left out):\n" +
		"  1.00  together=3/3   src/a.ts + src/b.ts\n"
	if out != want {
		t.Fatalf("want one pair, src/a.ts + src/b.ts at 1.00 over 3 of 3 commits:\n%s\ngot\n%s", want, out)
	}
	if strings.Contains(out, "c.ts") || strings.Contains(out, "test") {
		t.Fatalf("a pair an import explains, or a test, was reported:\n%s", out)
	}
	if want := "coupling: 6 commits in the last 6 months, 0 of them not counted for changing more than 20 files\n"; errOut != want {
		t.Fatalf("stderr does not say how much history the pairs rest on and what it left out:\n%s\ngot\n%s", want, errOut)
	}
}

// a Go pair is listed whether or not one imports the other, so the output has
// to say that imports went unread.
func TestCouplingSaysWhenItCannotReadImports(t *testing.T) {
	dir := couplingProject(t)
	writeFiles(t, dir, map[string]string{"cmd/main.go": "package main\n"})
	errOut := captureStderr(t, func() int {
		_, code := captureStdout(t, func() int { return couplingCmd([]string{dir}) })
		return code
	})
	if !strings.Contains(errOut, "imports are read in ts, tsx, js, jsx and py files only") {
		t.Fatalf("a Go file was walked and stderr did not say its imports go unread:\n%s", errOut)
	}
}

func TestCouplingPrintsJSON(t *testing.T) {
	dir := couplingProject(t)
	out, code := captureStdout(t, func() int { return couplingCmd([]string{dir, "--json"}) })
	if code != 0 {
		t.Fatalf("coupling --json exited %d", code)
	}
	var got struct {
		Pairs []struct {
			A        string  `json:"a"`
			B        string  `json:"b"`
			Together int     `json:"together"`
			ChangesA int     `json:"changes_a"`
			ChangesB int     `json:"changes_b"`
			Score    float64 `json:"score"`
		} `json:"pairs"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not json: %v\n%s", err, out)
	}
	if len(got.Pairs) != 1 || got.Pairs[0].A != "src/a.ts" || got.Pairs[0].B != "src/b.ts" ||
		got.Pairs[0].Together != 3 || got.Pairs[0].ChangesA != 3 || got.Pairs[0].ChangesB != 3 || got.Pairs[0].Score != 1 {
		t.Fatalf("want the one pair a + b, got %+v", got.Pairs)
	}
}

// the signal a CTO reads first goes at the top of the audit, above the
// appendix-tier findings.
func TestReportLeadsWithCoupledFiles(t *testing.T) {
	dir := couplingProject(t)
	md, code := captureStdout(t, func() int { return reportCmd([]string{dir}) })
	if code != 0 {
		t.Fatalf("report exited %d", code)
	}
	lead := strings.Index(md, "## lead signals")
	pair := strings.Index(md, "`src/a.ts` + `src/b.ts`")
	hot := strings.Index(md, "## hotspots")
	if lead < 0 || pair < lead || hot < pair {
		t.Fatalf("want the lead signals section, naming src/a.ts + src/b.ts, above the hotspots:\n%s", md)
	}
	if find := strings.Index(md, "## findings"); find >= 0 && find < pair {
		t.Fatalf("the findings sit above the lead signals:\n%s", md)
	}

	out, code := captureStdout(t, func() int { return reportCmd([]string{dir, "--json"}) })
	if code != 0 {
		t.Fatalf("report --json exited %d", code)
	}
	var pack struct {
		Coupling []struct {
			A string `json:"a"`
			B string `json:"b"`
		} `json:"coupling"`
	}
	if err := json.Unmarshal([]byte(out), &pack); err != nil {
		t.Fatal(err)
	}
	if len(pack.Coupling) != 1 || pack.Coupling[0].A != "src/a.ts" || pack.Coupling[0].B != "src/b.ts" {
		t.Fatalf("the pack does not carry the coupled pair: %+v\n%s", pack.Coupling, out)
	}
}
