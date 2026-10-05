package mcp

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "commit.gpgsign=false",
		"-c", "user.name=Ada", "-c", "user.email=a@example.com"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commit(t *testing.T, repo, file, body string) {
	t.Helper()
	p := filepath.Join(repo, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", "--", file)
	gitIn(t, repo, "commit", "-m", "edit "+file)
}

func tsFiles(dir string) []string {
	var out []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() && filepath.Ext(p) == ".ts" {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// chain writes src/a.ts -> src/b.ts -> src/c.ts, so both of the others depend on c.
func chain(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range map[string]string{
		"src/a.ts": "import { b } from \"./b\";\nexport const a = b + 1;\n",
		"src/b.ts": "import { c } from \"./c\";\nexport const b = c + 1;\n",
		"src/c.ts": "export const c = 1;\n",
	} {
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

func TestToolsListOffersRiskAndTestgap(t *testing.T) {
	var names []string
	var riskSchema map[string]any
	for _, spec := range toolSpecs() {
		name := spec["name"].(string)
		names = append(names, name)
		if name == "risk" {
			riskSchema = spec["inputSchema"].(map[string]any)
		}
	}
	if got, want := strings.Join(names, ","), "hotspots,trace,risk,testgap"; got != want {
		t.Fatalf("tools/list offers %s, want %s", got, want)
	}
	files, _ := riskSchema["properties"].(map[string]any)["files"].(map[string]any)
	if files["type"] != "array" {
		t.Fatalf("risk takes files as %v, want an array", files["type"])
	}
	if got := strings.Join(riskSchema["required"].([]string), ","); got != "dir,files" {
		t.Fatalf("risk requires %s, want dir,files", got)
	}
}

func TestTraceToolListsTheDependents(t *testing.T) {
	dir := chain(t)
	want := "2 files depend on src/c.ts\nsrc/a.ts\nsrc/b.ts\n"
	if got := callText(t, "trace", map[string]any{"dir": dir, "file": "src/c.ts"}); got != want {
		t.Errorf("absolute: trace text = %q, want %q", got, want)
	}
	t.Chdir(dir)
	if got := callText(t, "trace", map[string]any{"dir": ".", "file": "src/c.ts"}); got != want {
		t.Errorf("dot: trace text = %q, want %q", got, want)
	}
}

func TestTestgapToolRanksUntestedFilesByBlastRadius(t *testing.T) {
	dir := chain(t)
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	setup := "import { a } from \"../src/a\";\nexport const s = a;\n"
	if err := os.WriteFile(filepath.Join(dir, "tests", "setup.ts"), []byte(setup), 0o644); err != nil {
		t.Fatal(err)
	}
	want := "src/c.ts  3\nsrc/b.ts  2\n"
	if got := callText(t, "testgap", map[string]any{"dir": dir}); got != want {
		t.Errorf("testgap text = %q, want %q", got, want)
	}
}

func TestRiskToolGivesTheVerdictUnderTheProjectThreshold(t *testing.T) {
	dir := chain(t)
	if err := os.WriteFile(filepath.Join(dir, "warpmap.json"), []byte(`{"thresholds":{"blast":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	risky := callText(t, "risk", map[string]any{"dir": dir, "files": []string{"src/c.ts"}})
	want := "  src/c.ts: blast=2, UNTESTED\ncombined blast radius: 2 files\n" +
		"verdict: 1 changed file(s) are high-blast AND untested - add tests before changing\n"
	if risky != want {
		t.Errorf("risk on c text = %q, want %q", risky, want)
	}
	calm := callText(t, "risk", map[string]any{"dir": dir, "files": []string{"src/a.ts", "src/b.ts"}})
	want = "  src/a.ts: blast=0, UNTESTED\n  src/b.ts: blast=1, UNTESTED\ncombined blast radius: 1 files\nverdict: manageable\n"
	if calm != want {
		t.Errorf("risk on a, b text = %q, want %q", calm, want)
	}
}

func callText(t *testing.T, tool string, args map[string]any) string {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"name": tool, "arguments": args})
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	handleCall(request{ID: json.RawMessage("1"), Method: "tools/call", Params: params}, tsFiles)
	os.Stdout = stdout
	w.Close()
	raw, _ := io.ReadAll(r)
	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Result.Content) != 1 {
		t.Fatalf("reply %q: %v", raw, err)
	}
	return resp.Result.Content[0].Text
}

func TestHotspotsToolPrintsPathThenScore(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "init")
	// git writes its objects read-only, which the temp-dir cleanup cannot remove on windows.
	t.Cleanup(func() {
		filepath.WalkDir(filepath.Join(repo, ".git"), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				os.Chmod(p, 0o666)
			}
			return nil
		})
	})
	branchy := "export function a(x) {\n  if (x) { return 1; }\n  if (!x) { return 2; }\n  return 3;\n}\n"
	commit(t, repo, "src/a.ts", branchy)
	commit(t, repo, "src/a.ts", branchy+"// 2\n")
	commit(t, repo, "src/a.ts", branchy+"// 3\n")
	commit(t, repo, "src/util/b.ts", "export const b = 1;\n")

	want := "src/a.ts  1.000\nsrc/util/b.ts  0.028\n"
	check := func(label, dir string) {
		t.Helper()
		got := callText(t, "hotspots", map[string]any{"dir": dir})
		if got != want {
			t.Errorf("%s: hotspots text = %q, want %q", label, got, want)
		}
		for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				t.Errorf("%s: line %q has fewer than two fields", label, line)
				continue
			}
			if _, err := strconv.ParseFloat(fields[len(fields)-1], 64); err != nil {
				t.Errorf("%s: line %q does not end in a score", label, line)
			}
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(fields[0]))); err != nil {
				t.Errorf("%s: line %q does not start with a path relative to the project", label, line)
			}
		}
	}

	check("absolute", repo)
	t.Chdir(root)
	check("relative", "proj")
	t.Chdir(repo)
	check("dot", ".")
}
