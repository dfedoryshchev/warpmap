package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/graph"
	"github.com/dfedoryshchev/warpmap/internal/metrics"
)

// how far down the hotspot ranking the audit goes. both renderers read it, so
// they cannot disagree about what the report covers.
const topHotspots = 10

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
		Findings: Findings(g, dir),
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
	for _, h := range top(r.Hotspots, topHotspots) {
		fmt.Fprintf(&b, "| %.3f | %d | %d | %s |\n", h.Score, h.Churn, h.Complexity, relPath(r.Dir, h.File))
	}
	return b.String()
}

type pack struct {
	Files    int       `json:"files"`
	Edges    int       `json:"edges"`
	Findings []Finding `json:"findings"`
	Hotspots []ranked  `json:"hotspots"`
}

type ranked struct {
	File       string  `json:"file"`
	Score      float64 `json:"score"`
	Churn      int     `json:"churn"`
	Complexity int     `json:"complexity"`
}

// JSON renders the same audit Markdown does, for a reader that is a program:
// the evidence pack a client re-derives the claims from, and the shape an agent
// can act on.
func (r Report) JSON() ([]byte, error) {
	// a project with nothing to say still answers with lists, so a consumer
	// iterating the pack never has to special-case null.
	p := pack{
		Files:    r.Files,
		Edges:    r.Edges,
		Findings: []Finding{},
		Hotspots: []ranked{},
	}
	p.Findings = append(p.Findings, r.Findings...)
	for _, h := range top(r.Hotspots, topHotspots) {
		p.Hotspots = append(p.Hotspots, ranked{
			File:       relPath(r.Dir, h.File),
			Score:      h.Score,
			Churn:      h.Churn,
			Complexity: h.Complexity,
		})
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
