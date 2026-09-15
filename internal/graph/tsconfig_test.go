package graph

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveTsAliasThroughTsconfigPaths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tsconfig.json"), `{"compilerOptions":{"baseUrl":".","paths":{"@/*":["src/*"]}}}`)
	writeFile(t, filepath.Join(dir, "src", "a.ts"), "export const a = 1;\n")
	from := filepath.Join(dir, "src", "b.ts")
	writeFile(t, from, `import { a } from "@/a";`+"\n")

	want := filepath.Join(dir, "src", "a.ts")
	if got := resolveTs(from, "@/a"); got != want {
		t.Fatalf("resolveTs(%q, %q) = %q, want %q", from, "@/a", got, want)
	}
}

func TestResolveTsAliasMissingTargetReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tsconfig.json"), `{"compilerOptions":{"baseUrl":".","paths":{"@/*":["src/*"]}}}`)
	from := filepath.Join(dir, "src", "b.ts")
	writeFile(t, from, `import { a } from "@/a";`+"\n")

	if got := resolveTs(from, "@/a"); got != "" {
		t.Fatalf("resolveTs on a missing alias target = %q, want empty", got)
	}
}

func TestResolveTsNoTsconfigLeavesAliasUnresolved(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "src", "a.ts"), "export const a = 1;\n")
	from := filepath.Join(dir, "src", "b.ts")
	writeFile(t, from, `import { a } from "@/a";`+"\n")

	if got := resolveTs(from, "@/a"); got != "" {
		t.Fatalf("resolveTs with no tsconfig.json anywhere = %q, want empty", got)
	}
}

func TestResolveTsTsconfigWithoutPathsLeavesAliasUnresolved(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tsconfig.json"), `{"compilerOptions":{"target":"es2020"}}`)
	writeFile(t, filepath.Join(dir, "src", "a.ts"), "export const a = 1;\n")
	from := filepath.Join(dir, "src", "b.ts")
	writeFile(t, from, `import { a } from "@/a";`+"\n")

	if got := resolveTs(from, "@/a"); got != "" {
		t.Fatalf("resolveTs with a paths-less tsconfig.json = %q, want empty", got)
	}
}
