package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
}

func assertListed(t *testing.T, got []string, want, not []string) {
	t.Helper()
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("%s was not listed: %v", w, got)
		}
	}
	for _, n := range not {
		if slices.Contains(got, n) {
			t.Errorf("%s is ignored by its repo but was listed: %v", n, got)
		}
	}
}

// a directory .gitignore drops is not the project's source whatever it is
// called, and the built-in list can only ever name the usual few.
func TestSourceFilesSkipsWhatGitIgnores(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		".gitignore":        "generated/\n*.min.js\n",
		"src/a.ts":          "export const a = 1;\n",
		"generated/g.ts":    "export const g = 1;\n",
		"src/bundle.min.js": "var b=1;\n",
	})
	commitAll(t, dir)
	writeFiles(t, dir, map[string]string{"src/new.ts": "export const n = 1;\n"})

	assertListed(t, names(dir, sourceFiles(dir)),
		[]string{"src/a.ts", "src/new.ts"},
		[]string{"generated/g.ts", "src/bundle.min.js"})
}

// the built-ins and warpmap.json still apply to what git lists: a repo that
// commits its dist folder has not made it source.
func TestSourceFilesKeepsConfiguredIgnoresInARepo(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"warpmap.json":            `{"ignore":["vendor"]}`,
		"src/a.ts":                "export const a = 1;\n",
		"dist/a.js":               "var a=1;\n",
		"vendor/lib/v.ts":         "export const v = 1;\n",
		"packages/p/dist/b.js":    "var b=1;\n",
		"packages/p/src/index.ts": "export const p = 1;\n",
	})
	commitAll(t, dir)

	assertListed(t, names(dir, sourceFiles(dir)),
		[]string{"src/a.ts", "packages/p/src/index.ts"},
		[]string{"dist/a.js", "vendor/lib/v.ts", "packages/p/dist/b.js"})
}

// git lists a nested repo as one entry, committed or not, and never reads the
// nested .gitignore; each one has to be asked on its own.
func TestSourceFilesListsNestedReposWithTheirOwnIgnores(t *testing.T) {
	needGit(t)
	outer := t.TempDir()
	writeFiles(t, outer, map[string]string{"src/a.ts": "export const a = 1;\n"})

	committed := filepath.Join(outer, "libs", "core")
	writeFiles(t, committed, map[string]string{
		".gitignore": "out/\n",
		"index.ts":   "export const c = 1;\n",
		"out/c.js":   "var c=1;\n",
	})
	commitAll(t, committed)
	commitAll(t, outer)

	loose := filepath.Join(outer, "tools")
	writeFiles(t, loose, map[string]string{
		".gitignore": "cache/\n",
		"run.py":     "x = 1\n",
		"cache/c.py": "y = 1\n",
	})
	commitAll(t, loose)

	assertListed(t, names(outer, sourceFiles(outer)),
		[]string{"src/a.ts", "libs/core/index.ts", "tools/run.py"},
		[]string{"libs/core/out/c.js", "tools/cache/c.py"})
}

// a submodule's .git is a file pointing into the outer repo, not a directory.
func TestSourceFilesListsASubmoduleWithItsOwnIgnores(t *testing.T) {
	needGit(t)
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	writeFiles(t, lib, map[string]string{
		".gitignore": "tmp/\n",
		"l.ts":       "export const l = 1;\n",
	})
	commitAll(t, lib)
	outer := filepath.Join(root, "app")
	writeFiles(t, outer, map[string]string{"a.ts": "export const a = 1;\n"})
	commitAll(t, outer)
	cmd := exec.Command("git", "-C", outer, "-c", "protocol.file.allow=always", "-c", "user.name=Ada", "-c", "user.email=a@example.com",
		"submodule", "add", lib, "vendored/lib")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("submodule add: %v\n%s", err, out)
	}
	writeFiles(t, outer, map[string]string{"vendored/lib/tmp/t.ts": "export const t = 1;\n"})

	assertListed(t, names(outer, sourceFiles(outer)),
		[]string{"a.ts", "vendored/lib/l.ts"},
		[]string{"vendored/lib/tmp/t.ts"})
}

// a folder that is not a repo but holds several: each repo answers for itself,
// and what sits beside them is walked as before.
func TestSourceFilesListsEveryRepoInAFolderOfRepos(t *testing.T) {
	needGit(t)
	projects := t.TempDir()
	for _, name := range []string{"one", "two"} {
		writeFiles(t, filepath.Join(projects, name), map[string]string{
			".gitignore":     "build-out/\n",
			"main.ts":        "export const m = 1;\n",
			"build-out/m.js": "var m=1;\n",
		})
		commitAll(t, filepath.Join(projects, name))
	}
	writeFiles(t, projects, map[string]string{"scratch/s.py": "s = 1\n"})

	assertListed(t, names(projects, sourceFiles(projects)),
		[]string{"one/main.ts", "two/main.ts", "scratch/s.py"},
		[]string{"one/build-out/m.js", "two/build-out/m.js"})
}

// a repo that keeps a folder of clones out of its own history lists that folder
// as empty, so pointed at the folder the clones have to be found on disk.
func TestSourceFilesInsideAnIgnoredFolderOfRepos(t *testing.T) {
	needGit(t)
	outer := t.TempDir()
	writeFiles(t, outer, map[string]string{".gitignore": "clones/\n", "a.ts": "export const a = 1;\n"})
	commitAll(t, outer)
	clone := filepath.Join(outer, "clones", "c")
	writeFiles(t, clone, map[string]string{
		".gitignore": "gen/\n",
		"c.ts":       "export const c = 1;\n",
		"gen/g.ts":   "export const g = 1;\n",
	})
	commitAll(t, clone)

	dir := filepath.Join(outer, "clones")
	assertListed(t, names(dir, sourceFiles(dir)), []string{"c/c.ts"}, []string{"c/gen/g.ts"})
}

// churn and ownership come from the repo that holds the file. asked of the
// folder alone, a folder of repos has no history and a nested repo's files are
// not in the outer one's, so both read as zero.
func TestHotspotsAndOwnershipAskEachRepo(t *testing.T) {
	needGit(t)
	projects := t.TempDir()
	writeFiles(t, filepath.Join(projects, "one"), map[string]string{"src/a.ts": "export const a = 1;\n"})
	commitAll(t, filepath.Join(projects, "one"))
	writeFiles(t, filepath.Join(projects, "one", "inner"), map[string]string{"b.ts": "export const b = 1;\n"})
	commitAll(t, filepath.Join(projects, "one", "inner"))
	writeFiles(t, filepath.Join(projects, "two"), map[string]string{"c.ts": "export const c = 1;\n"})
	commitAll(t, filepath.Join(projects, "two"))

	for _, tc := range []struct {
		dir  string
		want []string
	}{
		{projects, []string{"one/src/a.ts", "one/inner/b.ts", "two/c.ts"}},
		{filepath.Join(projects, "one"), []string{"src/a.ts", "inner/b.ts"}},
	} {
		out, code := captureStdout(t, func() int { return hotspotsCmd([]string{tc.dir}) })
		if code != 0 {
			t.Fatalf("hotspots %s exited %d", tc.dir, code)
		}
		for _, f := range tc.want {
			if line := lineFor(out, f); !strings.Contains(line, "churn=1 ") {
				t.Errorf("hotspots %s: want churn=1 for %s, got %q in\n%s", tc.dir, f, line, out)
			}
		}
		out, code = captureStdout(t, func() int { return ownershipCmd([]string{tc.dir}) })
		if code != 0 {
			t.Fatalf("ownership %s exited %d", tc.dir, code)
		}
		for _, f := range tc.want {
			if line := lineFor(out, f); !strings.Contains(line, "authors=1 ") {
				t.Errorf("ownership %s: want authors=1 for %s, got %q in\n%s", tc.dir, f, line, out)
			}
		}
	}
}

func lineFor(out, file string) string {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for _, f := range fields {
			if f == file {
				return line
			}
		}
	}
	return ""
}
