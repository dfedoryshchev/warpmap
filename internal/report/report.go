package report

import (
	"fmt"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/graph"
	"github.com/dfedoryshchev/warpmap/internal/metrics"
)

type Report struct {
	Dir      string
	Files    int
	Edges    int
	Hotspots []metrics.Hotspot
	Findings []Finding
}

// Build runs the analyses and assembles a report for a project.
func Build(dir string, files []string, churn metrics.Churn) Report {
	g := graph.Build(files)
	return Report{
		Dir:      dir,
		Files:    len(g.Files),
		Edges:    len(g.Edges),
		Hotspots: metrics.Hotspots(dir, files, churn),
		Findings: Findings(g),
	}
}

func top(h []metrics.Hotspot, n int) []metrics.Hotspot {
	if n > len(h) {
		n = len(h)
	}
	return h[:n]
}

func (r Report) Markdown() string {
	var b strings.Builder
	b.WriteString("# warpmap audit\n\n")
	fmt.Fprintf(&b, "- files: %d\n- import edges: %d\n- findings: %d\n\n", r.Files, r.Edges, len(r.Findings))

	if len(r.Findings) > 0 {
		b.WriteString("## findings\n\n")
		for _, f := range r.Findings {
			fmt.Fprintf(&b, "- **[%s] %s** - %s\n  - %s\n", f.Severity, f.Kind, f.Detail, f.Recommendation)
		}
		b.WriteString("\n")
	}

	b.WriteString("## hotspots\n\n")
	b.WriteString("| score | churn | complexity | file |\n| ----- | ----- | ---------- | ---- |\n")
	for _, h := range top(r.Hotspots, 10) {
		fmt.Fprintf(&b, "| %.3f | %d | %d | %s |\n", h.Score, h.Churn, h.Complexity, h.File)
	}
	return b.String()
}
