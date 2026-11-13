package trace

import (
	"fmt"
	"slices"
	"testing"

	"github.com/dfedoryshchev/warpmap/internal/graph"
)

// a quoted `trace` block has to come out byte-identical on a re-run, so the
// dependents are listed in path order however the walk reached them.
func TestBlastRadiusListsDependentsInPathOrder(t *testing.T) {
	g := graph.Graph{Files: []string{"hub.ts", "mid.ts"}}
	for i := 30; i > 0; i-- {
		f := fmt.Sprintf("leaf%02d.ts", i)
		g.Files = append(g.Files, f)
		to := "hub.ts"
		if i%2 == 0 {
			to = "mid.ts"
		}
		g.Edges = append(g.Edges, graph.Edge{From: f, To: to})
	}
	g.Edges = append(g.Edges, graph.Edge{From: "mid.ts", To: "hub.ts"})

	for run := 0; run < 20; run++ {
		got := BlastRadius(g, "hub.ts", 0)
		if len(got) != 31 || !slices.IsSorted(got) {
			t.Fatalf("run %d: want the 31 dependents in path order, got %v", run, got)
		}
	}
}
