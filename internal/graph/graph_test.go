package graph

import (
	"strings"
	"testing"
)

func sample() Graph {
	// b imports a, c imports b  (an acyclic chain)
	return Graph{
		Files: []string{"a.ts", "b.ts", "c.ts"},
		Edges: []Edge{{From: "b.ts", To: "a.ts"}, {From: "c.ts", To: "b.ts"}},
	}
}

func TestOrphans(t *testing.T) {
	got := Orphans(sample())
	if len(got) != 1 || got[0] != "c.ts" {
		t.Fatalf("want [c.ts] (nothing imports it), got %v", got)
	}
}

func TestOrphansSkipsPythonAndTypeTests(t *testing.T) {
	g := Graph{Files: []string{"src/form.test-d.ts", "src/a.spec-d.ts", "pkg/test_a.py", "pkg/a_test.py", "pkg/conftest.py", "pkg/unused.py"}}
	if got := Orphans(g); len(got) != 1 || got[0] != "pkg/unused.py" {
		t.Fatalf("want [pkg/unused.py], got %v", got)
	}
}

func TestCyclesFindsCycle(t *testing.T) {
	cyclic := Graph{
		Files: []string{"x.ts", "y.ts"},
		Edges: []Edge{{From: "x.ts", To: "y.ts"}, {From: "y.ts", To: "x.ts"}},
	}
	if len(Cycles(cyclic)) == 0 {
		t.Fatal("expected a cycle between x and y")
	}
	if got := Cycles(sample()); len(got) != 0 {
		t.Fatalf("acyclic graph should have no cycles, got %v", got)
	}
}

func edges(pairs ...string) []Edge {
	var es []Edge
	for i := 0; i+1 < len(pairs); i += 2 {
		es = append(es, Edge{From: pairs[i], To: pairs[i+1]})
	}
	return es
}

func TestCyclesReportOneTangleOnce(t *testing.T) {
	g := Graph{
		Files: []string{"a.ts", "b.ts", "c.ts"},
		Edges: edges("a.ts", "b.ts", "b.ts", "a.ts", "b.ts", "c.ts", "c.ts", "a.ts"),
	}
	got := Cycles(g)
	if len(got) != 1 {
		t.Fatalf("a, b and c all reach each other, so one tangle; got %d: %v", len(got), got)
	}
	if strings.Join(got[0].Files, ",") != "a.ts,b.ts,c.ts" || got[0].Ring {
		t.Fatalf("want the tangle's files a, b, c in path order, not a ring; got %+v", got[0])
	}
}

func TestCyclesKeepImportOrderForARing(t *testing.T) {
	g := Graph{
		Files: []string{"a.ts", "b.ts", "c.ts"},
		Edges: edges("a.ts", "c.ts", "c.ts", "b.ts", "b.ts", "a.ts", "a.ts", "c.ts"),
	}
	got := Cycles(g)
	if len(got) != 1 || !got[0].Ring || strings.Join(got[0].Files, ",") != "a.ts,c.ts,b.ts" {
		t.Fatalf("want one ring a -> c -> b, got %+v", got)
	}
}

func TestCyclesReportDisjointTanglesLargestFirst(t *testing.T) {
	g := Graph{
		Files: []string{"x.ts", "y.ts", "p.ts", "q.ts", "r.ts", "s.ts", "lone.ts"},
		Edges: edges(
			"x.ts", "y.ts", "y.ts", "x.ts",
			"p.ts", "q.ts", "q.ts", "r.ts", "r.ts", "p.ts", "q.ts", "p.ts",
			"s.ts", "s.ts",
			"lone.ts", "x.ts", "r.ts", "x.ts",
		),
	}
	got := Cycles(g)
	if len(got) != 2 {
		t.Fatalf("want two tangles (p-q-r and x-y), got %d: %v", len(got), got)
	}
	if strings.Join(got[0].Files, ",") != "p.ts,q.ts,r.ts" || strings.Join(got[1].Files, ",") != "x.ts,y.ts" {
		t.Fatalf("want [p q r] then [x y], got %v", got)
	}
}

func TestCyclesIgnoresSelfImport(t *testing.T) {
	self := Graph{Files: []string{"z.ts"}, Edges: []Edge{{From: "z.ts", To: "z.ts"}}}
	if got := Cycles(self); len(got) != 0 {
		t.Fatalf("a self-import is not a real cycle, got %v", got)
	}
}

func TestGodModules(t *testing.T) {
	if len(GodModules(sample())) == 0 {
		t.Fatal("expected fan-in/out to rank some modules")
	}
}

func TestIsGodCountsFanInAndFanOutTogetherFromTheThreshold(t *testing.T) {
	for _, c := range []struct {
		m    Module
		want bool
	}{
		{Module{FanIn: GodThreshold - 1}, false},
		{Module{FanIn: GodThreshold}, true},
		{Module{FanIn: 12, FanOut: GodThreshold - 12}, true},
		{Module{FanOut: GodThreshold - 1}, false},
	} {
		if got := c.m.IsGod(); got != c.want {
			t.Fatalf("in=%d out=%d: IsGod() = %v, want %v", c.m.FanIn, c.m.FanOut, got, c.want)
		}
	}
}
