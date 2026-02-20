package graph

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Edge struct {
	From string
	To   string
}

type Graph struct {
	Files []string
	Edges []Edge
}

// --- ts / js ---

// matches: import ... from "spec" | export ... from "spec" | require("spec") |
// import("spec") | import "spec"
var tsImportRe = regexp.MustCompile(
	`(?:import|export)[^'"]*?from\s*['"]([^'"]+)['"]` +
		`|(?:require|import)\(\s*['"]([^'"]+)['"]\s*\)` +
		`|(?:^|\n)\s*import\s*['"]([^'"]+)['"]`)

var tsExts = []string{".ts", ".tsx", ".js", ".jsx"}
var tsIndex = []string{"index.ts", "index.tsx", "index.js", "index.jsx"}

func extractTs(src string) []string {
	var specs []string
	for _, m := range tsImportRe.FindAllStringSubmatch(src, -1) {
		for _, g := range m[1:] {
			if g != "" {
				specs = append(specs, g)
			}
		}
	}
	return specs
}

func resolveTs(fromFile, spec string) string {
	if !strings.HasPrefix(spec, ".") {
		return ""
	}
	base := filepath.Join(filepath.Dir(fromFile), spec)
	if hasExt(base, tsExts) && exists(base) {
		return base
	}
	for _, ext := range tsExts {
		if exists(base + ext) {
			return base + ext
		}
	}
	for _, idx := range tsIndex {
		if cand := filepath.Join(base, idx); exists(cand) {
			return cand
		}
	}
	return ""
}

// --- python (relative imports only, which are the reliable intra-project ones) ---

var pyImportRe = regexp.MustCompile(`(?m)^\s*from\s+(\.+[\w.]*)\s+import`)

func extractPy(src string) []string {
	var specs []string
	for _, m := range pyImportRe.FindAllStringSubmatch(src, -1) {
		if m[1] != "" {
			specs = append(specs, m[1])
		}
	}
	return specs
}

func resolvePy(fromFile, spec string) string {
	dots := 0
	for dots < len(spec) && spec[dots] == '.' {
		dots++
	}
	dir := filepath.Dir(fromFile)
	for i := 1; i < dots; i++ { // one leading dot = the current package
		dir = filepath.Dir(dir)
	}
	rest := strings.ReplaceAll(spec[dots:], ".", string(filepath.Separator))
	base := dir
	if rest != "" {
		base = filepath.Join(dir, rest)
	}
	for _, cand := range []string{base + ".py", filepath.Join(base, "__init__.py")} {
		if exists(cand) {
			return cand
		}
	}
	return ""
}

// --- dispatch by extension ---

func extractImports(path, src string) []string {
	if filepath.Ext(path) == ".py" {
		return extractPy(src)
	}
	return extractTs(src)
}

func resolve(fromFile, spec string) string {
	if filepath.Ext(fromFile) == ".py" {
		return resolvePy(fromFile, spec)
	}
	return resolveTs(fromFile, spec)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func hasExt(p string, exts []string) bool {
	e := filepath.Ext(p)
	for _, x := range exts {
		if e == x {
			return true
		}
	}
	return false
}

// Build resolves intra-project imports into a dependency graph.
func Build(files []string) Graph {
	g := Graph{Files: files}
	seen := map[string]bool{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, spec := range extractImports(f, string(src)) {
			to := resolve(f, spec)
			if to == "" {
				continue
			}
			key := f + "\x00" + to
			if seen[key] {
				continue
			}
			seen[key] = true
			g.Edges = append(g.Edges, Edge{From: f, To: to})
		}
	}
	return g
}
