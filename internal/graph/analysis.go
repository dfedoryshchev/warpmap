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
	case IsTestName(base):
		return true
	}
	return false
}

// IsTestName reports whether a file name, without its directory, follows a test
// naming convention: `.test.` / `.spec.` and their type-test forms (`.test-d.ts`),
// Go's `_test.go`, and pytest's `test_*.py`, `*_test.py` and `conftest.py`.
func IsTestName(name string) bool {
	base := strings.ToLower(name)
	for _, mark := range []string{".test.", ".spec.", ".test-d.", ".spec-d."} {
		if strings.Contains(base, mark) {
			return true
		}
	}
	return strings.HasSuffix(base, "_test.go") ||
		(strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py")) ||
		strings.HasSuffix(base, "_test.py") || base == "conftest.py"
}

// Cycle is one import tangle: a set of files that all reach each other through
// imports (a strongly connected component of two or more files).
type Cycle struct {
	// Files is in import order when Ring is true, each file importing the next and
	// the last importing the first; otherwise it is in path order.
	Files []string
	// Ring reports whether the tangle is a single loop, every file importing exactly
	// one other file of the tangle.
	Ring bool
}

// Cycles returns the import tangles in the graph, one per strongly connected
// component, largest first. circular imports are a maintenance smell: no file in
// a tangle can be understood or changed without the others.
func Cycles(g Graph) []Cycle {
	adj := map[string][]string{}
	for _, e := range g.Edges {
		if e.From == e.To {
			continue // a file that re-exports itself is not a real cycle
		}
		adj[e.From] = append(adj[e.From], e.To)
	}
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var comps [][]string
	var visit func(node string)
	visit = func(node string) {
		index[node] = len(index)
		low[node] = index[node]
		stack = append(stack, node)
		onStack[node] = true
		for _, next := range adj[node] {
			if _, seen := index[next]; !seen {
				visit(next)
				low[node] = min(low[node], low[next])
			} else if onStack[next] {
				low[node] = min(low[node], index[next])
			}
		}
		if low[node] != index[node] {
			return
		}
		var comp []string
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[top] = false
			comp = append(comp, top)
			if top == node {
				break
			}
		}
		if len(comp) > 1 {
			comps = append(comps, comp)
		}
	}
	for _, f := range g.Files {
		if _, seen := index[f]; !seen {
			visit(f)
		}
	}
	var cycles []Cycle
	for _, comp := range comps {
		cycles = append(cycles, tangle(comp, adj))
	}
	sort.Slice(cycles, func(i, j int) bool {
		if len(cycles[i].Files) != len(cycles[j].Files) {
			return len(cycles[i].Files) > len(cycles[j].Files)
		}
		return cycles[i].Files[0] < cycles[j].Files[0]
	})
	return cycles
}

func tangle(comp []string, adj map[string][]string) Cycle {
	sort.Strings(comp)
	in := map[string]bool{}
	for _, f := range comp {
		in[f] = true
	}
	next := map[string]string{}
	for _, f := range comp {
		for _, t := range adj[f] {
			if !in[t] || t == next[f] {
				continue
			}
			if next[f] != "" {
				return Cycle{Files: comp}
			}
			next[f] = t
		}
	}
	ring := make([]string, 0, len(comp))
	for f := comp[0]; len(ring) < len(comp); f = next[f] {
		ring = append(ring, f)
	}
	return Cycle{Files: ring, Ring: true}
}

type Module struct {
	File   string
	FanIn  int
	FanOut int
}

// GodThreshold is the fan-in + fan-out at which a file counts as a god module.
const GodThreshold = 20

// IsGod reports whether m is central enough to count as a god module.
func (m Module) IsGod() bool { return m.FanIn+m.FanOut >= GodThreshold }

// GodModules ranks files by fan-in + fan-out: too many things depend on it, or it
// depends on too much. high scores are architectural risk - a change there is hard
// to reason about because it touches (or is touched by) half the codebase.
func GodModules(g Graph) []Module {
	in := map[string]int{}
	out := map[string]int{}
	for _, e := range g.Edges {
		out[e.From]++
		in[e.To]++
	}
	var mods []Module
	for _, f := range g.Files {
		if in[f]+out[f] == 0 {
			continue
		}
		mods = append(mods, Module{File: f, FanIn: in[f], FanOut: out[f]})
	}
	sort.Slice(mods, func(i, j int) bool {
		return mods[i].FanIn+mods[i].FanOut > mods[j].FanIn+mods[j].FanOut
	})
	return mods
}
