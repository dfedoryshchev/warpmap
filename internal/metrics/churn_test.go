package metrics

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func commitFiles(t *testing.T, repo, msg string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "-c", "user.name=Ada", "-c", "user.email=a@example.com", "commit", "-m", msg)
}

// churn reads one count per file per commit, a rename against the new path, and
// only the commits that touched the directory asked about, keyed relative to it.
func TestGitChurnCountsOnePerFilePerCommit(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	commitFiles(t, repo, "one", map[string]string{"pkg/a.ts": "a\n", "pkg/x/old.ts": "o\no\no\no\no\n", "top.ts": "t\n"})
	commitFiles(t, repo, "two", map[string]string{"pkg/a.ts": "a2\n"})
	gitIn(t, repo, "mv", "pkg/x/old.ts", "pkg/x/new.ts")
	commitFiles(t, repo, "three", map[string]string{"pkg/a.ts": "a3\n"})
	commitFiles(t, repo, "four", map[string]string{"top.ts": "t2\n"})
	gitIn(t, repo, "-c", "user.name=Ada", "-c", "user.email=a@example.com", "commit", "--allow-empty", "-m", "empty")

	churn, commits, err := GitChurn(repo, 6)
	if err != nil {
		t.Fatal(err)
	}
	want := Churn{"pkg/a.ts": 3, "pkg/x/old.ts": 1, "pkg/x/new.ts": 1, "top.ts": 2}
	if !reflect.DeepEqual(churn, want) || commits != 4 {
		t.Fatalf("whole repo: got %v over %d commits, want %v over 4", churn, commits, want)
	}

	churn, commits, err = GitChurn(filepath.Join(repo, "pkg"), 6)
	if err != nil {
		t.Fatal(err)
	}
	want = Churn{"a.ts": 3, "x/old.ts": 1, "x/new.ts": 1}
	if !reflect.DeepEqual(churn, want) || commits != 3 {
		t.Fatalf("subdirectory: got %v over %d commits, want %v over 3", churn, commits, want)
	}
}

func TestWindowNote(t *testing.T) {
	churned := []Hotspot{{File: "a.ts", Churn: 2}, {File: "b.ts"}}
	flat := []Hotspot{{File: "a.ts"}, {File: "b.ts"}}
	for _, tc := range []struct {
		name   string
		w      Window
		ranked []Hotspot
		want   string
	}{
		{"no git", Window{Months: 6, Err: errors.New("not a repo")}, flat,
			"churn: no git history to read here - ranked by complexity alone"},
		{"empty", Window{Months: 6}, flat,
			"churn: 0 commits in the last 6 months - ranked by complexity alone; --months widens the window"},
		{"none to a ranked file", Window{Months: 1, Commits: 4}, flat,
			"churn: 4 commits in the last 1 month, none to a ranked file - ranked by complexity alone"},
		{"thin", Window{Months: 6, Commits: ThinWindow - 1}, churned,
			"churn: 19 commits in the last 6 months - under 20, so churn differences are mostly noise; --months widens the window"},
		{"enough", Window{Months: 6, Commits: ThinWindow}, churned,
			"churn: 20 commits in the last 6 months"},
	} {
		if got := tc.w.Note(tc.ranked); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
		if strings.ContainsAny(tc.w.Note(tc.ranked), "\n\t") {
			t.Errorf("%s: the note is not one line", tc.name)
		}
	}
}
