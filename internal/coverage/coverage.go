package coverage

import (
	"path/filepath"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/graph"
)

// IsTest reports whether a file looks like a test. rel is its path relative to
// the project root: a directory above the project says nothing about its files.
func IsTest(rel string) bool {
	base := strings.ToLower(filepath.Base(rel))
	lf := "/" + strings.ToLower(filepath.ToSlash(rel))
	return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		strings.Contains(lf, "/__tests__/") || strings.Contains(lf, "/tests/") ||
		strings.Contains(lf, "/e2e/")
}

// Kind classifies a test file as unit / integration / e2e by convention.
func Kind(f string) string {
	lf := strings.ToLower(filepath.ToSlash(f))
	switch {
	case strings.Contains(lf, "e2e") || strings.Contains(lf, "cypress") || strings.Contains(lf, "playwright"):
		return "e2e"
	case strings.Contains(lf, "integration") || strings.Contains(lf, ".int."):
		return "integration"
	default:
		return "unit"
	}
}

// Untested returns the source files that no test file imports - structural coverage
// from the graph (not line coverage). a file a test imports is considered exercised.
// the graph is keyed by the walk's paths under dir, and so is what comes back.
func Untested(dir string, g graph.Graph) []string {
	isTest := func(f string) bool { return IsTest(relTo(dir, f)) }
	tested := map[string]bool{}
	for _, e := range g.Edges {
		if isTest(e.From) {
			tested[e.To] = true
		}
	}
	var out []string
	for _, f := range g.Files {
		if isTest(f) || tested[f] {
			continue
		}
		out = append(out, f)
	}
	return out
}

func relTo(dir, f string) string {
	r, err := filepath.Rel(dir, f)
	if err != nil {
		return f
	}
	return r
}
