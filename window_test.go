package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// windowProject writes a small file and a branchy one, in that walk order. With
// no churn to go on, walk order is the wrong answer: the branchy file has to
// come first.
func windowProject(t *testing.T, dir string) {
	t.Helper()
	writeFiles(t, dir, map[string]string{
		"src/a.ts":    "export const a = 1;\n",
		"src/busy.ts": strings.Repeat("if (x) { y(); }\n", 30),
	})
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	needGit(t)
	full := append([]string{"-C", dir, "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false", "-c", "user.name=Ada", "-c", "user.email=a@example.com"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	if args[0] == "init" {
		// git writes its objects read-only, which the temp-dir cleanup cannot remove on windows.
		t.Cleanup(func() {
			filepath.WalkDir(filepath.Join(dir, ".git"), func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					os.Chmod(p, 0o666)
				}
				return nil
			})
		})
	}
}

func hotspotsBoth(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	stderr = captureStderr(t, func() int {
		var code int
		stdout, code = captureStdout(t, func() int { return hotspotsCmd(args) })
		return code
	})
	return stdout, stderr
}

// a code drop with no git, and a directory no commit in the window touched,
// both read as zero churn on every file. Multiplying by zero used to score
// every file 0.000 and leave them in walk order, so the most complex file could
// sit at the bottom of a table that looked like a ranking.
func TestHotspotsRankByComplexityWhenTheWindowIsEmpty(t *testing.T) {
	for _, tc := range []struct {
		name    string
		project func(t *testing.T) string
	}{
		{"no git", func(t *testing.T) string {
			dir := t.TempDir()
			windowProject(t, dir)
			return dir
		}},
		{"nothing in the window", func(t *testing.T) string {
			repo := t.TempDir()
			writeFiles(t, repo, map[string]string{"other/o.ts": "export const o = 1;\n"})
			gitIn(t, repo, "init")
			gitIn(t, repo, "add", "--", "other")
			gitIn(t, repo, "commit", "-m", "add other")
			dir := filepath.Join(repo, "pkg")
			windowProject(t, dir)
			return dir
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut := hotspotsBoth(t, tc.project(t))
			rows := strings.Split(strings.TrimSpace(out), "\n")
			if len(rows) != 2 || !strings.HasPrefix(rows[0], "1.000  churn=0 ") || !strings.HasSuffix(rows[0], " src/busy.ts") {
				t.Fatalf("want src/busy.ts first at 1.000 on complexity alone, got\n%s", out)
			}
			if strings.HasPrefix(rows[1], "0.000") {
				t.Errorf("the simpler file scored 0.000, so the ranking still multiplies by zero churn:\n%s", out)
			}
			if !strings.Contains(errOut, "ranked by complexity alone") {
				t.Errorf("stderr did not say the ranking fell back to complexity:\n%s", errOut)
			}
		})
	}
}

// git reports numstat paths from the repo root. Asked about a directory below
// it, every lookup missed by that directory's prefix and every file read as
// churn 0, however busy its history.
func TestHotspotsInASubdirectoryReadsItsChurn(t *testing.T) {
	repo := t.TempDir()
	pkg := filepath.Join(repo, "pkg")
	windowProject(t, pkg)
	writeFiles(t, repo, map[string]string{"other/o.ts": "export const o = 1;\n"})
	gitIn(t, repo, "init")
	gitIn(t, repo, "add", "--", "pkg", "other")
	gitIn(t, repo, "commit", "-m", "add both")
	gitIn(t, repo, "commit", "--allow-empty", "-m", "touch nothing")

	check := func(label, dir string) {
		t.Helper()
		out, errOut := hotspotsBoth(t, dir)
		for _, f := range []string{"src/a.ts", "src/busy.ts"} {
			if line := lineFor(out, f); !strings.Contains(line, "churn=1 ") {
				t.Errorf("%s: want churn=1 for %s, got %q in\n%s", label, f, line, out)
			}
		}
		if strings.Contains(out, "o.ts") {
			t.Errorf("%s: a file outside the directory was ranked:\n%s", label, out)
		}
		if !strings.Contains(errOut, "churn: 1 commit in the last 6 months") {
			t.Errorf("%s: stderr did not count the one commit that touched the directory:\n%s", label, errOut)
		}
	}
	check("absolute", pkg)
	t.Chdir(repo)
	check("relative", "pkg")
	t.Chdir(pkg)
	check("dot", ".")
}

func TestHotspotsReadsTheMonthsFlag(t *testing.T) {
	dir := t.TempDir()
	windowProject(t, dir)
	gitIn(t, dir, "init")
	gitIn(t, dir, "add", "--", "src")
	gitIn(t, dir, "commit", "-m", "add the project")
	out, errOut := hotspotsBoth(t, dir, "--months", "36")
	if line := lineFor(out, "src/a.ts"); !strings.Contains(line, "churn=1 ") {
		t.Fatalf("--months 36 lost the commit:\n%s", out)
	}
	if !strings.Contains(errOut, "churn: 1 commit in the last 36 months") {
		t.Fatalf("stderr did not name the window that was read:\n%s", errOut)
	}
	m := monthsValue(6)
	for _, bad := range []string{"0", "-1", "six"} {
		if m.Set(bad) == nil {
			t.Errorf("--months %s was accepted", bad)
		}
	}
}

// with a handful of commits in the window, churn 2 against churn 3 is chance,
// and normalising by the busiest file hides how few commits it came from.
func TestHotspotsWarnsWhenTheWindowIsThin(t *testing.T) {
	dir := t.TempDir()
	windowProject(t, dir)
	gitIn(t, dir, "init")
	gitIn(t, dir, "add", "--", "src")
	gitIn(t, dir, "commit", "-m", "add the project")
	for _, edit := range []string{"2", "3"} {
		writeFiles(t, dir, map[string]string{"src/a.ts": "export const a = 1;\n// " + edit + "\n"})
		gitIn(t, dir, "commit", "-am", "edit "+edit)
	}
	_, errOut := hotspotsBoth(t, dir)
	if !strings.Contains(errOut, "churn: 3 commits in the last 6 months") {
		t.Fatalf("stderr did not print the window's commit count:\n%s", errOut)
	}
	if !strings.Contains(errOut, "under 20") || !strings.Contains(errOut, "--months") {
		t.Fatalf("stderr did not warn that a 3-commit window ranks on noise:\n%s", errOut)
	}
}
