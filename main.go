package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/graph"
	"github.com/dfedoryshchev/warpmap/internal/metrics"
	"github.com/dfedoryshchev/warpmap/internal/trace"
)

func usage() {
	fmt.Fprintln(os.Stderr, "warpmap: map a codebase before you change it")
	fmt.Fprintln(os.Stderr, "usage: warpmap <command> [args]")
	fmt.Fprintln(os.Stderr, "commands: hotspots | analyze | trace | dead | cycles  (all take <dir>)")
}

var sourceExt = map[string]bool{".ts": true, ".tsx": true, ".js": true, ".jsx": true}

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
	fset.Parse(args)
	dir := fset.Arg(0)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: warpmap analyze <dir>")
		return 2
	}
	g := graph.Build(sourceFiles(dir))
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(g)
		return 0
	}
	fmt.Printf("%d files, %d import edges\n", len(g.Files), len(g.Edges))
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
	default:
		usage()
		os.Exit(2)
	}
}
