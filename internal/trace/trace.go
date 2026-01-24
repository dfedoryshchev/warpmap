package trace

import "github.com/dfedoryshchev/warpmap/internal/graph"

func reverseIndex(g graph.Graph) map[string][]string {
	rev := map[string][]string{}
	for _, e := range g.Edges {
		rev[e.To] = append(rev[e.To], e.From)
	}
	return rev
}

// BlastRadius returns everything that transitively depends on file - the set of
// files a change to it could ripple into.
func BlastRadius(g graph.Graph, file string) []string {
	rev := reverseIndex(g)
	seen := map[string]bool{}
	queue := []string{file}
	for len(queue) > 0 {
		cur := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, importer := range rev[cur] {
			if !seen[importer] {
				seen[importer] = true
				queue = append(queue, importer)
			}
		}
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	return out
}
