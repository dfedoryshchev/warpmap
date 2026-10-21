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

func aliasEdges(t *testing.T, files ...string) []Edge {
	t.Helper()
	return Build(files).Edges
}

func TestAliasEdgesSameFromPackageRootAndSubdirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "tsconfig.json"), `{"compilerOptions":{"baseUrl":".","paths":{"@/*":["pkg/src/*"]}}}`)
	writeFile(t, filepath.Join(root, "pkg", "src", "a.ts"), "export const a = 1;\n")
	writeFile(t, filepath.Join(root, "pkg", "src", "b.ts"), `import { a } from "@/a";`+"\n")

	t.Chdir(root)
	fromRoot := aliasEdges(t, filepath.Join("pkg", "src", "a.ts"), filepath.Join("pkg", "src", "b.ts"))
	if len(fromRoot) != 1 {
		t.Fatalf("edges from the tsconfig directory = %v, want 1", fromRoot)
	}

	t.Chdir(filepath.Join(root, "pkg"))
	fromPkg := aliasEdges(t, filepath.Join("src", "a.ts"), filepath.Join("src", "b.ts"))
	want := Edge{From: filepath.Join("src", "b.ts"), To: filepath.Join("src", "a.ts")}
	if len(fromPkg) != 1 || fromPkg[0] != want {
		t.Fatalf("edges from a package below the tsconfig = %v, want [%v]", fromPkg, want)
	}
}

func TestFindTsconfigStopsAtRepoRoot(t *testing.T) {
	outer := t.TempDir()
	writeFile(t, filepath.Join(outer, "tsconfig.json"), `{"compilerOptions":{"baseUrl":".","paths":{"@/*":["src/*"]}}}`)
	writeFile(t, filepath.Join(outer, "src", "a.ts"), "export const a = 1;\n")
	repo := filepath.Join(outer, "repo")
	writeFile(t, filepath.Join(repo, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeFile(t, filepath.Join(repo, "src", "a.ts"), "export const a = 1;\n")
	from := filepath.Join(repo, "src", "b.ts")
	writeFile(t, from, `import { a } from "@/a";`+"\n")

	if got := resolveTs(from, "@/a"); got != "" {
		t.Fatalf("resolveTs used a tsconfig above the repo root: %q", got)
	}
}

func TestResolveTsJsSpecifierFindsTsSource(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "src", "helpers.ts"), "export const h = 1;\n")
	writeFile(t, filepath.Join(dir, "src", "view.tsx"), "export const v = 1;\n")
	from := filepath.Join(dir, "src", "main.ts")
	writeFile(t, from, `import { h } from "./helpers.js";`+"\n"+`import { v } from "./view.jsx";`+"\n")

	for spec, want := range map[string]string{
		"./helpers.js": filepath.Join(dir, "src", "helpers.ts"),
		"./view.jsx":   filepath.Join(dir, "src", "view.tsx"),
		"./view.js":    filepath.Join(dir, "src", "view.tsx"),
	} {
		if got := resolveTs(from, spec); got != want {
			t.Errorf("resolveTs(%q, %q) = %q, want %q", from, spec, got, want)
		}
	}
}

func TestResolveTsJsSpecifierPrefersTsSourceOverCompiledJs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "src", "helpers.js"), "export const h = 1;\n")
	writeFile(t, filepath.Join(dir, "src", "helpers.ts"), "export const h = 1;\n")
	from := filepath.Join(dir, "src", "main.ts")
	writeFile(t, from, `import { h } from "./helpers.js";`+"\n")

	want := filepath.Join(dir, "src", "helpers.ts")
	if got := resolveTs(from, "./helpers.js"); got != want {
		t.Fatalf("resolveTs(%q, %q) = %q, want %q", from, "./helpers.js", got, want)
	}
}

func TestResolveTsJsSpecifierInPlainJsProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "src", "helpers.js"), "export const h = 1;\n")
	from := filepath.Join(dir, "src", "main.js")
	writeFile(t, from, `import { h } from "./helpers.js";`+"\n")

	want := filepath.Join(dir, "src", "helpers.js")
	if got := resolveTs(from, "./helpers.js"); got != want {
		t.Fatalf("resolveTs(%q, %q) = %q, want %q", from, "./helpers.js", got, want)
	}
}

func TestResolveTsJsSpecifierWithNoSourceReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "src", "main.ts")
	writeFile(t, from, `import { h } from "./helpers.js";`+"\n")

	if got := resolveTs(from, "./helpers.js"); got != "" {
		t.Fatalf("resolveTs on a .js specifier with no source = %q, want empty", got)
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
