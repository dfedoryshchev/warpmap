package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if spec, ok := os.LookupEnv("WARPMAP_FAKE_LIZARD"); ok {
		os.Exit(fakeLizard(os.Args[1:], spec))
	}
	os.Exit(m.Run())
}

// fakeLizard prints what lizard 1.24.0 prints for `--csv` (lizard_ext/csvoutput.py):
// no header, one row per function, NLOC,CCN,token,PARAM,length,
// "name@start-end@file","file","name","long_name",start,end. spec gives the CCN
// of each function per file base name, `a.go=3+2;b.cs=4`.
func fakeLizard(args []string, spec string) int {
	var list string
	csv := false
	for i, a := range args {
		switch {
		case a == "--csv":
			csv = true
		case a == "-f" && i+1 < len(args):
			list = args[i+1]
		}
	}
	if !csv || list == "" {
		fmt.Fprintln(os.Stderr, "fake lizard: want --csv -f <list>, got", args)
		return 2
	}
	if spec == "fail" {
		return 1
	}
	b, err := os.ReadFile(list)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if spec == "stranger" {
		fmt.Println(`3,2,20,0,3,"f@1-3@not/given.go","not/given.go","f","f( )",1,3`)
		return 0
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

// installFakeLizard puts a copy of this test binary in a fresh directory under the
// name lizard and returns its path; spec is what it answers with.
func installFakeLizard(t *testing.T, spec string) string {
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
	bin := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(bin, b, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WARPMAP_FAKE_LIZARD", spec)
	return bin
}

func TestLizardComplexitySumsEachFilesFunctions(t *testing.T) {
	dir := t.TempDir()
	files := writeFiles(t, dir, map[string]string{
		"cmd/a.go":       "package main\n",
		"odd,dir/b.cs":   "class B {}\n",
		"lib/c.rb":       "X = 1\n",
		"src/nothing.rs": "",
	})
	bin := installFakeLizard(t, "a.go=3+2;b.cs=4")
	cx, err := LizardComplexity(bin, files)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"cmd/a.go": 5, "odd,dir/b.cs": 4, "lib/c.rb": 0, "src/nothing.rs": 0}
	for _, f := range files {
		r, _ := filepath.Rel(dir, f)
		if got, w := cx[f], want[filepath.ToSlash(r)]; got != w {
			t.Errorf("%s: complexity %d, want %d", filepath.ToSlash(r), got, w)
		}
	}
}

func TestLizardComplexityRefusesOutputAboutOtherFiles(t *testing.T) {
	files := writeFiles(t, t.TempDir(), map[string]string{"a.go": "package a\n"})
	if _, err := LizardComplexity(installFakeLizard(t, "stranger"), files); err == nil {
		t.Fatal("lizard named only a file it was not given, and that was read as every given file scoring 0")
	}
}

func TestLizardComplexityReportsAFailedRun(t *testing.T) {
	files := writeFiles(t, t.TempDir(), map[string]string{"a.go": "package a\n"})
	if _, err := LizardComplexity(installFakeLizard(t, "fail"), files); err == nil {
		t.Fatal("lizard exited 1 and no error came back")
	}
}

func TestLizardHotspotsRankByLizardsNumbers(t *testing.T) {
	dir := t.TempDir()
	branchy := strings.Repeat("if x { y() }\n", 30)
	files := writeFiles(t, dir, map[string]string{
		"a.go":      "package a\n",
		"b.go":      "package b\n" + branchy,
		"b_test.go": "package b\n" + branchy,
	})
	churn := Churn{"a.go": 2, "b.go": 2, "b_test.go": 2}
	ranked, err := LizardHotspots(installFakeLizard(t, "a.go=9;b.go=1;b_test.go=50"), dir, files, churn)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range ranked {
		r, _ := filepath.Rel(dir, h.File)
		got = append(got, fmt.Sprintf("%s=%d", filepath.ToSlash(r), h.Complexity))
	}
	if want := "a.go=9 b.go=1"; strings.Join(got, " ") != want {
		t.Fatalf("ranked %q, want %q", strings.Join(got, " "), want)
	}
}
