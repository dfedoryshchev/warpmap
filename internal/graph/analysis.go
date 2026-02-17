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

// Cycles returns import cycles in the graph, each as the files on the cycle.
// circular imports are a real maintenance smell: neither file can be understood or
// changed without the other.
func Cycles(g Graph) [][]string {
	adj := map[string][]string{}
	for _, e := range g.Edges {
		if e.From == e.To {
			continue // a file that re-exports itself is not a real cycle
		}
		adj[e.From] = append(adj[e.From], e.To)
	}
	const (
		unvisited = 0
		onStack   = 1
		done      = 2
	)
	state := map[string]int{}
	var stack []string
	var cycles [][]string
	var dfs func(node string)
	dfs = func(node string) {
		state[node] = onStack
		stack = append(stack, node)
		for _, next := range adj[node] {
			switch state[next] {
			case unvisited:
				dfs(next)
			case onStack:
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] == next {
						cycle := append([]string{}, stack[i:]...)
						cycles = append(cycles, cycle)
						break
					}
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = done
	}
	for _, f := range g.Files {
		if state[f] == unvisited {
			dfs(f)
		}
	}
	return cycles
}

type Module struct {
	File   string
	FanIn  int
	FanOut int
}

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
