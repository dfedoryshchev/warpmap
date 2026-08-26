package main

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
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

func names(dir string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, rel(dir, f))
	}
	sort.Strings(out)
	return out
}
