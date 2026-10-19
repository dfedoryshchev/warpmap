package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const seed = "WARPMAP-SEED-7f3a91"

// seededProject writes a small project whose every source line carries the seed,
// in a comment, a string literal and a default argument, so any copy of a
// source line into the folder shows up as the seed.
func seededProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"src/a.ts":    "// " + seed + "\nimport { b } from \"./b\";\nexport const token = \"" + seed + "\";\nexport function a(k = \"" + seed + "\") { if (b) { return k; } return b; }\n",
		"src/b.ts":    "export const b = \"" + seed + "\";\n",
		"app/main.py": "# " + seed + "\ndef run(key=\"" + seed + "\"):\n    if key:\n        return key\n",
	}
	for name, src := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// fakeTool answers as gitleaks, lizard or jscpd, by the name it was started
// under, with the output shapes those tools write - each carrying the seed in
// every field that holds source text, an author or a commit message.
func fakeTool(args []string, seed string) int {
	name := strings.TrimSuffix(filepath.Base(args[0]), ".exe")
	rest := args[1:]
	if slices.Contains(rest, "--version") || (len(rest) == 1 && rest[0] == "version") {
		fmt.Println("9.9.9")
		return 0
	}
	after := func(flag string) string {
		i := slices.Index(rest, flag)
		if i < 0 || i+1 >= len(rest) {
			return ""
		}
		return rest[i+1]
	}
	switch name {
	case "gitleaks":
		report := after("--report-path")
		if report == "" || !slices.Contains(rest, "--redact") {
			fmt.Fprintln(os.Stderr, "fake gitleaks: want --redact and --report-path, got", rest)
			return 2
		}
		finding := []map[string]any{{
			"RuleID": "generic-api-key", "Description": "Generic API Key",
			"StartLine": 3, "EndLine": 3, "StartColumn": 14, "EndColumn": 40,
			"Line": "export const token = \"" + seed + "\";", "Match": "token = \"" + seed + "\"",
			"Secret": "REDACTED", "File": "src/a.ts", "SymlinkFile": "", "Commit": "0123abc",
			"Link": "https://example.invalid/" + seed, "Entropy": 3.5,
			"Author": seed, "Email": seed + "@example.invalid", "Date": "2026-01-01T00:00:00Z",
			"Message": "add " + seed, "Tags": []string{}, "Fingerprint": "0123abc:src/a.ts:generic-api-key:3",
			"Fragment": map[string]any{"Raw": seed},
		}}
		b, _ := json.Marshal(finding)
		if err := os.WriteFile(report, b, 0o644); err != nil {
			return 2
		}
		return 0
	case "lizard":
		list := after("-f")
		b, err := os.ReadFile(list)
		if err != nil {
			return 2
		}
		for _, f := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			f = strings.TrimRight(f, "\r")
			fmt.Printf("4,2,30,1,4,\"run@1-4@%s\",\"%s\",\"run\",\"run( key = \"\"%s\"\" )\",1,4\n", f, f, seed)
		}
		return 0
	case "jscpd":
		out := after("--output")
		if out == "" || after("--reporters") != "json" {
			fmt.Fprintln(os.Stderr, "fake jscpd: want --reporters json --output <dir>, got", rest)
			return 2
		}
		row := map[string]any{"lines": 4, "tokens": 30, "sources": 2, "clones": 1, "duplicatedLines": 4, "percentage": 12.5}
		report := map[string]any{
			"statistics": map[string]any{
				"detectionDate": "2026-01-01T00:00:00Z",
				"total":         row,
				"formats": map[string]any{"typescript": map[string]any{
					"total":   row,
					"sources": map[string]any{"src/a.ts": map[string]any{"lines": 4, "note": seed}},
				}},
			},
			"duplicates": []map[string]any{{
				"format": "typescript", "lines": 4, "fragment": "export const token = \"" + seed + "\";", "tokens": 30,
				"isNew": false, "kind": "exact",
				"firstFile":  map[string]any{"name": "src/a.ts", "start": 1, "end": 4, "startLoc": map[string]any{"line": 1}, "blame": map[string]any{"author": seed}},
				"secondFile": map[string]any{"name": "src/b.ts", "start": 1, "end": 1, "blame": map[string]any{"author": seed}},
			}},
		}
		b, _ := json.Marshal(report)
		if err := os.WriteFile(filepath.Join(out, "jscpd-report.json"), b, 0o644); err != nil {
			return 2
		}
		return 0
	}
	fmt.Fprintln(os.Stderr, "fake tool: unknown name", name)
	return 2
}

// fakeTools makes findTool answer with copies of this test binary named after
// each tool in names; every other tool reads as absent.
func fakeTools(t *testing.T, names ...string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	found := map[string]string{}
	for _, n := range names {
		file := n
		if runtime.GOOS == "windows" {
			file += ".exe"
		}
		p := filepath.Join(bin, file)
		if err := os.WriteFile(p, b, 0o755); err != nil {
			t.Fatal(err)
		}
		found[n] = p
	}
	t.Setenv("WARPMAP_FAKE_SEED", seed)
	old := findTool
	findTool = func(name string) (string, error) {
		if p, ok := found[name]; ok {
			return p, nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { findTool = old })
}

func folderFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

func TestCollectWritesNoLineOfAnySourceFile(t *testing.T) {
	dir := seededProject(t)
	fakeTools(t, "gitleaks", "lizard", "jscpd")
	out := filepath.Join(t.TempDir(), "evidence")
	if code := collectCmd([]string{dir, "-o", out}); code != 0 {
		t.Fatalf("collect exited %d", code)
	}
	got := folderFiles(t, out)
	want := []string{"analyze.json", "gitleaks.json", "jscpd.json", "lizard.csv", "report.json", "tools.txt"}
	if !slices.Equal(got, want) {
		t.Fatalf("folder holds %v, want %v", got, want)
	}
	for _, name := range got {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), seed) {
			t.Errorf("%s carries text from a source file:\n%s", name, b)
		}
		if strings.Contains(string(b), filepath.ToSlash(dir)) || strings.Contains(string(b), dir) {
			t.Errorf("%s names the directory it ran in", name)
		}
	}
}

func TestCollectKeepsWhatEachToolFound(t *testing.T) {
	dir := seededProject(t)
	fakeTools(t, "gitleaks", "lizard", "jscpd")
	out := filepath.Join(t.TempDir(), "evidence")
	if code := collectCmd([]string{"-o", out, dir}); code != 0 {
		t.Fatalf("collect exited %d", code)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	var leaks []map[string]any
	if err := json.Unmarshal([]byte(read("gitleaks.json")), &leaks); err != nil {
		t.Fatal(err)
	}
	if len(leaks) != 1 || leaks[0]["RuleID"] != "generic-api-key" || leaks[0]["File"] != "src/a.ts" || leaks[0]["StartLine"] != 3.0 {
		t.Fatalf("gitleaks finding lost its rule, file or line: %v", leaks)
	}
	for _, k := range []string{"Match", "Secret", "Line", "Author", "Email", "Message", "Link", "Fragment"} {
		if _, ok := leaks[0][k]; ok {
			t.Errorf("gitleaks.json kept %s", k)
		}
	}

	lizard := read("lizard.csv")
	if !strings.HasPrefix(lizard, "nloc,ccn,tokens,params,length,file,function,start,end\n") {
		t.Fatalf("lizard.csv header: %q", lizard)
	}
	for _, f := range []string{"src/a.ts", "src/b.ts", "app/main.py"} {
		if !strings.Contains(lizard, "4,2,30,1,4,"+f+",run,1,4\n") {
			t.Errorf("lizard.csv has no row for %s, it reads\n%s", f, lizard)
		}
	}

	var dup struct {
		Statistics map[string]any   `json:"statistics"`
		Duplicates []map[string]any `json:"duplicates"`
	}
	if err := json.Unmarshal([]byte(read("jscpd.json")), &dup); err != nil {
		t.Fatal(err)
	}
	if len(dup.Duplicates) != 1 {
		t.Fatalf("jscpd.json duplicates: %v", dup.Duplicates)
	}
	d := dup.Duplicates[0]
	if _, ok := d["fragment"]; ok {
		t.Error("jscpd.json kept the fragment")
	}
	first, _ := d["firstFile"].(map[string]any)
	if first["name"] != "src/a.ts" || first["start"] != 1.0 || first["end"] != 4.0 || d["lines"] != 4.0 {
		t.Errorf("jscpd duplicate lost where it is: %v", d)
	}
	if total, _ := dup.Statistics["total"].(map[string]any); total["clones"] != 1.0 {
		t.Errorf("jscpd statistics lost the total: %v", dup.Statistics)
	}

	tools := read("tools.txt")
	for _, line := range []string{"warpmap " + version, "gitleaks 9.9.9", "lizard 9.9.9", "jscpd 9.9.9"} {
		if !strings.Contains(tools, line+"\n") {
			t.Errorf("tools.txt has no line %q, it reads\n%s", line, tools)
		}
	}
}

func TestCollectListsMissingToolsAndCarriesOn(t *testing.T) {
	dir := seededProject(t)
	fakeTools(t)
	out := filepath.Join(t.TempDir(), "evidence")
	if code := collectCmd([]string{dir, "-o", out}); code != 0 {
		t.Fatalf("collect exited %d with every optional tool missing", code)
	}
	got := folderFiles(t, out)
	if want := []string{"analyze.json", "report.json", "tools.txt"}; !slices.Equal(got, want) {
		t.Fatalf("folder holds %v, want %v", got, want)
	}
	b, err := os.ReadFile(filepath.Join(out, "tools.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := "warpmap " + version + "\ngitleaks absent\nlizard absent\njscpd absent\n"
	if string(b) != want {
		t.Fatalf("tools.txt = %q, want %q", b, want)
	}
}

// the folder is the same evidence the two commands print, not a second
// rendering of it that could drift.
func TestCollectWritesWhatReportAndAnalyzePrint(t *testing.T) {
	dir := seededProject(t)
	fakeTools(t)
	out := filepath.Join(t.TempDir(), "evidence")
	if code := collectCmd([]string{dir, "-o", out}); code != 0 {
		t.Fatalf("collect exited %d", code)
	}
	reportOut := filepath.Join(t.TempDir(), "report.json")
	if code := reportCmd([]string{dir, "--json", "-o", reportOut}); code != 0 {
		t.Fatalf("report exited %d", code)
	}
	want, _ := os.ReadFile(reportOut)
	got, _ := os.ReadFile(filepath.Join(out, "report.json"))
	if string(got) != string(want) {
		t.Errorf("report.json differs from report --json:\n%s\nwant\n%s", got, want)
	}
	var g struct {
		Files []string `json:"Files"`
	}
	b, _ := os.ReadFile(filepath.Join(out, "analyze.json"))
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(g.Files, "src/a.ts") || !slices.Contains(g.Files, "app/main.py") {
		t.Errorf("analyze.json files: %v", g.Files)
	}
}

func TestCollectRefusesAFolderThatIsNotEmpty(t *testing.T) {
	dir := seededProject(t)
	fakeTools(t)
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "notes.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := collectCmd([]string{dir, "-o", out}); code != 2 {
		t.Fatalf("collect into a folder holding other files exited %d, want 2", code)
	}
	if got := folderFiles(t, out); !slices.Equal(got, []string{"notes.txt"}) {
		t.Fatalf("collect wrote into a folder it refused: %v", got)
	}
	if code := collectCmd([]string{dir}); code != 2 {
		t.Fatalf("collect with no -o exited %d, want 2", code)
	}
}
