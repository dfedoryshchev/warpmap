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

func callText(t *testing.T, tool string, args map[string]string) string {
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
		got := callText(t, "hotspots", map[string]string{"dir": dir})
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
