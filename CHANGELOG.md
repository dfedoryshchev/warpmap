# Changelog

All notable changes to this project are documented in this file.

## 0.1.0 - 2026-08-02

### Added
- Dependency graph for TypeScript / JavaScript / Python, resolving relative imports to files
  on disk (`analyze`, with `--json` and graphviz `--dot` output)
- Churn from git history, a complexity proxy, and the churn x complexity ranking (`hotspots`)
- Blast radius of a file, with a hop limit (`trace --depth`)
- Structural findings: orphans (`dead`), import cycles (`cycles`), high fan-in/out (`god`)
- Ownership and bus factor from git history (`ownership`)
- Untested files ranked by blast radius (`testgap`)
- Markdown audit report with a severity + recommendation findings model (`report`)
- The ratchet: a baseline snapshot in `.warpmap/baseline.json` (`baseline`) and a
  better-or-worse comparison that exits non-zero on regression (`diff`)
- Change risk for the files you are about to touch (`risk`)
- Agent context pack for a single file (`brief`)
- Hotspot narration through an LLM, with a deterministic offline fallback (`explain`)
- MCP server over stdio, exposing hotspots and trace to an agent (`mcp`)

### Known limitations
- Imports are extracted with regular expressions rather than a parser, so unusual forms
  (a specifier built at runtime, for instance) can be missed
- Only relative specifiers resolve; path aliases and package specifiers are skipped
- Markdown is the only report format; there is no config file and no CI action yet
