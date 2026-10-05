package main

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dfedoryshchev/warpmap/internal/config"
	"github.com/dfedoryshchev/warpmap/internal/graph"
)

// every command prints the same file the same way. half of them used to print
// the path the walk produced, which on windows is backslashed and carries
// whatever prefix the caller typed.
func TestRelIsSlashedAndRelative(t *testing.T) {
	dir := filepath.Join("..", "proj")
	got := rel(dir, filepath.Join("..", "proj", "src", "a.ts"))
	if got != "src/a.ts" {
		t.Fatalf("rel = %q, want %q", got, "src/a.ts")
	}
}

// the caller can name the project any way it likes; what comes out is the same
// path either way. this is the shape that bites - a plain relative directory is
// the one case where relativising a second time succeeds and silently prepends
// `..`, so only the walk's own path may be handed in.
func TestRelFromAPlainRelativeDir(t *testing.T) {
	got := rel("proj", filepath.Join("proj", "src", "a.ts"))
	if got != "src/a.ts" {
		t.Fatalf("rel = %q, want %q", got, "src/a.ts")
	}
}

// some pairs have no relative form at all: filepath.Rel refuses to walk out of
// a base that starts with "..". the file is still printed, slashed, rather than
// coming out as the empty string.
func TestRelFallsBackToSlashedInput(t *testing.T) {
	dir := filepath.Join("..", "proj")
	if _, err := filepath.Rel(dir, filepath.Join("src", "a.ts")); err == nil {
		t.Fatalf("no fallback to exercise: filepath.Rel accepted the pair")
	}
	got := rel(dir, filepath.Join("src", "a.ts"))
	if got != "src/a.ts" {
		t.Fatalf("rel = %q, want %q", got, "src/a.ts")
	}
}

// the walk has to honour the project's own ignore list, not just the built-in
// one. this is the wiring test: config_test proves the matcher, this proves
// sourceFiles actually asks it.
func TestSourceFilesHonoursConfiguredIgnores(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"src", "vendor", "node_modules"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, d, "a.ts"), []byte("export const a=1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// with no config the built-ins still apply and nothing else is dropped.
	got := names(dir, sourceFiles(dir))
	if !slices.Contains(got, "vendor/a.ts") {
		t.Fatalf("vendor was dropped without being configured: %v", got)
	}
	if slices.Contains(got, "node_modules/a.ts") {
		t.Fatalf("node_modules survived the default walk: %v", got)
	}

	// naming vendor drops it, and does NOT bring node_modules back.
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(`{"ignore":["vendor"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got = names(dir, sourceFiles(dir))
	if slices.Contains(got, "vendor/a.ts") {
		t.Fatalf("configured ignore had no effect: %v", got)
	}
	if slices.Contains(got, "node_modules/a.ts") {
		t.Fatalf("a project ignore list replaced the built-ins instead of adding to them: %v", got)
	}
	if !slices.Contains(got, "src/a.ts") {
		t.Fatalf("src was swallowed: %v", got)
	}
}

func TestSourceFilesSkipsVirtualEnvironmentsByDefault(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"app/main.py": "import os\n",
		".venv/lib/python3.12/site-packages/x.py": "x = 1\n",
		"venv/lib/python3.12/site-packages/y.py":  "y = 1\n",
		"web/.next/static/chunks/main.js":         "var a=1;\n",
		".turbo/cache/out.js":                     "var b=1;\n",
		"web/src/index.ts":                        "export const c=1;\n",
	}
	for name, src := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := names(dir, sourceFiles(dir))
	slices.Sort(got)
	want := []string{"app/main.py", "web/src/index.ts"}
	if !slices.Equal(got, want) {
		t.Fatalf("walked %v, want only the project's own sources %v", got, want)
	}
}

// the dashboard is the one command whose output is meant to be opened rather
// than read in a terminal, so -o has to produce the same document stdout does,
// and it has to produce one row per source file.
func TestDashboardWritesTheSamePageToAFileAsToStdout(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{
		"src/a.ts": "import { b } from \"./b\";\nexport const a = b + 1;\n",
		"src/b.ts": "export const b = 1;\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out := filepath.Join(dir, "dashboard.html")
	if code := dashboardCmd([]string{"-o", out, dir}); code != 0 {
		t.Fatalf("dashboard -o exited %d", code)
	}
	page, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("-o wrote nothing: %v", err)
	}

	got := string(page)
	if !strings.HasPrefix(got, "<!doctype html>") || !strings.HasSuffix(got, "</html>\n") {
		t.Fatal("-o did not write a whole document")
	}
	for _, want := range []string{"src/a.ts", "src/b.ts", "2 files, 1 import edges, 2 ranked"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the page is missing %q", want)
		}
	}
	if n := strings.Count(got, "<rect class=\"file\""); n != 2 {
		t.Fatalf("%d boxes on the map, want one per source file (2)", n)
	}
	// the page must not depend on where it was written: a second run into a
	// different file is byte for byte the same document.
	twin := filepath.Join(t.TempDir(), "twin.html")
	if code := dashboardCmd([]string{"-o", twin, dir}); code != 0 {
		t.Fatalf("second dashboard -o exited %d", code)
	}
	again, err := os.ReadFile(twin)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != got {
		t.Fatal("two runs over one project wrote different pages")
	}
}

// analyze is the one command whose output is written for another program to
// read, and it was the one handing out the path the walk produced rather than
// the path every other command prints. named absolutely - which is how a script
// and the ci action both name a project - the graph came back keyed by the
// analysing machine's own absolute, backslashed paths.
func TestAnalyzeDotNamesFilesLikeEveryOtherCommand(t *testing.T) {
	dir := chainProject(t)
	out, code := captureStdout(t, func() int { return analyzeCmd([]string{dir, "--dot"}) })
	if code != 0 {
		t.Fatalf("analyze exited %d", code)
	}
	want := "digraph deps {\n  rankdir=LR;\n" +
		"  \"src/a.ts\" -> \"src/b.ts\";\n" +
		"  \"src/b.ts\" -> \"src/c.ts\";\n" +
		"}\n"
	if out != want {
		t.Fatalf("analyze --dot printed\n%swant\n%s", out, want)
	}
}

func TestAnalyzeJSONNamesFilesLikeEveryOtherCommand(t *testing.T) {
	dir := chainProject(t)
	out, code := captureStdout(t, func() int { return analyzeCmd([]string{dir, "--json"}) })
	if code != 0 {
		t.Fatalf("analyze exited %d", code)
	}
	var g graph.Graph
	if err := json.Unmarshal([]byte(out), &g); err != nil {
		t.Fatalf("analyze --json printed something that is not json: %v\n%s", err, out)
	}
	wantFiles := []string{"src/a.ts", "src/b.ts", "src/c.ts"}
	if !slices.Equal(g.Files, wantFiles) {
		t.Fatalf("files = %v, want %v", g.Files, wantFiles)
	}
	wantEdges := []graph.Edge{{From: "src/a.ts", To: "src/b.ts"}, {From: "src/b.ts", To: "src/c.ts"}}
	if !slices.Equal(g.Edges, wantEdges) {
		t.Fatalf("edges = %v, want %v", g.Edges, wantEdges)
	}
	// the whole point of the spelling: the graph lines up with what the walk -
	// and so every other command - calls the same files.
	walked := names(dir, sourceFiles(dir))
	joined := append([]string(nil), g.Files...)
	sort.Strings(joined)
	if !slices.Equal(joined, walked) {
		t.Fatalf("the graph names files %v, every other command names them %v", joined, walked)
	}
}

// respelling the paths is all it does. a project with no source files still
// encodes the way it always has, so a consumer that already handles the empty
// graph keeps working.
func TestAnalyzeJSONKeepsTheEmptyGraphShape(t *testing.T) {
	out, code := captureStdout(t, func() int { return analyzeCmd([]string{t.TempDir(), "--json"}) })
	if code != 0 {
		t.Fatalf("analyze exited %d", code)
	}
	if want := "{\n  \"Files\": null,\n  \"Edges\": null\n}\n"; out != want {
		t.Fatalf("the empty graph changed shape:\n%swant\n%s", out, want)
	}
}

func TestHotspotsRanksTheCodeNotItsTests(t *testing.T) {
	dir := chainProject(t)
	for name, src := range map[string]string{
		"src/a.test.ts":      "import { a } from \"./a\";\nif (a) { if (a) { if (a) {} } }\n",
		"tests/b.ts":         "import { b } from \"../src/b\";\nif (b) { if (b) { if (b) {} } }\n",
		"src/x/a.spec.tsx":   "import { a } from \"../a\";\nif (a) { if (a) { if (a) {} } }\n",
		"src/__tests__/c.js": "if (1) { if (1) { if (1) {} } }\n",
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, code := captureStdout(t, func() int { return hotspotsCmd([]string{dir}) })
	if code != 0 {
		t.Fatalf("hotspots exited %d", code)
	}
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		got = append(got, fields[len(fields)-1])
	}
	sort.Strings(got)
	if want := []string{"src/a.ts", "src/b.ts", "src/c.ts"}; !slices.Equal(got, want) {
		t.Fatalf("hotspots ranked %q, want only the shipped files %q", got, want)
	}
}

// the directory comes first in every example the readme and `warpmap help`
// print, which is the order the flag package cannot read: it stops at the first
// non-flag argument, so everything after the directory was swallowed into
// fset.Args() and the command silently ran with its defaults and exited 0. the
// five tests below are one per flag-taking shape - a count flag, a format flag,
// a value flag on a two-positional command, and the two commands that write a
// file.

func TestHotspotsReadsTheCountFlagAfterTheDirectory(t *testing.T) {
	dir := chainProject(t)
	out, code := captureStdout(t, func() int { return hotspotsCmd([]string{dir, "-n", "1"}) })
	if code != 0 {
		t.Fatalf("hotspots exited %d", code)
	}
	if n := countLines(out); n != 1 {
		t.Fatalf("hotspots <dir> -n 1 printed %d rows, want 1:\n%s", n, out)
	}
}

func TestAnalyzeReadsTheFormatFlagAfterTheDirectory(t *testing.T) {
	dir := chainProject(t)
	out, code := captureStdout(t, func() int { return analyzeCmd([]string{dir, "--json"}) })
	if code != 0 {
		t.Fatalf("analyze exited %d", code)
	}
	if !strings.HasPrefix(out, "{") {
		t.Fatalf("analyze <dir> --json printed the plain summary, not json:\n%s", out)
	}
	dot, code := captureStdout(t, func() int { return analyzeCmd([]string{dir, "--dot"}) })
	if code != 0 {
		t.Fatalf("analyze exited %d", code)
	}
	if !strings.HasPrefix(dot, "digraph") {
		t.Fatalf("analyze <dir> --dot printed the plain summary, not dot:\n%s", dot)
	}
}

func TestTraceReadsTheDepthFlagAfterTheDirectory(t *testing.T) {
	dir := chainProject(t)
	// a.ts -> b.ts -> c.ts, so one hop out of c.ts reaches b.ts and only b.ts.
	out, code := captureStdout(t, func() int {
		return traceCmd([]string{dir, "src/c.ts", "--depth", "1"})
	})
	if code != 0 {
		t.Fatalf("trace exited %d", code)
	}
	if !strings.HasPrefix(out, "1 files depend on") {
		t.Fatalf("trace <dir> <file> --depth 1 did not stop at one hop:\n%s", out)
	}
}

func TestBriefListsDependentsInAStableOrderAndSaysSo(t *testing.T) {
	dir := chainProject(t)
	// a.ts -> b.ts -> c.ts, so both of the others depend on c.ts.
	want := "2 of the 2 dependents, alphabetically:\n  - src/a.ts\n  - src/b.ts\n"
	for i := 0; i < 20; i++ {
		out, code := captureStdout(t, func() int { return briefCmd([]string{dir, "src/c.ts"}) })
		if code != 0 {
			t.Fatalf("brief exited %d", code)
		}
		if !strings.Contains(out, want) {
			t.Fatalf("run %d printed\n%swant it to contain\n%s", i, out, want)
		}
	}
}

func TestReportReadsTheOutputFlagAfterTheDirectory(t *testing.T) {
	dir := chainProject(t)
	dest := filepath.Join(t.TempDir(), "audit.md")
	out, code := captureStdout(t, func() int { return reportCmd([]string{dir, "-o", dest}) })
	if code != 0 {
		t.Fatalf("report exited %d", code)
	}
	if out != "" {
		t.Fatalf("report <dir> -o <file> printed the audit to stdout instead of writing it:\n%s", out)
	}
	md, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("report <dir> -o <file> wrote no file: %v", err)
	}
	if !strings.HasPrefix(string(md), "#") {
		t.Fatalf("the file is not the markdown audit:\n%s", md)
	}
}

func TestDashboardReadsTheOutputFlagAfterTheDirectory(t *testing.T) {
	dir := chainProject(t)
	dest := filepath.Join(t.TempDir(), "dashboard.html")
	out, code := captureStdout(t, func() int { return dashboardCmd([]string{dir, "-o", dest}) })
	if code != 0 {
		t.Fatalf("dashboard exited %d", code)
	}
	if out != "" {
		t.Fatalf("dashboard <dir> -o <file> printed the page to stdout instead of writing it:\n%s", out)
	}
	page, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("dashboard <dir> -o <file> wrote no file: %v", err)
	}
	if !strings.HasPrefix(string(page), "<!doctype html>") {
		t.Fatalf("the file is not the dashboard page:\n%s", page)
	}
}

// the audit's machine-readable half has to reach the same two places its
// markdown does, because the evidence pack is written next to the document it
// backs.
func TestReportWritesTheJSONVariant(t *testing.T) {
	dir := chainProject(t)
	out, code := captureStdout(t, func() int { return reportCmd([]string{dir, "--json"}) })
	if code != 0 {
		t.Fatalf("report --json exited %d", code)
	}
	var pack struct {
		Files    int `json:"files"`
		Edges    int `json:"edges"`
		Hotspots []struct {
			File string `json:"file"`
		} `json:"hotspots"`
	}
	if err := json.Unmarshal([]byte(out), &pack); err != nil {
		t.Fatalf("report --json printed something that is not json: %v\n%s", err, out)
	}
	if pack.Files != 3 || pack.Edges != 2 {
		t.Fatalf("the pack describes a different project: %d files, %d edges", pack.Files, pack.Edges)
	}
	got := make([]string, 0, len(pack.Hotspots))
	for _, h := range pack.Hotspots {
		got = append(got, h.File)
	}
	sort.Strings(got)
	if want := names(dir, sourceFiles(dir)); !slices.Equal(got, want) {
		t.Fatalf("the pack ranks %v, the walk found %v", got, want)
	}

	md, code := captureStdout(t, func() int { return reportCmd([]string{dir}) })
	if code != 0 {
		t.Fatalf("report exited %d", code)
	}
	if !strings.HasPrefix(md, "# warpmap audit") {
		t.Fatalf("the default audit is no longer markdown:\n%s", md)
	}

	dest := filepath.Join(t.TempDir(), "audit.json")
	written, code := captureStdout(t, func() int { return reportCmd([]string{dir, "--json", "-o", dest}) })
	if code != 0 {
		t.Fatalf("report --json -o exited %d", code)
	}
	if written != "" {
		t.Fatalf("report --json -o <file> printed the pack to stdout as well:\n%s", written)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("report --json -o <file> wrote no file: %v", err)
	}
	if string(b) != out {
		t.Fatalf("-o wrote a different pack than stdout printed:\n%s\n%s", b, out)
	}
}

// the order that already worked has to keep working: accepting flags anywhere is
// only allowed to add spellings, never to move one.
func TestFlagsBeforeTheDirectoryStillWork(t *testing.T) {
	dir := chainProject(t)
	out, code := captureStdout(t, func() int { return hotspotsCmd([]string{"-n", "2", dir}) })
	if code != 0 {
		t.Fatalf("hotspots exited %d", code)
	}
	if n := countLines(out); n != 2 {
		t.Fatalf("hotspots -n 2 <dir> printed %d rows, want 2:\n%s", n, out)
	}
	depth, code := captureStdout(t, func() int {
		return traceCmd([]string{"--depth", "1", dir, "src/c.ts"})
	})
	if code != 0 {
		t.Fatalf("trace exited %d", code)
	}
	if !strings.HasPrefix(depth, "1 files depend on") {
		t.Fatalf("trace --depth 1 <dir> <file> lost its depth limit:\n%s", depth)
	}
}

// pulling positionals out of the middle must not cost the one escape hatch a
// caller has: after an explicit `--` nothing is a flag, however it is spelled.
func TestDoubleDashStillEndsTheFlags(t *testing.T) {
	dir := chainProject(t)
	odd := "src/-n.ts"
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(odd)), []byte("export const n = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := captureStdout(t, func() int { return traceCmd([]string{"--", dir, odd}) })
	if code != 0 {
		t.Fatalf("trace exited %d", code)
	}
	if !strings.HasPrefix(out, "0 files depend on "+odd) {
		t.Fatalf("a file named like a flag was not traced as a file:\n%s", out)
	}
}

// the walk spells a top-level file with no leading separator when the project
// is named `.`, and as `<dir>/...` otherwise, so both spellings have to agree.
func TestTestgapCountsTheSameWhateverTheProjectIsCalled(t *testing.T) {
	dir := chainProject(t)
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	setup := "import { a } from \"../src/a\";\nexport const s = a;\n"
	if err := os.WriteFile(filepath.Join(dir, "tests", "setup.ts"), []byte(setup), 0o644); err != nil {
		t.Fatal(err)
	}
	named, code := captureStdout(t, func() int { return testgapCmd([]string{dir}) })
	if code != 0 {
		t.Fatalf("testgap exited %d", code)
	}
	t.Chdir(dir)
	dot, code := captureStdout(t, func() int { return testgapCmd([]string{"."}) })
	if code != 0 {
		t.Fatalf("testgap . exited %d", code)
	}
	want := "2 untested files; the riskiest (highest blast radius) first - test these before you change them:\n" +
		"  blast=3    src/c.ts\n  blast=2    src/b.ts\n"
	if named != want || dot != want {
		t.Fatalf("testgap <dir> printed\n%stestgap . printed\n%swant\n%s", named, dot, want)
	}
}

func TestRiskFailsOnAnUntestedChangePastTheThreshold(t *testing.T) {
	dir := chainProject(t)
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(`{"thresholds":{"blast":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := captureStdout(t, func() int { return riskCmd([]string{dir, "src/c.ts"}) })
	want := "  src/c.ts: blast=2, UNTESTED\ncombined blast radius: 2 files\n" +
		"verdict: 1 changed file(s) are high-blast AND untested - add tests before changing\n"
	if code != 1 || out != want {
		t.Fatalf("risk exited %d and printed\n%swant 1 and\n%s", code, out, want)
	}
	out, code = captureStdout(t, func() int { return riskCmd([]string{dir, "src/a.ts", "src/b.ts"}) })
	want = "  src/a.ts: blast=0, UNTESTED\n  src/b.ts: blast=1, UNTESTED\ncombined blast radius: 1 files\nverdict: manageable\n"
	if code != 0 || out != want {
		t.Fatalf("risk exited %d and printed\n%swant 0 and\n%s", code, out, want)
	}
}

// git reads the path it is given from inside the project, so a project named
// relative to where the caller stands used to match no history at all and
// every file printed `authors=0`.
func TestOwnershipCountsTheSameWhateverTheProjectIsCalled(t *testing.T) {
	dir := chainProject(t)
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "commit.gpgsign=false", "-c", "user.name=Ada", "-c", "user.email=a@example.com"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	// git writes its objects read-only, which the temp-dir cleanup cannot remove on windows.
	t.Cleanup(func() {
		filepath.WalkDir(filepath.Join(dir, ".git"), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				os.Chmod(p, 0o666)
			}
			return nil
		})
	})
	git("add", "--", "src")
	git("commit", "-m", "add the chain")

	run := func(project string) []string {
		t.Helper()
		out, code := captureStdout(t, func() int { return ownershipCmd([]string{project}) })
		if code != 0 {
			t.Fatalf("ownership %s exited %d", project, code)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")[1:]
		sort.Strings(lines)
		return lines
	}
	want := []string{
		"  authors=1  src/a.ts  <- bus factor 1",
		"  authors=1  src/b.ts  <- bus factor 1",
		"  authors=1  src/c.ts  <- bus factor 1",
	}
	if got := run(dir); !slices.Equal(got, want) {
		t.Fatalf("ownership <absolute> printed %q, want %q", got, want)
	}
	t.Chdir(filepath.Dir(dir))
	if got := run(filepath.Base(dir)); !slices.Equal(got, want) {
		t.Fatalf("ownership <relative> printed %q, want %q", got, want)
	}
}

// chainProject writes a three-file import chain: a.ts -> b.ts -> c.ts. it gives
// hotspots more than one row to cap and trace more than one hop to stop at.
func chainProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{
		"src/a.ts": "import { b } from \"./b\";\nexport const a = b + 1;\n",
		"src/b.ts": "import { c } from \"./c\";\nexport const b = c + 1;\n",
		"src/c.ts": "export const c = 1;\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// captureStdout runs fn with os.Stdout pointed at a pipe and returns everything
// it printed. the commands print with fmt.Printf, so reading the descriptor back
// is the only way to assert on what the caller actually sees.
func captureStdout(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	return capture(t, &os.Stdout, fn)
}

func captureStderr(t *testing.T, fn func() int) string {
	t.Helper()
	out, _ := capture(t, &os.Stderr, fn)
	return out
}

func capture(t *testing.T, stream **os.File, fn func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := *stream
	*stream = w
	read := make(chan string, 1)
	go func() {
		var b strings.Builder
		io.Copy(&b, r)
		read <- b.String()
	}()
	code := fn()
	*stream = saved
	w.Close()
	out := <-read
	r.Close()
	return out, code
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(strings.TrimSuffix(s, "\n"), "\n"))
}

func names(dir string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, rel(dir, f))
	}
	sort.Strings(out)
	return out
}
