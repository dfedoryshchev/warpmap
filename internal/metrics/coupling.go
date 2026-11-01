package metrics

import (
	"path/filepath"
	"sort"

	"github.com/dfedoryshchev/warpmap/internal/coverage"
	"github.com/dfedoryshchev/warpmap/internal/graph"
)

// The thresholds are codebase-memory-mcp's (MIT), src/pipeline/pass_githistory.c:
// GH_MAX_FILES, GH_MIN_COMMITS and MIN_COUPLING_SCORE.
const (
	// CouplingMaxFiles is the most files a commit may change and still count:
	// a bulk rename or a formatting pass changes everything together and says
	// nothing about which files depend on each other.
	CouplingMaxFiles = 20
	// CouplingMinShared is the fewest commits a pair must share.
	CouplingMinShared = 3
	// CouplingMinScore is the lowest score a pair is kept at.
	CouplingMinScore = 0.3
)

// Pair is two files that changed in the same commits. A sorts before B.
// Score is Shared over the smaller of the two files' change counts: how often
// the quieter file changed with the other one.
type Pair struct {
	A, B     string
	Shared   int
	ChangesA int
	ChangesB int
	Score    float64
}

// LargeCommits counts the commits Coupling skips for changing too many files.
func LargeCommits(commits []Commit) int {
	n := 0
	for _, c := range commits {
		if len(distinct(c.Files)) > CouplingMaxFiles {
			n++
		}
	}
	return n
}

// Coupling finds the pairs of files keep accepts that changed together in at
// least CouplingMinShared commits at a score of CouplingMinScore or more,
// leaving out a pair linked reports as already explained. A commit that changed
// more than CouplingMaxFiles files counts for nothing, its files' change counts
// included. Pairs come highest score first, then most shared commits, then by
// path.
func Coupling(commits []Commit, keep func(string) bool, linked func(a, b string) bool) []Pair {
	changes := map[string]int{}
	shared := map[[2]string]int{}
	for _, c := range commits {
		files := distinct(c.Files)
		if len(files) > CouplingMaxFiles {
			continue
		}
		var kept []string
		for _, f := range files {
			if keep(f) {
				kept = append(kept, f)
			}
		}
		sort.Strings(kept)
		for i, a := range kept {
			changes[a]++
			for _, b := range kept[i+1:] {
				shared[[2]string{a, b}]++
			}
		}
	}
	var pairs []Pair
	for k, n := range shared {
		if n < CouplingMinShared {
			continue
		}
		p := Pair{A: k[0], B: k[1], Shared: n, ChangesA: changes[k[0]], ChangesB: changes[k[1]]}
		p.Score = float64(n) / float64(min(p.ChangesA, p.ChangesB))
		if p.Score < CouplingMinScore {
			continue
		}
		if linked != nil && linked(p.A, p.B) {
			continue
		}
		pairs = append(pairs, p)
	}
	sort.Slice(pairs, func(i, j int) bool {
		x, y := pairs[i], pairs[j]
		switch {
		case x.Score != y.Score:
			return x.Score > y.Score
		case x.Shared != y.Shared:
			return x.Shared > y.Shared
		case x.A != y.A:
			return x.A < y.A
		}
		return x.B < y.B
	})
	return pairs
}

// HiddenCoupling is Coupling over a project: the shipped files among files,
// leaving out every pair g joins with an import either way. files and g are
// keyed by the walk's paths under dir; the pairs come back keyed as commits are.
func HiddenCoupling(dir string, files []string, g graph.Graph, commits []Commit) []Pair {
	key := func(f string) string {
		r, err := filepath.Rel(dir, f)
		if err != nil {
			return filepath.ToSlash(f)
		}
		return filepath.ToSlash(r)
	}
	shipped := map[string]bool{}
	for _, f := range files {
		if k := key(f); !coverage.IsTest(k) {
			shipped[k] = true
		}
	}
	edges := map[[2]string]bool{}
	for _, e := range g.Edges {
		edges[[2]string{key(e.From), key(e.To)}] = true
	}
	return Coupling(commits,
		func(f string) bool { return shipped[f] },
		func(a, b string) bool { return edges[[2]string{a, b}] || edges[[2]string{b, a}] })
}

func distinct(files []string) []string {
	seen := make(map[string]bool, len(files))
	out := make([]string, 0, len(files))
	for _, f := range files {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}
