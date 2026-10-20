package report

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/graph"
)

type Severity string

const (
	High Severity = "high"
	Med  Severity = "medium"
	Low  Severity = "low"
)

const namedFiles = 10

type Finding struct {
	Severity       Severity `json:"severity"`
	Kind           string   `json:"kind"`
	Detail         string   `json:"detail"`
	Recommendation string   `json:"recommendation"`
	Files          []string `json:"files"`
}

func relPath(dir, f string) string {
	r, err := filepath.Rel(dir, f)
	if err != nil {
		return f
	}
	return filepath.ToSlash(r)
}

func relPaths(dir string, fs []string) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = relPath(dir, f)
	}
	return out
}

func name(files []string, sep string) string {
	if len(files) <= namedFiles {
		return strings.Join(files, sep)
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(files[:namedFiles], sep), len(files)-namedFiles)
}

// Findings turns the graph analyses into ranked, actionable findings.
func Findings(g graph.Graph, dir string) []Finding {
	var fs []Finding
	for _, m := range graph.GodModules(g) {
		if !m.IsGod() {
			break
		}
		f := relPath(dir, m.File)
		fs = append(fs, Finding{
			Severity:       High,
			Kind:           "god-module",
			Detail:         fmt.Sprintf("%s (in=%d out=%d)", f, m.FanIn, m.FanOut),
			Recommendation: "too central; a change here has a wide blast radius - split responsibilities",
			Files:          []string{f},
		})
	}
	for _, c := range graph.Cycles(g) {
		files := relPaths(dir, c)
		fs = append(fs, Finding{
			Severity:       Med,
			Kind:           "cycle",
			Detail:         fmt.Sprintf("import cycle across %d files: %s", len(files), name(files, " -> ")),
			Recommendation: "break it - extract the shared piece into a third module",
			Files:          files,
		})
	}
	if files := relPaths(dir, graph.Orphans(g)); len(files) > 0 {
		sort.Strings(files)
		fs = append(fs, Finding{
			Severity:       Low,
			Kind:           "dead-code",
			Detail:         fmt.Sprintf("%d files nothing imports: %s", len(files), name(files, ", ")),
			Recommendation: "confirm they are entry points, or delete them",
			Files:          files,
		})
	}
	return fs
}
