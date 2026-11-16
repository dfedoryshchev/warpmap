package trace

import (
	"fmt"
	"path/filepath"
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

func TestSplitSeparatesShippedDependentsFromTests(t *testing.T) {
	dir := filepath.Join("proj")
	in := []string{
		filepath.Join(dir, "src", "a.test.ts"),
		filepath.Join(dir, "src", "a.ts"),
		filepath.Join(dir, "src", "__tests__", "b.ts"),
		filepath.Join(dir, "src", "b.ts"),
		filepath.Join(dir, "tests", "setup.ts"),
	}
	shipped, tests := Split(dir, in)
	if want := []string{in[1], in[3]}; !slices.Equal(shipped, want) {
		t.Errorf("shipped = %v, want %v", shipped, want)
	}
	if want := []string{in[0], in[2], in[4]}; !slices.Equal(tests, want) {
		t.Errorf("tests = %v, want %v", tests, want)
	}
}
