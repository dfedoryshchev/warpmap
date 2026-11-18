package coverage

import (
	"path/filepath"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/graph"
)

// IsTest reports whether a file looks like a test. rel is its path relative to
// the project root: a directory above the project says nothing about its files.
func IsTest(rel string) bool {
	if graph.IsTestName(filepath.Base(rel)) {
		return true
	}
	lf := "/" + strings.ToLower(filepath.ToSlash(rel))
	for _, d := range []string{"/__tests__/", "/__typetest__/", "/tests/", "/test/", "/e2e/"} {
		if strings.Contains(lf, d) {
			return true
		}
	}
	return false
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
	untested, _ := split(dir, g)
	return untested
}

// TypesOnly returns the source files that tests import only for their types. the
// compiler erases such an import, so the suite never runs the file, but it does
// type-check it; these are neither tested nor untested.
func TypesOnly(dir string, g graph.Graph) []string {
	_, typesOnly := split(dir, g)
	return typesOnly
}

func split(dir string, g graph.Graph) (untested, typesOnly []string) {
	isTest := func(f string) bool { return IsTest(relTo(dir, f)) }
	runs, names := map[string]bool{}, map[string]bool{}
	for _, e := range g.Edges {
		if !isTest(e.From) {
			continue
		}
		if e.TypeOnly {
			names[e.To] = true
		} else {
			runs[e.To] = true
		}
	}
	for _, f := range g.Files {
		switch {
		case isTest(f) || runs[f]:
		case names[f]:
			typesOnly = append(typesOnly, f)
		default:
			untested = append(untested, f)
		}
	}
	return untested, typesOnly
}

func relTo(dir, f string) string {
	r, err := filepath.Rel(dir, f)
	if err != nil {
		return f
	}
	return r
}
