package graph

import (
	"path/filepath"
	"sort"
	"strings"
)

// Orphans returns files that nothing imports - candidate dead code. entry points
// (index/main/app, *.d.ts, test files) legitimately have no importers, so they are
// excluded to cut the noise.
func Orphans(g Graph) []string {
	imported := map[string]bool{}
	for _, e := range g.Edges {
		imported[e.To] = true
	}
	var out []string
	for _, f := range g.Files {
		if imported[f] || isEntryPoint(f) {
			continue
		}
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

func isEntryPoint(f string) bool {
	base := strings.ToLower(filepath.Base(f))
	switch {
	case strings.HasPrefix(base, "index."), strings.HasPrefix(base, "main."), strings.HasPrefix(base, "app."):
		return true
	case strings.HasSuffix(base, ".d.ts"):
		return true
	case strings.Contains(base, ".test."), strings.Contains(base, ".spec."):
		return true
	}
	return false
}
