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
const (
	topHotspots = 10
	topPairs    = 10
)

type Report struct {
	Dir      string
	Files    int
	Edges    int
	Coupling []metrics.Pair
	Hotspots []metrics.Hotspot
	Findings []Finding
	// Authors is how many people made the commits in the window.
	Authors int
	Held    []metrics.Held
}

// OneAuthor is what the report and `warpmap ownership` say instead of listing
// every hotspot when one person made every commit in the window.
const OneAuthor = "one author across the window: every file has a bus factor of 1"

// Build runs the analyses and assembles a report for a project. commits is the
// history churn was counted from; the coupled pairs and the author count come
// from it.
func Build(dir string, files []string, churn metrics.Churn, commits []metrics.Commit) Report {
	g := graph.Build(files)
	r := Report{
		Dir:      dir,
		Files:    len(g.Files),
		Edges:    len(g.Edges),
		Coupling: metrics.HiddenCoupling(dir, files, g, commits),
		Hotspots: metrics.Hotspots(dir, files, churn),
		Findings: Findings(g, dir),
		Authors:  len(metrics.Authors(commits)),
	}
	if r.Authors != 1 {
		var checked []string
		for _, h := range top(r.Hotspots, topHotspots) {
			checked = append(checked, h.File)
		}
		r.Held = metrics.HeldByOne(checked)
	}
	return r
}

func topCoupled(p []metrics.Pair) []metrics.Pair {
	return p[:min(topPairs, len(p))]
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

	b.WriteString("## lead signals\n\n")
	b.WriteString("### files that change together with no import between them\n\n")
	fmt.Fprintf(&b, "Pairs that changed in %d or more of the same commits, at a score of %.2f or more: the share of the quieter file's changes made together with the other. Commits that changed more than %d files are not counted.\n\n",
		metrics.CouplingMinShared, metrics.CouplingMinScore, metrics.CouplingMaxFiles)
	if len(r.Coupling) == 0 {
		b.WriteString("- none\n")
	}
	for _, p := range topCoupled(r.Coupling) {
		fmt.Fprintf(&b, "- `%s` + `%s` - changed together in %d of the quieter file's %d commits (score %.2f)\n",
			p.A, p.B, p.Shared, min(p.ChangesA, p.ChangesB), p.Score)
	}
	b.WriteString("\n")

	b.WriteString("### hotspots one person holds\n\n")
	fmt.Fprintf(&b, "Of the top %d hotspots, the files whose leading author made more than half of the commits across the file's whole history.\n\n", topHotspots)
	switch {
	case r.Authors == 1:
		b.WriteString("- " + OneAuthor + "\n")
	case len(r.Held) == 0:
		b.WriteString("- none\n")
	}
	for _, h := range r.Held {
		fmt.Fprintf(&b, "- `%s` - %s made %d of its %d commits (%.0f%%)\n",
			relPath(r.Dir, h.File), h.Author, h.Commits, h.Total, 100*share(h))
	}
	b.WriteString("\n")

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
	Files         int       `json:"files"`
	Edges         int       `json:"edges"`
	Coupling      []Coupled `json:"coupling"`
	WindowAuthors int       `json:"window_authors"`
	HeldByOne     []held    `json:"held_by_one"`
	Findings      []Finding `json:"findings"`
	Hotspots      []ranked  `json:"hotspots"`
}

type held struct {
	File    string  `json:"file"`
	Author  string  `json:"author"`
	Commits int     `json:"commits"`
	Total   int     `json:"total"`
	Share   float64 `json:"share"`
}

func share(h metrics.Held) float64 {
	return float64(h.Commits) / float64(h.Total)
}

// Coupled is one pair of files that change together, as the evidence pack and
// `warpmap coupling --json` both print it.
type Coupled struct {
	A        string  `json:"a"`
	B        string  `json:"b"`
	Together int     `json:"together"`
	ChangesA int     `json:"changes_a"`
	ChangesB int     `json:"changes_b"`
	Score    float64 `json:"score"`
}

// CoupledPairs renders pairs for a json reader, always as a list.
func CoupledPairs(pairs []metrics.Pair) []Coupled {
	out := make([]Coupled, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, Coupled{A: p.A, B: p.B, Together: p.Shared, ChangesA: p.ChangesA, ChangesB: p.ChangesB, Score: p.Score})
	}
	return out
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
		Files:         r.Files,
		Edges:         r.Edges,
		WindowAuthors: r.Authors,
		Coupling:      CoupledPairs(topCoupled(r.Coupling)),
		HeldByOne:     []held{},
		Findings:      []Finding{},
		Hotspots:      []ranked{},
	}
	for _, h := range r.Held {
		p.HeldByOne = append(p.HeldByOne, held{File: relPath(r.Dir, h.File), Author: h.Author, Commits: h.Commits, Total: h.Total, Share: share(h)})
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
