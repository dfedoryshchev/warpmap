package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/baseline"
	"github.com/dfedoryshchev/warpmap/internal/coverage"
	"github.com/dfedoryshchev/warpmap/internal/graph"
	"github.com/dfedoryshchev/warpmap/internal/metrics"
	"github.com/dfedoryshchev/warpmap/internal/report"
	"github.com/dfedoryshchev/warpmap/internal/trace"
)

func usage() {
	fmt.Fprintln(os.Stderr, "warpmap: map a codebase before you change it")
	fmt.Fprintln(os.Stderr, "usage: warpmap <command> [args]")
	fmt.Fprintln(os.Stderr, "commands: hotspots | analyze | trace | dead | cycles  (all take <dir>)")
}

var sourceExt = map[string]bool{".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".py": true}

func sourceFiles(dir string) []string {
	var out []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if sourceExt[filepath.Ext(p)] {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func hotspotsCmd(args []string) int {
	fset := flag.NewFlagSet("hotspots", flag.ExitOnError)
	top := fset.Int("n", 15, "how many to show")
	fset.Parse(args)
	dir := fset.Arg(0)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap hotspots <dir>")
		return 2
	}
	files := sourceFiles(dir)
	churn, err := metrics.GitChurn(dir, 6)
	if err != nil {
		churn = metrics.Churn{}
	}
	ranked := metrics.Hotspots(dir, files, churn)
	limit := *top
	if limit > len(ranked) {
		limit = len(ranked)
	}
	for _, h := range ranked[:limit] {
		fmt.Printf("%.3f  churn=%-3d cx=%-4d %s\n", h.Score, h.Churn, h.Complexity, h.File)
	}
	return 0
}

func analyzeCmd(args []string) int {
	fset := flag.NewFlagSet("analyze", flag.ExitOnError)
	asJSON := fset.Bool("json", false, "print the full graph as json")
	asDot := fset.Bool("dot", false, "print the graph as graphviz dot")
	fset.Parse(args)
	dir := fset.Arg(0)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap analyze <dir>")
		return 2
	}
	g := graph.Build(sourceFiles(dir))
	switch {
	case *asJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(g)
	case *asDot:
		fmt.Print(graph.ToDot(g))
	default:
		fmt.Printf("%d files, %d import edges\n", len(g.Files), len(g.Edges))
	}
	return 0
}

func traceCmd(args []string) int {
	fset := flag.NewFlagSet("trace", flag.ExitOnError)
	depth := fset.Int("depth", 0, "limit how many import hops to walk (0 = all)")
	fset.Parse(args)
	dir := fset.Arg(0)
	file := fset.Arg(1)
	if dir == "" || file == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap trace <dir> <file>")
		return 2
	}
	target := filepath.Join(dir, file)
	g := graph.Build(sourceFiles(dir))
	affected := trace.BlastRadius(g, target, *depth)
	fmt.Printf("%d files depend on %s\n", len(affected), file)
	for _, f := range affected {
		fmt.Printf("  %s\n", f)
	}
	return 0
}

func deadCmd(args []string) int {
	fset := flag.NewFlagSet("dead", flag.ExitOnError)
	fset.Parse(args)
	dir := fset.Arg(0)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap dead <dir>")
		return 2
	}
	orphans := graph.Orphans(graph.Build(sourceFiles(dir)))
	fmt.Printf("%d files nothing imports (candidate dead code):\n", len(orphans))
	for _, f := range orphans {
		fmt.Printf("  %s\n", f)
	}
	return 0
}

func cyclesCmd(args []string) int {
	fset := flag.NewFlagSet("cycles", flag.ExitOnError)
	fset.Parse(args)
	dir := fset.Arg(0)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap cycles <dir>")
		return 2
	}
	cycles := graph.Cycles(graph.Build(sourceFiles(dir)))
	fmt.Printf("%d import cycles:\n", len(cycles))
	for _, c := range cycles {
		fmt.Printf("  %s\n", strings.Join(c, " -> "))
	}
	return 0
}

func godCmd(args []string) int {
	fset := flag.NewFlagSet("god", flag.ExitOnError)
	top := fset.Int("n", 15, "how many to show")
	fset.Parse(args)
	dir := fset.Arg(0)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap god <dir>")
		return 2
	}
	mods := graph.GodModules(graph.Build(sourceFiles(dir)))
	limit := *top
	if limit > len(mods) {
		limit = len(mods)
	}
	fmt.Println("files too many things depend on, or that depend on too much:")
	for _, m := range mods[:limit] {
		fmt.Printf("  in=%-3d out=%-3d %s\n", m.FanIn, m.FanOut, m.File)
	}
	return 0
}

func testgapCmd(args []string) int {
	fset := flag.NewFlagSet("testgap", flag.ExitOnError)
	top := fset.Int("n", 15, "how many to show")
	fset.Parse(args)
	dir := fset.Arg(0)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap testgap <dir>")
		return 2
	}
	g := graph.Build(sourceFiles(dir))
	type row struct {
		file  string
		blast int
	}
	var rows []row
	for _, f := range coverage.Untested(g) {
		rows = append(rows, row{f, len(trace.BlastRadius(g, f, 0))})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].blast > rows[j].blast })
	fmt.Printf("%d untested files; the riskiest (highest blast radius) first - test these before you change them:\n", len(rows))
	limit := *top
	if limit > len(rows) {
		limit = len(rows)
	}
	for _, r := range rows[:limit] {
		rel, _ := filepath.Rel(dir, r.file)
		fmt.Printf("  blast=%-4d %s\n", r.blast, filepath.ToSlash(rel))
	}
	return 0
}

func diffCmd(args []string) int {
	fset := flag.NewFlagSet("diff", flag.ExitOnError)
	fset.Parse(args)
	dir := fset.Arg(0)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap diff <dir>  (compares against .warpmap/baseline.json)")
		return 2
	}
	before, err := baseline.Load(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no baseline; run `warpmap baseline %s` first\n", dir)
		return 1
	}
	d := baseline.Compare(before, baseline.Capture(dir, sourceFiles(dir)))
	fmt.Printf("since baseline: edges %+d, cycles %+d, orphans %+d, god-modules %+d\n",
		d.Edges, d.Cycles, d.Orphans, d.GodModules)
	if len(d.Worsened) > 0 {
		fmt.Printf("%d files got more complex:\n", len(d.Worsened))
		for _, c := range d.Worsened[:min(5, len(d.Worsened))] {
			fmt.Printf("  +%d  %s\n", c.After-c.Before, c.File)
		}
	}
	if d.RiskUp() {
		fmt.Println("verdict: RISK UP - this change made the codebase harder to work on safely")
		return 1
	}
	fmt.Println("verdict: ok - no net degradation")
	return 0
}

func baselineCmd(args []string) int {
	fset := flag.NewFlagSet("baseline", flag.ExitOnError)
	fset.Parse(args)
	dir := fset.Arg(0)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap baseline <dir>")
		return 2
	}
	snap := baseline.Capture(dir, sourceFiles(dir))
	if err := baseline.Save(dir, snap); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("baseline saved: %d files, %d edges, %d cycles, %d god-modules\n",
		snap.Files, snap.Edges, snap.Cycles, snap.GodModules)
	return 0
}

func reportCmd(args []string) int {
	fset := flag.NewFlagSet("report", flag.ExitOnError)
	out := fset.String("o", "", "write to a file instead of stdout")
	fset.Parse(args)
	dir := fset.Arg(0)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap report <dir>")
		return 2
	}
	files := sourceFiles(dir)
	churn, err := metrics.GitChurn(dir, 6)
	if err != nil {
		churn = metrics.Churn{}
	}
	md := report.Build(dir, files, churn).Markdown()
	if *out != "" {
		if err := os.WriteFile(*out, []byte(md), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else {
		fmt.Print(md)
	}
	return 0
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "hotspots":
		os.Exit(hotspotsCmd(os.Args[2:]))
	case "analyze":
		os.Exit(analyzeCmd(os.Args[2:]))
	case "trace":
		os.Exit(traceCmd(os.Args[2:]))
	case "dead":
		os.Exit(deadCmd(os.Args[2:]))
	case "cycles":
		os.Exit(cyclesCmd(os.Args[2:]))
	case "god":
		os.Exit(godCmd(os.Args[2:]))
	case "report":
		os.Exit(reportCmd(os.Args[2:]))
	case "baseline":
		os.Exit(baselineCmd(os.Args[2:]))
	case "diff":
		os.Exit(diffCmd(os.Args[2:]))
	case "testgap":
		os.Exit(testgapCmd(os.Args[2:]))
	default:
		usage()
		os.Exit(2)
	}
}
