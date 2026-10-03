package metrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, dir string, files map[string]string) []string {
	t.Helper()
	var out []string
	for name, src := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func TestHotspotsLeavesTestFilesOut(t *testing.T) {
	busy := strings.Repeat("if (x) { y(); }\n", 40)
	files := map[string]string{
		"src/form.ts":                 "export const f = (x) => { if (x) { return 1; } return 2; };\n",
		"src/form.test.ts":            busy,
		"src/form.spec.ts":            busy,
		"src/__tests__/form.ts":       busy,
		"tests/helpers.ts":            busy,
		"e2e/login.ts":                busy,
		"src/testing/fixtures.ts":     "export const fixture = 1;\n",
		"src/contest/leaderboard.tsx": "export const board = 1;\n",
	}
	churn := Churn{"src/form.ts": 3, "src/testing/fixtures.ts": 1, "src/contest/leaderboard.tsx": 1}
	for name := range files {
		if _, ok := churn[name]; !ok {
			churn[name] = 9
		}
	}

	check := func(label, dir string) {
		t.Helper()
		ranked := Hotspots(dir, writeFiles(t, dir, files), churn)
		got := map[string]Hotspot{}
		for _, h := range ranked {
			r, _ := filepath.Rel(dir, h.File)
			got[filepath.ToSlash(r)] = h
		}
		for _, name := range []string{"src/form.test.ts", "src/form.spec.ts", "src/__tests__/form.ts", "tests/helpers.ts", "e2e/login.ts"} {
			if _, ok := got[name]; ok {
				t.Errorf("%s: %s is a test file and was ranked", label, name)
			}
		}
		for _, name := range []string{"src/form.ts", "src/testing/fixtures.ts", "src/contest/leaderboard.tsx"} {
			if _, ok := got[name]; !ok {
				t.Errorf("%s: %s is not a test file and was left out", label, name)
			}
		}
		if h := got["src/form.ts"]; h.Score != 1 {
			t.Errorf("%s: src/form.ts scored %.3f, want 1.000 - the busiest, most complex shipped file sets the scale", label, h.Score)
		}
	}

	check("absolute", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	check("dot", ".")
}
