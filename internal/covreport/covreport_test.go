package covreport

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func parseFile(t *testing.T, name string) []File {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	files, err := ParseLCOV(f)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func byPath(files []File) map[string]File {
	out := map[string]File{}
	for _, f := range files {
		out[f.Path] = f
	}
	return out
}

func TestLCOVReadsLinesAndFunctionsPerFile(t *testing.T) {
	files := parseFile(t, "lcov.info")
	got := byPath(files)
	if len(files) != 4 {
		t.Fatalf("want 4 files, one per SF path, got %d: %+v", len(files), files)
	}
	a := got["src/a.ts"]
	if want := map[int]int{1: 3, 2: 3, 5: 0, 6: 0}; !reflect.DeepEqual(a.Lines, want) {
		t.Fatalf("src/a.ts lines %v, want %v", a.Lines, want)
	}
	if want := []Function{{"a", 1, 3}, {"helper", 5, 0}}; !reflect.DeepEqual(a.Functions, want) {
		t.Fatalf("src/a.ts functions %+v, want %+v", a.Functions, want)
	}
	if !a.Ran() {
		t.Fatalf("src/a.ts ran three lines and reads as not run")
	}
}

// one report can hold a file more than once, a record per test name; the hits
// add up.
func TestLCOVAddsUpAFileRecordedMoreThanOnce(t *testing.T) {
	b := byPath(parseFile(t, "lcov.info"))["src/b.ts"]
	if want := map[int]int{1: 0, 2: 4}; !reflect.DeepEqual(b.Lines, want) {
		t.Fatalf("src/b.ts lines %v, want %v", b.Lines, want)
	}
	if !b.Ran() {
		t.Fatalf("src/b.ts ran under the second test name and reads as not run")
	}
}

func TestLCOVToleratesMissingOptionalRecords(t *testing.T) {
	got := byPath(parseFile(t, "lcov.info"))
	d := got["src/d.ts"]
	if d.Ran() || len(d.Lines) != 1 || len(d.Functions) != 1 {
		t.Fatalf("src/d.ts, with no TN, LF or LH, read as %+v", d)
	}
	c, ok := got["./src/c.ts"]
	if !ok || !c.Ran() || c.Lines[1] != 1 {
		t.Fatalf("the last record, with no end_of_record, read as %+v (found %v)", c, ok)
	}
}

func TestLCOVReadsWindowsLineEndings(t *testing.T) {
	files, err := ParseLCOV(strings.NewReader("SF:src/a.ts\r\nDA:1,2\r\nend_of_record\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "src/a.ts" || files[0].Lines[1] != 2 {
		t.Fatalf("got %+v", files)
	}
}

func TestLCOVNamesTheLineItCannotRead(t *testing.T) {
	for in, want := range map[string]string{
		"TN:x\nDA:1,1\n":                 "line 2: DA with no SF record",
		"SF:a.ts\nDA:one,1\n":            "line 2: DA:one,1",
		"SF:a.ts\nFNDA:x,f\n":            "line 2: FNDA:x,f",
		"SF:a.ts\nend_of_record\nLH:1\n": "line 3: LH with no SF record",
	} {
		_, err := ParseLCOV(strings.NewReader(in))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q gave error %v, want one naming %q", in, err, want)
		}
	}
}

func writeReport(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func walked(dir string, rel ...string) []string {
	out := make([]string, len(rel))
	for i, r := range rel {
		out[i] = filepath.Join(dir, filepath.FromSlash(r))
	}
	return out
}

// a runner writes absolute paths, or paths relative to where it ran, with or
// without a leading ./; all of them name the same walked file.
func TestTestedMapsEverySpellingOfAPathOntoTheWalk(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "src", "a.ts")
	writeReport(t, dir, "coverage/lcov.info",
		"SF:"+abs+"\nDA:1,1\nend_of_record\n"+
			"SF:./src/b.ts\nDA:1,2\nend_of_record\n"+
			"SF:src/c.ts\nDA:1,0\nend_of_record\n"+
			"SF:../elsewhere/x.ts\nDA:1,5\nend_of_record\n")
	files := walked(dir, "src/a.ts", "src/b.ts", "src/c.ts", "src/d.ts")
	ran, from, note := Tested(dir, files)
	want := map[string]bool{files[0]: true, files[1]: true}
	if !reflect.DeepEqual(ran, want) || from != "coverage/lcov.info" {
		t.Fatalf("ran %v from %q, want %v from coverage/lcov.info", ran, from, want)
	}
	if want := "tested: a line ran under the suite, read from coverage/lcov.info"; note != want {
		t.Fatalf("note %q, want %q", note, want)
	}
}

func TestTestedFallsBackToTheImportEdgeWithoutAReport(t *testing.T) {
	dir := t.TempDir()
	ran, from, note := Tested(dir, walked(dir, "src/a.ts"))
	if ran != nil || from != "" {
		t.Fatalf("no report, and ran is %v from %q", ran, from)
	}
	if want := "tested: a test imports the file (structural; no coverage report at coverage/lcov.info or lcov.info)"; note != want {
		t.Fatalf("note %q, want %q", note, want)
	}
}

func TestTestedFindsAReportAtTheProjectRoot(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "lcov.info", "SF:src/a.ts\nDA:1,1\nend_of_record\n")
	files := walked(dir, "src/a.ts")
	if ran, from, note := Tested(dir, files); !ran[files[0]] || from != "lcov.info" || !strings.HasSuffix(note, "read from lcov.info") {
		t.Fatalf("ran %v from %q, note %q", ran, from, note)
	}
}

// a report whose paths name none of the walked files was written somewhere
// else, a CI runner's checkout or another package; reading it would call every
// file untested.
func TestTestedRefusesAReportThatNamesNoWalkedFile(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "coverage/lcov.info",
		"SF:/ci/checkout/app/src/a.ts\nDA:1,1\nend_of_record\nSF:pkg/b.ts\nDA:1,1\nend_of_record\n")
	ran, _, note := Tested(dir, walked(dir, "src/a.ts"))
	if ran != nil {
		t.Fatalf("ran %v from a report about other files", ran)
	}
	if !strings.HasPrefix(note, "tested: a test imports the file (structural; ") ||
		!strings.Contains(note, "names 2 files, none of them in the analysed tree") {
		t.Fatalf("note %q", note)
	}
}

func TestTestedFallsBackOnAReportItCannotRead(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "coverage/lcov.info", "DA:1,1\n")
	ran, _, note := Tested(dir, walked(dir, "src/a.ts"))
	if ran != nil || !strings.Contains(note, "structural; ") || !strings.Contains(note, "line 1: DA with no SF record") {
		t.Fatalf("ran %v, note %q", ran, note)
	}
}

func TestAdapterIsQuietWhenThereIsNoReport(t *testing.T) {
	fs, err := Adapter{}.Findings(t.TempDir())
	if fs != nil || err != nil {
		t.Fatalf("no report gave %+v, %v", fs, err)
	}
	if (Adapter{}).Tool() != "lcov" {
		t.Fatalf("tool %q", Adapter{}.Tool())
	}
}

func TestAdapterNamesTheFilesNoLineOfWhichRan(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "coverage/lcov.info",
		"SF:"+filepath.Join(dir, "src", "z.ts")+"\nDA:1,0\nend_of_record\n"+
			"SF:src/a.ts\nDA:1,1\nend_of_record\n"+
			"SF:./src/m.ts\nFN:1,m\nFNDA:0,m\nDA:1,0\nDA:2,0\nend_of_record\n"+
			"SF:src/types.ts\nend_of_record\n")
	fs, err := Adapter{}.Findings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("want one finding, got %+v", fs)
	}
	f := fs[0]
	if f.RuleID != "never-ran" || f.Severity != "low" {
		t.Fatalf("rule %q severity %q", f.RuleID, f.Severity)
	}
	if want := "2 of the 3 files with lines to run ran none of them under the suite"; f.Message != want {
		t.Fatalf("message %q, want %q", f.Message, want)
	}
	var at []string
	for _, l := range f.Locations {
		at = append(at, l.File)
	}
	if strings.Join(at, " ") != "src/m.ts src/z.ts" {
		t.Fatalf("locations %v, want src/m.ts src/z.ts", at)
	}
}

func TestAdapterHasNothingToSayWhenEveryFileRan(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "coverage/lcov.info", "SF:src/a.ts\nDA:1,1\nend_of_record\n")
	if fs, err := (Adapter{}).Findings(dir); fs != nil || err != nil {
		t.Fatalf("got %+v, %v", fs, err)
	}
}

func TestAdapterNamesTheReportItCannotRead(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "coverage/lcov.info", "TN:x\nDA:1,1\n")
	_, err := Adapter{}.Findings(dir)
	if err == nil || err.Error() != "coverage/lcov.info: line 2: DA with no SF record" {
		t.Fatalf("error %v", err)
	}
}
