package coverage

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/dfedoryshchev/warpmap/internal/graph"
)

func TestIsTestSeesATopLevelTestDirectory(t *testing.T) {
	for _, f := range []string{"tests/setup.ts", "__tests__/a.ts", "e2e/login.ts", "src/tests/a.ts"} {
		if !IsTest(filepath.FromSlash(f)) {
			t.Errorf("IsTest(%q) = false, want true", f)
		}
	}
	for _, f := range []string{"src/a.ts", "contests/a.ts", "src/latests.ts"} {
		if IsTest(filepath.FromSlash(f)) {
			t.Errorf("IsTest(%q) = true, want false", f)
		}
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
