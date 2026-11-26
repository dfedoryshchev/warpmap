// Package risk joins the import graph with test coverage, executed lines from a
// coverage report when there is one and the import edge otherwise: which
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

// TypesOnlyTag names a file that tests import only for its types.
const TypesOnlyTag = "types only, checked by the compiler"

// Gap is a file no test runs: how many files depend on it, how many shipped
// files import it directly, and whether tests name it for its types.
type Gap struct {
	File      string
	Blast     int
	Direct    int
	TypesOnly bool
}

// TestGaps returns the files no test runs, widest blast radius first. On a
// barrel-heavy project most files share a near-identical blast radius, so ties
// go to the file more shipped files import directly. Files are the walk's own
// paths, as in g. ran is what coverage.Split takes: nil reads the import edge.
func TestGaps(dir string, g graph.Graph, ran map[string]bool) []Gap {
	direct := map[string]int{}
	for _, e := range g.Edges {
		if !coverage.IsTest(relTo(dir, e.From)) {
			direct[e.To]++
		}
	}
	var gaps []Gap
	add := func(files []string, typesOnly bool) {
		for _, f := range files {
			gaps = append(gaps, Gap{f, len(trace.BlastRadius(g, f, 0)), direct[f], typesOnly})
		}
	}
	untested, typesOnly := coverage.Split(dir, g, ran)
	add(untested, false)
	add(typesOnly, true)
	sort.Slice(gaps, func(i, j int) bool {
		a, b := gaps[i], gaps[j]
		if a.Blast != b.Blast {
			return a.Blast > b.Blast
		}
		if a.Direct != b.Direct {
			return a.Direct > b.Direct
		}
		return a.File < b.File
	})
	return gaps
}

func relTo(dir, f string) string {
	r, err := filepath.Rel(dir, f)
	if err != nil {
		return f
	}
	return filepath.ToSlash(r)
}

// Change is one file of a planned change, named as the caller named it. Shipped
// is the part of Blast that is not tests.
type Change struct {
	File      string
	Blast     int
	Shipped   int
	Untested  bool
	TypesOnly bool
}

// Assessment is the verdict on a planned change.
type Assessment struct {
	Files           []Change
	Combined        int
	CombinedShipped int
	// Risky counts the changed files that are untested and have more than the
	// threshold's worth of dependents.
	Risky int
}

// Assess weighs changing the given files, each relative to dir, against a
// blast threshold, with ran read as TestGaps reads it.
func Assess(dir string, g graph.Graph, ran map[string]bool, changed []string, blastLimit int) Assessment {
	untested, typesOnly := map[string]bool{}, map[string]bool{}
	u, t := coverage.Split(dir, g, ran)
	for _, f := range u {
		untested[f] = true
	}
	for _, f := range t {
		typesOnly[f] = true
	}
	var a Assessment
	blast := map[string]bool{}
	for _, cf := range changed {
		abs := filepath.Join(dir, cf)
		br := trace.BlastRadius(g, abs, 0)
		for _, b := range br {
			blast[b] = true
		}
		shipped, _ := trace.Split(dir, br)
		a.Files = append(a.Files, Change{cf, len(br), len(shipped), untested[abs], typesOnly[abs]})
		if len(br) > blastLimit && untested[abs] {
			a.Risky++
		}
	}
	all := make([]string, 0, len(blast))
	for b := range blast {
		all = append(all, b)
	}
	shipped, _ := trace.Split(dir, all)
	a.Combined, a.CombinedShipped = len(all), len(shipped)
	return a
}

// Write prints the assessment as `warpmap risk` does: one line per file, the
// combined blast radius, then the verdict.
func (a Assessment) Write(w io.Writer) {
	for _, c := range a.Files {
		tag := "tested"
		switch {
		case c.Untested:
			tag = "UNTESTED"
		case c.TypesOnly:
			tag = TypesOnlyTag
		}
		fmt.Fprintf(w, "  %s: blast=%d, shipped=%d, %s\n", c.File, c.Blast, c.Shipped, tag)
	}
	fmt.Fprintf(w, "combined blast radius: %d files, %d shipped\n", a.Combined, a.CombinedShipped)
	if a.Risky > 0 {
		fmt.Fprintf(w, "verdict: %d changed file(s) are high-blast AND untested - add tests before changing\n", a.Risky)
		return
	}
	fmt.Fprintln(w, "verdict: manageable")
}
