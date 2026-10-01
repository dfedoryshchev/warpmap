package metrics

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitAs(t *testing.T, repo, author, file, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(file)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", "--", file)
	gitIn(t, repo, "-c", "user.name="+author, "-c", "user.email=a@example.com", "commit", "-m", "edit "+file)
}

// the walk hands Owners a path spelled from wherever the caller stood, while
// git reads it from inside the repository, so the same file has to come back
// with the same authors however the project was named.
func TestOwnersAgreesWhateverTheProjectIsCalled(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
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
	commitAs(t, repo, "Ada", "src/a.ts", "export const a = 1;\n")
	commitAs(t, repo, "Bo", "src/a.ts", "export const a = 2;\n")
	commitAs(t, repo, "Ada", "src/b.ts", "export const b = 1;\n")

	want := map[string][]string{"src/a.ts": {"Ada", "Bo"}, "src/b.ts": {"Ada"}}
	check := func(label, dir string) {
		t.Helper()
		for f, authors := range want {
			got := Owners(dir, filepath.Join(dir, filepath.FromSlash(f)))
			slices.Sort(got)
			if !slices.Equal(got, authors) {
				t.Errorf("%s: Owners(%q, %s) = %v, want %v", label, dir, f, got, authors)
			}
		}
	}

	check("absolute", repo)
	t.Chdir(root)
	check("relative", "proj")
	check("dot-slash", "."+string(filepath.Separator)+"proj")
	t.Chdir(filepath.Join(repo, "src"))
	check("parent", "..")
	t.Chdir(filepath.Join(root))
	if err := os.Mkdir("other", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir("other")
	check("sibling", filepath.Join("..", "proj"))
}
