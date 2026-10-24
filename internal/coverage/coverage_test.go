package coverage

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/dfedoryshchev/warpmap/internal/graph"
)

func TestIsTestSeesATopLevelTestDirectory(t *testing.T) {
	for _, f := range []string{"tests/setup.ts", "__tests__/a.ts", "e2e/login.ts", "src/tests/a.ts", "pkg/walk_test.go"} {
		if !IsTest(filepath.FromSlash(f)) {
			t.Errorf("IsTest(%q) = false, want true", f)
		}
	}
	for _, f := range []string{"src/a.ts", "contests/a.ts", "src/latests.ts", "pkg/contest.go"} {
		if IsTest(filepath.FromSlash(f)) {
			t.Errorf("IsTest(%q) = true, want false", f)
		}
	}
}

func TestIsTestSeesPythonAndTypeTests(t *testing.T) {
	for _, f := range []string{
		"src/__typetest__/form.test-d.ts", "src/__typetest__/__fixtures__/type.ts", "src/a.spec-d.ts",
		"pkg/test_walk.py", "pkg/walk_test.py", "conftest.py", "pkg/conftest.py", "test/walk.py", "pkg/test/a.ts",
	} {
		if !IsTest(filepath.FromSlash(f)) {
			t.Errorf("IsTest(%q) = false, want true", f)
		}
	}
	for _, f := range []string{"pkg/test.py", "pkg/testing.py", "pkg/contest_a.py", "pkg/attest.py", "testdata/a.py", "pkg/latest/a.ts", "src/types.ts"} {
		if IsTest(filepath.FromSlash(f)) {
			t.Errorf("IsTest(%q) = true, want false", f)
		}
	}
}

func TestUntestedCountsPythonAndTypeTests(t *testing.T) {
	g := graph.Graph{
		Files: []string{"src/types.ts", "src/__typetest__/form.test-d.ts", "pkg/a.py", "pkg/b.py", "pkg/test_a.py", "pkg/b_test.py"},
		Edges: []graph.Edge{
			{From: "src/__typetest__/form.test-d.ts", To: "src/types.ts"},
			{From: "pkg/test_a.py", To: "pkg/a.py"},
			{From: "pkg/b_test.py", To: "pkg/b.py"},
		},
	}
	if got := Untested(".", g); len(got) != 0 {
		t.Errorf("untested = %v, want none", got)
	}
}

// the same two files, the project named from inside it and from outside it.
// a `tests/` directory above the project is not part of it, and a `tests/`
// directory at its top is.
func TestUntestedReadsPathsRelativeToTheProject(t *testing.T) {
	for _, dir := range []string{".", filepath.Join("work", "tests", "proj")} {
		src := filepath.Join(dir, "src", "a.ts")
		setup := filepath.Join(dir, "tests", "setup.ts")
		g := graph.Graph{Files: []string{src, setup}}
		got := Untested(dir, g)
		if want := []string{src}; !slices.Equal(got, want) {
			t.Errorf("project %q: untested = %v, want %v", dir, got, want)
		}
	}
}
