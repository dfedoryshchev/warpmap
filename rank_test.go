package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
)

// a lizard installed on the machine running the tests would otherwise answer
// every hotspot test here, so it is taken off PATH before any of them run.
func TestMain(m *testing.M) {
	if spec, ok := os.LookupEnv("WARPMAP_FAKE_LIZARD"); ok {
		os.Exit(fakeLizard(os.Args[1:], spec))
	}
	var keep []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if _, err := exec.LookPath(filepath.Join(d, "lizard")); err != nil {
			keep = append(keep, d)
		}
	}
	os.Setenv("PATH", strings.Join(keep, string(os.PathListSeparator)))
	os.Exit(m.Run())
}

// fakeLizard prints what lizard 1.24.0 prints for `--csv` (lizard_ext/csvoutput.py):
// no header, one row per function, NLOC,CCN,token,PARAM,length,
// "name@start-end@file","file","name","long_name",start,end. spec gives the CCN
// of each function per file base name, `a.go=3+2;b.cs=4`.
func fakeLizard(args []string, spec string) int {
	i := slices.Index(args, "-f")
	if !slices.Contains(args, "--csv") || i < 0 || i+1 >= len(args) {
		fmt.Fprintln(os.Stderr, "fake lizard: want --csv -f <list>, got", args)
		return 2
	}
	if spec == "fail" {
		return 1
	}
	b, err := os.ReadFile(args[i+1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	ccn := map[string][]string{}
	for _, part := range strings.Split(spec, ";") {
		name, nums, _ := strings.Cut(part, "=")
		ccn[name] = strings.Split(nums, "+")
	}
	for _, f := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f = strings.TrimRight(f, "\r")
		for i, n := range ccn[filepath.Base(f)] {
			name := fmt.Sprintf("f%d", i)
			start, end := 10*i+1, 10*i+6
			fmt.Printf("6,%s,40,1,6,\"%s@%d-%d@%s\",\"%s\",\"%s\",\"%s( x )\",%d,%d\n",
				n, name, start, end, f, f, name, name, start, end)
		}
	}
	return 0
}

// installFakeLizard puts a copy of this test binary first on PATH under the name
// lizard; spec is what it answers with.
func installFakeLizard(t *testing.T, spec string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	name := "lizard"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WARPMAP_FAKE_LIZARD", spec)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// polyglotProject commits one source file per language hotspots ranks, plus a
// go test file and a markdown file it must not rank, so every file has churn.
func polyglotProject(t *testing.T) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"cmd/main.go":      "package main\n\nfunc main() {\n\tif len(x) > 0 {\n\t\trun()\n\t}\n}\n",
		"src/Service.cs":   "class Service { void Run() { if (ok) { Go(); } } }\n",
		"src/Main.java":    "class Main { void run() { for (;;) {} } }\n",
		"lib/task.rb":      "def task\n  if ready\n    go\n  end\nend\n",
		"src/lib.rs":       "fn run() { if ok { go(); } }\n",
		"web/index.php":    "<?php if ($ok) { go(); }\n",
		"ios/View.swift":   "func run() { if ok { go() } }\n",
		"native/io.c":      "int run(void) { if (ok) return 1; return 0; }\n",
		"native/io.h":      "int run(void);\n",
		"native/codec.cpp": "int run() { while (ok) {} return 0; }\n",
		"android/Main.kt":  "fun run() { if (ok) go() }\n",
		"jobs/Job.scala":   "object Job { def run() = if (ok) go() }\n",
		"ui/app.ts":        "export const app = () => (ok ? 1 : 2);\n",
		"tools/report.py":  "def report():\n    if ok:\n        go()\n",
		"cmd/main_test.go": "package main\n\nfunc TestMain() { if true {} }\n",
		"README.md":        "# notes\n",
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
	commitAll(t, dir)
	var want []string
	for name := range files {
		if name != "cmd/main_test.go" && name != "README.md" {
			want = append(want, name)
		}
	}
	sort.Strings(want)
	return dir, want
}

func commitAll(t *testing.T, dir string) {
	t.Helper()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false", "-c", "user.name=Ada", "-c", "user.email=a@example.com"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	// git writes its objects read-only, which the temp-dir cleanup cannot remove on windows.
	t.Cleanup(func() {
		filepath.WalkDir(filepath.Join(dir, ".git"), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				os.Chmod(p, 0o666)
			}
			return nil
		})
	})
	git("add", "--", ".")
	git("commit", "-m", "add the project")
}

func lastFields(out string, skip int) []string {
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n")[skip:] {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			got = append(got, fields[len(fields)-1])
		}
	}
	return got
}

// churn comes out of git and knows no language, so a file is left out of the
// ranking only because the walk never read it - which on a go or c# repo used to
// leave a stray docs config as the whole answer.
func TestHotspotsAndOwnershipRankEveryCommonLanguage(t *testing.T) {
	dir, want := polyglotProject(t)

	out, code := captureStdout(t, func() int { return hotspotsCmd([]string{dir, "-n", "50"}) })
	if code != 0 {
		t.Fatalf("hotspots exited %d", code)
	}
	got := lastFields(out, 0)
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Fatalf("hotspots ranked\n%q\nwant\n%q", got, want)
	}

	out, code = captureStdout(t, func() int { return ownershipCmd([]string{dir, "-n", "50"}) })
	if code != 0 {
		t.Fatalf("ownership exited %d", code)
	}
	var owned []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n")[1:] {
		owned = append(owned, strings.Fields(line)[1])
	}
	sort.Strings(owned)
	if !slices.Equal(owned, want) {
		t.Fatalf("ownership checked\n%q\nwant\n%q", owned, want)
	}
}

// the import graph still reads only what it can parse: a go file there would be
// a file nothing imports, and `dead` would list every one of them.
func TestTheGraphCommandsStillWalkOnlyTheParsedLanguages(t *testing.T) {
	dir, _ := polyglotProject(t)
	if got, want := names(dir, sourceFiles(dir)), []string{"tools/report.py", "ui/app.ts"}; !slices.Equal(got, want) {
		t.Fatalf("the graph walk read %q, want %q", got, want)
	}
}

func TestHotspotsSaysTheInternalMeasureRanItWithoutLizard(t *testing.T) {
	dir, _ := polyglotProject(t)
	var out string
	errOut := captureStderr(t, func() int {
		var code int
		out, code = captureStdout(t, func() int { return hotspotsCmd([]string{dir, "-n", "3"}) })
		return code
	})
	if !strings.HasPrefix(errOut, "complexity: internal") || !strings.Contains(errOut, "lizard") {
		t.Fatalf("stderr did not name the internal measure or how to get lizard's:\n%s", errOut)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if !strings.Contains(line, " cx=") {
			t.Fatalf("a row from the internal measure is not labelled cx=: %q", line)
		}
	}
}

func TestHotspotsTakesComplexityFromLizardOnPath(t *testing.T) {
	dir, _ := polyglotProject(t)
	installFakeLizard(t, "Service.cs=7+5;lib.rs=4;main.go=2;main_test.go=40")
	var out string
	errOut := captureStderr(t, func() int {
		var code int
		out, code = captureStdout(t, func() int { return hotspotsCmd([]string{dir, "-n", "4"}) })
		return code
	})
	if !strings.HasPrefix(errOut, "complexity: lizard") {
		t.Fatalf("stderr did not name lizard as the measure:\n%s", errOut)
	}
	want := []string{
		"1.000  churn=1   ccn=12   src/Service.cs",
		"0.333  churn=1   ccn=4    src/lib.rs",
		"0.167  churn=1   ccn=2    cmd/main.go",
	}
	got := strings.Split(strings.TrimSpace(out), "\n")
	if len(got) != 4 || !slices.Equal(got[:3], want) || !strings.Contains(got[3], "0.000  churn=1   ccn=0 ") {
		t.Fatalf("hotspots printed\n%s\nwant the first three\n%s\nthen a ccn=0 row", out, strings.Join(want, "\n"))
	}
}

func TestHotspotsFallsBackAndSaysSoWhenLizardFails(t *testing.T) {
	dir, _ := polyglotProject(t)
	installFakeLizard(t, "fail")
	var out string
	errOut := captureStderr(t, func() int {
		var code int
		out, code = captureStdout(t, func() int { return hotspotsCmd([]string{dir, "-n", "2"}) })
		return code
	})
	if !strings.HasPrefix(errOut, "complexity: internal") || !strings.Contains(errOut, "lizard is on PATH but failed") {
		t.Fatalf("a failed lizard run was not named on stderr:\n%s", errOut)
	}
	if n := countLines(out); n != 2 || strings.Count(out, " cx=") != 2 {
		t.Fatalf("want two rows from the internal measure, got:\n%s", out)
	}
}
