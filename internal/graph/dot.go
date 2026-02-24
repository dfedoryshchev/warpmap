package graph

import (
	"fmt"
	"strings"
)

// ToDot renders the graph as graphviz dot. pipe through `dot -Tsvg` to render an image.
func ToDot(g Graph) string {
	var b strings.Builder
	b.WriteString("digraph deps {\n  rankdir=LR;\n")
	for _, e := range g.Edges {
		fmt.Fprintf(&b, "  %q -> %q;\n", e.From, e.To)
	}
	b.WriteString("}\n")
	return b.String()
}
