package metrics

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dfedoryshchev/warpmap/internal/graph"
)

func abs(dir string, rel ...string) []string {
	out := make([]string, len(rel))
	for i, r := range rel {
		out[i] = filepath.Join(dir, filepath.FromSlash(r))
	}
	return out
}

func TestCouplingFindsFilesThatChangeTogether(t *testing.T) {
	commits := []Commit{
		{Hash: "1", Files: []string{"src/a.ts", "src/b.ts"}},
		{Hash: "2", Files: []string{"src/a.ts", "src/b.ts"}},
		{Hash: "3", Files: []string{"src/b.ts", "src/a.ts", "README.md"}},
	}
	got := Coupling(commits, func(string) bool { return true }, nil)
	// README.md changed with them once, under the shared-commit floor.
	want := []Pair{{A: "src/a.ts", B: "src/b.ts", Shared: 3, ChangesA: 3, ChangesB: 3, Score: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCouplingSkipsACommitThatTouchesTooManyFiles(t *testing.T) {
	big := []string{"src/a.ts", "src/b.ts"}
	for i := 0; i < CouplingMaxFiles-1; i++ {
		big = append(big, fmt.Sprintf("src/other%02d.ts", i))
	}
	commits := []Commit{
		{Hash: "1", Files: []string{"src/a.ts", "src/b.ts"}},
		{Hash: "2", Files: []string{"src/a.ts", "src/b.ts"}},
		{Hash: "3", Files: big},
	}
	if got := Coupling(commits, func(string) bool { return true }, nil); len(got) != 0 {
		t.Fatalf("a %d-file commit counted toward a pair: %+v", len(big), got)
	}
	commits[2].Files = big[:CouplingMaxFiles]
	if got := Coupling(commits, func(string) bool { return true }, nil); len(got) != 1 || got[0].Shared != 3 {
		t.Fatalf("a %d-file commit is inside the cutoff and should count: %+v", CouplingMaxFiles, got)
	}
}

func TestCouplingDropsAWeakPairAndAnImportedOne(t *testing.T) {
	var commits []Commit
	add := func(n int, files ...string) {
		for i := 0; i < n; i++ {
			commits = append(commits, Commit{Hash: fmt.Sprintf("%d", len(commits)), Files: files})
		}
	}
	// a and b: 3 shared of 10 changes each, exactly the 0.3 floor, kept.
	add(3, "a.ts", "b.ts")
	add(7, "a.ts")
	add(7, "b.ts")
	// c and d: 3 shared of 11 each, under the floor.
	add(3, "c.ts", "d.ts")
	add(8, "c.ts")
	add(8, "d.ts")
	// e and f: always together, but e imports f.
	add(3, "e.ts", "f.ts")
	// g and h: always together, but g is not a file the caller kept.
	add(3, "g.ts", "h.ts")
	linked := func(x, y string) bool { return x == "e.ts" && y == "f.ts" }
	keep := func(f string) bool { return f != "g.ts" }
	got := Coupling(commits, keep, linked)
	want := []Pair{{A: "a.ts", B: "b.ts", Shared: 3, ChangesA: 10, ChangesB: 10, Score: 0.3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCouplingOrdersByScoreThenSharedThenPath(t *testing.T) {
	var commits []Commit
	add := func(n int, files ...string) {
		for i := 0; i < n; i++ {
			commits = append(commits, Commit{Hash: fmt.Sprintf("%d", len(commits)), Files: files})
		}
	}
	add(3, "y.ts", "z.ts")
	add(3, "b.ts", "a.ts")
	add(4, "m.ts", "n.ts")
	add(2, "p.ts")
	add(2, "q.ts")
	add(3, "p.ts", "q.ts")
	got := Coupling(commits, func(string) bool { return true }, nil)
	var names []string
	for _, p := range got {
		names = append(names, p.A+"+"+p.B)
	}
	want := []string{"m.ts+n.ts", "a.ts+b.ts", "y.ts+z.ts", "p.ts+q.ts"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("got %v, want %v", names, want)
	}
}

// the pairs come from the repo's own history, joined with what the graph sees:
// a pair an import already explains is not hidden, and a test changes with its
// subject by design.
func TestHiddenCouplingOnARealHistory(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	for i := 0; i < 3; i++ {
		commitFiles(t, repo, fmt.Sprintf("a and b %d", i), map[string]string{
			"src/a.ts":      fmt.Sprintf("export const a = %d;\n", i),
			"src/b.ts":      fmt.Sprintf("export const b = %d;\n", i),
			"src/a.test.ts": fmt.Sprintf("import { a } from \"./a\";\n// %d\n", i),
		})
		commitFiles(t, repo, fmt.Sprintf("c and d %d", i), map[string]string{
			"src/c.ts": fmt.Sprintf("import { d } from \"./d\";\nexport const c = d + %d;\n", i),
			"src/d.ts": fmt.Sprintf("export const d = %d;\n", i),
		})
	}
	files := abs(repo, "src/a.ts", "src/b.ts", "src/c.ts", "src/d.ts", "src/a.test.ts")
	commits, err := GitCommits(repo, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 6 {
		t.Fatalf("want 3 commits, got %+v", commits)
	}
	got := HiddenCoupling(repo, files, graph.Build(files), commits)
	if len(got) != 1 || got[0].A != "src/a.ts" || got[0].B != "src/b.ts" || got[0].Score != 1 || got[0].Shared != 3 {
		t.Fatalf("want only src/a.ts + src/b.ts at 1.0 over 3 commits, got %+v", got)
	}
}
