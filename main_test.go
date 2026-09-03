package main

import (
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

func names(dir string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, rel(dir, f))
	}
	sort.Strings(out)
	return out
}
