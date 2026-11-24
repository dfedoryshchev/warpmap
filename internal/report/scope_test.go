package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dfedoryshchev/warpmap/internal/metrics"
)

var idRe = regexp.MustCompile(`^wm-(god-module|cycle|dead-code)-[0-9a-f]{8}$`)

func ids(r Report) []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, f.ID)
	}
	return out
}

// an id is what a later run and a remediation ticket refer back to, so it has
// to come out the same on another machine, in another checkout, and after the
// project gains a file the finding does not involve.
func TestFindingIDsAreStableAcrossRunsAndMachines(t *testing.T) {
	one := auditProject(t)
	two := auditProject(t)
	a := Build(one, sources(t, one), metrics.Churn{}, nil)
	b := Build(two, sources(t, two), metrics.Churn{}, nil)

	got := ids(a)
	if len(got) < 3 {
		t.Fatalf("the fixture stopped producing findings: %+v", a.Findings)
	}
	seen := map[string]bool{}
	for i, id := range got {
		if !idRe.MatchString(id) {
			t.Fatalf("finding %d (%s) has id %q, want wm-<kind>-<8 hex>", i, a.Findings[i].Kind, id)
		}
		if !strings.HasPrefix(id, "wm-"+a.Findings[i].Kind+"-") {
			t.Fatalf("finding %d is a %s and its id is %q", i, a.Findings[i].Kind, id)
		}
		if seen[id] {
			t.Fatalf("two findings share the id %s: %v", id, got)
		}
		seen[id] = true
	}
	if !slices.Equal(got, ids(b)) {
		t.Fatalf("the same project in two places got different ids:\n%v\n%v", got, ids(b))
	}

	if err := os.WriteFile(filepath.Join(one, "src", "leaf20.ts"), []byte("import { hub } from \"./hub\";\nexport const leaf = hub;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	grown := Build(one, sources(t, one), metrics.Churn{}, nil)
	if !slices.Equal(got, ids(grown)) {
		t.Fatalf("one more dead file moved the ids:\n%v\n%v", got, ids(grown))
	}

	md := a.Markdown()
	raw, err := a.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var pack struct {
		Findings []struct {
			ID string `json:"id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(raw, &pack); err != nil {
		t.Fatal(err)
	}
	for i, id := range got {
		if !strings.Contains(md, "`"+id+"`") {
			t.Fatalf("the markdown does not carry the id %s, in\n%s", id, md)
		}
		if pack.Findings[i].ID != id {
			t.Fatalf("json finding %d has id %q, the report %q", i, pack.Findings[i].ID, id)
		}
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// digest is the recipe the readme gives a client: sha256sum each file, sort by
// path, sha256sum that list.
func digest(t *testing.T, dir string, files []string) string {
	t.Helper()
	var lines []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		r, _ := filepath.Rel(dir, f)
		lines = append(lines, hex.EncodeToString(sum[:])+"  "+filepath.ToSlash(r)+"\n")
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i][66:] < lines[j][66:] })
	sum := sha256.Sum256([]byte(strings.Join(lines, "")))
	return hex.EncodeToString(sum[:])
}

// a report nobody can tie to a tree is a claim about some code, somewhere: the
// scope names the commit it read, says when the tree had moved past it, and
// gives a digest the client can recompute without warpmap.
func TestScopePinsTheCommitAndTheInputs(t *testing.T) {
	dir := auditProject(t)
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "init")
	head := git(t, dir, "rev-parse", "HEAD")

	files := sources(t, dir)
	r := Build(dir, files, metrics.Churn{}, nil)
	if r.Commit != head || r.Dirty {
		t.Fatalf("scope says commit %q dirty=%v, want %s clean", r.Commit, r.Dirty, head)
	}
	if want := digest(t, dir, files); r.Digest != want {
		t.Fatalf("input digest %s, the sha256sum recipe gives %s", r.Digest, want)
	}
	md := r.Markdown()
	if !strings.Contains(md, "- commit: `"+head+"`\n") {
		t.Fatalf("the scope does not pin the commit:\n%s", md)
	}
	if !strings.Contains(md, fmt.Sprintf("- inputs: %d files, sha256 `%s`\n", len(files), r.Digest)) {
		t.Fatalf("the scope does not carry the input digest:\n%s", md)
	}

	if err := os.WriteFile(filepath.Join(dir, "src", "hub.ts"), []byte("export const hub = 2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved := Build(dir, files, metrics.Churn{}, nil)
	if !moved.Dirty || moved.Digest == r.Digest {
		t.Fatalf("an edited tree reads dirty=%v with digest %s (was %s)", moved.Dirty, moved.Digest, r.Digest)
	}
	if want := "- commit: `" + head + "` with uncommitted changes\n"; !strings.Contains(moved.Markdown(), want) {
		t.Fatalf("want %q in\n%s", want, moved.Markdown())
	}

	raw, err := moved.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var pack struct {
		Scope struct {
			Commit      string `json:"commit"`
			Uncommitted bool   `json:"uncommitted_changes"`
			Inputs      struct {
				Files  int    `json:"files"`
				SHA256 string `json:"sha256"`
				Each   []struct {
					File   string `json:"file"`
					SHA256 string `json:"sha256"`
				} `json:"each"`
			} `json:"inputs"`
		} `json:"scope"`
	}
	if err := json.Unmarshal(raw, &pack); err != nil {
		t.Fatal(err)
	}
	s := pack.Scope
	if s.Commit != head || !s.Uncommitted || s.Inputs.Files != len(files) || s.Inputs.SHA256 != moved.Digest || len(s.Inputs.Each) != len(files) {
		t.Fatalf("the pack's scope disagrees with the report: %+v", s)
	}
	if s.Inputs.Each[0].File != "src/hub.ts" {
		t.Fatalf("inputs are not listed by project-relative path in order: first is %q", s.Inputs.Each[0].File)
	}
}

func TestAProjectOutsideGitSaysNoCommitPinsIt(t *testing.T) {
	dir := auditProject(t)
	r := Build(dir, sources(t, dir), metrics.Churn{}, nil)
	if r.Commit != "" {
		t.Fatalf("a temp dir outside git reads commit %q", r.Commit)
	}
	if want := "- commit: none - not a git checkout, so only the input digest pins this run\n"; !strings.Contains(r.Markdown(), want) {
		t.Fatalf("want %q in\n%s", want, r.Markdown())
	}
}

// limitations are counts the run produced, not a paragraph: what was parsed,
// what was found and not parsed, how much of what the files import the graph
// could place, and what the churn column rests on.
func TestLimitationsCarryTheRunsOwnNumbers(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"a.ts": "import { b } from \"./b\";\nimport type { C } from \"./c\";\nimport { gone } from \"./gone\";\nimport React from \"react\";\n",
		"b.ts": "export const b = 1;\n",
		"c.ts": "export type C = 1;\n",
	} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := Build(dir, sources(t, dir), metrics.Churn{}, nil)
	r.Version = "9.9.9"
	r.Window = metrics.Window{Months: 6, Commits: 12}
	r.Unparsed = []string{filepath.Join(dir, "cmd", "main.go")}

	md := r.Markdown()
	for _, want := range []string{
		"- warpmap: 9.9.9\n",
		"- churn: 12 commits in the last 6 months\n",
		"- network: no call made - `explain`, the one command that can make one, did not run\n",
		"- coverage: partial\n",
		"## limitations\n",
		"- parsed: 3 files (.ts 3)\n",
		"- not parsed: 1 file (.go 1) - no imports, findings or hotspots for these\n",
		"- import specifiers: 2 resolved to a file in the project, 2 did not; 1 of those is relative to the importing file\n",
		"- type-only edges: 1 of 2 (50%)\n",
		"- nothing was built, installed or run: imports are read from the source text, and no test suite ran\n",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("want %q in\n%s", want, md)
		}
	}
	if strings.Index(md, "## limitations") > strings.Index(md, "## lead signals") {
		t.Fatalf("limitations come after the findings they qualify:\n%s", md)
	}

	raw, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var pack struct {
		Limitations struct {
			Parsed     map[string]int `json:"parsed"`
			NotParsed  map[string]int `json:"not_parsed"`
			Specifiers struct {
				Resolved        int `json:"resolved"`
				Dropped         int `json:"dropped"`
				DroppedRelative int `json:"dropped_relative"`
			} `json:"specifiers"`
			Edges         int `json:"edges"`
			TypeOnlyEdges int `json:"type_only_edges"`
		} `json:"limitations"`
	}
	if err := json.Unmarshal(raw, &pack); err != nil {
		t.Fatal(err)
	}
	l := pack.Limitations
	if l.Parsed[".ts"] != 3 || l.NotParsed[".go"] != 1 || l.Specifiers.Resolved != 2 || l.Specifiers.Dropped != 2 ||
		l.Specifiers.DroppedRelative != 1 || l.Edges != 2 || l.TypeOnlyEdges != 1 {
		t.Fatalf("the pack's limitations disagree with the markdown: %+v", l)
	}
}

// coverage is one word a reader can filter on, with the checks that could not
// run named by the same ids the findings use.
func TestCoverageSaysWhatTheRunCouldNotCheck(t *testing.T) {
	dir := auditProject(t)
	files := sources(t, dir)
	all := []string{"coupling", "cycle", "dead-code", "god-module", "held-by-one", "hotspots"}

	cases := []struct {
		name        string
		files       []string
		window      metrics.Window
		unparsed    []string
		status      string
		unsupported []string
	}{
		{"everything ran", files, metrics.Window{Months: 6, Commits: 3}, nil, "complete", []string{}},
		{"other languages found", files, metrics.Window{Months: 6, Commits: 3}, []string{filepath.Join(dir, "x.go")}, "partial", []string{}},
		{"no git history", files, metrics.Window{Months: 6, Err: errors.New("not a git repository")}, nil, "partial", []string{"coupling", "held-by-one"}},
		{"nothing parsed", nil, metrics.Window{Months: 6, Err: errors.New("not a git repository")}, []string{filepath.Join(dir, "x.go")}, "unavailable", all},
	}
	for _, c := range cases {
		r := Build(dir, c.files, metrics.Churn{}, nil)
		r.Window = c.window
		r.Unparsed = c.unparsed
		status, unsupported := r.Coverage()
		if status != c.status || !slices.Equal(unsupported, c.unsupported) {
			t.Errorf("%s: coverage %s %v, want %s %v", c.name, status, unsupported, c.status, c.unsupported)
		}
		raw, err := r.JSON()
		if err != nil {
			t.Fatal(err)
		}
		var pack struct {
			Scope struct {
				Coverage    string   `json:"coverage"`
				Unsupported []string `json:"unsupported_rule_ids"`
			} `json:"scope"`
		}
		if err := json.Unmarshal(raw, &pack); err != nil {
			t.Fatal(err)
		}
		if pack.Scope.Coverage != c.status || pack.Scope.Unsupported == nil || !slices.Equal(pack.Scope.Unsupported, c.unsupported) {
			t.Errorf("%s: the pack says %s %v", c.name, pack.Scope.Coverage, pack.Scope.Unsupported)
		}
		md := r.Markdown()
		if !strings.Contains(md, "- coverage: "+c.status+"\n") {
			t.Errorf("%s: markdown lacks the coverage line:\n%s", c.name, md)
		}
		if len(c.unsupported) > 0 && !strings.Contains(md, "- not run: "+strings.Join(c.unsupported, ", ")+"\n") {
			t.Errorf("%s: markdown does not name what did not run:\n%s", c.name, md)
		}
	}
}
