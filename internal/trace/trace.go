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
// files a change to it could ripple into. maxDepth caps how many import hops out to
// walk; maxDepth <= 0 walks the whole cone.
func BlastRadius(g graph.Graph, file string, maxDepth int) []string {
	rev := reverseIndex(g)
	seen := map[string]bool{}
	frontier := []string{file}
	for depth := 0; len(frontier) > 0 && (maxDepth <= 0 || depth < maxDepth); depth++ {
		var next []string
		for _, cur := range frontier {
			for _, importer := range rev[cur] {
				if !seen[importer] {
					seen[importer] = true
					next = append(next, importer)
				}
			}
		}
		frontier = next
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	return out
}
