package main

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"
)

type hotspotsJSON struct {
	Measure  string `json:"measure"`
	Hotspots []struct {
		File       string  `json:"file"`
		Score      float64 `json:"score"`
		Churn      int     `json:"churn"`
		Complexity int     `json:"complexity"`
		LOC        int     `json:"loc"`
	} `json:"hotspots"`
}

func TestHotspotsAnswersInJSON(t *testing.T) {
	dir := chainProject(t)
	out, code := captureStdout(t, func() int { return hotspotsCmd([]string{dir, "--json"}) })
	if code != 0 {
		t.Fatalf("hotspots --json exited %d", code)
	}
	var got hotspotsJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not json: %v\n%s", err, out)
	}
	if got.Measure != "cx" && got.Measure != "ccn" {
		t.Fatalf("measure = %q, want cx or ccn", got.Measure)
	}
	loc := map[string]int{}
	for _, h := range got.Hotspots {
		loc[h.File] = h.LOC
	}
	if want := map[string]int{"src/a.ts": 2, "src/b.ts": 2, "src/c.ts": 1}; !maps.Equal(loc, want) {
		t.Fatalf("hotspots by project-relative path and loc = %v, want %v\n%s", loc, want, out)
	}

	out, code = captureStdout(t, func() int { return hotspotsCmd([]string{dir, "--json", "-n", "1"}) })
	if code != 0 {
		t.Fatalf("hotspots --json -n 1 exited %d", code)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || len(got.Hotspots) != 1 {
		t.Fatalf("-n 1 did not cap the json list to one row (err %v):\n%s", err, out)
	}
}

func TestTestgapAnswersInJSON(t *testing.T) {
	dir := chainProject(t)
	writeFiles(t, dir, map[string]string{"tests/setup.ts": "import { a } from \"../src/a\";\nexport const s = a;\n"})
	out, code := captureStdout(t, func() int { return testgapCmd([]string{dir, "--json", "-n", "1"}) })
	if code != 0 {
		t.Fatalf("testgap --json exited %d", code)
	}
	var got struct {
		Untested int `json:"untested"`
		Files    []struct {
			File  string `json:"file"`
			Blast int    `json:"blast"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not json: %v\n%s", err, out)
	}
	if got.Untested != 2 || len(got.Files) != 1 || got.Files[0].File != "src/c.ts" || got.Files[0].Blast != 3 {
		t.Fatalf("want 2 untested, the first being src/c.ts at blast 3, got %+v", got)
	}
}

func TestOwnershipAnswersInJSON(t *testing.T) {
	dir := chainProject(t)
	gitIn(t, dir, "init")
	gitIn(t, dir, "add", "--", "src")
	gitIn(t, dir, "commit", "-m", "add the chain")
	out, code := captureStdout(t, func() int { return ownershipCmd([]string{dir, "--json"}) })
	if code != 0 {
		t.Fatalf("ownership --json exited %d", code)
	}
	var got struct {
		Hotspots []struct {
			File    string `json:"file"`
			Authors int    `json:"authors"`
		} `json:"hotspots"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not json: %v\n%s", err, out)
	}
	var files []string
	for _, h := range got.Hotspots {
		if h.Authors != 1 {
			t.Fatalf("%s: authors = %d, want 1", h.File, h.Authors)
		}
		files = append(files, h.File)
	}
	slices.Sort(files)
	if want := []string{"src/a.ts", "src/b.ts", "src/c.ts"}; !slices.Equal(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
}

// a project with nothing to rank still answers with lists, the way `report
// --json` does, so a consumer iterating them never has to special-case null.
func TestRankingJSONIsAListWhenEmpty(t *testing.T) {
	for name, cmd := range map[string]func([]string) int{
		"hotspots":  hotspotsCmd,
		"testgap":   testgapCmd,
		"ownership": ownershipCmd,
	} {
		out, code := captureStdout(t, func() int { return cmd([]string{t.TempDir(), "--json"}) })
		if code != 0 {
			t.Fatalf("%s --json exited %d", name, code)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("%s: not json: %v\n%s", name, err, out)
		}
		key := map[string]string{"hotspots": "hotspots", "testgap": "files", "ownership": "hotspots"}[name]
		if list, ok := got[key].([]any); !ok || len(list) != 0 {
			t.Fatalf("%s: %q = %#v, want an empty list\n%s", name, key, got[key], out)
		}
	}
}
