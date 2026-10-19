package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/graph"
	"github.com/dfedoryshchev/warpmap/internal/metrics"
	"github.com/dfedoryshchev/warpmap/internal/report"
)

var findTool = exec.LookPath

// an optional tool collect runs when it is on PATH. run returns what goes into
// the folder: the tool's findings with every field that carries source text,
// an author or a commit message left out.
type optionalTool struct {
	name    string
	file    string
	version []string
	run     func(bin, dir string) ([]byte, error)
}

var optionalTools = []optionalTool{
	{name: "gitleaks", file: "gitleaks.json", version: []string{"version"}, run: runGitleaks},
	{name: "lizard", file: "lizard.csv", version: []string{"--version"}, run: runLizard},
	{name: "jscpd", file: "jscpd.json", version: []string{"--version"}, run: runJscpd},
}

func collectCmd(args []string) int {
	fset := flag.NewFlagSet("collect", flag.ExitOnError)
	out := fset.String("o", "", "the folder to write, new or empty")
	months := monthsFlag(fset)
	dir := arg(parseArgs(fset, args), 0)
	if dir == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap collect <dir> -o <folder>")
		return 2
	}
	if entries, err := os.ReadDir(*out); err == nil && len(entries) > 0 {
		fmt.Fprintf(os.Stderr, "%s is not empty; collect writes only into a new or empty folder\n", *out)
		return 2
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	folder := map[string][]byte{}
	files := sourceFiles(dir)
	churn, window := metrics.ProjectChurn(dir, files, int(*months))
	r := report.Build(dir, files, churn)
	fmt.Fprintln(os.Stderr, window.Note(r.Hotspots))
	pack, err := r.JSON()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	folder["report.json"] = pack
	var g bytes.Buffer
	if err := writeGraphJSON(&g, dir, graph.Build(files)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	folder["analyze.json"] = g.Bytes()

	tools := "warpmap " + version + "\n"
	for _, t := range optionalTools {
		bin, err := findTool(t.name)
		if err != nil {
			tools += t.name + " absent\n"
			continue
		}
		tools += t.name + " " + toolVersion(bin, t.version) + "\n"
		body, err := t.run(bin, dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s failed, left out of the folder: %v\n", t.name, err)
			tools = strings.TrimSuffix(tools, "\n") + ", run failed\n"
			continue
		}
		folder[t.file] = body
	}
	folder["tools.txt"] = []byte(tools)

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for name, body := range folder {
		if err := os.WriteFile(filepath.Join(*out, name), body, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "wrote %d files to %s; read them, then send the folder yourself\n", len(folder), *out)
	return 0
}

func toolVersion(bin string, args []string) string {
	b, err := exec.Command(bin, args...).Output()
	if err != nil {
		return "version unknown"
	}
	for _, line := range strings.Split(string(b), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "version unknown"
}

// projectPath spells a path a tool printed the way every command prints one:
// relative to the project, forward slashes.
func projectPath(dir, p string) string {
	if filepath.IsAbs(p) {
		if abs, err := filepath.Abs(dir); err == nil {
			if r, err := filepath.Rel(abs, p); err == nil {
				p = r
			}
		}
	}
	return strings.TrimPrefix(filepath.ToSlash(p), "./")
}

func inGitRepo(dir string) bool {
	return exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree").Run() == nil
}

// gitleaks with --redact still keeps the text around a secret in Match, and
// the commit's author, email and message, so only these fields are kept.
var gitleaksFields = []string{
	"RuleID", "Description", "File", "StartLine", "EndLine", "StartColumn", "EndColumn",
	"Commit", "Date", "Entropy", "Tags", "Fingerprint",
}

func runGitleaks(bin, dir string) ([]byte, error) {
	tmp, err := os.MkdirTemp("", "warpmap-gitleaks-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	reportPath := filepath.Join(tmp, "report.json")
	args := []string{"detect", "--source", ".", "--redact", "--no-banner",
		"--report-format", "json", "--report-path", reportPath, "--exit-code", "0"}
	if !inGitRepo(dir) {
		args = append(args, "--no-git")
	}
	if err := runIn(dir, bin, args...); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, err
	}
	var findings []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &findings); err != nil {
		return nil, fmt.Errorf("reading gitleaks' report: %w", err)
	}
	kept := make([]map[string]json.RawMessage, 0, len(findings))
	for _, f := range findings {
		k := map[string]json.RawMessage{}
		for _, name := range gitleaksFields {
			if v, ok := f[name]; ok {
				k[name] = v
			}
		}
		var file string
		if json.Unmarshal(k["File"], &file) == nil {
			k["File"], _ = json.Marshal(projectPath(dir, file))
		}
		kept = append(kept, k)
	}
	return indentJSON(kept)
}

// lizard's csv names each function's signature in full (long_name), default
// values included, so the rows keep the numbers, the file and the bare name.
func runLizard(bin, dir string) ([]byte, error) {
	raw, err := metrics.LizardCSV(bin, rankedFiles(dir))
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Write([]string{"nloc", "ccn", "tokens", "params", "length", "file", "function", "start", "end"})
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading lizard's csv: %w", err)
		}
		if rec[0] == "NLOC" {
			continue
		}
		if len(rec) < 11 {
			return nil, fmt.Errorf("lizard's csv has %d columns, want 11", len(rec))
		}
		w.Write([]string{rec[0], rec[1], rec[2], rec[3], rec[4], rel(dir, rec[6]), rec[7], rec[9], rec[10]})
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

// jscpd's json report carries each duplicated block as text (fragment), and
// with blame the authors, so only counts and locations are kept.
func runJscpd(bin, dir string) ([]byte, error) {
	tmp, err := os.MkdirTemp("", "warpmap-jscpd-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := runIn(dir, bin, ".", "--reporters", "json", "--output", tmp, "--silent"); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(tmp, "jscpd-report.json"))
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var rep struct {
		Statistics struct {
			Total   map[string]any `json:"total"`
			Formats map[string]struct {
				Total map[string]any `json:"total"`
			} `json:"formats"`
		} `json:"statistics"`
		Duplicates []map[string]any `json:"duplicates"`
	}
	if err := dec.Decode(&rep); err != nil {
		return nil, fmt.Errorf("reading jscpd's report: %w", err)
	}
	formats := map[string]any{}
	for name, f := range rep.Statistics.Formats {
		formats[name] = map[string]any{"total": numbersOnly(f.Total)}
	}
	dups := make([]map[string]any, 0, len(rep.Duplicates))
	for _, d := range rep.Duplicates {
		k := numbersOnly(d)
		for _, name := range []string{"format", "kind", "method"} {
			if s, ok := d[name].(string); ok {
				k[name] = s
			}
		}
		if b, ok := d["isNew"].(bool); ok {
			k["isNew"] = b
		}
		for _, side := range []string{"firstFile", "secondFile"} {
			f, _ := d[side].(map[string]any)
			s := numbersOnly(f)
			if name, ok := f["name"].(string); ok {
				s["name"] = projectPath(dir, name)
			}
			k[side] = s
		}
		dups = append(dups, k)
	}
	return indentJSON(map[string]any{
		"statistics": map[string]any{"total": numbersOnly(rep.Statistics.Total), "formats": formats},
		"duplicates": dups,
	})
}

func numbersOnly(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if n, ok := v.(json.Number); ok {
			out[k] = n
		}
	}
	return out
}

func indentJSON(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// runIn runs a tool from inside dir, so the paths it prints are the project's
// own and not the machine's; its stderr is kept for the error.
func runIn(dir, bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
