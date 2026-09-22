package report

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dfedoryshchev/warpmap/internal/metrics"
)

// decoded is read back from the encoded pack rather than reusing the renderer's
// own types, so a field renamed on one side fails the test instead of following
// it.
type decoded struct {
	Files    int `json:"files"`
	Edges    int `json:"edges"`
	Findings []struct {
		Severity       string `json:"severity"`
		Kind           string `json:"kind"`
		Detail         string `json:"detail"`
		Recommendation string `json:"recommendation"`
	} `json:"findings"`
	Hotspots []struct {
		File       string  `json:"file"`
		Score      float64 `json:"score"`
		Churn      int     `json:"churn"`
		Complexity int     `json:"complexity"`
	} `json:"hotspots"`
}

// the json variant is a second renderer over one report, so the thing worth
// pinning is not its shape but that it states what the markdown states. two
// renderers that can disagree about the same run is the duplication this tool
// exists to find.
func TestJSONStatesWhatTheMarkdownStates(t *testing.T) {
	dir := auditProject(t)
	r := Build(dir, sources(t, dir), metrics.Churn{"src/hub.ts": 9, "src/p.ts": 4, "src/q.ts": 2})

	raw, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var got decoded
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the evidence pack is not json: %v\n%s", err, raw)
	}
	md := r.Markdown()

	if got.Files != r.Files || got.Edges != r.Edges {
		t.Fatalf("json says %d files / %d edges, the report holds %d / %d", got.Files, got.Edges, r.Files, r.Edges)
	}
	if want := fmt.Sprintf("- files: %d\n- import edges: %d\n- findings: %d\n", got.Files, got.Edges, len(got.Findings)); !strings.Contains(md, want) {
		t.Fatalf("the markdown summary does not match the json one, want\n%s\ngot\n%s", want, md)
	}

	if len(got.Findings) != len(r.Findings) {
		t.Fatalf("json carries %d findings, the report holds %d", len(got.Findings), len(r.Findings))
	}
	if len(got.Findings) < 3 {
		t.Fatalf("the fixture stopped producing findings to compare: %+v", got.Findings)
	}
	kinds := map[string]bool{}
	for i, f := range got.Findings {
		kinds[f.Kind] = true
		line := fmt.Sprintf("- **[%s] %s** - %s\n  - %s\n", f.Severity, f.Kind, f.Detail, f.Recommendation)
		if !strings.Contains(md, line) {
			t.Fatalf("finding %d is not in the markdown:\n%s", i, line)
		}
		if f.Severity != string(r.Findings[i].Severity) || f.Kind != r.Findings[i].Kind {
			t.Fatalf("finding %d reordered: json has [%s] %s, the report has [%s] %s", i, f.Severity, f.Kind, r.Findings[i].Severity, r.Findings[i].Kind)
		}
	}
	for _, k := range []string{"god-module", "cycle", "dead-code"} {
		if !kinds[k] {
			t.Fatalf("the fixture no longer produces a %s finding: %+v", k, got.Findings)
		}
	}

	rows := strings.Count(md, "\n| 0.") + strings.Count(md, "\n| 1.")
	if rows != len(got.Hotspots) {
		t.Fatalf("the markdown table has %d rows, json carries %d hotspots", rows, len(got.Hotspots))
	}
	for i, h := range got.Hotspots {
		row := fmt.Sprintf("| %.3f | %d | %d | %s |\n", h.Score, h.Churn, h.Complexity, h.File)
		if !strings.Contains(md, row) {
			t.Fatalf("hotspot %d is not in the markdown table:\n%s", i, row)
		}
	}
	if got.Hotspots[0].Churn != 9 {
		t.Fatalf("the ranking did not survive the encoding: top row is %+v", got.Hotspots[0])
	}
}

// the pack is handed to whoever bought the audit, so it must name files the way
// the markdown does and not the way the analysing machine's disk does.
func TestJSONNamesFilesRelativeToTheProject(t *testing.T) {
	dir := auditProject(t)
	r := Build(dir, sources(t, dir), metrics.Churn{"src/hub.ts": 1})

	raw, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if strings.Contains(string(raw), filepath.ToSlash(dir)) {
		t.Fatalf("the pack carries the analysing machine's own path:\n%s", raw)
	}
	var got decoded
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	for _, h := range got.Hotspots {
		if strings.Contains(h.File, "\\") || strings.HasPrefix(h.File, "/") || !strings.HasPrefix(h.File, "src/") {
			t.Fatalf("hotspot file %q is not a slashed project-relative path", h.File)
		}
	}
}

// a project with nothing to say still answers in the same shape: a consumer
// iterating the pack must not have to special-case null.
func TestJSONKeepsEmptyListsAsLists(t *testing.T) {
	r := Build(t.TempDir(), nil, metrics.Churn{})
	raw, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	for _, want := range []string{"\"findings\": []", "\"hotspots\": []"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("want %s in\n%s", want, raw)
		}
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("the pack does not end in a newline:\n%q", raw)
	}
}

// auditProject writes a project that produces one of every finding: a hub 20
// files import (god-module), a two-file import cycle, and the importers
// themselves, which nothing imports (dead-code).
func auditProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("hub.ts", "export const hub = 1;\nif (hub) { console.log(hub); }\n")
	for i := 0; i < 20; i++ {
		write(fmt.Sprintf("leaf%02d.ts", i), "import { hub } from \"./hub\";\nexport const leaf = hub;\n")
	}
	write("p.ts", "import { q } from \"./q\";\nexport const p = q;\n")
	write("q.ts", "import { p } from \"./p\";\nexport const q = p;\n")
	return dir
}

func sources(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(p) == ".ts" {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
