// Package risk joins the import graph with structural test coverage: which
// untested files carry the widest blast radius, and whether a planned change
// touches one. The command line and the MCP server both answer from here.
package risk

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"github.com/dfedoryshchev/warpmap/internal/coverage"
	"github.com/dfedoryshchev/warpmap/internal/graph"
	"github.com/dfedoryshchev/warpmap/internal/trace"
)

// Gap is an untested file and how many files depend on it.
type Gap struct {
	File  string
	Blast int
}

// TestGaps returns the files no test imports, widest blast radius first. Files
// are the walk's own paths, as in g.
func TestGaps(dir string, g graph.Graph) []Gap {
	var gaps []Gap
	for _, f := range coverage.Untested(dir, g) {
		gaps = append(gaps, Gap{f, len(trace.BlastRadius(g, f, 0))})
	}
	sort.SliceStable(gaps, func(i, j int) bool { return gaps[i].Blast > gaps[j].Blast })
	return gaps
}

// Change is one file of a planned change, named as the caller named it.
type Change struct {
	File     string
	Blast    int
	Untested bool
}

// Assessment is the verdict on a planned change.
type Assessment struct {
	Files    []Change
	Combined int
	// Risky counts the changed files that are untested and have more than the
	// threshold's worth of dependents.
	Risky int
}

// Assess weighs changing the given files, each relative to dir, against a
// blast threshold.
func Assess(dir string, g graph.Graph, changed []string, blastLimit int) Assessment {
	untested := map[string]bool{}
	for _, f := range coverage.Untested(dir, g) {
		untested[f] = true
	}
	var a Assessment
	blast := map[string]bool{}
	for _, cf := range changed {
		abs := filepath.Join(dir, cf)
		br := trace.BlastRadius(g, abs, 0)
		for _, b := range br {
			blast[b] = true
		}
		a.Files = append(a.Files, Change{cf, len(br), untested[abs]})
		if len(br) > blastLimit && untested[abs] {
			a.Risky++
		}
	}
	a.Combined = len(blast)
	return a
}

// Write prints the assessment as `warpmap risk` does: one line per file, the
// combined blast radius, then the verdict.
func (a Assessment) Write(w io.Writer) {
	for _, c := range a.Files {
		tag := "tested"
		if c.Untested {
			tag = "UNTESTED"
		}
		fmt.Fprintf(w, "  %s: blast=%d, %s\n", c.File, c.Blast, tag)
	}
	fmt.Fprintf(w, "combined blast radius: %d files\n", a.Combined)
	if a.Risky > 0 {
		fmt.Fprintf(w, "verdict: %d changed file(s) are high-blast AND untested - add tests before changing\n", a.Risky)
		return
	}
	fmt.Fprintln(w, "verdict: manageable")
}
