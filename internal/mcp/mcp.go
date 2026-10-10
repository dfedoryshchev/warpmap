// Package mcp exposes warpmap's audit as an MCP server over stdio, so an agent can
// query the risk map (hotspots, blast radius, test gaps) before it touches code.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/config"
	"github.com/dfedoryshchev/warpmap/internal/graph"
	"github.com/dfedoryshchev/warpmap/internal/metrics"
	"github.com/dfedoryshchev/warpmap/internal/risk"
	"github.com/dfedoryshchev/warpmap/internal/trace"
)

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func reply(id json.RawMessage, result any) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	fmt.Println(string(b))
}

func toolSpecs() []map[string]any {
	dirArg := map[string]any{
		"type":       "object",
		"properties": map[string]any{"dir": map[string]any{"type": "string"}},
		"required":   []string{"dir"},
	}
	traceArg := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"dir":  map[string]any{"type": "string"},
			"file": map[string]any{"type": "string"},
		},
		"required": []string{"dir", "file"},
	}
	riskArg := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"dir":   map[string]any{"type": "string"},
			"files": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []string{"dir", "files"},
	}
	return []map[string]any{
		{"name": "hotspots", "description": "rank the riskiest files (churn x complexity)", "inputSchema": dirArg},
		{"name": "trace", "description": "blast radius: files that depend on a given file", "inputSchema": traceArg},
		{"name": "risk", "description": "blast radius + test gaps for the files a change touches, with a verdict", "inputSchema": riskArg},
		{"name": "testgap", "description": "untested files ranked by blast radius", "inputSchema": dirArg},
	}
}

// Serve runs a minimal MCP server (newline-delimited JSON-RPC over stdio). version is
// reported in the initialize handshake; sources returns the source files under a
// directory (injected so this package stays small).
func Serve(version string, sources func(string) []string) {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if json.Unmarshal(line, &req) != nil {
			continue
		}
		switch req.Method {
		case "initialize":
			reply(req.ID, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "warpmap", "version": version},
			})
		case "tools/list":
			reply(req.ID, map[string]any{"tools": toolSpecs()})
		case "tools/call":
			handleCall(req, sources)
		}
	}
}

func projectPath(dir, f string) string {
	r, err := filepath.Rel(dir, f)
	if err != nil {
		return filepath.ToSlash(f)
	}
	return filepath.ToSlash(r)
}

func handleCall(req request, sources func(string) []string) {
	var p struct {
		Name string `json:"name"`
		Args struct {
			Dir   string   `json:"dir"`
			File  string   `json:"file"`
			Files []string `json:"files"`
		} `json:"arguments"`
	}
	json.Unmarshal(req.Params, &p)

	var text string
	switch p.Name {
	case "hotspots":
		files := sources(p.Args.Dir)
		churn, err := metrics.ProjectChurn(p.Args.Dir, files, 6)
		if err != nil {
			churn = metrics.Churn{}
		}
		for i, h := range metrics.Hotspots(p.Args.Dir, files, churn) {
			if i >= 10 {
				break
			}
			text += fmt.Sprintf("%s  %.3f\n", projectPath(p.Args.Dir, h.File), h.Score)
		}
	case "trace":
		g := graph.Build(sources(p.Args.Dir))
		br := trace.BlastRadius(g, filepath.Join(p.Args.Dir, p.Args.File), 0)
		text = fmt.Sprintf("%d files depend on %s\n", len(br), p.Args.File)
		deps := make([]string, 0, len(br))
		for _, f := range br {
			deps = append(deps, projectPath(p.Args.Dir, f))
		}
		sort.Strings(deps)
		for _, d := range deps {
			text += d + "\n"
		}
	case "testgap":
		for i, gap := range risk.TestGaps(p.Args.Dir, graph.Build(sources(p.Args.Dir))) {
			if i >= 10 {
				break
			}
			text += fmt.Sprintf("%s  %d\n", projectPath(p.Args.Dir, gap.File), gap.Blast)
		}
	case "risk":
		if len(p.Args.Files) == 0 {
			text = "risk needs at least one file in files"
			break
		}
		g := graph.Build(sources(p.Args.Dir))
		cfg, err := config.Load(p.Args.Dir)
		if err != nil {
			text = fmt.Sprintf("%s: %v", config.FileName, err)
			break
		}
		var b strings.Builder
		risk.Assess(p.Args.Dir, g, p.Args.Files, cfg.Thresholds.Blast).Write(&b)
		text = b.String()
	default:
		text = "unknown tool: " + p.Name
	}
	reply(req.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": text}}})
}
