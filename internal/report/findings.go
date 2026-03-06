package report

import (
	"fmt"

	"github.com/dfedoryshchev/warpmap/internal/graph"
)

type Severity string

const (
	High Severity = "high"
	Med  Severity = "medium"
	Low  Severity = "low"
)

type Finding struct {
	Severity       Severity
	Kind           string
	Detail         string
	Recommendation string
}

// Findings turns the graph analyses into ranked, actionable findings.
func Findings(g graph.Graph) []Finding {
	var fs []Finding
	for _, m := range graph.GodModules(g) {
		if m.FanIn+m.FanOut < 20 {
			break
		}
		fs = append(fs, Finding{
			Severity:       High,
			Kind:           "god-module",
			Detail:         fmt.Sprintf("%s (in=%d out=%d)", m.File, m.FanIn, m.FanOut),
			Recommendation: "too central; a change here has a wide blast radius - split responsibilities",
		})
	}
	for _, c := range graph.Cycles(g) {
		fs = append(fs, Finding{
			Severity:       Med,
			Kind:           "cycle",
			Detail:         fmt.Sprintf("import cycle across %d files", len(c)),
			Recommendation: "break it - extract the shared piece into a third module",
		})
	}
	if n := len(graph.Orphans(g)); n > 0 {
		fs = append(fs, Finding{
			Severity:       Low,
			Kind:           "dead-code",
			Detail:         fmt.Sprintf("%d files nothing imports", n),
			Recommendation: "confirm they are entry points, or delete them",
		})
	}
	return fs
}
