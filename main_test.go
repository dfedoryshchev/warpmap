package main

import (
	"path/filepath"
	"testing"
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
