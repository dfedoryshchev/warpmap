package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dfedoryshchev/warpmap/internal/config"
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
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	read := make(chan string, 1)
	go func() {
		var b strings.Builder
		io.Copy(&b, r)
		read <- b.String()
	}()
	code := fn()
	os.Stdout = saved
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
