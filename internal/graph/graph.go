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

// matches: import ... from "spec" | export ... from "spec" | require("spec") |
// import("spec") | import "spec"
var importRe = regexp.MustCompile(
	`(?:import|export)[^'"]*?from\s*['"]([^'"]+)['"]` +
		`|(?:require|import)\(\s*['"]([^'"]+)['"]\s*\)` +
		`|(?:^|\n)\s*import\s*['"]([^'"]+)['"]`)

func extractImports(path string) []string {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var specs []string
	for _, m := range importRe.FindAllStringSubmatch(string(src), -1) {
		for _, g := range m[1:] {
			if g != "" {
				specs = append(specs, g)
			}
		}
	}
	return specs
}

// resolve a relative specifier to a file on disk. bare/external specifiers return "".
func resolve(fromFile, spec string) string {
	if !strings.HasPrefix(spec, ".") {
		return ""
	}
	base := filepath.Join(filepath.Dir(fromFile), spec)
	if _, err := os.Stat(base + ".ts"); err == nil {
		return base + ".ts"
	}
	return ""
}

// Build resolves intra-project imports into a dependency graph.
func Build(files []string) Graph {
	g := Graph{Files: files}
	seen := map[string]bool{}
	for _, f := range files {
		for _, spec := range extractImports(f) {
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
